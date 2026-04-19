# Sidebar Version Badge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface the running Deployik build (commit SHA + GitHub Actions run) in the sidebar footer with one click each to the GitHub commit and the Actions run that built it.

**Architecture:** Bake `GIT_SHA`, `BUILD_TIME`, `GH_RUN_ID`, `GH_REPO` into the Go binary at build time via Docker build args + `go build -ldflags`. Pass them through `RouterConfig` to a typed `version.Info`, which a refactored `HealthHandler` includes in the existing `GET /api/health` JSON response. The SPA fetches `/api/health` once at app boot via React Query (`staleTime: Infinity`), and a new `VersionRow` component in `AppSidebar.tsx` renders a muted row with two icon links above the user/workspace dropdown, collapsing to a single tooltipped icon in icon-only mode.

**Tech Stack:** Go 1.25 (chi, ldflags), Docker multi-stage build (BuildKit ARG), GitHub Actions (`docker/build-push-action@v6`), React 19 + TanStack Query + shadcn/ui sidebar primitives.

**Reference:** Design doc at `docs/plans/2026-04-19-sidebar-version-badge-design.md`.

---

## File Structure

### New files
- `internal/version/version.go` — `Info` struct + `New()` constructor that builds `commit_url` / `run_url` from raw inputs.
- `internal/version/version_test.go` — table-driven tests for URL construction and short-SHA derivation.
- `internal/api/handlers/health.go` — `HealthHandler` struct that owns the `/api/health` route (replaces the inline closure in `router.go`).
- `internal/api/handlers/health_test.go` — HTTP test asserting JSON shape with and without a populated `version.Info`.
- `web/src/components/layout/VersionRow.tsx` — Sidebar footer row component; renders muted commit chip + GH Actions icon link, collapses gracefully in icon-only sidebar mode.

### Modified files
- `cmd/server/main.go` — declare ldflag-injected vars, build a `version.Info`, pass it into `RouterConfig`.
- `internal/api/router.go` — add `Version *version.Info` to `RouterConfig`; replace inline health closure with `HealthHandler` registration.
- `docker/Dockerfile` — add `ARG GIT_SHA`, `ARG BUILD_TIME`, `ARG GH_RUN_ID`, `ARG GH_REPO` and forward to `go build -ldflags`.
- `.github/workflows/ci.yml` — pass `build-args` to `docker/build-push-action@v6`.
- `web/src/types/api.ts` — add `VersionInfo` and `HealthResponse` interfaces.
- `web/src/lib/api.ts` — add `getHealth()` method.
- `web/src/lib/queryKeys.ts` — add `health` query key.
- `web/src/components/layout/AppSidebar.tsx` — render `<VersionRow />` between `</SidebarContent>` and the user `DropdownMenu`.

---

## Task 1: `internal/version` package

**Files:**
- Create: `internal/version/version.go`
- Create: `internal/version/version_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/version/version_test.go`:

