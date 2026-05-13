# Node-API Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a first-class `node-api` framework preset that lets users deploy NestJS / Express / Hono / Fastify with a configurable start command + health path, producing a multi-stage `node:22-alpine` image.

**Architecture:** New framework constant + new runtime constant + new Dockerfile generator branch. `start_command` and `health_path` become first-class project columns (defaults derived from the runtime). Existing nextjs/vite/astro/static projects are unaffected — they keep deterministic defaults via the existing `RuntimeForFramework` dispatch.

**Tech Stack:** Go 1.25, SQLite (pure Go via modernc.org/sqlite), Docker SDK, BuildKit, React 19 + TanStack Router + shadcn/ui, Bun, Tailwind 4.

**Source design doc:** `~/.claude/plans/i-would-like-to-crystalline-trinket.md` (Decisions #1, #3, #9).

**Out of scope (separate plans):** Postgres sidecar (Phase 2), backup/restore/scheduled (Phase 3). Both will get their own plans once this lands.

---

## File Structure

| Path | Role | Status |
|---|---|---|
| `internal/db/migrations/022_node_api_runtime.sql` | New migration: `projects.start_command`, `projects.health_path` | Create |
| `internal/db/models.go` | Add `StartCommand`, `HealthPath` to `Project` struct | Modify |
| `internal/db/queries_projects.go` | Extend SELECT / INSERT / UPDATE to cover the two new columns | Modify |
| `internal/db/queries_projects_node_api_test.go` | Roundtrip test: Create → Get → Update preserves new fields | Create |
| `internal/projectconfig/defaults.go` | New `FrameworkNodeAPI`, `RuntimeNodeAPI`, `DefaultStartCommand`, `DefaultHealthPath`; thread through `Settings`/`Resolve` | Modify |
| `internal/projectconfig/defaults_test.go` | Unit tests for the new framework, runtime, defaults, override behaviour | Modify |
| `internal/build/dockerfile.go` | Add `StartCommand` + `HealthPath` to `DockerfileData`, add `generateNodeAPIDockerfile` branch | Modify |
| `internal/build/dockerfile_test.go` | Snapshot test for node-api Dockerfile (defaults + overrides) | Modify |
| `internal/build/pipeline.go` | Thread `StartCommand` + `HealthPath` from `settings` into `DockerfileData` (line 237-248) | Modify |
| `internal/api/handlers/projects.go` | Accept new fields in Create + Update request structs | Modify |
| `internal/api/handlers/projects_test.go` | Test Create + PATCH carry new fields end-to-end | Modify |
| `web/src/types/api.ts` | Extend `Project` type | Modify |
| `web/src/components/projects/build-settings.tsx` | Add Node API option + conditional Start Command / Health Path inputs | Modify |
| `web/src/lib/deployment-helpers.ts` | Add `FRAMEWORK_META["node-api"]` label entry (if a framework label map exists) | Modify if applicable |

---

## Task 1: Migration 022 — `start_command` and `health_path` columns

**Files:**
- Create: `internal/db/migrations/022_node_api_runtime.sql`

- [ ] **Step 1: Create the migration file**

Write to `internal/db/migrations/022_node_api_runtime.sql`:

```sql
-- Migration 022: per-project start command and health check path.
--
-- These two columns make the new 'node-api' framework usable: the generated
-- Dockerfile needs a CMD (Express/Hono/Fastify all differ from NestJS's
-- canonical 'node dist/main.js') and a HEALTHCHECK target (most APIs expose
-- /health; some use /healthz or /api/health). The values are stored as plain
-- TEXT and validated in projectconfig.Resolve so the SQL stays permissive.
--
-- Defaults are empty strings; projectconfig.Resolve fills in framework-aware
-- defaults at runtime, so pre-existing projects continue to behave identically
-- (their framework is still nextjs/vite/astro/static and those runtimes ignore
-- start_command + health_path entirely).
ALTER TABLE projects ADD COLUMN start_command TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN health_path   TEXT NOT NULL DEFAULT '';
```

- [ ] **Step 2: Verify the migration runs cleanly**

Run: `cd /Users/your-github-username/Documents/Work/lovinka-deployik && go test ./internal/db/... -run TestMigrations -v`

Expected: PASS. If `TestMigrations` doesn't exist, run `go test ./internal/db/... -v` and confirm all existing tests still pass (the embed-FS migration runner picks up the new file automatically via `migrations.go`).

- [ ] **Step 3: Commit**

```bash
cd /Users/your-github-username/Documents/Work/lovinka-deployik
git add internal/db/migrations/022_node_api_runtime.sql
git commit -m "feat(db): add start_command and health_path columns for node-api runtime"
```

---

## Task 2: `Project` struct gains StartCommand + HealthPath

**Files:**
- Modify: `internal/db/models.go` (Project struct)
- Modify: `internal/db/queries_projects.go` (extend SELECT/INSERT/UPDATE)
- Create: `internal/db/queries_projects_node_api_test.go`

- [ ] **Step 1: Write the failing roundtrip test first**

Create `internal/db/queries_projects_node_api_test.go`:

```go
package db

import "testing"

func TestProjectStartCommandAndHealthPathRoundtrip(t *testing.T) {
	t.Parallel()

	db, err := OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer db.Close()

	userID := NewID()
	if _, err := db.Exec(`INSERT INTO users (id, github_id, username, email, github_token, role)
		VALUES (?, 1, 'tester', 'tester@example.com', 'tok', 'user')`, userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	project := &Project{
		ID:           NewID(),
		Name:         "node-api-sample",
		GithubRepo:   "sample",
		GithubOwner:  "tester",
		Branch:       "main",
		UserID:       userID,
		Framework:    "node-api",
		StartCommand: "node bin/server.js",
		HealthPath:   "/api/health",
		Port:         3000,
	}
	if err := db.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	got, err := db.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got == nil {
		t.Fatal("GetProject returned nil")
	}
	if got.StartCommand != "node bin/server.js" {
		t.Errorf("StartCommand = %q, want %q", got.StartCommand, "node bin/server.js")
	}
	if got.HealthPath != "/api/health" {
		t.Errorf("HealthPath = %q, want %q", got.HealthPath, "/api/health")
	}

	if err := db.UpdateProject(project.ID, map[string]any{
		"start_command": "bun run dist/main.js",
		"health_path":   "/healthz",
	}); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}

	got, err = db.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject after update: %v", err)
	}
	if got.StartCommand != "bun run dist/main.js" {
		t.Errorf("StartCommand after update = %q", got.StartCommand)
	}
	if got.HealthPath != "/healthz" {
		t.Errorf("HealthPath after update = %q", got.HealthPath)
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `go test ./internal/db/ -run TestProjectStartCommandAndHealthPath -v`

Expected: FAIL (struct missing fields OR queries don't read/write columns).

- [ ] **Step 3: Add the fields to the `Project` struct**

In `internal/db/models.go`, locate the `Project` struct. Add (next to `Port int`):

```go
	// StartCommand and HealthPath drive the generated node-api Dockerfile's
	// CMD and HEALTHCHECK. Empty values mean "use the runtime default" — see
	// projectconfig.DefaultStartCommand and DefaultHealthPath.
	StartCommand string `db:"start_command" json:"start_command"`
	HealthPath   string `db:"health_path"   json:"health_path"`
```

- [ ] **Step 4: Extend `ListProjects` SELECT + Scan**

In `internal/db/queries_projects.go`, `ListProjects`: append `p.start_command, p.health_path` to the SELECT column list (after `p.port, COALESCE(p.resource_tier, 'small')`). Then extend the `rows.Scan` arg list with `&p.StartCommand, &p.HealthPath`. Do the same in `ListProjectsWithLatestDeployment` if it has its own SELECT.

- [ ] **Step 5: Extend `GetProject` SELECT + Scan**

Same change in `GetProject`: SELECT list gains `p.start_command, p.health_path` (place after `p.port, COALESCE(...)` and before the deployment subqueries). `Scan` gains `&p.StartCommand, &p.HealthPath` in the same slot.

- [ ] **Step 6: Extend `CreateProject` INSERT**

In `CreateProject`, add `start_command, health_path` to the INSERT column list and matching `?, ?` placeholders. Bind `project.StartCommand, project.HealthPath` in the `db.Exec` call.

- [ ] **Step 7: Extend `UpdateProject` allowlist**

`UpdateProject` accepts a `map[string]any`. Locate the allowed-keys allowlist (search for `"package_manager"` or `"node_version"`). Add `"start_command"` and `"health_path"` to it so the test's `UpdateProject(...)` call can flow through.

- [ ] **Step 8: Run the test — confirm green**

Run: `go test ./internal/db/ -run TestProjectStartCommandAndHealthPath -v`

Expected: PASS.

- [ ] **Step 9: Run the full DB test suite**

Run: `go test ./internal/db/...`

Expected: all green. If any pre-existing test fails because it constructs `*Project` literals and scans into the old column list, fix the SELECT/Scan ordering — the new columns must come at the **end** of the column list so existing positional scans need no change beyond the two added bindings.

- [ ] **Step 10: Commit**

```bash
git add internal/db/models.go internal/db/queries_projects.go internal/db/queries_projects_node_api_test.go
git commit -m "feat(db): persist project start_command and health_path"
```

---

## Task 3: `projectconfig` — `FrameworkNodeAPI` constant + normalization

**Files:**
- Modify: `internal/projectconfig/defaults.go`
- Modify: `internal/projectconfig/defaults_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/projectconfig/defaults_test.go`:

```go
func TestNormalizeFrameworkNodeAPI(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"node-api":  FrameworkNodeAPI,
		"NODE-API":  FrameworkNodeAPI,
		" node-api": FrameworkNodeAPI,
	}
	for input, want := range cases {
		if got := NormalizeFramework(input); got != want {
			t.Errorf("NormalizeFramework(%q) = %q, want %q", input, got, want)
		}
	}
}
```

- [ ] **Step 2: Run the test — confirm FAIL**

Run: `go test ./internal/projectconfig/ -run TestNormalizeFrameworkNodeAPI -v`

Expected: FAIL — `FrameworkNodeAPI` is undefined.

- [ ] **Step 3: Add the constant + normalize branch**

In `internal/projectconfig/defaults.go`, add to the framework const block (after `FrameworkStatic = "static"`):

```go
	FrameworkNodeAPI = "node-api"
