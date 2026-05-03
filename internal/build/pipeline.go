package build

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/domain"
	"github.com/LEFTEQ/lovinka-deployik/internal/projectconfig"
	"github.com/LEFTEQ/lovinka-deployik/internal/ws"
)

// defaultMaxBuildDuration caps a single deployment when MaxBuildDuration is unset.
const defaultMaxBuildDuration = 15 * time.Minute

// Pipeline orchestrates the full deploy flow.
type Pipeline struct {
	DB            *db.DB
	Docker        *DockerClient
	Encryptor     *crypto.Encryptor
	Semaphore     *Semaphore
	DomainManager *domain.Manager
	BuildDir      string
	ProxyNetwork  string // Docker network name (e.g., "proxy")
	ProxyType     string // "docker" | "host-port"
	Hub           *ws.Hub
	ScreenshotDir string // Directory to store deployment screenshots (in-container path)
	// ScreenshotHostDir is the host-filesystem path that backs ScreenshotDir
	// when deployik runs inside a container. The Docker daemon resolves
	// nested-container bind sources on the host, not inside the caller, so we
	// pass this as the bind Source. Empty falls back to ScreenshotDir (dev).
	ScreenshotHostDir string
	// JWTSecret signs short-lived site-auth bypass tokens so the post-deploy
	// screenshot capture can render password-protected homepages without going
	// through the human-facing login form. Empty string disables bypass minting.
	JWTSecret string

	// Ctx is the pipeline-level parent context. When cancelled, in-flight deploys
	// abort cleanly. Handler goroutines derive per-deploy contexts from this.
	Ctx context.Context
	// Wg tracks active deploy + screenshot goroutines so the server can drain
	// cleanly on shutdown.
	Wg *sync.WaitGroup
	// MaxBuildDuration is the per-deploy wall-clock cap. Zero means defaultMaxBuildDuration.
	MaxBuildDuration time.Duration
	// EnqueueOnly leaves dispatch-created deployments queued without starting
	// the async deploy worker. This is used by control-plane tests.
	EnqueueOnly bool
}

// Dispatch starts a deployment asynchronously. Returns immediately; the deploy
// runs on a pipeline-owned goroutine that participates in the WaitGroup and
// respects the pipeline context.
func (p *Pipeline) Dispatch(project *db.Project, deployment *db.Deployment, githubToken string, onLog LogCallback) {
	if p.EnqueueOnly {
		return
	}
	if p.Wg != nil {
		p.Wg.Add(1)
	}
	go func() {
		if p.Wg != nil {
			defer p.Wg.Done()
		}
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("Pipeline panic for deployment %s: %v", deployment.ID, rec)
				if err := p.DB.UpdateDeploymentStatus(deployment.ID, "failed", "internal error"); err != nil {
					log.Printf("Failed to mark deployment %s failed after panic: %v", deployment.ID, err)
				}
			}
		}()

		parent := p.Ctx
		if parent == nil {
			parent = context.Background()
		}
		duration := p.MaxBuildDuration
		if duration <= 0 {
			duration = defaultMaxBuildDuration
		}
		ctx, cancel := context.WithTimeout(parent, duration)
		defer cancel()

		p.Deploy(ctx, project, deployment, githubToken, onLog)
	}()
}

// LogCallback is called for each build log line.
type LogCallback func(line string, stream string)