```go
package version

import "testing"

func TestNew(t *testing.T) {
	tests := []struct {
		name           string
		gitSHA         string
		buildTime      string
		ghRunID        string
		ghRepo         string
		wantShortSHA   string
		wantCommitURL  string
		wantRunURL     string
	}{
		{
			name:          "full release build",
			gitSHA:        "abc1234567890fedcba0123456789abcdef01234",
			buildTime:     "2026-04-19T10:23:11Z",
			ghRunID:       "12345678",
			ghRepo:        "lefteq/lovinka-deployik",
			wantShortSHA:  "abc1234",
			wantCommitURL: "https://github.com/lefteq/lovinka-deployik/commit/abc1234567890fedcba0123456789abcdef01234",
			wantRunURL:    "https://github.com/lefteq/lovinka-deployik/actions/runs/12345678",
		},
		{
			name:          "missing run id (PR build)",
			gitSHA:        "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
			buildTime:     "2026-04-19T10:23:11Z",
			ghRunID:       "",
			ghRepo:        "lefteq/lovinka-deployik",
			wantShortSHA:  "deadbee",
			wantCommitURL: "https://github.com/lefteq/lovinka-deployik/commit/deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
			wantRunURL:    "",
		},
		{
			name:          "local dev build",
			gitSHA:        "dev",
			buildTime:     "unknown",
			ghRunID:       "",
			ghRepo:        "lefteq/lovinka-deployik",
			wantShortSHA:  "dev",
			wantCommitURL: "",
			wantRunURL:    "",
		},
		{
			name:          "short sha shorter than 7",
			gitSHA:        "abc",
			buildTime:     "unknown",
			ghRunID:       "",
			ghRepo:        "lefteq/lovinka-deployik",
			wantShortSHA:  "abc",
			wantCommitURL: "",
			wantRunURL:    "",
		},
		{
			name:          "missing repo defeats both URLs",
			gitSHA:        "abc1234567890fedcba0123456789abcdef01234",
			buildTime:     "2026-04-19T10:23:11Z",
			ghRunID:       "12345678",
			ghRepo:        "",
			wantShortSHA:  "abc1234",
			wantCommitURL: "",
			wantRunURL:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := New(tc.gitSHA, tc.buildTime, tc.ghRunID, tc.ghRepo)
			if info.GitSHA != tc.wantShortSHA {
				t.Errorf("GitSHA: got %q, want %q", info.GitSHA, tc.wantShortSHA)
			}
			if info.GitSHAFull != tc.gitSHA {
				t.Errorf("GitSHAFull: got %q, want %q", info.GitSHAFull, tc.gitSHA)
			}
			if info.CommitURL != tc.wantCommitURL {
				t.Errorf("CommitURL: got %q, want %q", info.CommitURL, tc.wantCommitURL)
			}
			if info.RunURL != tc.wantRunURL {
				t.Errorf("RunURL: got %q, want %q", info.RunURL, tc.wantRunURL)
			}
			if info.GHRepo != tc.ghRepo {
				t.Errorf("GHRepo: got %q, want %q", info.GHRepo, tc.ghRepo)
			}
		})
	}
}

// IsDev should be true only when no real git SHA was injected (i.e., local
// `make dev-api`). Used by tests and possibly callers that want to special-case
// dev rendering.
func TestIsDev(t *testing.T) {
	if !New("dev", "unknown", "", "lefteq/lovinka-deployik").IsDev() {
		t.Error("expected IsDev() == true for sha=\"dev\"")
	}
	if !New("", "unknown", "", "lefteq/lovinka-deployik").IsDev() {
		t.Error("expected IsDev() == true for empty sha")
	}
	if New("abc1234567890fedcba0123456789abcdef01234", "", "", "").IsDev() {
		t.Error("expected IsDev() == false for real sha")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/version/...`
Expected: FAIL with "no Go files in" or "package version is not in std" — package doesn't exist yet.

- [ ] **Step 3: Write the implementation**

Create `internal/version/version.go`:

```go
// Package version exposes build-time version metadata baked into the binary
// via -ldflags. Constructed in main.go from package-level vars and passed to
// the API layer through RouterConfig.
package version

import "fmt"

// Info is the resolved, presentation-ready build metadata. It is safe to
// marshal directly into JSON responses.
type Info struct {
	GitSHA     string `json:"git_sha"`      // short (7-char) SHA for display; falls back to whatever was injected if shorter
	GitSHAFull string `json:"git_sha_full"` // full SHA, used for the commit URL and as a stable identifier
	BuildTime  string `json:"build_time"`   // RFC3339-ish timestamp of the commit (or build start) — display only
	GHRepo     string `json:"gh_repo"`      // "owner/name", e.g. "lefteq/lovinka-deployik"
	GHRunID    string `json:"gh_run_id"`    // GitHub Actions run id; empty for local/PR builds
	CommitURL  string `json:"commit_url"`   // built server-side; empty when sha or repo is missing
	RunURL     string `json:"run_url"`      // built server-side; empty when run id or repo is missing
}

// New constructs an Info from raw build-injected strings. URL fields are
// derived here so the SPA never has to know GitHub URL templates.
func New(gitSHA, buildTime, ghRunID, ghRepo string) *Info {
	short := gitSHA
	if len(short) > 7 {
		short = short[:7]
	}

	info := &Info{
		GitSHA:     short,
		GitSHAFull: gitSHA,
		BuildTime:  buildTime,
		GHRepo:     ghRepo,
		GHRunID:    ghRunID,
	}

	if ghRepo != "" && gitSHA != "" && gitSHA != "dev" {
		info.CommitURL = fmt.Sprintf("https://github.com/%s/commit/%s", ghRepo, gitSHA)
	}
	if ghRepo != "" && ghRunID != "" {
		info.RunURL = fmt.Sprintf("https://github.com/%s/actions/runs/%s", ghRepo, ghRunID)
	}

	return info
}

// IsDev reports whether this binary was built without a real git SHA
// (typically a local `make dev-api` run). Callers can use this to suppress
// links that would 404 on github.com.
func (i *Info) IsDev() bool {
	return i.GitSHAFull == "" || i.GitSHAFull == "dev"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/version/...`
