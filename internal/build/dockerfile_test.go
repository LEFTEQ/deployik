package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LEFTEQ/lovinka-deployik/internal/projectconfig"
)

func TestGenerateDockerfileSupportsRootDirectoryAndNextOutput(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, "apps/web"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "pnpm-lock.yaml"), []byte("lockfileVersion: '9.0'"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dockerfilePath, err := GenerateDockerfile(repoDir, DockerfileData{
		PackageManager:  projectconfig.PackageManagerPnpm,
		NodeVersion:     "22",
		RootDirectory:   "apps/web",
		OutputDirectory: ".next",
		Runtime:         projectconfig.RuntimeNextJSStandalone,
		BuildCommand:    "pnpm run build",
		InstallCommand:  "pnpm install",
	})
	if err != nil {
		t.Fatalf("GenerateDockerfile: %v", err)
	}

	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	got := string(content)
	if !strings.HasPrefix(got, "# syntax=docker/dockerfile:") {
		t.Fatalf("expected BuildKit syntax directive on first line, got:\n%s", got)
	}
	if !strings.Contains(got, "WORKDIR /app/apps/web") {
		t.Fatalf("expected build workdir to target root directory, got:\n%s", got)
	}
	if !strings.Contains(got, "COPY --from=builder --chown=nextjs:nodejs /app/apps/web/.next/standalone ./") {
		t.Fatalf("expected standalone copy path to include root directory, got:\n%s", got)
	}
	if !strings.Contains(got, "COPY --from=builder --chown=nextjs:nodejs /app/apps/web/.next/static /app/.next/static") {
		t.Fatalf("expected static asset copy path to include output directory, got:\n%s", got)
	}
	if !strings.Contains(got, "--mount=type=cache,target=/root/.local/share/pnpm/store") {
		t.Fatalf("expected pnpm install cache mount, got:\n%s", got)
	}
	if !strings.Contains(got, "--mount=type=cache,target=/app/apps/web/.next/cache") {
		t.Fatalf("expected next incremental-build cache mount scoped to app dir, got:\n%s", got)
	}
	if !strings.Contains(got, "--mount=type=secret,id=deployik-secrets") {
		t.Fatalf("expected secret mount for build-time env vars, got:\n%s", got)
	}
	if !strings.Contains(got, "if [ -f /run/secrets/deployik-secrets ]") {
		t.Fatalf("expected build command to source secrets file when present, got:\n%s", got)
	}
}

func TestGenerateDockerfileSupportsStaticRuntime(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoDir, "package.json"), []byte(`{"name":"app"}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dockerfilePath, err := GenerateDockerfile(repoDir, DockerfileData{
		PackageManager:  projectconfig.PackageManagerNpm,
		NodeVersion:     "22",
		OutputDirectory: "dist",
		Runtime:         projectconfig.RuntimeStatic,
		BuildCommand:    "npm run build",
		InstallCommand:  "npm ci",
	})
	if err != nil {
		t.Fatalf("GenerateDockerfile: %v", err)
	}

	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	got := string(content)
	if !strings.HasPrefix(got, "# syntax=docker/dockerfile:") {
		t.Fatalf("expected BuildKit syntax directive on first line, got:\n%s", got)
	}
	if !strings.Contains(got, "RUN npm i -g serve@14") {
		t.Fatalf("expected static runtime to install serve, got:\n%s", got)
	}
	if !strings.Contains(got, "COPY --from=builder /app/dist ./site") {
		t.Fatalf("expected static runtime to copy dist output, got:\n%s", got)
	}
	if !strings.Contains(got, "CMD [\"serve\", \"-s\", \"site\", \"-l\", \"3000\"]") {
		t.Fatalf("expected static runtime serve command, got:\n%s", got)
	}
}

func TestGenerateDockerfileQuotesBuildEnvValues(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoDir, "package.json"), []byte(`{"name":"app"}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dockerfilePath, err := GenerateDockerfile(repoDir, DockerfileData{
		PackageManager: projectconfig.PackageManagerNpm,
		NodeVersion:    "22",
		Runtime:        projectconfig.RuntimeStatic,
		BuildCommand:   "npm run build",
		InstallCommand: "npm ci",
		BuildEnvVars: []EnvVar{
			{Key: "NEXT_PUBLIC_SITE_NAME", Value: `Acme "Preview"`},
		},
	})
	if err != nil {
		t.Fatalf("GenerateDockerfile: %v", err)
	}

	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	got := string(content)
	if !strings.Contains(got, `ENV NEXT_PUBLIC_SITE_NAME="Acme \"Preview\""`) {
		t.Fatalf("expected quoted env assignment, got:\n%s", got)
	}
}