// Deploy runs the full deployment pipeline for a project.
func (p *Pipeline) Deploy(ctx context.Context, project *db.Project, deployment *db.Deployment, githubToken string, onLog LogCallback) {
	startTime := time.Now()

	logLineNum := 0
	emit := func(msg string) {
		logLineNum++
		p.DB.InsertBuildLog(deployment.ID, logLineNum, msg, "stdout")
		if p.Hub != nil {
			p.Hub.Publish(ws.LogLine{DeploymentID: deployment.ID, LineNumber: logLineNum, Content: msg, Stream: "stdout"})
		}
		if onLog != nil {
			onLog(msg, "stdout")
		}
	}
	emitErr := func(msg string) {
		logLineNum++
		p.DB.InsertBuildLog(deployment.ID, logLineNum, msg, "stderr")
		if p.Hub != nil {
			p.Hub.Publish(ws.LogLine{DeploymentID: deployment.ID, LineNumber: logLineNum, Content: msg, Stream: "stderr"})
		}
		if onLog != nil {
			onLog(msg, "stderr")
		}
	}

	fail := func(err error, msg string) {
		errMsg := fmt.Sprintf("%s: %v", msg, err)
		emitErr(errMsg)
		p.DB.UpdateDeploymentStatus(deployment.ID, "failed", errMsg)
		p.DB.UpdateDeploymentDuration(deployment.ID, int(time.Since(startTime).Seconds()))
	}

	// Acquire build slot
	emit("Waiting for build slot...")
	p.Semaphore.Acquire()
	defer p.Semaphore.Release()

	// Step 1: Update status to building
	p.DB.UpdateDeploymentStatus(deployment.ID, "building", "")
	emit(fmt.Sprintf("Starting build for %s/%s@%s", project.GithubOwner, project.GithubRepo, deployment.Branch))

	// Step 2: Clone repository
	emit("Cloning repository...")
	buildDir := fmt.Sprintf("%s/%s", p.BuildDir, deployment.ID)
	os.MkdirAll(buildDir, 0755)
	defer os.RemoveAll(buildDir) // Always clean up

	repoDir, err := CloneRepo(buildDir, project.GithubOwner, project.GithubRepo, deployment.Branch, githubToken)
	if err != nil {
		fail(err, "Clone failed")
		return
	}
	emit("Repository cloned")

	// Get commit info
	sha, message, err := GetHeadCommit(repoDir)
	if err != nil {
		log.Printf("Warning: could not get commit info: %v", err)
	} else {
		p.DB.Exec("UPDATE deployments SET commit_sha = ?, commit_message = ? WHERE id = ?",
			sha, message, deployment.ID)
		emit(fmt.Sprintf("Commit: %s %s", sha[:8], message))
	}

	settings, err := projectconfig.Resolve(project)
	if err != nil {
		fail(err, "Invalid build settings")
		return
	}

	projectDir := repoDir
	if settings.RootDirectory != "" {
		projectDir = filepath.Join(repoDir, filepath.FromSlash(settings.RootDirectory))
		if _, err := os.Stat(projectDir); err != nil {
			fail(err, "Project root directory not found")
			return
		}
	}

	// Step 3: Patch Next.js config for standalone output
	if settings.Runtime == projectconfig.RuntimeNextJSStandalone {
		emit("Patching Next.js config for standalone output...")
		if err := PatchNextConfig(projectDir); err != nil {
			emitErr(fmt.Sprintf("Warning: could not patch next.config: %v", err))
		}
	}

	// Step 4: Get project variables
	envVars, err := p.DB.ListResolvedEnvVars(project.ID, deployment.Environment)
	if err != nil {
		log.Printf("Warning: could not load env vars: %v", err)
	}
	secretVars, err := p.DB.ListResolvedSecrets(project.ID, deployment.Environment)
	if err != nil {
		log.Printf("Warning: could not load secrets: %v", err)
	}

	decryptedEnvVars := make([]db.ProjectVariable, 0, len(envVars))
	for _, ev := range envVars {
		value, err := p.Encryptor.Decrypt(ev.Value)
		if err != nil {
			emitErr(fmt.Sprintf("Warning: could not decrypt env var %s", ev.Key))
			continue
		}
		decryptedEnvVars = append(decryptedEnvVars, db.ProjectVariable{
			Key:   ev.Key,
			Value: value,
		})
	}

	decryptedSecrets := make([]db.ProjectVariable, 0, len(secretVars))
	for _, secret := range secretVars {
		value, err := p.Encryptor.Decrypt(secret.Value)
		if err != nil {
			emitErr(fmt.Sprintf("Warning: could not decrypt secret %s", secret.Key))
			continue
		}
		decryptedSecrets = append(decryptedSecrets, db.ProjectVariable{
			Key:   secret.Key,
			Value: value,
		})
	}

	buildEnvVars, runtimeEnvVars := resolveDeploymentVariables(decryptedEnvVars, decryptedSecrets)
	buildSecrets := resolveBuildSecrets(decryptedEnvVars, decryptedSecrets)

	// Step 5: Generate Dockerfile
	emit("Generating Dockerfile...")
	dockerfilePath, err := GenerateDockerfile(repoDir, DockerfileData{
		PackageManager:  settings.PackageManager,
		NodeVersion:     settings.NodeVersion,
		InstallCommand:  settings.InstallCommand,
		BuildCommand:    settings.BuildCommand,
		RootDirectory:   settings.RootDirectory,
		OutputDirectory: settings.OutputDirectory,
		Runtime:         settings.Runtime,
		BuildEnvVars:    buildEnvVars,
		ProjectID:       project.ID,
		Port:            project.Port,
	})
	if err != nil {
		fail(err, "Dockerfile generation failed")
		return
	}
	emit("Dockerfile ready")

	// Write project env vars + secrets to a short-lived file that BuildKit mounts
	// as /run/secrets/deployik-secrets inside build RUN steps that opt in via
	// `--mount=type=secret,id=deployik-secrets`. Values never land in image
	// layers. The file lives in buildDir so it's cleaned up with the rest of the
	// build scratch area.
	secretsFile, err := writeBuildSecretsFile(buildDir, buildSecrets)
	if err != nil {
		fail(err, "Prepare build secrets failed")
		return
	}
	if secretsFile != "" {
		defer os.Remove(secretsFile)
	}

	// Step 5: Docker build
	containerName := fmt.Sprintf("deployik-%s-%s", project.Name, deployment.Environment)
	imageTag := fmt.Sprintf("deployik-%s-%s:%s", project.Name, deployment.Environment, deployment.ID[:8])

	emit(fmt.Sprintf("Building image %s...", imageTag))
	_, err = p.Docker.BuildImage(ctx, repoDir, dockerfilePath, imageTag, BuildImageOptions{
		SecretsFile: secretsFile,
		OnLog: func(line string) {
			logLineNum++
			p.DB.InsertBuildLog(deployment.ID, logLineNum, line, "stdout")
			if p.Hub != nil {
				p.Hub.Publish(ws.LogLine{DeploymentID: deployment.ID, LineNumber: logLineNum, Content: line, Stream: "stdout"})
			}
		},
	})
	if err != nil {
		fail(err, "Docker build failed")
		return
	}
	emit("Image built successfully")

	// Step 6: Deploy container
	p.DB.UpdateDeploymentStatus(deployment.ID, "deploying", "")
	emit("Deploying container...")

	// Check if old container exists
	oldContainerID, oldExists := p.Docker.ContainerExists(ctx, containerName)

	// Build RunContainerOptions from project settings
	var extraHosts []string
	if project.HostNetworkAccess {
		extraHosts = []string{"host.docker.internal:host-gateway"}
	}

	var volumeBinds []string
	if project.DataVolumeEnabled {
		volumeName := fmt.Sprintf("deployik-%s-%s-data", project.Name, deployment.Environment)
		emit(fmt.Sprintf("Ensuring data volume %s...", volumeName))
		if err := p.Docker.EnsureVolume(ctx, volumeName); err != nil {
			emitErr(fmt.Sprintf("Warning: could not ensure data volume: %v", err))
		} else {
			mountPath := project.DataMountPath
			if mountPath == "" {
				mountPath = "/app/data"
			}
			volumeBinds = []string{volumeName + ":" + mountPath}
			emit(fmt.Sprintf("Volume mounted at %s", mountPath))
		}
	}

	// Start new container with temporary name
	tempName := containerName + "-" + deployment.ID[:8]
	opts := RunContainerOptions{
		ExtraHosts:   extraHosts,
		BindHostPort: p.ProxyType == "host-port",
		VolumeBinds:  volumeBinds,
		Port:         project.Port,
	}
	newContainerID, err := p.Docker.RunContainer(ctx, tempName, imageTag, runtimeEnvVars, p.ProxyNetwork, opts)
	if err != nil {
		fail(err, "Container start failed")
		return
	}
	emit("Container started, waiting for health check...")

	// Step 7: Health check
	err = p.Docker.WaitForHealthy(ctx, newContainerID, 60*time.Second)
	if err != nil {
		emitErr(fmt.Sprintf("Health check failed: %v", err))
		p.Docker.StopContainer(ctx, newContainerID)
		fail(err, "Health check failed")
		return
	}
	emit("Health check passed")

	// Determine the upstream address for proxy config
	targetPort := project.Port
	if targetPort <= 0 {
		targetPort = 3000
	}
	containerUpstream := fmt.Sprintf("%s:%d", containerName, targetPort)
	if p.ProxyType == "host-port" {
		hostPort, err := p.Docker.GetHostPort(ctx, newContainerID, targetPort)
		if err != nil {
			emitErr(fmt.Sprintf("Warning: could not get host port, falling back to container name: %v", err))
		} else {
			containerUpstream = "127.0.0.1:" + hostPort
			emit(fmt.Sprintf("Container bound to host port %s", hostPort))
		}
	}

	if p.DomainManager != nil {
		emit("Ensuring environment domains...")
		if err := p.ensureEnvironmentDomains(project, deployment, containerName, containerUpstream, emit); err != nil {
			p.Docker.StopContainer(ctx, newContainerID)
			fail(err, "Domain provisioning failed")
			return
		}
	}

	// Step 8: Swap containers (blue-green)
	if oldExists {
		emit("Stopping old container...")
		p.Docker.StopContainer(ctx, oldContainerID)
	}

	// Rename new container to the canonical name
	p.Docker.cli.ContainerRename(ctx, newContainerID, containerName)

	// Step 9: Mark previous live deployment as replaced
	if liveDeploy, _ := p.DB.GetLiveDeployment(project.ID, deployment.Environment); liveDeploy != nil && liveDeploy.ID != deployment.ID {
		p.DB.UpdateDeploymentStatus(liveDeploy.ID, "replaced", "")
	}

	// Step 10: Finalize
	duration := int(time.Since(startTime).Seconds())
	p.DB.UpdateDeploymentContainer(deployment.ID, newContainerID, containerName, imageTag)
	p.DB.UpdateDeploymentDuration(deployment.ID, duration)
	p.DB.UpdateDeploymentStatus(deployment.ID, "live", "")

	emit(fmt.Sprintf("Deployment live! (%ds)", duration))

	// Step 11: Capture screenshot asynchronously. Participates in the pipeline
	// WaitGroup and honors pipeline-level cancellation.
	if p.ScreenshotDir != "" && p.Docker != nil {
		if p.Wg != nil {
			p.Wg.Add(1)
		}
		go func() {
			if p.Wg != nil {
				defer p.Wg.Done()
			}
			parent := p.Ctx
			if parent == nil {
				parent = context.Background()
			}
			screenshotCtx, cancel := context.WithTimeout(parent, 90*time.Second)
			defer cancel()

			select {
			case <-screenshotCtx.Done():
				return
			case <-time.After(5 * time.Second):
			}

			domain, err := p.DB.GetPrimaryDomain(project.ID, deployment.Environment)
			if err != nil {
				log.Printf("Screenshot: failed to look up primary domain for %s: %v", deployment.ID, err)
				return
			}
			if domain == nil {
				log.Printf("Screenshot: no active domain for deployment %s", deployment.ID)
				return
			}
			screenshotURL := "https://" + domain.DomainName
			if project.IsEnvironmentProtected(deployment.Environment) && p.JWTSecret != "" {
				token := auth.MintSiteAuthBypassToken(p.JWTSecret, project.ID, deployment.Environment)
				screenshotURL = AppendBypassToken(screenshotURL, auth.SiteAuthBypassParam, token)
			}
			path, err := CaptureScreenshot(screenshotCtx, p.Docker, screenshotURL, deployment.ID, p.ScreenshotDir, p.ScreenshotHostDir, p.ProxyNetwork)
			if err != nil {
				log.Printf("Screenshot capture failed for %s: %v", deployment.ID, err)
				return
			}
			if err := p.DB.UpdateDeploymentScreenshot(deployment.ID, path); err != nil {
				log.Printf("Screenshot: failed to save path for %s: %v", deployment.ID, err)
			}
		}()
	}
}

