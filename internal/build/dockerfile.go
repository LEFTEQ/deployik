package build

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/LEFTEQ/lovinka-deployik/internal/projectconfig"
)

// DockerfileData holds values for the Dockerfile template.
type DockerfileData struct {
	PackageManager   string
	NodeVersion      string
	InstallCommand   string
	BuildCommand     string
	RootDirectory    string
	OutputDirectory  string
	Runtime          string
	HasBunLock       bool
	HasPnpmLock      bool
	HasYarnLock      bool
	HasNpmLock       bool
	UseBun           bool
	UsePnpm          bool
	UseYarn          bool
	BuildEnvVars     []EnvVar
	InstallDirectory string
	// ProjectID scopes project-specific BuildKit cache mounts (e.g. Next.js
	// `.next/cache`) so incremental-build caches of one project don't leak into
	// another. Optional — falls back to a shared cache when empty.
	ProjectID string
	// Port is the TCP port the generated container binds to. Injected as
	// `ENV PORT=<port>`, `EXPOSE <port>`, HEALTHCHECK URL, and `serve -l` port.
	// Zero defaults to 3000 (Deployik's historical default).
	Port int
}

type EnvVar struct {
	Key   string
	Value string
}

// GenerateDockerfile creates a Dockerfile in the repo directory.
// If a Dockerfile already exists, it is used as-is.
// Otherwise, generates one from the selected framework runtime.
func GenerateDockerfile(repoDir string, data DockerfileData) (string, error) {
	dockerfilePath := filepath.Join(repoDir, "Dockerfile")
	appDir := repoDir
	if data.RootDirectory != "" {
		appDir = filepath.Join(repoDir, filepath.FromSlash(data.RootDirectory))
	}

	// Check if user already has a Dockerfile
	if _, err := os.Stat(dockerfilePath); err == nil {
		return dockerfilePath, nil
	}
	if data.RootDirectory != "" {
		appDockerfilePath := filepath.Join(appDir, "Dockerfile")
		if _, err := os.Stat(appDockerfilePath); err == nil {
			return appDockerfilePath, nil
		}
	}

	// Detect package manager from lock files, preferring repo root for monorepos.
	data.InstallDirectory = detectInstallDirectory(repoDir, data.RootDirectory)
	installDirAbs := repoDir
	if data.InstallDirectory != "" {
		installDirAbs = filepath.Join(repoDir, filepath.FromSlash(data.InstallDirectory))
	}

	if _, err := os.Stat(filepath.Join(installDirAbs, "bun.lockb")); err == nil {
		data.HasBunLock = true
	} else if _, err := os.Stat(filepath.Join(installDirAbs, "bun.lock")); err == nil {
		data.HasBunLock = true
	} else if _, err := os.Stat(filepath.Join(installDirAbs, "pnpm-lock.yaml")); err == nil {
		data.HasPnpmLock = true
	} else if _, err := os.Stat(filepath.Join(installDirAbs, "yarn.lock")); err == nil {
		data.HasYarnLock = true
	} else if _, err := os.Stat(filepath.Join(installDirAbs, "package-lock.json")); err == nil {
		data.HasNpmLock = true
	}

	effectiveManager := resolvePackageManager(data)
	switch effectiveManager {
	case projectconfig.PackageManagerPnpm:
		data.UsePnpm = true
	case projectconfig.PackageManagerYarn:
		data.UseYarn = true
	case projectconfig.PackageManagerNpm:
	default:
		data.UseBun = true
	}

	if data.InstallCommand == "" || (isAutoPackageManager(data.PackageManager) && isKnownInstallDefault(data.InstallCommand)) {
		data.InstallCommand = projectconfig.DefaultInstallCommand(effectiveManager)
	}
	if data.BuildCommand == "" || (isAutoPackageManager(data.PackageManager) && isKnownBuildDefault(data.BuildCommand)) {
		data.BuildCommand = projectconfig.DefaultBuildCommand(effectiveManager)
	}

	if data.NodeVersion == "" {
		data.NodeVersion = "22"
	}
	if data.OutputDirectory == "" {
		if data.Runtime == projectconfig.RuntimeNextJSStandalone {
			data.OutputDirectory = ".next"
		} else {
			data.OutputDirectory = "dist"
		}
	}

	// Generate Dockerfile content
	content := generateDockerfileContent(data)

	if err := os.WriteFile(dockerfilePath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write Dockerfile: %w", err)
	}

	return dockerfilePath, nil
}