Expected: PASS — both `TestNew` (5 subtests) and `TestIsDev`.

- [ ] **Step 5: Commit**

```bash
git add internal/version/
git commit -m "feat(version): add build metadata package with derived GitHub URLs"
```

---

## Task 2: Extract `HealthHandler` and include version in response

**Files:**
- Create: `internal/api/handlers/health.go`
- Create: `internal/api/handlers/health_test.go`
- Modify: `internal/api/router.go` (add `Version` field to `RouterConfig`, register `HealthHandler`)

- [ ] **Step 1: Write the failing test**

Create `internal/api/handlers/health_test.go`:

```go
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LEFTEQ/lovinka-deployik/internal/version"
)

func TestHealthHandler_WithVersion(t *testing.T) {
	h := &HealthHandler{
		Version: version.New(
			"abc1234567890fedcba0123456789abcdef01234",
			"2026-04-19T10:23:11Z",
			"12345678",
			"lefteq/lovinka-deployik",
		),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type: got %q, want application/json", ct)
	}

	var body struct {
		Status  string        `json:"status"`
		Version *version.Info `json:"version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status field: got %q, want %q", body.Status, "ok")
	}
	if body.Version == nil {
		t.Fatal("version: missing from response")
	}
	if body.Version.GitSHA != "abc1234" {
		t.Errorf("version.git_sha: got %q, want %q", body.Version.GitSHA, "abc1234")
	}
	if body.Version.CommitURL != "https://github.com/lefteq/lovinka-deployik/commit/abc1234567890fedcba0123456789abcdef01234" {
		t.Errorf("version.commit_url: got %q", body.Version.CommitURL)
	}
	if body.Version.RunURL != "https://github.com/lefteq/lovinka-deployik/actions/runs/12345678" {
		t.Errorf("version.run_url: got %q", body.Version.RunURL)
	}
}

