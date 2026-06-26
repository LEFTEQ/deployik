package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lefteq/lovinka-deployik/internal/projectconfig"
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
	// pnpm 10/11 makes ERR_PNPM_IGNORED_BUILDS a HARD error in non-TTY
	// contexts (docker buildx). The `.npmrc` / npm_config_* approaches are
	// silently ignored — only `--config.<key>=<value>` CLI flags work. We
	// append two of them to the install command for pnpm so:
	//   • install scripts of native deps (sharp/node-gyp/esbuild prebuild)
	//     actually run instead of being silently skipped, and
	//   • the build step doesn't re-trigger pnpm 11's verify-deps-before-run
	//     check that would otherwise re-fire the gate.
	if !strings.Contains(got, "--config.dangerously-allow-all-builds=true") {
		t.Fatalf("expected pnpm install to inject --config.dangerously-allow-all-builds=true (so sharp install scripts run), got:\n%s", got)
	}
	if !strings.Contains(got, "--config.verify-deps-before-run=false") {
		t.Fatalf("expected pnpm install to inject --config.verify-deps-before-run=false (so `next build` doesn't re-check), got:\n%s", got)
	}
	// Sanity: previous broken approach (npmrc/env) should NOT still be there.
	if strings.Contains(got, "/root/.npmrc") {
		t.Fatalf("legacy .npmrc-write approach should be gone (pnpm 11 ignored it), got:\n%s", got)
	}
}

func TestPrepareDockerBuildUsesRootDirectoryContextForAppDockerfile(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	appDir := filepath.Join(repoDir, "csob", "tracker")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ".dockerignore"), []byte("*\n"), 0644); err != nil {
		t.Fatalf("WriteFile .dockerignore: %v", err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "Dockerfile"), []byte("FROM scratch\n"), 0644); err != nil {
		t.Fatalf("WriteFile Dockerfile: %v", err)
	}

	plan, err := PrepareDockerBuild(repoDir, DockerfileData{
		RootDirectory: "csob/tracker",
	})
	if err != nil {
		t.Fatalf("PrepareDockerBuild: %v", err)
	}

	if got, want := filepath.Clean(plan.DockerfilePath), filepath.Join(appDir, "Dockerfile"); got != want {
		t.Fatalf("DockerfilePath = %q, want %q", got, want)
	}
	if got, want := filepath.Clean(plan.ContextDir), appDir; got != want {
		t.Fatalf("ContextDir = %q, want %q", got, want)
	}
}

func TestPrepareDockerBuildUsesRepoContextForMonorepoAppDockerfile(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	appDir := filepath.Join(repoDir, "apps", "api")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// A workspace lockfile at the repo root marks this as a monorepo, so the
	// app's user Dockerfile must build from the repo root to install the
	// workspace and reach sibling packages.
	if err := os.WriteFile(filepath.Join(repoDir, "bun.lock"), []byte("{}"), 0644); err != nil {
		t.Fatalf("WriteFile bun.lock: %v", err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "Dockerfile"), []byte("FROM scratch\n"), 0644); err != nil {
		t.Fatalf("WriteFile Dockerfile: %v", err)
	}

	plan, err := PrepareDockerBuild(repoDir, DockerfileData{
		RootDirectory: "apps/api",
	})
	if err != nil {
		t.Fatalf("PrepareDockerBuild: %v", err)
	}

	if got, want := filepath.Clean(plan.DockerfilePath), filepath.Join(appDir, "Dockerfile"); got != want {
		t.Fatalf("DockerfilePath = %q, want %q", got, want)
	}
	if got, want := filepath.Clean(plan.ContextDir), filepath.Clean(repoDir); got != want {
		t.Fatalf("ContextDir = %q, want repo root %q", got, want)
	}
}