func generateDockerfileContent(data DockerfileData) string {
	if data.Runtime == projectconfig.RuntimeStatic {
		return generateStaticDockerfile(data)
	}
	return generateNextJSDockerfile(data)
}

func generateNextJSDockerfile(data DockerfileData) string {
	var b strings.Builder
	appDir := dockerAppDir(data.RootDirectory)
	installDir := dockerAppDir(data.InstallDirectory)
	outputDir := dockerProjectPath(data.RootDirectory, data.OutputDirectory)
	staticTarget := dockerProjectPath("", path.Join(data.OutputDirectory, "static"))
	nextCacheDir := path.Join(appDir, ".next/cache")

	// `# syntax` must be the very first line of the file for BuildKit to honor it.
	b.WriteString("# syntax=docker/dockerfile:1.7\n")
	b.WriteString(fmt.Sprintf("FROM node:%s-alpine AS base\n", data.NodeVersion))
	b.WriteString("WORKDIR /app\n\n")

	// Dependencies stage
	b.WriteString("FROM base AS deps\n")
	b.WriteString("COPY . .\n")
	if installDir != "/app" {
		b.WriteString(fmt.Sprintf("WORKDIR %s\n", installDir))
	}

	// Install dependencies
	if data.UseBun {
		b.WriteString("RUN npm i -g bun\n")
	} else if data.UsePnpm {
		b.WriteString("RUN corepack enable\n")
	} else if data.UseYarn {
		b.WriteString("RUN corepack enable\n")
	}
	b.WriteString(installRunLine(data))

	// Build stage — package manager must also be available here
	b.WriteString("\nFROM deps AS builder\n")
	b.WriteString(fmt.Sprintf("WORKDIR %s\n", appDir))

	// Build-time env vars (NEXT_PUBLIC_*)
	for _, ev := range data.BuildEnvVars {
		b.WriteString(fmt.Sprintf("ENV %s=%s\n", ev.Key, strconv.Quote(ev.Value)))
	}

	buildCacheID := nextCacheMountID(data.ProjectID)
	b.WriteString(buildRunLine(data.BuildCommand, []string{
		fmt.Sprintf("--mount=type=cache,target=%s,id=%s,sharing=locked", nextCacheDir, buildCacheID),
		"--mount=type=secret,id=deployik-secrets",
	}))
	b.WriteString("\nRUN mkdir -p /tmp/deployik/public && if [ -d public ]; then cp -R public/. /tmp/deployik/public/; fi\n\n")

	// Runtime stage
	b.WriteString("FROM base AS runner\n")
	b.WriteString("ENV NODE_ENV=production\n")
	b.WriteString("RUN addgroup --system --gid 1001 nodejs\n")
	b.WriteString("RUN adduser --system --uid 1001 nextjs\n")
	b.WriteString("RUN apk --no-cache del wget curl 2>/dev/null; rm -rf /var/cache/apk/*\n\n")

	b.WriteString("COPY --from=builder /tmp/deployik/public ./public\n")
	b.WriteString(fmt.Sprintf("COPY --from=builder --chown=nextjs:nodejs %s/standalone ./\n", outputDir))
	b.WriteString(fmt.Sprintf("COPY --from=builder --chown=nextjs:nodejs %s/static %s\n\n", outputDir, staticTarget))

	port := effectivePort(data.Port)
	b.WriteString("USER nextjs\n")
	b.WriteString(fmt.Sprintf("EXPOSE %d\n", port))
	b.WriteString(fmt.Sprintf("ENV PORT=%d\n", port))
	b.WriteString("ENV HOSTNAME=\"0.0.0.0\"\n")
	b.WriteString("HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \\\n")
	b.WriteString(fmt.Sprintf("  CMD node -e \"require('http').get('http://localhost:%d/',(r)=>{process.exit(r.statusCode<400?0:1)}).on('error',()=>process.exit(1))\"\n", port))
	b.WriteString("CMD [\"node\", \"server.js\"]\n")

	return b.String()
}