func TestHealthHandler_WithoutVersion(t *testing.T) {
	// Defensive: nil Version (older deploys, tests) must not panic and must
	// still report status:ok so docker healthchecks keep working.
	h := &HealthHandler{Version: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	var body struct {
		Status  string          `json:"status"`
		Version json.RawMessage `json:"version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status: got %q, want ok", body.Status)
	}
	if len(body.Version) != 0 && string(body.Version) != "null" {
		t.Errorf("version: expected null/omitted, got %s", string(body.Version))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/handlers/ -run TestHealthHandler -v`
Expected: FAIL with "undefined: HealthHandler".

- [ ] **Step 3: Write the implementation**

Create `internal/api/handlers/health.go`:

```go
package handlers

import (
	"net/http"

	"github.com/LEFTEQ/lovinka-deployik/internal/version"
)

// HealthHandler serves GET /api/health. Includes build version metadata so
// the SPA can render a "running build" badge in the sidebar footer. Used by
// the docker HEALTHCHECK and by scripts/deploy-vps.sh, so the response stays
// small and the status field is always "ok" when the process is up.
type HealthHandler struct {
	Version *version.Info // nil-safe; omitted from response when nil
}

type healthResponse struct {
	Status  string        `json:"status"`
	Version *version.Info `json:"version,omitempty"`
}

func (h *HealthHandler) Get(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:  "ok",
		Version: h.Version,
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/handlers/ -run TestHealthHandler -v`
Expected: PASS — both `TestHealthHandler_WithVersion` and `TestHealthHandler_WithoutVersion`.

- [ ] **Step 5: Wire into the router**

Edit `internal/api/router.go`. Add the import:

```go
"github.com/LEFTEQ/lovinka-deployik/internal/version"
```

Add `Version` to `RouterConfig` (after `DevMode`):

```go
type RouterConfig struct {
	DB             *db.DB
	JWTSecret      string
	Encryptor      *crypto.Encryptor
	OAuthConfig    *github.OAuthConfig
	AllowedUsers   []string
	AdminUsers     []string
	FrontendURL    string
	CookieSecure   bool
	AllowedOrigins []string
	Pipeline       *build.Pipeline
	DomainManager  *domain.Manager
	WSHub          *ws.Hub
	Analytics      *analytics.Service
	WebhookURL     string
	ScreenshotDir  string
	DevMode        bool
	Version        *version.Info
}
```

Replace the inline `/health` closure (currently lines 83-86):

```go
		// Public routes
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ok"}`))
		})
```

with:

```go
		// Public routes
		healthHandler := &handlers.HealthHandler{Version: cfg.Version}
		r.Get("/health", healthHandler.Get)
```

- [ ] **Step 6: Run all Go tests**

Run: `go test ./...`
Expected: PASS for every package. The router still compiles because `Version` defaults to `nil` for any caller that hasn't set it (still legal — `HealthHandler` is nil-safe).

- [ ] **Step 7: Commit**

```bash
git add internal/api/handlers/health.go internal/api/handlers/health_test.go internal/api/router.go
git commit -m "feat(api): include build version in /api/health response"
```

---

## Task 3: Inject ldflag vars in `main.go`

**Files:**
- Modify: `cmd/server/main.go` (declare vars, build `version.Info`, pass to router)

- [ ] **Step 1: Add the ldflag-injected vars and version import**

Edit `cmd/server/main.go`. Add to the import block:

```go
"github.com/LEFTEQ/lovinka-deployik/internal/version"
```

Immediately after the `//go:embed` directive and `embeddedWeb` declaration (after line 29), add:

```go
// Build metadata injected by `go build -ldflags="-X main.<name>=<value>"`.
// Populated in CI via Docker build args; defaults below apply for local
// `make dev-api` (or any build that omits -ldflags).
var (
	gitSHA    = "dev"
	buildTime = "unknown"
	ghRunID   = ""
	ghRepo    = "lefteq/lovinka-deployik"
)
```

- [ ] **Step 2: Build the `version.Info` and pass it into the router**

In `main()`, immediately before `router := api.NewRouter(...)` (around line 142), insert:

```go
	versionInfo := version.New(gitSHA, buildTime, ghRunID, ghRepo)
```

In the `&api.RouterConfig{...}` literal, append a `Version` field after `DevMode`:

```go
		DevMode:        os.Getenv("DEV_MODE") == "true",
		Version:        versionInfo,
```

- [ ] **Step 3: Build and run locally to confirm it compiles and serves the version**

Run: `make build && ./bin/deployik &`

(If `make build` produces a different output path, adapt — the goal is `go build ./cmd/server/`.)

Then in another terminal:

Run: `curl -s http://localhost:8080/api/health | head -c 400`

Expected: JSON like `{"status":"ok","version":{"git_sha":"dev","git_sha_full":"dev","build_time":"unknown","gh_repo":"lefteq/lovinka-deployik","gh_run_id":"","commit_url":"","run_url":""}}`

Stop the server: `kill %1`

- [ ] **Step 4: Run all Go tests**

Run: `go test ./...`
Expected: PASS for every package.