func TestGenerateDockerfileAutoDetectsNpmFromPackageLockJSON(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoDir, "package.json"), []byte(`{"name":"app"}`), 0644); err != nil {
		t.Fatalf("WriteFile package.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "package-lock.json"), []byte(`{"lockfileVersion":3}`), 0644); err != nil {
		t.Fatalf("WriteFile package-lock.json: %v", err)
	}

	dockerfilePath, err := GenerateDockerfile(repoDir, DockerfileData{
		PackageManager:  projectconfig.PackageManagerAuto,
		NodeVersion:     "22",
		OutputDirectory: "dist",
		Runtime:         projectconfig.RuntimeStatic,
		// Bun defaults from Resolve() — auto detection should override these.
		InstallCommand: "bun install --frozen-lockfile",
		BuildCommand:   "bun run build",
	})
	if err != nil {
		t.Fatalf("GenerateDockerfile: %v", err)
	}

	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	got := string(content)
	if strings.Contains(got, "npm i -g bun") {
		t.Fatalf("expected no bun install when package-lock.json is present, got:\n%s", got)
	}
	if !strings.Contains(got, "--mount=type=cache,target=/root/.npm") {
		t.Fatalf("expected npm install cache mount, got:\n%s", got)
	}
	if !strings.Contains(got, "npm ci") {
		t.Fatalf("expected npm ci install command, got:\n%s", got)
	}
	if !strings.Contains(got, "npm run build") {
		t.Fatalf("expected npm run build command, got:\n%s", got)
	}
}

