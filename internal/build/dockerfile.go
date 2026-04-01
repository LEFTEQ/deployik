package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// DockerfileData holds values for the Dockerfile template.
type DockerfileData struct {
	NodeVersion    string
	InstallCommand string
	BuildCommand   string
	HasBunLock     bool
	HasPnpmLock    bool
	UseBun         bool
	UsePnpm        bool
	BuildEnvVars   []EnvVar
}

type EnvVar struct {
	Key   string
	Value string
}

// GenerateDockerfile creates a Dockerfile in the repo directory.
// If a Dockerfile already exists, it is used as-is.
// Otherwise, generates one from the Next.js template.
func GenerateDockerfile(repoDir string, data DockerfileData) (string, error) {
	dockerfilePath := filepath.Join(repoDir, "Dockerfile")

	// Check if user already has a Dockerfile
	if _, err := os.Stat(dockerfilePath); err == nil {
		return dockerfilePath, nil
	}

	// Detect package manager from lock files
	if _, err := os.Stat(filepath.Join(repoDir, "bun.lockb")); err == nil {
		data.HasBunLock = true
		data.UseBun = true
	} else if _, err := os.Stat(filepath.Join(repoDir, "bun.lock")); err == nil {
		data.HasBunLock = true
		data.UseBun = true
	} else if _, err := os.Stat(filepath.Join(repoDir, "pnpm-lock.yaml")); err == nil {
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

	// Generate Dockerfile content
	content := generateNextJSDockerfile(data)

	if err := os.WriteFile(dockerfilePath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write Dockerfile: %w", err)
	}

	return dockerfilePath, nil
}

func generateNextJSDockerfile(data DockerfileData) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("FROM node:%s-alpine AS base\n", data.NodeVersion))
	b.WriteString("WORKDIR /app\n\n")

	// Dependencies stage
	b.WriteString("FROM base AS deps\n")

	// Copy lock files
	copyFiles := []string{"package.json"}
	if data.HasBunLock {
		copyFiles = append(copyFiles, "bun.lockb", "bun.lock")
	} else if data.HasPnpmLock {
		copyFiles = append(copyFiles, "pnpm-lock.yaml")
	} else {
		copyFiles = append(copyFiles, "package-lock.json")
	}
	b.WriteString(fmt.Sprintf("COPY %s ./\n", strings.Join(copyFiles, " ")))

	// Install dependencies
	if data.UseBun {
		b.WriteString("RUN npm i -g bun && bun install --frozen-lockfile\n\n")
	} else if data.UsePnpm {
		b.WriteString("RUN corepack enable && pnpm install --frozen-lockfile\n\n")
	} else {
		b.WriteString("RUN npm ci\n\n")
	}

	// Build stage — package manager must also be available here
	b.WriteString("FROM base AS builder\n")
	if data.UseBun {
		b.WriteString("RUN npm i -g bun\n")
	} else if data.UsePnpm {
		b.WriteString("RUN corepack enable\n")
	}
	b.WriteString("COPY --from=deps /app/node_modules ./node_modules\n")
	b.WriteString("COPY . .\n")

	// Build-time env vars (NEXT_PUBLIC_*)
	for _, ev := range data.BuildEnvVars {
		b.WriteString(fmt.Sprintf("ENV %s=%s\n", ev.Key, ev.Value))
	}

	b.WriteString(fmt.Sprintf("RUN %s\n\n", data.BuildCommand))

	// Runtime stage
	b.WriteString("FROM base AS runner\n")
	b.WriteString("ENV NODE_ENV=production\n")
	b.WriteString("RUN addgroup --system --gid 1001 nodejs\n")
	b.WriteString("RUN adduser --system --uid 1001 nextjs\n\n")

	b.WriteString("COPY --from=builder /app/public ./public\n")
	b.WriteString("COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./\n")
	b.WriteString("COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static\n\n")

	b.WriteString("USER nextjs\n")
	b.WriteString("EXPOSE 3000\n")
	b.WriteString("ENV PORT=3000\n")
	b.WriteString("ENV HOSTNAME=\"0.0.0.0\"\n")
	b.WriteString("CMD [\"node\", \"server.js\"]\n")

	return b.String()
}

// ParseTemplateFile parses a Go template file (for future extensibility).
func ParseTemplateFile(path string) (*template.Template, error) {
	return template.ParseFiles(path)
}
