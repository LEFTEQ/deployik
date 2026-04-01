package build

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/domain"
	"github.com/LEFTEQ/lovinka-deployik/internal/projectconfig"
	"github.com/LEFTEQ/lovinka-deployik/internal/ws"
)

// Pipeline orchestrates the full deploy flow.
type Pipeline struct {
	DB            *db.DB
	Docker        *DockerClient
	Encryptor     *crypto.Encryptor
	Semaphore     *Semaphore
	DomainManager *domain.Manager
	BuildDir      string
	ProxyNetwork  string // Docker network name (e.g., "proxy")
	Hub           *ws.Hub
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
	emit(fmt.Sprintf("Starting build for %s/%s@%s", project.GithubOwner, project.GithubRepo, project.Branch))

	// Step 2: Clone repository
	emit("Cloning repository...")
	buildDir := fmt.Sprintf("%s/%s", p.BuildDir, deployment.ID)
	os.MkdirAll(buildDir, 0755)
	defer os.RemoveAll(buildDir) // Always clean up

	repoDir, err := CloneRepo(buildDir, project.GithubOwner, project.GithubRepo, project.Branch, githubToken)
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
	})
	if err != nil {
		fail(err, "Dockerfile generation failed")
		return
	}
	emit("Dockerfile ready")

	// Step 5: Docker build
	containerName := fmt.Sprintf("deployik-%s-%s", project.Name, deployment.Environment)
	imageTag := fmt.Sprintf("deployik-%s-%s:%s", project.Name, deployment.Environment, deployment.ID[:8])

	emit(fmt.Sprintf("Building image %s...", imageTag))
	_, err = p.Docker.BuildImage(ctx, repoDir, dockerfilePath, imageTag, func(line string) {
		logLineNum++
		p.DB.InsertBuildLog(deployment.ID, logLineNum, line, "stdout")
		if p.Hub != nil {
			p.Hub.Publish(ws.LogLine{DeploymentID: deployment.ID, LineNumber: logLineNum, Content: line, Stream: "stdout"})
		}
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

	// Start new container with temporary name
	tempName := containerName + "-" + deployment.ID[:8]
	newContainerID, err := p.Docker.RunContainer(ctx, tempName, imageTag, runtimeEnvVars, p.ProxyNetwork)
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

	if p.DomainManager != nil {
		emit("Ensuring environment domains...")
		if err := p.ensureEnvironmentDomains(project, deployment, containerName, emit); err != nil {
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
}

func (p *Pipeline) ensureEnvironmentDomains(project *db.Project, deployment *db.Deployment, containerName string, emit func(string)) error {
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

		verified, err := p.DomainManager.VerifyDomainDNS(d.DomainName)
		if err != nil {
			_ = p.DB.UpdateDomainDNS(d.ID, false)
			_ = p.DB.UpdateDomainSSL(d.ID, "error", d.SSLExpiresAt)
			return fmt.Errorf("%s: verify dns: %w", d.DomainName, err)
		}
		_ = p.DB.UpdateDomainDNS(d.ID, verified)
		if !verified {
			_ = p.DB.UpdateDomainSSL(d.ID, "pending", d.SSLExpiresAt)
			return fmt.Errorf("%s: %w", d.DomainName, domain.ErrDNSNotVerified)
		}

		emit(fmt.Sprintf("Provisioning domain %s...", d.DomainName))
		err = p.DomainManager.ProvisionDomain(domain.ProvisionConfig{
			ProjectName:   project.Name,
			Domain:        d.DomainName,
			ContainerName: containerName,
		}, false)
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