- [ ] **Step 5: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat(server): inject build metadata via ldflags and expose to router"
```

---

## Task 4: Pass build args through the Dockerfile

**Files:**
- Modify: `docker/Dockerfile`

- [ ] **Step 1: Add ARGs and forward them to `go build`**

Edit `docker/Dockerfile`. In the `go-builder` stage (currently lines 10-18), add `ARG` declarations after `WORKDIR /app` and before `COPY go.mod go.sum`:

```dockerfile
# Stage 2: Build Go binary
FROM golang:1.25-alpine AS go-builder
WORKDIR /app
ARG GIT_SHA=dev
ARG BUILD_TIME=unknown
ARG GH_RUN_ID=
ARG GH_REPO=lefteq/lovinka-deployik
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /app/web/dist cmd/server/web_dist
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w \
  -X main.gitSHA=${GIT_SHA} \
  -X main.buildTime=${BUILD_TIME} \
  -X main.ghRunID=${GH_RUN_ID} \
  -X main.ghRepo=${GH_REPO}" \
  -o /deployik ./cmd/server/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /deployik-backup ./cmd/backup/
```

(The `deployik-backup` binary is unaffected — version metadata is only relevant to the long-running server.)

- [ ] **Step 2: Verify the image builds and reports the injected version**

Run: `docker build --file docker/Dockerfile --build-arg GIT_SHA=abc1234567890fedcba0123456789abcdef01234 --build-arg BUILD_TIME=2026-04-19T10:23:11Z --build-arg GH_RUN_ID=12345678 --build-arg GH_REPO=lefteq/lovinka-deployik -t deployik:plan-test .`

Expected: build succeeds.

Run: `docker run --rm -d --name deployik-plan-test -e JWT_SECRET=x -e ENCRYPTION_KEY=x -e GITHUB_CLIENT_ID=x -e GITHUB_CLIENT_SECRET=x -e DEV_MODE=true -p 8081:8080 deployik:plan-test`

Run: `sleep 2 && curl -s http://localhost:8081/api/health`

Expected: `version.git_sha == "abc1234"`, `commit_url` and `run_url` populated with the injected SHA / run id.

Cleanup: `docker stop deployik-plan-test`

- [ ] **Step 3: Verify a no-build-args build still works (backwards compat)**

Run: `docker build --file docker/Dockerfile -t deployik:plan-test-noargs .`

Expected: build succeeds; `git_sha == "dev"`, both URLs empty (binary built with default ARG values).

Cleanup: `docker rmi deployik:plan-test deployik:plan-test-noargs`

- [ ] **Step 4: Commit**

```bash
git add docker/Dockerfile
git commit -m "build(docker): forward GIT_SHA/BUILD_TIME/GH_RUN_ID/GH_REPO build args to ldflags"
```

---

## Task 5: Pass build args from CI

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Add `build-args` to the `docker/build-push-action` step**

Edit `.github/workflows/ci.yml`. Replace the `docker/build-push-action@v6` step (currently lines 67-73):

```yaml
      - uses: docker/build-push-action@v6
        with:
          context: .
          file: docker/Dockerfile
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
```

with:

```yaml
      - uses: docker/build-push-action@v6
        with:
          context: .
          file: docker/Dockerfile
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          build-args: |
            GIT_SHA=${{ github.sha }}
            BUILD_TIME=${{ github.event.head_commit.timestamp || github.run_started_at }}
            GH_RUN_ID=${{ github.run_id }}
            GH_REPO=${{ github.repository }}
```

The `||` fallback covers cases where `head_commit.timestamp` is absent (rare on `push`, but defensive).

- [ ] **Step 2: Lint the workflow file (syntax sanity)**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo OK`

Expected: `OK` (no YAML parse error).

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: forward git SHA, run id and repo as Docker build args"
```

---

## Task 6: Frontend types + API client + query key

**Files:**
- Modify: `web/src/types/api.ts` (add `VersionInfo` and `HealthResponse`)
- Modify: `web/src/lib/api.ts` (add `getHealth` method)
- Modify: `web/src/lib/queryKeys.ts` (add `health` key)

- [ ] **Step 1: Add types**

Edit `web/src/types/api.ts`. Append to the end of the file:

```ts
export interface VersionInfo {
  git_sha: string;
  git_sha_full: string;
  build_time: string;
  gh_repo: string;
  gh_run_id: string;
  commit_url: string;
  run_url: string;
}

export interface HealthResponse {
  status: "ok";
  version?: VersionInfo;
}
```

- [ ] **Step 2: Add the API method**

Edit `web/src/lib/api.ts`. Add `HealthResponse` to the import block at the top:

```ts
import type {
  AuthResponse,
  User,
  Organization,
  Project,
  Deployment,
  Domain,
  EnvVariable,
  SecretVariable,
  VariableScope,
  BuildLog,
  GitHubRepo,
  PlatformInfo,
  AnalyticsEnvironmentFilter,
  AnalyticsRangePreset,
  ProjectAnalyticsPayload,
  AutoBuildConfig,
  DeploymentListFilters,
  DeploymentListResponse,
  ProtectionStatus,
  ProtectionUpdateResponse,
  VerifyDomainResponse,
  HealthResponse,
} from "@/types/api";
```

Inside the `ApiClient` class, after the existing `getMe` method (around line 117), add:

```ts
  async getHealth(): Promise<HealthResponse> {
    return this.request("/health", { method: "GET" }, false);
  }
```

(Passing `allowRefresh = false` because `/api/health` is public and never returns 401 — no point routing it through the refresh dance.)

- [ ] **Step 3: Add the query key**

Edit `web/src/lib/queryKeys.ts`. Inside the `queryKeys` object, after the existing `platform` key (around line 10), add:

```ts
  health: () => ["health"] as const,
```

- [ ] **Step 4: Typecheck**

Run: `cd web && bunx tsc --noEmit`

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/types/api.ts web/src/lib/api.ts web/src/lib/queryKeys.ts
git commit -m "feat(web): add HealthResponse type, getHealth() and health query key"
```

---

## Task 7: `VersionRow` sidebar component

**Files:**
- Create: `web/src/components/layout/VersionRow.tsx`

- [ ] **Step 1: Create the component**

Create `web/src/components/layout/VersionRow.tsx`:

```tsx
import { useQuery } from "@tanstack/react-query";
import { GitCommit, ExternalLink } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { useSidebar } from "@/components/ui/sidebar";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";