func TestGenerateDockerfileExplicitNpmDoesNotInstallBun(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoDir, "package.json"), []byte(`{"name":"app"}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dockerfilePath, err := GenerateDockerfile(repoDir, DockerfileData{
		PackageManager:  projectconfig.PackageManagerNpm,
		NodeVersion:     "22",
		OutputDirectory: "dist",
		Runtime:         projectconfig.RuntimeStatic,
		InstallCommand:  "npm ci",
		BuildCommand:    "npm run build",
	})
	if err != nil {
		t.Fatalf("GenerateDockerfile: %v", err)
	}

	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	got := string(content)
	if strings.Contains(got, "npm i -g bun") {
		t.Fatalf("expected no bun install with explicit npm package manager, got:\n%s", got)
	}
	if !strings.Contains(got, "npm ci") {
		t.Fatalf("expected npm ci install command, got:\n%s", got)
	}
}

func TestGenerateDockerfileHonorsCustomPort(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoDir, "package.json"), []byte(`{"name":"app"}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dockerfilePath, err := GenerateDockerfile(repoDir, DockerfileData{
		PackageManager:  projectconfig.PackageManagerNpm,
		NodeVersion:     "22",
		OutputDirectory: "dist",
		Runtime:         projectconfig.RuntimeStatic,
		InstallCommand:  "npm ci",
		BuildCommand:    "npm run build",
		Port:            8080,
	})
	if err != nil {
		t.Fatalf("GenerateDockerfile: %v", err)
	}

	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	got := string(content)
	for _, want := range []string{
		"EXPOSE 8080",
		"ENV PORT=8080",
		"localhost:8080",
		`"-l", "8080"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected generated Dockerfile to contain %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "3000") {
		t.Fatalf("expected no 3000 fallback when custom port is set, got:\n%s", got)
	}
}

func TestGenerateDockerfileDefaultsPortTo3000(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoDir, "package.json"), []byte(`{"name":"app"}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dockerfilePath, err := GenerateDockerfile(repoDir, DockerfileData{
		PackageManager:  projectconfig.PackageManagerNpm,
		NodeVersion:     "22",
		OutputDirectory: "dist",
		Runtime:         projectconfig.RuntimeStatic,
		InstallCommand:  "npm ci",
		BuildCommand:    "npm run build",
		// Port intentionally unset
	})
	if err != nil {
		t.Fatalf("GenerateDockerfile: %v", err)
	}

	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	got := string(content)
	for _, want := range []string{"EXPOSE 3000", "ENV PORT=3000", `"-l", "3000"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected default port 3000, got:\n%s", got)
		}
	}
}

func TestGenerateDockerfileSupportsYarnPackageManager(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoDir, "package.json"), []byte(`{"name":"app"}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dockerfilePath, err := GenerateDockerfile(repoDir, DockerfileData{
		PackageManager:  projectconfig.PackageManagerYarn,
		NodeVersion:     "22",
		Runtime:         projectconfig.RuntimeStatic,
		BuildCommand:    "yarn build",
		InstallCommand:  "yarn install --frozen-lockfile",
		OutputDirectory: "dist",
	})
	if err != nil {
		t.Fatalf("GenerateDockerfile: %v", err)
	}

	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	got := string(content)
	if !strings.Contains(got, "RUN corepack enable") {
		t.Fatalf("expected yarn builds to enable corepack, got:\n%s", got)
	}
	if !strings.Contains(got, "--mount=type=cache,target=/root/.cache/yarn") {
		t.Fatalf("expected yarn install cache mount, got:\n%s", got)
	}
	if !strings.Contains(got, "yarn install --frozen-lockfile") {
		t.Fatalf("expected yarn install command, got:\n%s", got)
	}
	if !strings.Contains(got, "yarn build") {
		t.Fatalf("expected yarn build command, got:\n%s", got)
	}
}

func TestGenerateDockerfileSupportsNodeAPIRuntime(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoDir, "package.json"), []byte(`{"name":"api"}`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "bun.lock"), []byte("# bun lockfile v1\n"), 0644); err != nil {
		t.Fatalf("WriteFile bun.lock: %v", err)
	}

	dockerfilePath, err := GenerateDockerfile(repoDir, DockerfileData{
		PackageManager:  projectconfig.PackageManagerBun,
		NodeVersion:     "22",
		OutputDirectory: "dist",
		Runtime:         projectconfig.RuntimeNodeAPI,
		BuildCommand:    "bun run build",
		InstallCommand:  "bun install",
		StartCommand:    "node dist/main.js",
		HealthPath:      "/health",
		Port:            3000,
	})
	if err != nil {
		t.Fatalf("GenerateDockerfile: %v", err)
	}

	got, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(got)

	requireAll(t, content,
		"# syntax=docker/dockerfile:1.7",
		"FROM node:22-alpine AS base",
		"FROM base AS deps",
		"FROM deps AS builder",
		"--mount=type=secret,id=deployik-secrets",
		"FROM base AS runner",
		"ENV NODE_ENV=production",
		"COPY --from=builder /app/dist ./dist",
		"COPY --from=deps /app/node_modules ./node_modules",
		"EXPOSE 3000",
		"ENV PORT=3000",
		"ENV HOSTNAME=\"0.0.0.0\"",
		"HEALTHCHECK",
		"/health",
		`CMD ["sh", "-c", "node dist/main.js"]`,
	)
}

// requireAll fails the test if `content` is missing any of the expected
// substrings, reporting all misses in one shot instead of one per t.Fatalf.
func requireAll(t *testing.T, content string, needles ...string) {
	t.Helper()
	var missing []string
	for _, n := range needles {
		if !strings.Contains(content, n) {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("Dockerfile missing %d expected substrings: %v\n--- generated ---\n%s", len(missing), missing, content)
	}
}