func generateStaticDockerfile(data DockerfileData) string {
	var b strings.Builder
	appDir := dockerAppDir(data.RootDirectory)
	installDir := dockerAppDir(data.InstallDirectory)
	outputDir := dockerProjectPath(data.RootDirectory, data.OutputDirectory)

	b.WriteString("# syntax=docker/dockerfile:1.7\n")
	b.WriteString(fmt.Sprintf("FROM node:%s-alpine AS base\n", data.NodeVersion))
	b.WriteString("WORKDIR /app\n\n")

	b.WriteString("FROM base AS deps\n")
	b.WriteString("COPY . .\n")
	if installDir != "/app" {
		b.WriteString(fmt.Sprintf("WORKDIR %s\n", installDir))
	}
	if data.UseBun {
		b.WriteString("RUN npm i -g bun\n")
	} else if data.UsePnpm {
		b.WriteString("RUN corepack enable\n")
	} else if data.UseYarn {
		b.WriteString("RUN corepack enable\n")
	}
	b.WriteString(installRunLine(data))

	b.WriteString("\nFROM deps AS builder\n")
	b.WriteString(fmt.Sprintf("WORKDIR %s\n", appDir))
	for _, ev := range data.BuildEnvVars {
		b.WriteString(fmt.Sprintf("ENV %s=%s\n", ev.Key, strconv.Quote(ev.Value)))
	}
	b.WriteString(buildRunLine(data.BuildCommand, []string{
		"--mount=type=secret,id=deployik-secrets",
	}))
	b.WriteString("\n")

	b.WriteString("FROM base AS runner\n")
	b.WriteString("ENV NODE_ENV=production\n")
	b.WriteString("RUN npm i -g serve@14 && apk --no-cache del wget curl 2>/dev/null; rm -rf /var/cache/apk/*\n\n")
	b.WriteString(fmt.Sprintf("COPY --from=builder %s ./site\n\n", outputDir))
	port := effectivePort(data.Port)
	b.WriteString(fmt.Sprintf("EXPOSE %d\n", port))
	b.WriteString(fmt.Sprintf("ENV PORT=%d\n", port))
	b.WriteString("HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \\\n")
	b.WriteString(fmt.Sprintf("  CMD node -e \"require('http').get('http://localhost:%d/',(r)=>{process.exit(r.statusCode===200?0:1)}).on('error',()=>process.exit(1))\"\n", port))
	b.WriteString(fmt.Sprintf("CMD [\"serve\", \"-s\", \"site\", \"-l\", \"%d\"]\n", port))

	return b.String()
}

// installRunLine renders the install-command RUN step, prefixed with a
// BuildKit cache mount for the effective package manager's on-disk cache.
// Falls back to a plain `RUN <cmd>` when no cache target is known — keeps
// behavior identical on classic-builder fallbacks (should never happen now
// that Deployik always goes through buildx).
func installRunLine(data DockerfileData) string {
	cacheFlag := packageManagerCacheMount(resolvePackageManager(data))
	if cacheFlag == "" {
		return fmt.Sprintf("RUN %s\n", data.InstallCommand)
	}
	return fmt.Sprintf("RUN %s \\\n    %s\n", cacheFlag, data.InstallCommand)
}

// buildRunLine renders the build-command RUN step with the given BuildKit
// mount flags (expected to include `--mount=type=secret,id=deployik-secrets`
// and, for frameworks that have one, an incremental-build cache mount). When
// the secret mount is present, the command is wrapped so that the secrets
// file — if BuildKit materialised it — is sourced into the environment before
// the build runs. This matches the Vercel semantics where build steps can
// read any project env var without those values leaking into image layers.
func buildRunLine(command string, mountFlags []string) string {
	hasSecret := false
	for _, m := range mountFlags {
		if strings.Contains(m, "type=secret") && strings.Contains(m, "id=deployik-secrets") {
			hasSecret = true
			break
		}
	}

	body := command
	if hasSecret {
		body = fmt.Sprintf(
			"if [ -f /run/secrets/deployik-secrets ]; then set -a && . /run/secrets/deployik-secrets && set +a; fi && %s",
			command,
		)
	}

	if len(mountFlags) == 0 {
		return fmt.Sprintf("RUN %s\n", body)
	}
	return fmt.Sprintf("RUN %s \\\n    %s\n", strings.Join(mountFlags, " "), body)
}