// Renders the running build SHA + a link to the GitHub Actions run that
// produced it. Sits in the sidebar footer above the user/workspace dropdown.
// Hidden entirely when the binary was built without version metadata
// (impossible after Task 3 since defaults are "dev" rather than "").
export function VersionRow() {
  const { state } = useSidebar();
  const collapsed = state === "collapsed";

  const { data } = useQuery({
    queryKey: queryKeys.health(),
    queryFn: () => api.getHealth(),
    staleTime: Infinity,
    gcTime: Infinity,
    retry: 1,
  });

  const version = data?.version;
  if (!version) return null;

  const isDev = !version.git_sha_full || version.git_sha_full === "dev";
  const tooltipLabel = version.gh_run_id
    ? `v ${version.git_sha} \u00b7 build #${version.gh_run_id}`
    : `v ${version.git_sha}`;

  if (collapsed) {
    return (
      <TooltipProvider delayDuration={150}>
        <Tooltip>
          <TooltipTrigger asChild>
            {version.commit_url ? (
              <a
                href={version.commit_url}
                target="_blank"
                rel="noreferrer"
                className="mx-auto flex size-8 items-center justify-center rounded-md text-muted-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
                aria-label={tooltipLabel}
              >
                <GitCommit className="size-4" />
              </a>
            ) : (
              <span
                className="mx-auto flex size-8 items-center justify-center rounded-md text-muted-foreground"
                aria-label={tooltipLabel}
              >
                <GitCommit className="size-4" />
              </span>
            )}
          </TooltipTrigger>
          <TooltipContent side="right">{tooltipLabel}</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    );
  }

  return (
    <div className="flex items-center justify-between gap-2 px-2 py-1.5 text-xs text-muted-foreground">
      {version.commit_url ? (
        <a
          href={version.commit_url}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1.5 rounded hover:text-foreground"
          title={`Commit ${version.git_sha_full}`}
        >
          <GitCommit className="size-3.5" />
          <span className="font-mono">{version.git_sha}</span>
        </a>
      ) : (
        <span
          className="inline-flex items-center gap-1.5"
          title={isDev ? "Local development build" : version.git_sha_full}
        >
          <GitCommit className="size-3.5" />
          <span className="font-mono">{version.git_sha}</span>
        </span>
      )}

      {version.run_url && (
        <a
          href={version.run_url}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1 rounded hover:text-foreground"
          title={`GitHub Actions run #${version.gh_run_id}`}
        >
          <span>build</span>
          <ExternalLink className="size-3" />
        </a>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Typecheck**

Run: `cd web && bunx tsc --noEmit`

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/components/layout/VersionRow.tsx
git commit -m "feat(web): add VersionRow sidebar component"
```

---

## Task 8: Mount `VersionRow` in `AppSidebar`

**Files:**
- Modify: `web/src/components/layout/AppSidebar.tsx`

- [ ] **Step 1: Import the new component**

Edit `web/src/components/layout/AppSidebar.tsx`. After the existing `ProjectPicker` import (around line 26), add:

```tsx
import { VersionRow } from "@/components/layout/VersionRow";
```

- [ ] **Step 2: Render it inside `SidebarFooter`**

In `AppSidebar.tsx`, the current footer (around lines 316-368) starts with `<SidebarFooter>` and contains a single `<SidebarMenu>` with the user dropdown. Insert `<VersionRow />` immediately inside `<SidebarFooter>`, before the existing `<SidebarMenu>`:

```tsx
      <SidebarFooter>
        <VersionRow />
        <SidebarMenu>
          <SidebarMenuItem>
            {/* ...existing user/workspace dropdown unchanged... */}
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
```

(Do not modify the dropdown menu, the avatar, or anything below it.)

- [ ] **Step 3: Typecheck**

Run: `cd web && bunx tsc --noEmit`

Expected: no errors.

- [ ] **Step 4: Frontend build sanity check**

Run: `cd web && bun run build`

Expected: build succeeds, no warnings about missing exports.

- [ ] **Step 5: Manual smoke test (dev mode)**

Run (in two terminals):
- Terminal A: `make dev-api`
- Terminal B: `make dev-web`

Open http://localhost:5173 and log in (use `/api/auth/dev-login` if needed via the existing dev login flow).

Verify:
- Sidebar footer shows a row above the user dropdown reading `<commit-icon> dev` (no link, since `git_sha_full == "dev"`) and no `build` link.
- Toggle the sidebar to icon-only mode (Ctrl/Cmd+B). The row collapses to a single GitCommit icon with tooltip "v dev".
- Open browser devtools → Network tab → confirm `GET /api/health` is fired exactly once on app load.

If any of the above is wrong, fix before committing.

- [ ] **Step 6: Commit**

```bash
git add web/src/components/layout/AppSidebar.tsx
git commit -m "feat(web): render VersionRow in sidebar footer"
```

---

## Task 9: Update CLAUDE.md

**Files:**
- Modify: `.claude/CLAUDE.md` (document the new files and the build-arg pipeline)

- [ ] **Step 1: Add the version package and HealthHandler to the project structure**

Edit `.claude/CLAUDE.md`. Inside the `internal/` section, after the `audit/` block, insert:

```
  version/
    version.go            Build metadata (git SHA, build time, GH Actions run); New() derives commit/run URLs from raw inputs
```

Inside the `internal/api/handlers/` listing, add a line for `health.go`:

```
      health.go           HealthHandler: GET /api/health -- {status, version} JSON; nil-safe for tests/older builds
```

- [ ] **Step 2: Add the frontend file**

Inside the `web/src/components/layout/` block of `CLAUDE.md`, add:

```
    layout/VersionRow.tsx Sidebar footer row showing commit SHA + GH Actions run; collapses to icon + tooltip in icon-only sidebar mode
```

- [ ] **Step 3: Document the build-arg pipeline**

Find the existing "Design Decisions" or "Key Patterns and Conventions" section. Under "Go Backend", add a bullet:

```
- **Build metadata:** `cmd/server/main.go` declares `gitSHA`, `buildTime`, `ghRunID`, `ghRepo` package vars set at link time via `go build -ldflags="-X main.<name>=<value>"`. CI (`.github/workflows/ci.yml`) passes these as Docker `build-args` to `docker/build-push-action@v6`, which the `Dockerfile` `go-builder` stage forwards to the `RUN go build` line. The result is wrapped in `internal/version.Info` and surfaced via the `/api/health` JSON response.
```

Update the `/api/health` description in the "Public" API endpoints list:

Replace `- `GET  /api/health` -- Health check`` with:

```
- `GET  /api/health` -- Health check; response includes `version` block (git SHA, GitHub Actions run id, commit_url, run_url) for the SPA's sidebar build badge
```

- [ ] **Step 4: Commit**

```bash
git add .claude/CLAUDE.md
git commit -m "docs: document build metadata pipeline and version sidebar badge"
```

---

## Task 10: Final verification pass

- [ ] **Step 1: Run the full Go test suite**

Run: `go test ./...`
Expected: PASS for every package.

- [ ] **Step 2: Run the full frontend typecheck and build**

Run: `cd web && bunx tsc --noEmit && bun run build`
Expected: no errors.

- [ ] **Step 3: Confirm the production Dockerfile path still works without build args**

Run: `docker build --file docker/Dockerfile -t deployik:final-check .`
Expected: build succeeds.

Cleanup: `docker rmi deployik:final-check`

- [ ] **Step 4: Skim the diff**

Run: `git log --oneline main..HEAD`

Expected: 8-9 small commits, one per task. Each commit message follows the existing project convention (`feat(scope):`, `build(docker):`, `ci:`, `docs:`).

- [ ] **Step 5: Push the branch and open the PR**

This is the only step that touches a shared system — confirm with the user before pushing if no green light has been given. Once approved:

```bash
git push -u origin HEAD
gh pr create --title "Sidebar version badge" --body "$(cat <<'EOF'
## Summary
- Bake `GIT_SHA`, `BUILD_TIME`, `GH_RUN_ID`, `GH_REPO` into the Go binary via Docker build args + `-ldflags`
- Surface them in the existing `GET /api/health` response (server-built `commit_url` + `run_url`)
- Render a muted `<commit-icon> sha` + `build` row in the sidebar footer; collapses to a single tooltipped icon in icon-only mode

Design doc: `docs/plans/2026-04-19-sidebar-version-badge-design.md`

## Test plan
- [ ] Go test suite passes (`go test ./...`)
- [ ] Frontend typecheck + build pass (`cd web && bunx tsc --noEmit && bun run build`)
- [ ] Local Docker image with `--build-arg GIT_SHA=...` exposes the SHA at `/api/health`
- [ ] After merge, `https://deployik.example.com/api/health` returns the deployed commit's SHA + a working `commit_url` and `run_url`
- [ ] Sidebar footer shows the commit + build links; both open the right GitHub pages
- [ ] Sidebar collapsed mode shows a tooltipped icon
EOF
)"
```

---

## Open Questions / Future Work

(Carried over from the design doc — not blockers for this plan.)

- **Stale-SPA reload toast** — once a user has the SPA loaded, they keep their old JS until a hard refresh. A future enhancement could poll `/api/health` every minute and show a "new version available — reload" toast when `git_sha_full` changes from the boot-time value. Out of scope here; a single `useQuery` change with `refetchInterval: 60_000` would do it.
- **Build timestamp formatting** — `build_time` is shown raw (RFC3339). If a user-friendly relative time is desired in the tooltip, reuse `formatRelativeDate` from `lib/deployment-helpers.ts` in a follow-up.
- **PR builds** — PR builds are tested in CI but not pushed to GHCR or deployed (workflow gates `build-and-push` on `github.ref == 'refs/heads/main'`). The build-args still apply on `main`-only pushes, so PRs are unaffected by this plan.