func TestPrepareDockerBuildKeepsRepoContextForRootDockerfile(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	appDir := filepath.Join(repoDir, "apps", "web")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "Dockerfile"), []byte("FROM scratch\n"), 0644); err != nil {
		t.Fatalf("WriteFile Dockerfile: %v", err)
	}

	plan, err := PrepareDockerBuild(repoDir, DockerfileData{
		RootDirectory: "apps/web",
	})
	if err != nil {
		t.Fatalf("PrepareDockerBuild: %v", err)
	}

	if got, want := filepath.Clean(plan.DockerfilePath), filepath.Join(repoDir, "Dockerfile"); got != want {
		t.Fatalf("DockerfilePath = %q, want %q", got, want)
	}
	if got, want := filepath.Clean(plan.ContextDir), repoDir; got != want {
		t.Fatalf("ContextDir = %q, want %q", got, want)
	}
}

func TestPrepareDockerBuildKeepsRepoContextForGeneratedDockerfile(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, "apps", "web"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "package.json"), []byte(`{"name":"app"}`), 0644); err != nil {
		t.Fatalf("WriteFile package.json: %v", err)
	}

	plan, err := PrepareDockerBuild(repoDir, DockerfileData{
		PackageManager:  projectconfig.PackageManagerNpm,
		NodeVersion:     "22",
		RootDirectory:   "apps/web",
		OutputDirectory: "dist",
		Runtime:         projectconfig.RuntimeStatic,
		BuildCommand:    "npm run build",
		InstallCommand:  "npm ci",
	})
	if err != nil {
		t.Fatalf("PrepareDockerBuild: %v", err)
	}

	if got, want := filepath.Clean(plan.DockerfilePath), filepath.Join(repoDir, "Dockerfile"); got != want {
		t.Fatalf("DockerfilePath = %q, want %q", got, want)
	}
	if got, want := filepath.Clean(plan.ContextDir), repoDir; got != want {
		t.Fatalf("ContextDir = %q, want %q", got, want)
	}
}

func TestPnpmFlagInjectionLeavesCustomInstallCommandsAlone(t *testing.T) {
	t.Parallel()

	// A user who set their own install command — like one piped through a
	// wrapper shell script — shouldn't get our pnpm flags appended to a
	// non-pnpm invocation. The injection is gated on the command starting
	// with `pnpm`.
	if got := augmentInstallCommandForPM("bash scripts/install.sh", projectconfig.PackageManagerPnpm); got != "bash scripts/install.sh" {
		t.Fatalf("non-pnpm wrapper script should pass through unchanged, got: %q", got)
	}
	if got := augmentInstallCommandForPM("npm ci", projectconfig.PackageManagerNpm); got != "npm ci" {
		t.Fatalf("npm command should pass through unchanged, got: %q", got)
	}
	// Idempotent: user already added the flag → don't double-append.
	already := "pnpm install --frozen-lockfile --config.dangerously-allow-all-builds=true"
	if got := augmentInstallCommandForPM(already, projectconfig.PackageManagerPnpm); got != already {
		t.Fatalf("already-augmented command should be left alone, got: %q", got)
	}
	// Happy path.
	got := augmentInstallCommandForPM("pnpm install --frozen-lockfile", projectconfig.PackageManagerPnpm)
	if !strings.Contains(got, "--config.dangerously-allow-all-builds=true") || !strings.Contains(got, "--config.verify-deps-before-run=false") {
		t.Fatalf("expected both pnpm flags appended, got: %q", got)
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

func TestGenerateDockerfileNodeAPIUserOverrides(t *testing.T) {
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
		OutputDirectory: "build",
		Runtime:         projectconfig.RuntimeNodeAPI,
		BuildCommand:    "bun run build",
		InstallCommand:  "bun install",
		StartCommand:    "bun run dist/server.js",
		HealthPath:      "/api/healthz",
		Port:            4321,
	})
	if err != nil {
		t.Fatalf("GenerateDockerfile: %v", err)
	}

	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(content)

	requireAll(t, got,
		"EXPOSE 4321",
		"ENV PORT=4321",
		"/api/healthz",
		`CMD ["sh", "-c", "bun run dist/server.js"]`,
		"COPY --from=builder /app/build ./dist",
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