func (p *Pipeline) ensureEnvironmentDomains(project *db.Project, deployment *db.Deployment, containerName, containerUpstream string, emit func(string)) error {
	domains, err := p.DB.ListDomains(project.ID)
	if err != nil {
		return fmt.Errorf("list domains: %w", err)
	}

	found := false
	for _, d := range domains {
		if d.Environment != deployment.Environment {
			continue
		}

		found = true
		if !d.IsAuto && !d.DNSVerified {
			emit(fmt.Sprintf("Skipping domain %s until DNS is verified", d.DomainName))
			continue
		}

		plan := domain.ResolveVariantPlan(d.DomainName, d.Environment)
		verified := true
		for _, hostname := range plan.AllDomains() {
			hostVerified, err := p.DomainManager.VerifyDomainDNS(hostname)
			if err != nil {
				_ = p.DB.UpdateDomainDNS(d.ID, false)
				_ = p.DB.UpdateDomainSSL(d.ID, "error", d.SSLExpiresAt)
				return fmt.Errorf("%s: verify dns: %w", hostname, err)
			}
			if !hostVerified {
				verified = false
				break
			}
		}

		_ = p.DB.UpdateDomainDNS(d.ID, verified)
		if !verified {
			_ = p.DB.UpdateDomainSSL(d.ID, "pending", d.SSLExpiresAt)
			return fmt.Errorf("%s: %w", d.DomainName, domain.ErrDNSNotVerified)
		}

		emit(fmt.Sprintf("Provisioning domain %s...", plan.CanonicalDomain))
		passwordProtected := (d.Environment == "preview" && project.PreviewPassword != "") ||
			(d.Environment == "production" && project.ProductionPassword != "")
		err = p.DomainManager.ProvisionDomain(domain.ProvisionConfig{
			ProjectID:         project.ID,
			ProjectName:       project.Name,
			Domain:            plan.CanonicalDomain,
			RedirectDomain:    plan.RedirectDomain,
			SSLDomains:        plan.AllDomains(),
			Environment:       d.Environment,
			ContainerName:     containerName,
			ContainerUpstream: containerUpstream,
			PasswordProtected: passwordProtected,
		}, false, nil)
		if err != nil {
			_ = p.DB.UpdateDomainSSL(d.ID, "error", d.SSLExpiresAt)
			return fmt.Errorf("%s: %w", d.DomainName, err)
		}

		_ = p.DB.UpdateDomainSSL(d.ID, "active", d.SSLExpiresAt)
		emit(fmt.Sprintf("Domain %s ready", d.DomainName))
	}

	if !found {
		emit("No domains configured for this environment")
	}

	return nil
}