```

In `NormalizeFramework`, add a case before the `default`:

```go
	case FrameworkNodeAPI:
		return FrameworkNodeAPI
```

- [ ] **Step 4: Run the test — confirm PASS**

Run: `go test ./internal/projectconfig/ -run TestNormalizeFrameworkNodeAPI -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/projectconfig/defaults.go internal/projectconfig/defaults_test.go
git commit -m "feat(projectconfig): add node-api framework constant"
```

---

## Task 4: `projectconfig` — `RuntimeNodeAPI` + dispatch

**Files:**
- Modify: `internal/projectconfig/defaults.go`
- Modify: `internal/projectconfig/defaults_test.go`

- [ ] **Step 1: Failing test**

Append to `internal/projectconfig/defaults_test.go`:

```go
func TestRuntimeForFrameworkNodeAPI(t *testing.T) {
	t.Parallel()

	if got := RuntimeForFramework(FrameworkNodeAPI); got != RuntimeNodeAPI {
		t.Errorf("RuntimeForFramework(node-api) = %q, want %q", got, RuntimeNodeAPI)
	}
	if got := RuntimeForFramework(FrameworkVite); got != RuntimeStatic {
		t.Errorf("RuntimeForFramework(vite) = %q, want %q (regression check)", got, RuntimeStatic)
	}
}
```

- [ ] **Step 2: Run — FAIL**

Run: `go test ./internal/projectconfig/ -run TestRuntimeForFrameworkNodeAPI -v`

Expected: FAIL — `RuntimeNodeAPI` undefined.

- [ ] **Step 3: Add the runtime const + dispatch branch**

In `defaults.go`, extend the runtime block:

```go
const (
	RuntimeNextJSStandalone = "nextjs-standalone"
	RuntimeStatic           = "static"
	RuntimeNodeAPI          = "node-api"
	defaultNodeVersion      = "22"
)
```

Replace `RuntimeForFramework`:

```go
func RuntimeForFramework(framework string) string {
	switch NormalizeFramework(framework) {
	case FrameworkNextJS:
		return RuntimeNextJSStandalone
	case FrameworkNodeAPI:
		return RuntimeNodeAPI
	default:
		return RuntimeStatic
	}
}
```

- [ ] **Step 4: Run — PASS**

Run: `go test ./internal/projectconfig/ -run TestRuntimeForFrameworkNodeAPI -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/projectconfig/defaults.go internal/projectconfig/defaults_test.go
git commit -m "feat(projectconfig): map node-api framework to node-api runtime"
```

---

## Task 5: `DefaultStartCommand` and `DefaultHealthPath`

**Files:**
- Modify: `internal/projectconfig/defaults.go`
- Modify: `internal/projectconfig/defaults_test.go`

- [ ] **Step 1: Failing test**

Append:

```go
func TestDefaultStartCommandByRuntime(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		RuntimeNodeAPI:          "node dist/main.js",
		RuntimeNextJSStandalone: "",
		RuntimeStatic:           "",
	}
	for runtime, want := range cases {
		if got := DefaultStartCommand(runtime); got != want {
			t.Errorf("DefaultStartCommand(%q) = %q, want %q", runtime, got, want)
		}
	}
}

