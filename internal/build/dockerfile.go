package build

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/LEFTEQ/lovinka-deployik/internal/projectconfig"
)

// DockerfileData holds values for the Dockerfile template.
type DockerfileData struct {
	NodeVersion      string
	InstallCommand   string
	BuildCommand     string
	RootDirectory    string
	OutputDirectory  string
	Runtime          string
	HasBunLock       bool
	HasPnpmLock      bool
	UseBun           bool
	UsePnpm          bool
	BuildEnvVars     []EnvVar
	InstallDirectory string
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
		data.UseBun = true
	} else if _, err := os.Stat(filepath.Join(installDirAbs, "bun.lock")); err == nil {
		data.HasBunLock = true
		data.UseBun = true
	} else if _, err := os.Stat(filepath.Join(installDirAbs, "pnpm-lock.yaml")); err == nil {
		data.HasPnpmLock = true
		data.UsePnpm = true
	}

	// Override install/build commands to match detected package manager.
	// The project config may have defaults (e.g. "bun run build") that don't
	// match the actual repo. The lock file is the source of truth.
	if data.UsePnpm {
		data.InstallCommand = "corepack enable && pnpm install --frozen-lockfile"
		if !strings.Contains(data.BuildCommand, "pnpm") {
			data.BuildCommand = "pnpm run build"
		}
	} else if data.UseBun {
		data.InstallCommand = "bun install --frozen-lockfile"
		if !strings.Contains(data.BuildCommand, "bun") {
			data.BuildCommand = "bun run build"
		}
	} else {
		if data.InstallCommand == "" {
			data.InstallCommand = "npm ci"
		}
		if data.BuildCommand == "" || strings.HasPrefix(data.BuildCommand, "bun ") || strings.HasPrefix(data.BuildCommand, "pnpm ") {
			data.BuildCommand = "npm run build"
		}
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
		b.WriteString("RUN npm i -g bun && bun install --frozen-lockfile\n\n")
	} else if data.UsePnpm {
		b.WriteString("RUN corepack enable && pnpm install --frozen-lockfile\n\n")
	} else {
		b.WriteString(fmt.Sprintf("RUN %s\n\n", data.InstallCommand))
	}

	// Build stage — package manager must also be available here
	b.WriteString("FROM deps AS builder\n")
	b.WriteString(fmt.Sprintf("WORKDIR %s\n", appDir))

	// Build-time env vars (NEXT_PUBLIC_*)
	for _, ev := range data.BuildEnvVars {
		b.WriteString(fmt.Sprintf("ENV %s=%s\n", ev.Key, ev.Value))
	}

	b.WriteString(fmt.Sprintf("RUN %s\n\n", data.BuildCommand))
	b.WriteString("RUN mkdir -p /tmp/deployik/public && if [ -d public ]; then cp -R public/. /tmp/deployik/public/; fi\n\n")

	// Runtime stage
	b.WriteString("FROM base AS runner\n")
	b.WriteString("ENV NODE_ENV=production\n")
	b.WriteString("RUN addgroup --system --gid 1001 nodejs\n")
	b.WriteString("RUN adduser --system --uid 1001 nextjs\n\n")

	b.WriteString("COPY --from=builder /tmp/deployik/public ./public\n")
	b.WriteString(fmt.Sprintf("COPY --from=builder --chown=nextjs:nodejs %s/standalone ./\n", outputDir))
	b.WriteString(fmt.Sprintf("COPY --from=builder --chown=nextjs:nodejs %s/static %s\n\n", outputDir, staticTarget))

	b.WriteString("USER nextjs\n")
	b.WriteString("EXPOSE 3000\n")
	b.WriteString("ENV PORT=3000\n")
	b.WriteString("ENV HOSTNAME=\"0.0.0.0\"\n")
	b.WriteString("CMD [\"node\", \"server.js\"]\n")

	return b.String()
}

func generateStaticDockerfile(data DockerfileData) string {
	var b strings.Builder
	appDir := dockerAppDir(data.RootDirectory)
	installDir := dockerAppDir(data.InstallDirectory)
	outputDir := dockerProjectPath(data.RootDirectory, data.OutputDirectory)

	b.WriteString(fmt.Sprintf("FROM node:%s-alpine AS base\n", data.NodeVersion))
	b.WriteString("WORKDIR /app\n\n")

	b.WriteString("FROM base AS deps\n")
	b.WriteString("COPY . .\n")
	if installDir != "/app" {
		b.WriteString(fmt.Sprintf("WORKDIR %s\n", installDir))
	}
	if data.UseBun {
		b.WriteString("RUN npm i -g bun && bun install --frozen-lockfile\n\n")
	} else if data.UsePnpm {
		b.WriteString("RUN corepack enable && pnpm install --frozen-lockfile\n\n")
	} else {
		b.WriteString(fmt.Sprintf("RUN %s\n\n", data.InstallCommand))
	}

	b.WriteString("FROM deps AS builder\n")
	b.WriteString(fmt.Sprintf("WORKDIR %s\n", appDir))
	for _, ev := range data.BuildEnvVars {
		b.WriteString(fmt.Sprintf("ENV %s=%s\n", ev.Key, ev.Value))
	}
	b.WriteString(fmt.Sprintf("RUN %s\n\n", data.BuildCommand))

	b.WriteString("FROM base AS runner\n")
	b.WriteString("ENV NODE_ENV=production\n")
	b.WriteString("RUN npm i -g serve@14\n\n")
	b.WriteString(fmt.Sprintf("COPY --from=builder %s ./site\n\n", outputDir))
	b.WriteString("EXPOSE 3000\n")
	b.WriteString("ENV PORT=3000\n")
	b.WriteString("CMD [\"serve\", \"-s\", \"site\", \"-l\", \"3000\"]\n")

	return b.String()
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
		for _, candidate := range []string{"bun.lockb", "bun.lock", "pnpm-lock.yaml", "package-lock.json"} {
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

// ParseTemplateFile parses a Go template file (for future extensibility).
func ParseTemplateFile(path string) (*template.Template, error) {
	return template.ParseFiles(path)
}