// packageManagerCacheMount returns the BuildKit cache-mount flag for the
// given package manager's on-disk install cache, or "" if none is known.
// Paths assume the default root user of the node:*-alpine base image (HOME=/root).
func packageManagerCacheMount(pm string) string {
	switch pm {
	case projectconfig.PackageManagerBun:
		return "--mount=type=cache,target=/root/.bun/install/cache,sharing=locked"
	case projectconfig.PackageManagerPnpm:
		return "--mount=type=cache,target=/root/.local/share/pnpm/store,sharing=locked"
	case projectconfig.PackageManagerYarn:
		return "--mount=type=cache,target=/root/.cache/yarn,sharing=locked"
	case projectconfig.PackageManagerNpm:
		return "--mount=type=cache,target=/root/.npm,sharing=locked"
	default:
		return ""
	}
}

// effectivePort returns the container listen port to bake into a generated
// Dockerfile, defaulting to 3000 when unset. Mirrors the same default used by
// docker.RunContainer and pipeline.go's upstream builder so the three stay in
// sync without callers having to pass matching zero-fallbacks.
func effectivePort(port int) int {
	if port < 1 || port > 65535 {
		return 3000
	}
	return port
}

// nextCacheMountID returns the BuildKit cache-mount `id=` for a project's
// Next.js incremental build cache. Scoped per project so incremental caches
// don't cross-contaminate between projects sharing the same builder.
func nextCacheMountID(projectID string) string {
	if projectID == "" {
		return "deployik-next-cache"
	}
	return "deployik-next-cache-" + projectID
}

func detectInstallDirectory(repoDir, rootDirectory string) string {
	directories := []struct {
		relative string
		absolute string
	}{
		{relative: "", absolute: repoDir},
	}

	if rootDirectory != "" {
		directories = append(directories, struct {
			relative string
			absolute string
		}{
			relative: rootDirectory,
			absolute: filepath.Join(repoDir, filepath.FromSlash(rootDirectory)),
		})
	}

	for _, dir := range directories {
		for _, candidate := range []string{"bun.lockb", "bun.lock", "pnpm-lock.yaml", "yarn.lock", "package-lock.json"} {
			if _, err := os.Stat(filepath.Join(dir.absolute, candidate)); err == nil {
				return dir.relative
			}
		}
	}

	for _, dir := range directories {
		for _, candidate := range []string{"package.json"} {
			if _, err := os.Stat(filepath.Join(dir.absolute, candidate)); err == nil {
				return dir.relative
			}
		}
	}

	return rootDirectory
}

func dockerAppDir(relative string) string {
	if relative == "" {
		return "/app"
	}
	return "/app/" + strings.TrimPrefix(path.Clean(relative), "./")
}

func dockerProjectPath(rootDirectory, relative string) string {
	parts := []string{"/app"}
	if rootDirectory != "" {
		parts = append(parts, strings.TrimPrefix(path.Clean(rootDirectory), "./"))
	}
	if relative != "" {
		parts = append(parts, strings.TrimPrefix(path.Clean(relative), "./"))
	}
	return path.Join(parts...)
}

func resolvePackageManager(data DockerfileData) string {
	normalized := projectconfig.NormalizePackageManager(data.PackageManager)
	if normalized != projectconfig.PackageManagerAuto {
		return normalized
	}
	if data.HasBunLock {
		return projectconfig.PackageManagerBun
	}
	if data.HasPnpmLock {
		return projectconfig.PackageManagerPnpm
	}
	if data.HasYarnLock {
		return projectconfig.PackageManagerYarn
	}
	if data.HasNpmLock {
		return projectconfig.PackageManagerNpm
	}

	commandText := strings.ToLower(strings.TrimSpace(data.InstallCommand + " " + data.BuildCommand))
	switch {
	case strings.Contains(commandText, " pnpm") || strings.HasPrefix(commandText, "pnpm"):
		return projectconfig.PackageManagerPnpm
	case strings.Contains(commandText, " yarn") || strings.HasPrefix(commandText, "yarn"):
		return projectconfig.PackageManagerYarn
	case strings.Contains(commandText, " npm") || strings.HasPrefix(commandText, "npm"):
		return projectconfig.PackageManagerNpm
	default:
		return projectconfig.PackageManagerBun
	}
}

func isAutoPackageManager(value string) bool {
	return projectconfig.NormalizePackageManager(value) == projectconfig.PackageManagerAuto
}

func isKnownInstallDefault(command string) bool {
	return projectconfig.IsKnownInstallDefault(command)
}

func isKnownBuildDefault(command string) bool {
	return projectconfig.IsKnownBuildDefault(command)
}

// ParseTemplateFile parses a Go template file (for future extensibility).
func ParseTemplateFile(path string) (*template.Template, error) {
	return template.ParseFiles(path)
}