func TestDefaultHealthPathByRuntime(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		RuntimeNodeAPI:          "/health",
		RuntimeNextJSStandalone: "/",
		RuntimeStatic:           "/",
	}
	for runtime, want := range cases {
		if got := DefaultHealthPath(runtime); got != want {
			t.Errorf("DefaultHealthPath(%q) = %q, want %q", runtime, got, want)
		}
	}
}
```

- [ ] **Step 2: Run — FAIL**

Run: `go test ./internal/projectconfig/ -run "TestDefault(StartCommand|HealthPath)ByRuntime" -v`

Expected: FAIL — functions undefined.

- [ ] **Step 3: Implement**

Add to `internal/projectconfig/defaults.go` (anywhere near the other `Default*` helpers):

```go
// DefaultStartCommand returns the runtime's default container start command.
// Only the node-api runtime has a non-empty default; the Next.js standalone
// and static runtimes bake their CMD into the generated Dockerfile directly.
func DefaultStartCommand(runtime string) string {
	if runtime == RuntimeNodeAPI {
		return "node dist/main.js"
	}
	return ""
}

// DefaultHealthPath returns the HTTP path the runtime's HEALTHCHECK probes.
// node-api defaults to /health (NestJS / Hono / Fastify convention); static
// and Next.js standalone probe the root document.
func DefaultHealthPath(runtime string) string {
	if runtime == RuntimeNodeAPI {
		return "/health"
	}
	return "/"
}
```

- [ ] **Step 4: Run — PASS**

Run: `go test ./internal/projectconfig/ -run "TestDefault(StartCommand|HealthPath)ByRuntime" -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/projectconfig/defaults.go internal/projectconfig/defaults_test.go
git commit -m "feat(projectconfig): add DefaultStartCommand and DefaultHealthPath helpers"
```

---

## Task 6: `Settings` carries StartCommand + HealthPath through `Resolve`

**Files:**
- Modify: `internal/projectconfig/defaults.go`
- Modify: `internal/projectconfig/defaults_test.go`
- Modify: `internal/db/models.go` (only if `ApplyProjectDefaults` also needs to write them back — it does, see Step 5)

- [ ] **Step 1: Failing test**

Append:

```go
func TestResolveNodeAPIDefaults(t *testing.T) {
	t.Parallel()

	settings, err := Resolve(&db.Project{Framework: "node-api"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if settings.Framework != FrameworkNodeAPI {
		t.Errorf("Framework = %q", settings.Framework)
	}
	if settings.Runtime != RuntimeNodeAPI {
		t.Errorf("Runtime = %q", settings.Runtime)
	}
	if settings.OutputDirectory != "dist" {
		t.Errorf("OutputDirectory = %q, want dist", settings.OutputDirectory)
	}
	if settings.StartCommand != "node dist/main.js" {
		t.Errorf("StartCommand = %q, want default", settings.StartCommand)
	}
	if settings.HealthPath != "/health" {
		t.Errorf("HealthPath = %q, want /health", settings.HealthPath)
	}
}

func TestResolveNodeAPIUserOverrides(t *testing.T) {
	t.Parallel()

	settings, err := Resolve(&db.Project{
		Framework:    "node-api",
		StartCommand: "bun run dist/server.js",
		HealthPath:   "/api/healthz",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if settings.StartCommand != "bun run dist/server.js" {
		t.Errorf("StartCommand = %q (user override should win)", settings.StartCommand)
	}
	if settings.HealthPath != "/api/healthz" {
		t.Errorf("HealthPath = %q (user override should win)", settings.HealthPath)
	}
}
```

- [ ] **Step 2: Run — FAIL**

Run: `go test ./internal/projectconfig/ -run TestResolveNodeAPI -v`

Expected: FAIL — `Settings` lacks `StartCommand`/`HealthPath` fields.

- [ ] **Step 3: Extend the `Settings` struct**

Add to `Settings` in `defaults.go`:

```go
	StartCommand    string
	HealthPath      string
```

- [ ] **Step 4: Populate inside `Resolve`**

At the end of `Resolve`, after the `nodeVersion` resolution and before the `return Settings{...}`, insert:

```go
	runtime := RuntimeForFramework(framework)

	startCommand := strings.TrimSpace(project.StartCommand)
	if startCommand == "" {
		startCommand = DefaultStartCommand(runtime)
	}

	healthPath := strings.TrimSpace(project.HealthPath)
	if healthPath == "" {
		healthPath = DefaultHealthPath(runtime)
	}
```

Then update the `return Settings{...}` literal: replace `Runtime: RuntimeForFramework(framework),` with `Runtime: runtime,` (avoid double-computing) and append:

```go
		StartCommand:    startCommand,
		HealthPath:      healthPath,
```

- [ ] **Step 5: Update `ApplyProjectDefaults`**

In `ApplyProjectDefaults`, after the existing `project.NodeVersion = settings.NodeVersion` line, add:

```go
	project.StartCommand = settings.StartCommand
	project.HealthPath = settings.HealthPath
```

- [ ] **Step 6: Run — PASS**

Run: `go test ./internal/projectconfig/ -run TestResolveNodeAPI -v`

Expected: PASS.

- [ ] **Step 7: Full projectconfig suite stays green**

Run: `go test ./internal/projectconfig/...`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/projectconfig/defaults.go internal/projectconfig/defaults_test.go
git commit -m "feat(projectconfig): thread StartCommand and HealthPath through Resolve"
```

---

## Task 7: `DockerfileData` gains StartCommand + HealthPath

**Files:**
- Modify: `internal/build/dockerfile.go`

This is a no-test compile-only change — the real assertion lands in Task 8.

- [ ] **Step 1: Add fields to `DockerfileData`**

In `internal/build/dockerfile.go`, extend the `DockerfileData` struct (after `Port int`):

```go
	// StartCommand is the container CMD for the node-api runtime. Ignored by
	// nextjs-standalone and static runtimes (they bake CMD directly). When
	// empty the runtime's DefaultStartCommand is substituted.
	StartCommand string
	// HealthPath is the HTTP path probed by the generated HEALTHCHECK. When
	// empty the runtime's DefaultHealthPath ("/" or "/health") is substituted.
	HealthPath string
```

- [ ] **Step 2: Compile**

Run: `go build ./...`

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/build/dockerfile.go
git commit -m "feat(build): add StartCommand and HealthPath to DockerfileData"
```

---

## Task 8: Generate the node-api Dockerfile (default values)

**Files:**
- Modify: `internal/build/dockerfile.go`
- Modify: `internal/build/dockerfile_test.go`

- [ ] **Step 1: Write the failing snapshot test**

Append to `internal/build/dockerfile_test.go`:

```go
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
```

- [ ] **Step 2: Run — FAIL**

Run: `go test ./internal/build/ -run TestGenerateDockerfileSupportsNodeAPIRuntime -v`

Expected: FAIL — generator currently falls through to `generateNextJSDockerfile` for unknown runtimes, so substrings like `COPY --from=deps /app/node_modules ./node_modules` and `CMD ["sh", "-c", "node dist/main.js"]` will be missing.

- [ ] **Step 3: Branch the generator**

In `internal/build/dockerfile.go`, replace `generateDockerfileContent`:

```go
func generateDockerfileContent(data DockerfileData) string {
	switch data.Runtime {
	case projectconfig.RuntimeStatic:
		return generateStaticDockerfile(data)
	case projectconfig.RuntimeNodeAPI:
		return generateNodeAPIDockerfile(data)
	default:
		return generateNextJSDockerfile(data)
	}
}
```

- [ ] **Step 4: Add `generateNodeAPIDockerfile`**

Add the new function in `dockerfile.go` (place it under `generateStaticDockerfile`):

```go
// generateNodeAPIDockerfile produces a multi-stage Dockerfile for a Node API
// (NestJS / Express / Hono / Fastify). The runner stage stays slim: it
// re-uses node_modules from the deps stage (assumed to be prod-only when the
// project's install command uses `--production`; otherwise it carries dev
// deps too, which is the same trade-off the static runtime already accepts).
// CMD is wrapped in `sh -c` so users can chain commands (e.g.
// "prisma migrate deploy && node dist/main.js") without rewriting the array.
func generateNodeAPIDockerfile(data DockerfileData) string {
	var b strings.Builder
	appDir := dockerAppDir(data.RootDirectory)
	installDir := dockerAppDir(data.InstallDirectory)
	outputDir := dockerProjectPath(data.RootDirectory, data.OutputDirectory)
	port := effectivePort(data.Port)
	healthPath := data.HealthPath
	if healthPath == "" {
		healthPath = "/health"
	}
	startCommand := data.StartCommand
	if startCommand == "" {
		startCommand = "node dist/main.js"
	}

	b.WriteString("# syntax=docker/dockerfile:1.7\n")
	b.WriteString(fmt.Sprintf("FROM node:%s-alpine AS base\n", data.NodeVersion))
	b.WriteString("WORKDIR /app\n\n")

	// Dependencies stage
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

	// Builder stage
	b.WriteString("\nFROM deps AS builder\n")
	b.WriteString(fmt.Sprintf("WORKDIR %s\n", appDir))
	for _, ev := range data.BuildEnvVars {
		b.WriteString(fmt.Sprintf("ENV %s=%s\n", ev.Key, strconv.Quote(ev.Value)))
	}
	b.WriteString(buildRunLine(data.BuildCommand, []string{
		"--mount=type=secret,id=deployik-secrets",
	}))
	b.WriteString("\n")

	// Runner stage — slim image that copies the built output + reuses
	// node_modules from deps. We keep the deps stage's node_modules instead of
	// reinstalling prod-only here because the install command is user-driven
	// (`bun install`, `pnpm install`, etc.) and reinstalling in the runner
	// would duplicate npm/pnpm/bun in the runtime image.
	b.WriteString("FROM base AS runner\n")
	b.WriteString("ENV NODE_ENV=production\n")
	b.WriteString("RUN apk --no-cache del wget curl 2>/dev/null; rm -rf /var/cache/apk/*\n\n")
	b.WriteString(fmt.Sprintf("COPY --from=builder %s ./dist\n", outputDir))
	b.WriteString(fmt.Sprintf("COPY --from=deps %s/node_modules ./node_modules\n", installDir))
	b.WriteString(fmt.Sprintf("COPY --from=deps %s/package.json ./package.json\n\n", installDir))

	b.WriteString(fmt.Sprintf("EXPOSE %d\n", port))
	b.WriteString(fmt.Sprintf("ENV PORT=%d\n", port))
	b.WriteString("HEALTHCHECK --interval=30s --timeout=3s --start-period=15s --retries=3 \\\n")
	b.WriteString(fmt.Sprintf("  CMD node -e \"require('http').get('http://localhost:%d%s',(r)=>{process.exit(r.statusCode<400?0:1)}).on('error',()=>process.exit(1))\"\n", port, healthPath))
	b.WriteString(fmt.Sprintf("CMD [\"sh\", \"-c\", %s]\n", strconv.Quote(startCommand)))

	return b.String()
}
```

- [ ] **Step 5: Run — PASS**

Run: `go test ./internal/build/ -run TestGenerateDockerfileSupportsNodeAPIRuntime -v`

Expected: PASS.

- [ ] **Step 6: Full build suite stays green (regression check)**

Run: `go test ./internal/build/...`

Expected: PASS. If the new branch accidentally diverted nextjs or static, the existing snapshot tests will catch it.

- [ ] **Step 7: Commit**

```bash
git add internal/build/dockerfile.go internal/build/dockerfile_test.go
git commit -m "feat(build): generate node-api runtime Dockerfile"
```

---

## Task 9: node-api Dockerfile respects user overrides

**Files:**
- Modify: `internal/build/dockerfile_test.go`

- [ ] **Step 1: Failing test**

Append:

```go
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
```

- [ ] **Step 2: Run — should already PASS**

Run: `go test ./internal/build/ -run TestGenerateDockerfileNodeAPIUserOverrides -v`

Expected: PASS (Task 8's generator already threads Port / HealthPath / StartCommand / OutputDirectory). If it fails, trace the missing wire-up before continuing.

- [ ] **Step 3: Commit**

```bash
git add internal/build/dockerfile_test.go
git commit -m "test(build): cover node-api Dockerfile user overrides"
```

---

## Task 10: Pipeline passes StartCommand + HealthPath into DockerfileData

**Files:**
- Modify: `internal/build/pipeline.go`

- [ ] **Step 1: Update the `GenerateDockerfile` call**

In `internal/build/pipeline.go`, around line 237-248, change the `DockerfileData` literal to include the two new fields:

```go
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
		StartCommand:    settings.StartCommand,
		HealthPath:      settings.HealthPath,
	})
```

- [ ] **Step 2: Compile + full suite**

Run: `go build ./... && go test ./internal/build/... ./internal/projectconfig/...`

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/build/pipeline.go
git commit -m "feat(build): wire StartCommand and HealthPath into deployment pipeline"
```

---

## Task 11: Project Create + Update handlers accept new fields

**Files:**
- Modify: `internal/api/handlers/projects.go`
- Modify: `internal/api/handlers/projects_test.go`

- [ ] **Step 1: Locate the request structs**

Open `internal/api/handlers/projects.go` and find:
- `createProjectRequest` (used by `Create`)
- `updateProjectRequest` (used by `Update`)

They live near the top of the file. Note their existing fields (`Framework`, `PackageManager`, `RootDirectory`, etc.).

- [ ] **Step 2: Failing handler test — Create persists node-api fields**

Append to `internal/api/handlers/projects_test.go` (mirror the style of an existing project-create test in the file — they pass a JSON body and assert on the response):

```go
func TestCreateProjectAcceptsNodeAPIFields(t *testing.T) {
	t.Parallel()

	env := newProjectHandlerTestEnv(t)
	defer env.Close()

	body := map[string]any{
		"name":          "node-api-create",
		"github_repo":   "api",
		"github_owner":  "tester",
		"branch":        "main",
		"framework":     "node-api",
		"start_command": "bun run dist/server.js",
		"health_path":   "/api/health",
		"port":          4321,
	}
	resp := env.PostJSON(t, "/api/projects", body)
	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}

	var created db.Project
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if created.StartCommand != "bun run dist/server.js" {
		t.Errorf("StartCommand = %q", created.StartCommand)
	}
	if created.HealthPath != "/api/health" {
		t.Errorf("HealthPath = %q", created.HealthPath)
	}
}

func TestUpdateProjectAcceptsNodeAPIFields(t *testing.T) {
	t.Parallel()

	env := newProjectHandlerTestEnv(t)
	defer env.Close()

	project := env.SeedProject(t, "node-api-update", "node-api")
	body := map[string]any{
		"start_command": "node bin/server.js",
		"health_path":   "/healthz",
	}
	resp := env.PatchJSON(t, "/api/projects/"+project.ID, body)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}

	fresh, err := env.DB.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if fresh.StartCommand != "node bin/server.js" {
		t.Errorf("StartCommand = %q", fresh.StartCommand)
	}
	if fresh.HealthPath != "/healthz" {
		t.Errorf("HealthPath = %q", fresh.HealthPath)
	}
}
```

*If* `newProjectHandlerTestEnv` / `PostJSON` / `PatchJSON` / `SeedProject` don't exist with those exact names in the file: open the file, find the test-helper conventions actually used (they will have similar functions under different names), and use those. Do not invent new helpers — the project's existing test harness is the canonical source.

- [ ] **Step 3: Run — FAIL**

Run: `go test ./internal/api/handlers/ -run "TestCreateProjectAcceptsNodeAPIFields|TestUpdateProjectAcceptsNodeAPIFields" -v`

Expected: FAIL — request struct ignores the new fields, so they never reach the DB.

- [ ] **Step 4: Extend `createProjectRequest`**

Add fields (preserve the existing JSON tag style):

```go
	StartCommand string `json:"start_command"`
	HealthPath   string `json:"health_path"`
```

In `Create`, after the existing field assignments onto `project`, add:

```go
	project.StartCommand = req.StartCommand
	project.HealthPath = req.HealthPath
```

(`projectconfig.ApplyProjectDefaults(project)`, which is called after, will fill the defaults if these are empty — no further change needed.)

- [ ] **Step 5: Extend `updateProjectRequest`**

Add the same two fields. In `Update`, find the existing `updates := map[string]any{}` block that conditionally adds keys when the request supplied them. Add:

```go
	if req.StartCommand != nil {
		updates["start_command"] = strings.TrimSpace(*req.StartCommand)
	}
	if req.HealthPath != nil {
		updates["health_path"] = strings.TrimSpace(*req.HealthPath)
	}
```

…and make the new request fields `*string` to support presence vs. empty-string. If the existing struct uses plain `string` for other text fields (no presence distinction), match that convention instead — pick whichever pattern dominates the surrounding code.

- [ ] **Step 6: Run — PASS**

Run: `go test ./internal/api/handlers/ -run "TestCreateProjectAcceptsNodeAPIFields|TestUpdateProjectAcceptsNodeAPIFields" -v`

Expected: PASS.

- [ ] **Step 7: Full handler suite stays green**

Run: `go test ./internal/api/...`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/api/handlers/projects.go internal/api/handlers/projects_test.go
git commit -m "feat(api): accept start_command and health_path on project create + update"
```

---

## Task 12: Top-level test gate

**Files:**
- (none — read-only verification)

- [ ] **Step 1: Run the full Go suite**

Run: `cd /Users/your-github-username/Documents/Work/lovinka-deployik && go test ./...`

Expected: all green. If any package fails, debug before continuing — frontend work depends on a green backend baseline.

- [ ] **Step 2: Run `go vet`**

Run: `go vet ./...`

Expected: silent (no findings).

---

## Task 13: Frontend type — `Project` carries new fields

**Files:**
- Modify: `web/src/types/api.ts`

- [ ] **Step 1: Locate `Project` type**

Open `web/src/types/api.ts`. Find the `Project` interface (or type alias).

- [ ] **Step 2: Extend the framework union + fields**

Find the `framework` property. It is currently typed as `'nextjs' | 'vite' | 'astro' | 'static'`. Replace with:

```ts
  framework: 'nextjs' | 'vite' | 'astro' | 'static' | 'node-api';
```

Add `start_command` and `health_path` (next to `port`):

```ts
  start_command: string;
  health_path: string;
```

- [ ] **Step 3: Typecheck**

Run: `cd /Users/your-github-username/Documents/Work/lovinka-deployik/web && bunx tsc --noEmit`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add web/src/types/api.ts
git commit -m "feat(web): add node-api framework + start/health to Project type"
```

---

## Task 14: `build-settings.tsx` — Node API framework option

**Files:**
- Modify: `web/src/components/projects/build-settings.tsx`

- [ ] **Step 1: Inspect the current framework selector**

Open the file. Find the framework `<Select>` and the `FRAMEWORKS` (or similarly named) array driving its options.

- [ ] **Step 2: Add the option**

Add to the array (place it after the last existing entry, e.g. `static`):

```ts
  { value: 'node-api', label: 'Node API (NestJS, Express, Hono, Fastify)' },
```

- [ ] **Step 3: Locate `BuildSettingsValues` type**

The file exports a `BuildSettingsValues` (or similarly named) interface used by parent forms. Add two optional fields:

```ts
  start_command?: string;
  health_path?: string;
```

- [ ] **Step 4: Add conditional inputs**

Inside the JSX, after the `Container Port` input, add (only when framework is `node-api`):

```tsx
{values.framework === 'node-api' && (
  <>
    <div className="grid gap-2">
      <Label htmlFor="start_command">Start command</Label>
      <Input
        id="start_command"
        value={values.start_command ?? ''}
        onChange={(e) => onChange({ ...values, start_command: e.target.value })}
        placeholder="node dist/main.js"
      />
      <p className="text-xs text-muted-foreground">
        How the container starts. Defaults to <code>node dist/main.js</code>. Use this to
        chain a migration (e.g. <code>prisma migrate deploy &amp;&amp; node dist/main.js</code>).
      </p>
    </div>
    <div className="grid gap-2">
      <Label htmlFor="health_path">Health check path</Label>
      <Input
        id="health_path"
        value={values.health_path ?? ''}
        onChange={(e) => onChange({ ...values, health_path: e.target.value })}
        placeholder="/health"
      />
      <p className="text-xs text-muted-foreground">
        Path the container's HEALTHCHECK probes. Defaults to <code>/health</code>.
      </p>
    </div>
  </>
)}
```

(Adjust to match the file's existing form-row pattern — `grid`, `Label`, `Input` may be wrapped in shared field components. Mirror the surrounding rows precisely.)

- [ ] **Step 5: Framework switch logic — reset on framework change**

If the file has a `useEffect` (or handler) that resets dependent fields when `framework` changes (e.g., it currently resets `output_directory` to the framework default), extend it so that switching **to** `node-api` resets `output_directory` to `dist`. Switching **away from** `node-api` should clear `start_command` and `health_path` to empty strings — those fields are meaningless for nextjs/static and we don't want stale values sneaking back when the user re-selects node-api.

- [ ] **Step 6: Typecheck + unit tests**

Run: `cd web && bunx tsc --noEmit && bun run test`

Expected: PASS. If unit tests assert on framework enum shape, update them in this commit.

- [ ] **Step 7: Commit**

```bash
git add web/src/components/projects/build-settings.tsx
git commit -m "feat(web): add Node API framework option to build settings"
```

---

## Task 15: `NewProject.tsx` — submit new fields

**Files:**
- Modify: `web/src/pages/NewProject.tsx`

- [ ] **Step 1: Locate the create-project mutation**

Find where `NewProject.tsx` calls `api.projects.create(...)` (or whatever the project's API client exposes).

- [ ] **Step 2: Include the new fields in the payload**

Confirm `start_command` and `health_path` are passed through from the build-settings form state into the POST body. If the form state was already typed as `BuildSettingsValues`, the fields flow through automatically — verify with `grep`:

```bash
grep -n "start_command\|health_path" web/src/pages/NewProject.tsx
```

If they aren't being sent, add them explicitly:

```ts
await api.projects.create({
  // ... existing fields
  start_command: values.start_command,
  health_path: values.health_path,
});
```

- [ ] **Step 3: Typecheck**

Run: `cd web && bunx tsc --noEmit`

Expected: PASS.

- [ ] **Step 4: Commit (only if changes were needed)**

```bash
git add web/src/pages/NewProject.tsx
git commit -m "feat(web): submit start_command and health_path from new-project flow"
```

If no edit was needed (the fields already flow through via the build-settings form), skip the commit.

---

## Task 16: `ProjectSettings.tsx` (Build tab) — edit new fields

**Files:**
- Modify: `web/src/pages/ProjectSettings.tsx` (Build tab uses the same `<BuildSettingsFields>` component)

- [ ] **Step 1: Verify the Build tab uses `<BuildSettingsFields>`**

Run: `grep -n "BuildSettingsFields" web/src/pages/ProjectSettings.tsx`

Expected: at least one match in the Build-tab section.

- [ ] **Step 2: Confirm the save-mutation includes the new fields**

Same shape as Task 15: confirm the PATCH payload carries `start_command` and `health_path`. Add them explicitly if not.

- [ ] **Step 3: Typecheck**

Run: `cd web && bunx tsc --noEmit`

Expected: PASS.

- [ ] **Step 4: Commit if needed**

```bash
git add web/src/pages/ProjectSettings.tsx
git commit -m "feat(web): edit start_command and health_path from settings"
```

Skip if Task 14's shared component already covers this.

---

## Task 17: Visual smoke test (dev server)

**Files:** (none)

- [ ] **Step 1: Start the dev backend**

Run: `cd /Users/your-github-username/Documents/Work/lovinka-deployik && make dev-api`

Wait for `Listening on :8080`.

- [ ] **Step 2: Start the dev frontend (in a second terminal)**

Run: `make dev-web`

Wait for Vite to print the local URL (typically `http://localhost:5173`).

- [ ] **Step 3: Seed dev data**

Run: `make dev-seed`

- [ ] **Step 4: Visual checks**

Open `http://localhost:5173` in a browser. Log in via dev-login. Walk through:

1. **New project** → pick any mock repo → on build-settings step, open the Framework dropdown. **Expect:** the new option `Node API (NestJS, Express, Hono, Fastify)` appears. Select it. **Expect:** Start Command and Health Path inputs appear below Container Port with placeholders `node dist/main.js` and `/health`. Submit.
2. **Project Settings → Build** for the new project. **Expect:** Framework shows `Node API`, Start Command + Health Path fields are visible and prefilled with the defaults from the new-project step.
3. **Switch framework to Vite, save, switch back to Node API.** **Expect:** Start Command + Health Path are empty (not stale). Set them to `node bin/api.js` and `/healthz`. Save.
4. **Trigger a deploy** of any existing project (does NOT need to be the node-api one — just confirm the dev server didn't regress). **Expect:** deploy succeeds; build log shows the generated Dockerfile is the existing nextjs/static path.
5. (Optional) **Trigger a deploy of the node-api project against a real NestJS repo on a private branch** (manual smoke). **Expect:** generated Dockerfile contains `FROM node:22-alpine`, `COPY --from=deps /app/node_modules`, `HEALTHCHECK ... /healthz`, `CMD ["sh", "-c", "node bin/api.js"]`.

- [ ] **Step 5: No commit — this is a verification gate**

---

## Task 18: Final verification + housekeeping

**Files:** (none)

- [ ] **Step 1: Run the entire test surface**

Run: `cd /Users/your-github-username/Documents/Work/lovinka-deployik && go test ./... && cd web && bunx tsc --noEmit && bun run test`

Expected: PASS across the board.

- [ ] **Step 2: Confirm `git status` is clean**

Run: `git status`

Expected: `nothing to commit, working tree clean`. If anything is dirty, decide whether it's a missed commit or local noise.

- [ ] **Step 3: Review the commit log**

Run: `git log --oneline -20`

Expected: a tidy linear sequence of `feat(db):`, `feat(projectconfig):`, `feat(build):`, `feat(api):`, `feat(web):` commits — one per logical step.

---

## Verification (top-level)

| What to verify | How |
|---|---|
| Migration 022 lands on a fresh DB | `rm -f data/deployik.db && make dev-api`; sqlite shows the columns. |
| Existing projects keep working | `make dev-seed` then deploy any seeded nextjs/vite project — still green. |
| Node API framework is selectable in NewProject | Browser smoke in Task 17. |
| Generated Dockerfile for node-api passes a real `docker buildx build` | Manual: spin up a minimal NestJS app, push, deploy. Out of scope for unit tests. |
| TypeScript builds cleanly | `cd web && bunx tsc --noEmit`. |
| Go suite stays green | `go test ./...`. |

## Risks (carry-over from design doc)

1. **Existing tests scan `*Project` by column position.** Adding new columns at the end of the SELECT preserves the order — but any place that does `SELECT *` and binds positionally is fragile. Search `grep -rn "SELECT \*" internal/db/` before merging.
2. **Frontend framework-change reset logic** can clobber user-typed values if not careful. Task 14 step 5 spells this out explicitly.
3. **Out of scope here:** running the deploy pipeline against a real NestJS repo end-to-end is a manual gate, not automated. Sub-plans for Phase 2 (Postgres sidecar) will add an integration test that exercises the node-api Dockerfile against an actual build.

## Next plans (separately authored)

- `docs/plans/2026-05-13-postgres-sidecar.md` — Phase 2: `project_services` schema, `internal/services` package, Services UI, pipeline integration, SSH-tunnel connect dialog, env-var auto-injection.
- `docs/plans/2026-05-13-postgres-backups.md` — Phase 3: manual backup, restore-from-upload, daily scheduled backups, retention pruning.

Re-run `/superpowers:writing-plans` against the design doc once Phase 1 ships — the codebase will have evolved (Services-tab placement, sidebar nav layout, possibly a new module split) and the detailed plan should reflect that state, not today's.
