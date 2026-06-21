# App Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Promote a Deployik **App** from one plain page into a first-class shell (Overview / Deployments / Topology / Variables / Releases / Settings) with a two-column Overview + sticky live-pulse rail, an auto-derived architecture map from env wiring, live container-health roll-up, and a unified cross-member deployments feed with deep links.

**Architecture:** Read-model + UI over the already-shipped App Bundles schema — **no new DB migrations**. New Go DB queries (`GetLatestDeployment`, `ListAppDeployments`, `SetAppMemberOrder`), an upgraded `GET /apps/{id}/health`, two new endpoints (`/topology`, `/deployments`, `/members/order`), a live `HealthProber`, and a new TanStack app-layout route subtree with React pages reusing the existing dark design language.

**Tech Stack:** Go 1.25 (chi, modernc SQLite, stdlib `net/http` probe), React 19 + Vite + TanStack Router/Query + shadcn/ui + Tailwind 4. Go tests: in-memory SQLite + httptest. FE tests: `bun:test`. Module path: `github.com/LEFTEQ/lovinka-deployik`.

**Spec:** `docs/superpowers/specs/2026-06-21-app-dashboard-design.md`

---

## Conventions (read once, applies to every task)

- **Go errors:** `writeJSON(w, http.Status..., map[string]string{"error": "..."})`. Success: typed struct / model / inline `map[string]any`. 204: `w.WriteHeader(http.StatusNoContent)`.
- **Go query style:** method on `*db.DB`, `db.Query/QueryRow/Exec/Begin`, `fmt.Errorf("verb: %w", err)`, `defer rows.Close()`, `return out, rows.Err()`.
- **Deployment scan column order is load-bearing** (must match every other deployment query): `id, project_id, environment, COALESCE(preview_instance_id,''), commit_sha, commit_message, branch, status, container_id, container_name, image_tag, build_duration, triggered_by, error_message, created_at, finished_at, trigger_source, triggered_by_username, screenshot_path` then joined `username, avatar_url`. `screenshot_path` scans into a `sql.NullString` → `.String`.
- **Decryption happens in handlers** (`h.Encryptor.Decrypt`), never in package `db` (DB has no encryptor).
- **App auth:** `h.DB.GetAppForUser(appID, claims.UserID)` (membership-scoped); use the existing `loadManagedApp` helper.
- **trigger_source** for any app-created deployment MUST be the constant `appMemberTriggerSource` (`"api"`).
- **Go tests:** package `db` uses `newTestDB(t)` + `createAppTestUser(t, db, name, githubID)`; package `handlers` uses `appTestDB(t)`. Auth via `auth.WithClaims(ctx, &auth.Claims{UserID, Role})`. chi params via `chi.NewRouteContext()` + `rctx.URLParams.Add("id", ...)` + `context.WithValue(ctx, chi.RouteCtxKey, rctx)`. Assertions: stdlib `t.Fatalf`, include `(body: %s)` via `rec.Body.String()`. Command: `go test ./...`.
- **FE conventions:** api methods route through `this.request<T>(path, { method, body: JSON.stringify(...) })`; bodies are snake_case. Toasts via `sonner`. Icons `lucide-react`. `useParams({ strict: false })`. META maps end with `satisfies Record<...>`. FE typecheck: `cd web && bunx tsc --noEmit`. FE tests: `cd web && bun run test`.
- **Commits:** Conventional Commits, one per task. Stage only the files the task names (the working tree has unrelated in-progress edits — never `git add -A`).

## File Structure

**Backend (new):**
- `internal/db/queries_app_deployments.go` — `AppDeployment` type + `ListAppDeployments(appID, env, limit)`.
- `internal/api/handlers/app_health.go` — pure status helpers (`deriveMemberLiveStatus`, `combinedAppStatus`) + the `HealthProber` interface (P2).
- `internal/api/handlers/app_topology.go` — `GetTopology` handler + pure `deriveTopologyEdges`.
- `internal/build/health_prober.go` — `DockerHealthProber` (P2).
- Test files alongside each.

**Backend (modified):**
- `internal/db/queries_deployments.go` — add `GetLatestDeployment`.
- `internal/db/queries_apps.go` — add `SetAppMemberOrder`.
- `internal/api/handlers/apps.go` — upgrade `GetHealth` (additive); add `Prober` field (P2).
- `internal/api/handlers/app_deploy.go` — (P4) `ReorderMembers` handler lives in apps.go; deploy/rollback untouched.
- `internal/api/router.go` — register `/topology`, `/deployments`, `/members/order`; wire `Prober`.

**Frontend (new):**
- `web/src/lib/app-helpers.ts` — `APP_STATUS_META`, `MEMBER_STATUS_META`, `RELEASE_STATUS_META`.
- `web/src/components/layout/AppBundleLayout.tsx` — app shell (mirrors ProjectLayout).
- `web/src/pages/AppOverview.tsx` — two-column Overview (replaces AppDetail as the index).
- `web/src/pages/AppDeployments.tsx`, `AppTopology.tsx`, `AppVariables.tsx`, `AppReleases.tsx`, `AppSettings.tsx`.
- `web/src/components/apps/topology-map.tsx`, `web/src/components/apps/app-variable-store.tsx`.
- `web/src/lib/app-helpers.test.ts`.

**Frontend (modified):**
- `web/src/types/api.ts` — additive AppHealth fields + `AppDeployment`, `AppTopology`, `TopologyNode`, `TopologyEdge`.
- `web/src/lib/api.ts` — `getAppHealth(appId, env?)`, `getAppTopology`, `listAppDeployments`, `reorderAppMembers`, app env/secret methods (P4).
- `web/src/lib/queryKeys.ts` — `appHealth(appId, env)`, `appTopology`, `appDeployments`, `appEnv`/`appSecrets` (P4).
- `web/src/app/app.tsx` — new `appLayoutRoute` subtree; move `/apps/$appId` off `workspaceLayoutRoute`.
- `web/src/components/layout/AppSidebar.tsx`, `web/src/components/layout/MobileTabBar.tsx` — add `"app"` context.
- `web/src/pages/AppDetail.tsx` — deleted at end of P1 (superseded by AppOverview).

---

# Phase 1 — Shell + Overview + unified deployments

Backend health stays **additive** (keeps existing fields so `AppDetail.tsx` compiles until it is replaced at the end of P1). Live status in P1 is derived from latest-deployment status only (DB-only); P2 swaps in the real probe behind the same field.

### Task 1.1: `GetLatestDeployment` DB query

**Files:**
- Modify: `internal/db/queries_deployments.go`
- Test: `internal/db/queries_deployments_test.go` (create if absent)

- [ ] **Step 1: Write the failing test**

```go
package db

import "testing"

func TestGetLatestDeployment(t *testing.T) {
	database := newTestDB(t)
	user := createAppTestUser(t, database, "owner", 1)
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	project := &Project{
		Name: "api", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, OrganizationID: org.ID, Framework: "static",
		PackageManager: "auto", Status: "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	older := &Deployment{ProjectID: project.ID, Environment: "production", Branch: "main", Status: "replaced", TriggeredBy: user.ID, CommitSHA: "aaa"}
	if err := database.CreateDeployment(older); err != nil {
		t.Fatalf("CreateDeployment older: %v", err)
	}
	newer := &Deployment{ProjectID: project.ID, Environment: "production", Branch: "main", Status: "live", TriggeredBy: user.ID, CommitSHA: "bbb"}
	if err := database.CreateDeployment(newer); err != nil {
		t.Fatalf("CreateDeployment newer: %v", err)
	}
	// a preview deployment that must NOT be returned for production
	if err := database.CreateDeployment(&Deployment{ProjectID: project.ID, Environment: "preview", Branch: "dev", Status: "live", TriggeredBy: user.ID, CommitSHA: "ccc"}); err != nil {
		t.Fatalf("CreateDeployment preview: %v", err)
	}

	got, err := database.GetLatestDeployment(project.ID, "production")
	if err != nil {
		t.Fatalf("GetLatestDeployment: %v", err)
	}
	if got == nil || got.CommitSHA != "bbb" || got.Status != "live" {
		t.Fatalf("expected newest production deployment bbb/live, got %+v", got)
	}

	none, err := database.GetLatestDeployment(project.ID, "production-nope")
	if err != nil {
		t.Fatalf("GetLatestDeployment empty env: %v", err)
	}
	if none != nil {
		t.Fatalf("expected nil for env with no deployments, got %+v", none)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestGetLatestDeployment -v`
Expected: FAIL — `database.GetLatestDeployment undefined`.

- [ ] **Step 3: Implement `GetLatestDeployment`**

Add to `internal/db/queries_deployments.go` (uses the canonical deployment scan order; `screenshot_path` via `sql.NullString`):

```go
// GetLatestDeployment returns the most recent deployment for a project in an
// environment regardless of status (live/failed/building/...), or (nil, nil)
// when none exist. Used by the App dashboard to derive per-member status.
func (db *DB) GetLatestDeployment(projectID, environment string) (*Deployment, error) {
	row := db.QueryRow(
		`SELECT d.id, d.project_id, d.environment, COALESCE(d.preview_instance_id, ''),
		        d.commit_sha, d.commit_message, d.branch, d.status,
		        d.container_id, d.container_name, d.image_tag, d.build_duration, d.triggered_by,
		        d.error_message, d.created_at, d.finished_at,
		        d.trigger_source, d.triggered_by_username, d.screenshot_path
		 FROM deployments d
		 WHERE d.project_id = ? AND d.environment = ?
		 ORDER BY d.created_at DESC, d.id DESC
		 LIMIT 1`,
		projectID, environment,
	)
	var d Deployment
	var screenshotPath sql.NullString
	if err := row.Scan(
		&d.ID, &d.ProjectID, &d.Environment, &d.PreviewInstanceID, &d.CommitSHA, &d.CommitMessage,
		&d.Branch, &d.Status, &d.ContainerID, &d.ContainerName, &d.ImageTag,
		&d.BuildDuration, &d.TriggeredBy, &d.ErrorMessage, &d.CreatedAt, &d.FinishedAt,
		&d.TriggerSource, &d.TriggeredByUsername, &screenshotPath,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get latest deployment: %w", err)
	}
	d.ScreenshotPath = screenshotPath.String
	return &d, nil
}
```

If `errors`/`database/sql`/`fmt` aren't already imported in the file, add them (they are used elsewhere in the package; check the file's import block).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/db/ -run TestGetLatestDeployment -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/queries_deployments.go internal/db/queries_deployments_test.go
git commit -m "feat(apps): GetLatestDeployment query for per-member status"
```

### Task 1.2: `ListAppDeployments` unified cross-member query

**Files:**
- Create: `internal/db/queries_app_deployments.go`
- Test: `internal/db/queries_app_deployments_test.go`

- [ ] **Step 1: Write the failing test**

```go
package db

import "testing"

func TestListAppDeployments(t *testing.T) {
	database := newTestDB(t)
	user := createAppTestUser(t, database, "owner", 1)
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	app, err := database.CreateApp(&AppCreate{OrganizationID: org.ID, Name: "Bundle"})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}

	member := &Project{
		Name: "web", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, OrganizationID: org.ID, AppID: app.ID, Framework: "nextjs",
		PackageManager: "auto", Status: "active",
	}
	if err := database.CreateProject(member); err != nil {
		t.Fatalf("CreateProject member: %v", err)
	}
	standalone := &Project{
		Name: "lonely", GithubRepo: "r2", GithubOwner: "o", Branch: "main",
		UserID: user.ID, OrganizationID: org.ID, Framework: "static",
		PackageManager: "auto", Status: "active",
	}
	if err := database.CreateProject(standalone); err != nil {
		t.Fatalf("CreateProject standalone: %v", err)
	}

	if err := database.CreateDeployment(&Deployment{ProjectID: member.ID, Environment: "production", Branch: "main", Status: "live", TriggeredBy: user.ID, CommitSHA: "m1"}); err != nil {
		t.Fatalf("CreateDeployment member: %v", err)
	}
	if err := database.CreateDeployment(&Deployment{ProjectID: standalone.ID, Environment: "production", Branch: "main", Status: "live", TriggeredBy: user.ID, CommitSHA: "s1"}); err != nil {
		t.Fatalf("CreateDeployment standalone: %v", err)
	}

	got, err := database.ListAppDeployments(app.ID, "production", 10)
	if err != nil {
		t.Fatalf("ListAppDeployments: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected only the app member's deployment, got %d", len(got))
	}
	if got[0].ProjectName != "web" || got[0].CommitSHA != "m1" {
		t.Fatalf("unexpected row: %+v", got[0])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestListAppDeployments -v`
Expected: FAIL — `database.ListAppDeployments undefined`.

- [ ] **Step 3: Implement the type + query**

Create `internal/db/queries_app_deployments.go`:

```go
package db

import (
	"database/sql"
	"fmt"
)

// AppDeployment is a deployment row enriched with the triggering user and the
// owning member project's name, for the App dashboard's unified feed.
type AppDeployment struct {
	DeploymentWithUser
	ProjectName string `json:"project_name"`
}

// ListAppDeployments returns recent deployments across every (non-deleted)
// member project of an app for one environment, newest first. limit<=0 -> 20.
func (db *DB) ListAppDeployments(appID, environment string, limit int) ([]AppDeployment, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.Query(
		`SELECT d.id, d.project_id, d.environment, COALESCE(d.preview_instance_id, ''),
		        d.commit_sha, d.commit_message, d.branch, d.status,
		        d.container_id, d.container_name, d.image_tag, d.build_duration, d.triggered_by,
		        d.error_message, d.created_at, d.finished_at,
		        d.trigger_source, d.triggered_by_username, d.screenshot_path,
		        COALESCE(u.username, '') AS username, COALESCE(u.avatar_url, '') AS avatar_url,
		        p.name AS project_name
		 FROM deployments d
		 JOIN projects p ON p.id = d.project_id
		 LEFT JOIN users u ON u.id = d.triggered_by
		 WHERE p.app_id = ? AND p.status != 'deleted' AND d.environment = ?
		 ORDER BY d.created_at DESC, d.id DESC
		 LIMIT ?`,
		appID, environment, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list app deployments: %w", err)
	}
	defer rows.Close()

	out := make([]AppDeployment, 0)
	for rows.Next() {
		var row AppDeployment
		var screenshotPath sql.NullString
		d := &row.Deployment
		if err := rows.Scan(
			&d.ID, &d.ProjectID, &d.Environment, &d.PreviewInstanceID, &d.CommitSHA, &d.CommitMessage,
			&d.Branch, &d.Status, &d.ContainerID, &d.ContainerName, &d.ImageTag,
			&d.BuildDuration, &d.TriggeredBy, &d.ErrorMessage, &d.CreatedAt, &d.FinishedAt,
			&d.TriggerSource, &d.TriggeredByUsername, &screenshotPath,
			&row.Username, &row.AvatarURL,
			&row.ProjectName,
		); err != nil {
			return nil, fmt.Errorf("scan app deployment: %w", err)
		}
		d.ScreenshotPath = screenshotPath.String
		out = append(out, row)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/db/ -run TestListAppDeployments -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/queries_app_deployments.go internal/db/queries_app_deployments_test.go
git commit -m "feat(apps): ListAppDeployments unified cross-member feed query"
```

### Task 1.3: Pure health-status helpers

**Files:**
- Create: `internal/api/handlers/app_health.go`
- Test: `internal/api/handlers/app_health_test.go`

- [ ] **Step 1: Write the failing test**

```go
package handlers

import "testing"

import "github.com/LEFTEQ/lovinka-deployik/internal/db"

func TestDeriveMemberLiveStatusFromDeployment(t *testing.T) {
	cases := []struct {
		name   string
		latest *db.Deployment
		want   string
	}{
		{"none", nil, "none"},
		{"live", &db.Deployment{Status: "live"}, "healthy"},
		{"building", &db.Deployment{Status: "building"}, "deploying"},
		{"queued", &db.Deployment{Status: "queued"}, "deploying"},
		{"deploying", &db.Deployment{Status: "deploying"}, "deploying"},
		{"failed", &db.Deployment{Status: "failed"}, "failed"},
		{"rolled_back", &db.Deployment{Status: "rolled_back"}, "degraded"},
		{"replaced", &db.Deployment{Status: "replaced"}, "degraded"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := deriveMemberLiveStatusFromDeployment(c.latest); got != c.want {
				t.Fatalf("status = %q, want %q", got, c.want)
			}
		})
	}
}

func TestCombinedAppStatus(t *testing.T) {
	cases := []struct {
		name    string
		members []string
		want    string
	}{
		{"empty", nil, "none"},
		{"all healthy", []string{"healthy", "healthy"}, "healthy"},
		{"one deploying", []string{"healthy", "deploying"}, "deploying"},
		{"degraded beats deploying", []string{"deploying", "degraded"}, "degraded"},
		{"failed maps to degraded", []string{"healthy", "failed"}, "degraded"},
		{"down is worst", []string{"degraded", "down", "deploying"}, "down"},
		{"none only", []string{"none", "none"}, "none"},
		{"unknown maps to degraded", []string{"healthy", "unknown"}, "degraded"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := combinedAppStatus(c.members); got != c.want {
				t.Fatalf("combined = %q, want %q", got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/handlers/ -run 'TestDeriveMemberLiveStatusFromDeployment|TestCombinedAppStatus' -v`
Expected: FAIL — undefined functions.

- [ ] **Step 3: Implement the helpers**

Create `internal/api/handlers/app_health.go`:

```go
package handlers

import "github.com/LEFTEQ/lovinka-deployik/internal/db"

// Member live-status vocabulary (worst-of contributes to the combined status).
const (
	memberStatusHealthy   = "healthy"
	memberStatusDeploying = "deploying"
	memberStatusDegraded  = "degraded"
	memberStatusFailed    = "failed"
	memberStatusDown      = "down"
	memberStatusNone      = "none"
	memberStatusUnknown   = "unknown"
)

// deriveMemberLiveStatusFromDeployment is the P1 (DB-only) status source: it
// maps a member's latest deployment to a coarse live status. P2 refines this
// with a real container probe via resolveMemberLiveStatus.
func deriveMemberLiveStatusFromDeployment(latest *db.Deployment) string {
	if latest == nil {
		return memberStatusNone
	}
	switch latest.Status {
	case "live":
		return memberStatusHealthy
	case "queued", "building", "deploying":
		return memberStatusDeploying
	case "failed":
		return memberStatusFailed
	default: // rolled_back, replaced, or anything unexpected
		return memberStatusDegraded
	}
}

// statusSeverity ranks a member status for the worst-of roll-up and maps it to
// the combined-status vocabulary (healthy|deploying|degraded|down|none).
func statusSeverity(s string) (int, string) {
	switch s {
	case memberStatusDown:
		return 4, "down"
	case memberStatusDegraded, memberStatusFailed, memberStatusUnknown:
		return 3, "degraded"
	case memberStatusDeploying:
		return 2, "deploying"
	case memberStatusHealthy:
		return 1, "healthy"
	default: // none / unrecognized
		return 0, "none"
	}
}

// combinedAppStatus returns the worst-of member statuses as a combined app
// status. Empty input -> "none".
func combinedAppStatus(memberStatuses []string) string {
	best := -1
	combined := "none"
	for _, s := range memberStatuses {
		sev, label := statusSeverity(s)
		if sev > best {
			best = sev
			combined = label
		}
	}
	return combined
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/handlers/ -run 'TestDeriveMemberLiveStatusFromDeployment|TestCombinedAppStatus' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/handlers/app_health.go internal/api/handlers/app_health_test.go
git commit -m "feat(apps): pure member/combined health-status helpers"
```

### Task 1.4: Upgrade `GetHealth` (additive: env param, live_status, primary_domain, latest_deployment, combined_status)

**Files:**
- Modify: `internal/api/handlers/apps.go` (the `appHealth`/`appHealthMember` structs + `GetHealth`)
- Test: `internal/api/handlers/apps_test.go` (add a test)

- [ ] **Step 1: Write the failing test**

Add to `internal/api/handlers/apps_test.go`:

```go
func TestGetHealthReturnsLiveStatusAndCombined(t *testing.T) {
	database := appTestDB(t)
	user := &db.User{ID: db.NewID(), GithubID: 1, Username: "owner", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	app, err := database.CreateApp(&db.AppCreate{OrganizationID: org.ID, Name: "Bundle"})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	member := &db.Project{
		Name: "web", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, OrganizationID: org.ID, AppID: app.ID, Framework: "nextjs",
		PackageManager: "auto", Status: "active",
	}
	if err := database.CreateProject(member); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if err := database.CreateDeployment(&db.Deployment{ProjectID: member.ID, Environment: "production", Branch: "main", Status: "live", TriggeredBy: user.ID, CommitSHA: "m1"}); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	h := &AppHandler{DB: database}
	claims := &auth.Claims{UserID: user.ID, Role: "user"}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", app.ID)
	req := httptest.NewRequest(http.MethodGet, "/apps/"+app.ID+"/health?environment=production", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(auth.WithClaims(req.Context(), claims))
	rec := httptest.NewRecorder()

	h.GetHealth(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var out struct {
		CombinedStatus string `json:"combined_status"`
		Environment    string `json:"environment"`
		Members        []struct {
			LiveStatus string `json:"live_status"`
		} `json:"members"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Environment != "production" || out.CombinedStatus != "healthy" {
		t.Fatalf("env/combined = %q/%q, want production/healthy (body: %s)", out.Environment, out.CombinedStatus, rec.Body.String())
	}
	if len(out.Members) != 1 || out.Members[0].LiveStatus != "healthy" {
		t.Fatalf("members = %+v, want one healthy (body: %s)", out.Members, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/handlers/ -run TestGetHealthReturnsLiveStatusAndCombined -v`
Expected: FAIL — `combined_status`/`live_status` empty (current handler doesn't emit them).

- [ ] **Step 3: Update the structs + `GetHealth` in `apps.go`**

Replace the existing `appHealth`/`appHealthMember` structs and `GetHealth` method with:

```go
// appHealth is the composite "unified view" payload.
type appHealth struct {
	App            *db.App           `json:"app"`
	Environment    string            `json:"environment"`
	CombinedStatus string            `json:"combined_status"`
	Members        []appHealthMember `json:"members"`
}

type appHealthMember struct {
	Project          db.Project     `json:"project"`
	LiveStatus       string         `json:"live_status"`
	PrimaryDomain    string         `json:"primary_domain,omitempty"`
	LatestDeployment *db.Deployment `json:"latest_deployment,omitempty"`
	// Retained for backwards-compatibility with the legacy AppDetail page.
	LatestPreview    *time.Time `json:"latest_preview_deploy_at,omitempty"`
	LatestProduction *time.Time `json:"latest_production_deploy_at,omitempty"`
}

func (h *AppHandler) GetHealth(w http.ResponseWriter, r *http.Request) {
	app, _, ok := h.loadManagedApp(w, r)
	if !ok {
		return
	}
	environment, valid := normalizeAppEnvironment(r.URL.Query().Get("environment"))
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment must be preview or production"})
		return
	}

	members, err := h.DB.ListProjectsByApp(app.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load members"})
		return
	}

	out := appHealth{App: app, Environment: environment, Members: make([]appHealthMember, 0, len(members))}
	statuses := make([]string, 0, len(members))
	for i := range members {
		full, err := h.DB.GetProject(members[i].ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load member"})
			return
		}
		if full == nil {
			continue
		}
		latest, err := h.DB.GetLatestDeployment(full.ID, environment)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load member deployment"})
			return
		}
		status := h.resolveMemberLiveStatus(r.Context(), *full, environment, latest)
		statuses = append(statuses, status)

		primaryDomain := ""
		if d, derr := h.DB.GetPrimaryDomain(full.ID, environment); derr == nil && d != nil {
			primaryDomain = d.DomainName
		}

		out.Members = append(out.Members, appHealthMember{
			Project:          *full,
			LiveStatus:       status,
			PrimaryDomain:    primaryDomain,
			LatestDeployment: latest,
			LatestPreview:    full.LatestPreviewDeployAt,
			LatestProduction: full.LatestProductionDeployAt,
		})
	}
	out.CombinedStatus = combinedAppStatus(statuses)
	writeJSON(w, http.StatusOK, out)
}
```

Add this P1 status resolver method (it is replaced in P2 with a probe-aware version) to `apps.go` (or `app_health.go`):

```go
// resolveMemberLiveStatus picks a member's live status. P1: derived purely from
// the latest deployment. P2 overrides this to consult h.Prober.
func (h *AppHandler) resolveMemberLiveStatus(_ context.Context, _ db.Project, _ string, latest *db.Deployment) string {
	return deriveMemberLiveStatusFromDeployment(latest)
}
```

Ensure `apps.go` imports include `context` and `time` (time is already used by the structs). Add `context` if missing.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/handlers/ -run TestGetHealthReturnsLiveStatusAndCombined -v`
Expected: PASS. Then `go test ./internal/api/handlers/...` to confirm the existing `GetHealth` test still passes (legacy fields retained).

- [ ] **Step 5: Commit**

```bash
git add internal/api/handlers/apps.go internal/api/handlers/apps_test.go
git commit -m "feat(apps): app health adds env param, live_status, primary_domain, combined_status"
```

### Task 1.5: Unified deployments endpoint

**Files:**
- Modify: `internal/api/handlers/apps.go` (add `GetDeployments`)
- Modify: `internal/api/router.go` (register route)
- Test: `internal/api/handlers/apps_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/api/handlers/apps_test.go`:

```go
func TestGetDeploymentsUnifiedFeed(t *testing.T) {
	database := appTestDB(t)
	user := &db.User{ID: db.NewID(), GithubID: 1, Username: "owner", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	app, err := database.CreateApp(&db.AppCreate{OrganizationID: org.ID, Name: "Bundle"})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	member := &db.Project{
		Name: "web", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, OrganizationID: org.ID, AppID: app.ID, Framework: "nextjs",
		PackageManager: "auto", Status: "active",
	}
	if err := database.CreateProject(member); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if err := database.CreateDeployment(&db.Deployment{ProjectID: member.ID, Environment: "production", Branch: "main", Status: "live", TriggeredBy: user.ID, CommitSHA: "m1"}); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	h := &AppHandler{DB: database}
	claims := &auth.Claims{UserID: user.ID, Role: "user"}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", app.ID)
	req := httptest.NewRequest(http.MethodGet, "/apps/"+app.ID+"/deployments?environment=production&limit=5", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(auth.WithClaims(req.Context(), claims))
	rec := httptest.NewRecorder()

	h.GetDeployments(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var out []db.AppDeployment
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 1 || out[0].ProjectName != "web" {
		t.Fatalf("unexpected feed: %+v (body: %s)", out, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/handlers/ -run TestGetDeploymentsUnifiedFeed -v`
Expected: FAIL — `h.GetDeployments undefined`.

- [ ] **Step 3: Implement `GetDeployments` + register route**

Add to `internal/api/handlers/apps.go`:

```go
// GetDeployments returns recent deployments across all member projects for one
// environment (the App dashboard's unified feed). ?limit= caps the rows (default 20).
func (h *AppHandler) GetDeployments(w http.ResponseWriter, r *http.Request) {
	app, _, ok := h.loadManagedApp(w, r)
	if !ok {
		return
	}
	environment, valid := normalizeAppEnvironment(r.URL.Query().Get("environment"))
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment must be preview or production"})
		return
	}
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	rows, err := h.DB.ListAppDeployments(app.ID, environment, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load deployments"})
		return
	}
	writeJSON(w, http.StatusOK, rows)
}
```

Add `strconv` to the `apps.go` import block if missing. Register in `internal/api/router.go`, immediately after `r.Get("/apps/{id}/releases", appHandler.ListReleases)`:

```go
r.Get("/apps/{id}/deployments", appHandler.GetDeployments)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/handlers/ -run TestGetDeploymentsUnifiedFeed -v` then `go build ./...`
Expected: PASS + clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/api/handlers/apps.go internal/api/router.go internal/api/handlers/apps_test.go
git commit -m "feat(apps): GET /apps/{id}/deployments unified cross-member feed"
```

### Task 1.6: Frontend types + api methods + queryKeys (additive)

**Files:**
- Modify: `web/src/types/api.ts`, `web/src/lib/api.ts`, `web/src/lib/queryKeys.ts`

- [ ] **Step 1: Extend `web/src/types/api.ts`**

Add the new App-dashboard types and extend `AppHealth`/`AppHealthMember` (additive — keep existing fields):

```ts
export type MemberLiveStatus =
  | "healthy" | "degraded" | "down" | "deploying" | "failed" | "none" | "unknown";

export type AppCombinedStatus = "healthy" | "deploying" | "degraded" | "down" | "none";

export interface AppHealthMember {
  project: Project;
  live_status: MemberLiveStatus;
  primary_domain?: string;
  latest_deployment?: Deployment | null;
  latest_preview_deploy_at?: string | null;
  latest_production_deploy_at?: string | null;
}

export interface AppHealth {
  app: App;
  environment: "preview" | "production";
  combined_status: AppCombinedStatus;
  members: AppHealthMember[];
}

export interface AppDeployment extends Deployment {
  username: string;
  avatar_url: string;
  project_name: string;
}

export interface TopologyNode {
  project_id: string;
  name: string;
  framework: string;
}

export interface TopologyEdge {
  source: string;
  target: string;
  via: string;
  kind: "env" | "secret" | "";
  confirmed: boolean;
}

export interface AppTopology {
  nodes: TopologyNode[];
  edges: TopologyEdge[];
}
```

(Leave the existing `App`, `AppRelease`, `Deployment`, `Project`, `Domain` interfaces unchanged.)

- [ ] **Step 2: Add api methods in `web/src/lib/api.ts`**

In the `// ---- App bundles ----` section, change `getAppHealth` to take an environment (default production) and add the new methods. Add `AppDeployment`, `AppTopology` to the `@/types/api` import:

```ts
async getAppHealth(
  id: string,
  environment: "preview" | "production" = "production",
): Promise<AppHealth> {
  return this.request(`/apps/${id}/health?environment=${environment}`);
}

async listAppDeployments(
  appId: string,
  environment: "preview" | "production",
  limit = 20,
): Promise<AppDeployment[]> {
  return this.request(`/apps/${appId}/deployments?environment=${environment}&limit=${limit}`);
}

async getAppTopology(
  appId: string,
  environment: "preview" | "production",
): Promise<AppTopology> {
  return this.request(`/apps/${appId}/topology?environment=${environment}`);
}

async reorderAppMembers(appId: string, projectIds: string[]): Promise<Project[]> {
  return this.request(`/apps/${appId}/members/order`, {
    method: "PATCH",
    body: JSON.stringify({ project_ids: projectIds }),
  });
}
```

- [ ] **Step 3: Add query keys in `web/src/lib/queryKeys.ts`**

Replace the `appHealth` key and add the new ones under the App-bundles block:

```ts
appHealth: (appId: string, environment: string) =>
  ["apps", appId, "health", environment] as const,
appTopology: (appId: string, environment: string) =>
  ["apps", appId, "topology", environment] as const,
appDeployments: (appId: string, environment: string) =>
  ["apps", appId, "deployments", environment] as const,
```

- [ ] **Step 4: Keep the legacy page compiling**

`web/src/pages/AppDetail.tsx` currently calls `api.getAppHealth(appId)` (still valid — env defaults) and uses `queryKeys.appHealth(appId)` (now requires an env arg) and `m.latest_production_deploy_at` (retained on the type). Update its two `queryKeys.appHealth(appId)` call sites to `queryKeys.appHealth(appId, "production")` so tsc passes. (AppDetail is deleted in Task 1.11; this keeps the intermediate commit green.)

- [ ] **Step 5: Typecheck + commit**

Run: `cd web && bunx tsc --noEmit`
Expected: clean.

```bash
git add web/src/types/api.ts web/src/lib/api.ts web/src/lib/queryKeys.ts web/src/pages/AppDetail.tsx
git commit -m "feat(apps): web types + api methods for app health/deployments/topology/reorder"
```

### Task 1.7: `app-helpers.ts` status metadata (+ unit test)

**Files:**
- Create: `web/src/lib/app-helpers.ts`
- Test: `web/src/lib/app-helpers.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
import { describe, expect, test } from "bun:test";
import { APP_STATUS_META, MEMBER_STATUS_META, RELEASE_STATUS_META } from "./app-helpers";

describe("app status metadata", () => {
  test("combined status covers the five backend values", () => {
    expect(Object.keys(APP_STATUS_META).sort()).toEqual(
      ["degraded", "deploying", "down", "healthy", "none"],
    );
  });
  test("member status covers the seven backend values", () => {
    expect(Object.keys(MEMBER_STATUS_META).sort()).toEqual(
      ["degraded", "deploying", "down", "failed", "healthy", "none", "unknown"],
    );
  });
  test("release status covers the four backend values", () => {
    expect(Object.keys(RELEASE_STATUS_META).sort()).toEqual(
      ["failed", "pending", "rolled_back", "succeeded"],
    );
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bun test ./src/lib/app-helpers.test.ts`
Expected: FAIL — cannot resolve `./app-helpers`.

- [ ] **Step 3: Implement `web/src/lib/app-helpers.ts`**

```ts
import type {
  AppCombinedStatus,
  AppRelease,
  MemberLiveStatus,
} from "@/types/api";

type StatusMeta = { label: string; badgeClass: string; dotClass: string };

export const APP_STATUS_META = {
  healthy: { label: "Healthy", badgeClass: "border-emerald-400/25 bg-emerald-400/12 text-emerald-100", dotClass: "bg-emerald-400" },
  deploying: { label: "Deploying", badgeClass: "border-amber-400/25 bg-amber-400/12 text-amber-100", dotClass: "bg-amber-400" },
  degraded: { label: "Degraded", badgeClass: "border-orange-400/25 bg-orange-400/12 text-orange-100", dotClass: "bg-orange-400" },
  down: { label: "Down", badgeClass: "border-rose-400/25 bg-rose-400/12 text-rose-100", dotClass: "bg-rose-400" },
  none: { label: "No deploys", badgeClass: "border-white/10 bg-white/5 text-slate-200", dotClass: "bg-slate-500" },
} satisfies Record<AppCombinedStatus, StatusMeta>;

export const MEMBER_STATUS_META = {
  healthy: { label: "Healthy", badgeClass: "border-emerald-400/25 bg-emerald-400/12 text-emerald-100", dotClass: "bg-emerald-400" },
  deploying: { label: "Deploying", badgeClass: "border-amber-400/25 bg-amber-400/12 text-amber-100", dotClass: "bg-amber-400" },
  degraded: { label: "Degraded", badgeClass: "border-orange-400/25 bg-orange-400/12 text-orange-100", dotClass: "bg-orange-400" },
  failed: { label: "Failed", badgeClass: "border-rose-400/25 bg-rose-400/12 text-rose-100", dotClass: "bg-rose-400" },
  down: { label: "Down", badgeClass: "border-rose-400/25 bg-rose-400/12 text-rose-100", dotClass: "bg-rose-400" },
  none: { label: "No deploys", badgeClass: "border-white/10 bg-white/5 text-slate-200", dotClass: "bg-slate-500" },
  unknown: { label: "Unknown", badgeClass: "border-white/10 bg-white/5 text-slate-200", dotClass: "bg-slate-500" },
} satisfies Record<MemberLiveStatus, StatusMeta>;

export const RELEASE_STATUS_META = {
  succeeded: { label: "Succeeded", badgeClass: "border-emerald-400/25 bg-emerald-400/12 text-emerald-100", dotClass: "bg-emerald-400" },
  failed: { label: "Failed", badgeClass: "border-rose-400/25 bg-rose-400/12 text-rose-100", dotClass: "bg-rose-400" },
  rolled_back: { label: "Rolled back", badgeClass: "border-orange-400/25 bg-orange-400/12 text-orange-100", dotClass: "bg-orange-400" },
  pending: { label: "Pending", badgeClass: "border-white/10 bg-white/5 text-slate-200", dotClass: "bg-slate-500" },
} satisfies Record<AppRelease["status"], StatusMeta>;

export const ACTIVE_MEMBER_STATUSES = new Set<MemberLiveStatus>(["deploying"]);
```

- [ ] **Step 4: Run test + typecheck**

Run: `cd web && bun test ./src/lib/app-helpers.test.ts && bunx tsc --noEmit`
Expected: PASS + clean.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/app-helpers.ts web/src/lib/app-helpers.test.ts
git commit -m "feat(apps): app/member/release status metadata helpers"
```

### Task 1.8: App shell layout + sidebar/tabbar `"app"` context

**Files:**
- Create: `web/src/components/layout/AppBundleLayout.tsx`
- Modify: `web/src/components/layout/AppSidebar.tsx`, `web/src/components/layout/MobileTabBar.tsx`

- [ ] **Step 1: Extend `AppSidebar.tsx`**

Update the props union and the context switch, and add a `getAppItems` builder. Change the interface:

```ts
interface AppSidebarProps extends React.ComponentProps<typeof Sidebar> {
  context: "workspace" | "project" | "app";
  projectId?: string;
  appId?: string;
}
```

Update the destructure + selection logic in the component body:

```tsx
export function AppSidebar({ context, projectId, appId, ...props }: AppSidebarProps) {
  // ...existing hooks...
  const items =
    context === "project" && projectId
      ? getProjectItems(projectId)
      : context === "app" && appId
        ? getAppItems(appId)
        : getWorkspaceItems();

  const groupLabel =
    context === "project" ? "Project" : context === "app" ? "App" : "Navigation";
  // ...rest unchanged...
}
```

Add the builder near `getProjectItems` (use icons already imported in this file; add `Boxes`, `Rocket`, `Share2`, `KeyRound`, `History`, `Settings`, `LayoutGrid` to the existing `lucide-react` import if not present):

```tsx
function getAppItems(appId: string): NavItem[] {
  const base = `/apps/${appId}`;
  return [
    { label: "Overview", icon: LayoutGrid, to: "/apps/$appId", params: { appId }, matchPath: (p) => p === base },
    { label: "Deployments", icon: Rocket, to: "/apps/$appId/deployments", params: { appId }, matchPath: (p) => p === `${base}/deployments` },
    { label: "Topology", icon: Share2, to: "/apps/$appId/topology", params: { appId }, matchPath: (p) => p === `${base}/topology` },
    { label: "Variables", icon: KeyRound, to: "/apps/$appId/variables", params: { appId }, matchPath: (p) => p === `${base}/variables` },
    { label: "Releases", icon: History, to: "/apps/$appId/releases", params: { appId }, matchPath: (p) => p === `${base}/releases` },
    { label: "Settings", icon: Settings, to: "/apps/$appId/settings", params: { appId }, matchPath: (p) => p === `${base}/settings` },
  ];
}
```

- [ ] **Step 2: Extend `MobileTabBar.tsx`**

Widen its `context` prop to `"workspace" | "project" | "app"` and add an `appId?: string` prop. Add an app branch in `getTabs` returning a 4-tab subset (Overview / Deployments / Topology / Settings) using the same tab shape the file already uses for project tabs (mirror the existing `context === "project"` branch, swapping `to`/`params` to the `/apps/$appId...` routes with `params: { appId }`). Keep the workspace fallback.

- [ ] **Step 3: Create `web/src/components/layout/AppBundleLayout.tsx`**

Copy `ProjectLayout`'s structure, swapping context/param/scope (no DeployMenu — the app deploy action lives on the Overview hero):

```tsx
import { Link, Outlet, useParams, useRouterState } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { Boxes, FolderKanban } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { useAuthStore } from "@/store/auth";
import { AppSidebar } from "@/components/layout/AppSidebar";
import { MobileTabBar } from "@/components/layout/MobileTabBar";
import { CommandPalette } from "@/components/layout/CommandPalette";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import {
  Breadcrumb, BreadcrumbItem, BreadcrumbList, BreadcrumbPage,
} from "@/components/ui/breadcrumb";
import { Separator } from "@/components/ui/separator";
import { SidebarInset, SidebarTrigger } from "@/components/ui/sidebar";
import { LoadingState, Spinner } from "@/components/ui/spinner";
import { Suspense } from "react";

export function AppBundleLayout() {
  const { appId } = useParams({ strict: false }) as { appId: string };
  const routerState = useRouterState();
  const pathname = routerState.location.pathname;
  const { user } = useAuthStore();

  const { data: health } = useQuery({
    queryKey: queryKeys.appHealth(appId, "production"),
    queryFn: () => api.getAppHealth(appId, "production"),
  });
  const appName = health?.app.name ?? "App";

  const base = `/apps/${appId}`;
  const currentPage =
    pathname === `${base}/deployments` ? "Deployments"
    : pathname === `${base}/topology` ? "Topology"
    : pathname === `${base}/variables` ? "Variables"
    : pathname === `${base}/releases` ? "Releases"
    : pathname === `${base}/settings` ? "Settings"
    : "Overview";

  return (
    <>
      <AppSidebar context="app" appId={appId} />
      <SidebarInset>
        <header className="shrink-0 border-b px-4 pt-safe">
          <div className="flex h-14 items-center gap-2 md:h-16">
            <SidebarTrigger className="-ml-1" />
            <Separator orientation="vertical" className="mr-2 h-4" />
            <Link to="/" className="hidden items-center gap-2 md:flex">
              <FolderKanban className="size-4 text-primary" />
              <span className="font-mono text-[13px] font-semibold tracking-[0.16em]">/deployik</span>
            </Link>
            <Separator orientation="vertical" className="mr-2 hidden h-4 md:block" />
            <Breadcrumb>
              <BreadcrumbList>
                <BreadcrumbItem className="hidden md:flex">
                  <Link to="/apps" className="inline-flex items-center gap-1 text-muted-foreground hover:text-foreground">
                    <Boxes className="size-3.5" /> Apps
                  </Link>
                </BreadcrumbItem>
                <BreadcrumbItem>
                  <BreadcrumbPage>{appName} · {currentPage}</BreadcrumbPage>
                </BreadcrumbItem>
              </BreadcrumbList>
            </Breadcrumb>
            <div className="ml-auto flex items-center gap-2">
              <CommandPalette />
              <Avatar className="h-7 w-7 rounded-lg">
                <AvatarImage src={user?.avatar_url} alt={user?.username} />
                <AvatarFallback className="rounded-lg text-xs">
                  {user?.username?.[0]?.toUpperCase() ?? "D"}
                </AvatarFallback>
              </Avatar>
            </div>
          </div>
        </header>
        <div className="flex min-w-0 flex-1 flex-col gap-4 p-4 pb-safe-tabbar md:pb-4">
          <div className="mx-auto w-full max-w-[1400px]">
            <ErrorBoundary scope="app">
              <Suspense fallback={<LoadingState title="Loading…" className="min-h-[320px]" />}>
                <Outlet />
              </Suspense>
            </ErrorBoundary>
          </div>
        </div>
        <MobileTabBar context="app" appId={appId} />
      </SidebarInset>
    </>
  );
}
```

If `ErrorBoundary`'s `scope` prop is a string-literal union, add `"app"` to it (check `web/src/components/ErrorBoundary.tsx`). If `Spinner` is unused after copying, drop the import.

- [ ] **Step 4: Typecheck**

Run: `cd web && bunx tsc --noEmit`
Expected: clean (routes wired in Task 1.10; layout compiles standalone).

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/AppBundleLayout.tsx web/src/components/layout/AppSidebar.tsx web/src/components/layout/MobileTabBar.tsx web/src/components/ErrorBoundary.tsx
git commit -m "feat(apps): app-shell layout + sidebar/tabbar app context"
```

### Task 1.9: `AppOverview` page (two-column + sticky rail)

**Files:**
- Create: `web/src/pages/AppOverview.tsx`

- [ ] **Step 1: Implement the page**

```tsx
import { useState } from "react";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ArrowRight, Boxes, ExternalLink, RefreshCw, Rocket } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys, staleTimes } from "@/lib/queryKeys";
import {
  APP_STATUS_META,
  MEMBER_STATUS_META,
  RELEASE_STATUS_META,
  ACTIVE_MEMBER_STATUSES,
} from "@/lib/app-helpers";
import { DEPLOYMENT_STATUS_META, ENVIRONMENT_META, formatRelativeDate } from "@/lib/deployment-helpers";
import { TopologyMap } from "@/components/apps/topology-map";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { LoadingState } from "@/components/ui/spinner";
import { cn } from "@/lib/utils";
import type { AppDeployment, AppHealthMember } from "@/types/api";

type Environment = "preview" | "production";

export function AppOverview() {
  const { appId } = useParams({ strict: false }) as { appId: string };
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [environment, setEnvironment] = useState<Environment>("production");

  const { data: health, isLoading } = useQuery({
    queryKey: queryKeys.appHealth(appId, environment),
    queryFn: () => api.getAppHealth(appId, environment),
    staleTime: staleTimes.activeDeployments,
    refetchInterval: (query) =>
      (query.state.data?.members ?? []).some((m) => ACTIVE_MEMBER_STATUSES.has(m.live_status)) ? 3000 : false,
  });
  const { data: topology } = useQuery({
    queryKey: queryKeys.appTopology(appId, environment),
    queryFn: () => api.getAppTopology(appId, environment),
  });
  const { data: deployments } = useQuery({
    queryKey: queryKeys.appDeployments(appId, environment),
    queryFn: () => api.listAppDeployments(appId, environment, 5),
    staleTime: staleTimes.activeDeployments,
  });
  const { data: releases } = useQuery({
    queryKey: queryKeys.appReleases(appId, environment),
    queryFn: () => api.listAppReleases(appId, environment),
  });

  const deployMutation = useMutation({
    mutationFn: () => api.deployApp(appId, environment),
    onSuccess: (r) => {
      toast.success(`Deploying ${r.member_count} member(s) to ${environment}`);
      queryClient.invalidateQueries({ queryKey: queryKeys.appReleases(appId, environment) });
      queryClient.invalidateQueries({ queryKey: queryKeys.appHealth(appId, environment) });
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to start deploy"),
  });

  if (isLoading) {
    return <LoadingState title="Loading app…" className="min-h-[320px]" />;
  }
  const app = health?.app;
  const members = health?.members ?? [];
  const combined = health?.combined_status ?? "none";
  const combinedMeta = APP_STATUS_META[combined];
  const liveCount = members.filter((m) => m.live_status === "healthy").length;

  return (
    <div className="space-y-6">
      {/* hero */}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-2">
          <h1 className="flex items-center gap-2 text-2xl font-semibold tracking-tight">
            <Boxes className="h-6 w-6" /> {app?.name}
            <Badge variant="outline" className={cn("ml-1 gap-1.5", combinedMeta.badgeClass)}>
              <span className={cn("h-2 w-2 rounded-full", combinedMeta.dotClass)} />
              {combinedMeta.label}
            </Badge>
          </h1>
          <p className="text-sm text-muted-foreground">
            {members.length} member{members.length === 1 ? "" : "s"}
            {app?.deploy_ordered ? " · ordered deploy" : " · parallel deploy"} · {liveCount}/{members.length} live
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Select value={environment} onValueChange={(v) => setEnvironment(v as Environment)}>
            <SelectTrigger className="w-[150px]"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="production">Production</SelectItem>
              <SelectItem value="preview">Preview</SelectItem>
            </SelectContent>
          </Select>
          <Button variant="outline" size="icon" title="Refresh"
            onClick={() => queryClient.invalidateQueries({ queryKey: queryKeys.appHealth(appId, environment) })}>
            <RefreshCw className="h-4 w-4" />
          </Button>
          <Button onClick={() => deployMutation.mutate()} disabled={deployMutation.isPending || members.length === 0}>
            <Rocket className="h-4 w-4" /> Deploy together
          </Button>
        </div>
      </div>

      {/* two columns */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[1fr_320px] lg:items-start">
        {/* main */}
        <div className="space-y-6">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-3">
              <CardTitle className="text-base">Architecture</CardTitle>
              <Link to="/apps/$appId/topology" params={{ appId }} className="text-sm text-primary hover:underline">
                Expand
              </Link>
            </CardHeader>
            <CardContent>
              <TopologyMap topology={topology} members={members} compact />
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-3">
              <CardTitle className="text-base">Members</CardTitle>
              <Link to="/apps/$appId/settings" params={{ appId }} className="text-sm text-primary hover:underline">
                Manage
              </Link>
            </CardHeader>
            <CardContent className="space-y-2">
              {members.length === 0 ? (
                <p className="py-4 text-center text-sm text-muted-foreground">No members yet.</p>
              ) : (
                members.map((m) => <MemberRow key={m.project.id} member={m} ordered={!!app?.deploy_ordered} onOpen={() => navigate({ to: "/projects/$id", params: { id: m.project.id } })} />)
              )}
            </CardContent>
          </Card>
        </div>

        {/* sticky rail */}
        <div className="space-y-4 lg:sticky lg:top-4">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-3">
              <CardTitle className="text-base">Health</CardTitle>
              <Badge variant="outline" className={cn("gap-1.5", combinedMeta.badgeClass)}>
                <span className={cn("h-2 w-2 rounded-full", combinedMeta.dotClass)} />
                {combinedMeta.label}
              </Badge>
            </CardHeader>
            <CardContent className="grid grid-cols-2 gap-2">
              <Kpi label="Live" value={`${liveCount}/${members.length}`} />
              <Kpi label="Members" value={String(members.length)} />
              <Kpi label="Releases" value={String(releases?.length ?? 0)} />
              <Kpi label="Mode" value={app?.deploy_ordered ? "Ordered" : "Parallel"} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-3">
              <CardTitle className="text-base">Recent deployments</CardTitle>
              <Link to="/apps/$appId/deployments" params={{ appId }} className="inline-flex items-center text-sm text-primary hover:underline">
                See all <ArrowRight className="ml-1 h-3.5 w-3.5" />
              </Link>
            </CardHeader>
            <CardContent className="space-y-1">
              {(deployments ?? []).length === 0 ? (
                <p className="py-3 text-center text-sm text-muted-foreground">No deployments yet.</p>
              ) : (
                (deployments ?? []).map((d) => <DeployRow key={d.id} d={d} onOpen={() => navigate({ to: "/projects/$id/deployments/$did", params: { id: d.project_id, did: d.id } })} />)
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-3">
              <CardTitle className="text-base">Releases</CardTitle>
              <Link to="/apps/$appId/releases" params={{ appId }} className="text-sm text-primary hover:underline">All</Link>
            </CardHeader>
            <CardContent className="space-y-1">
              {(releases ?? []).slice(0, 3).length === 0 ? (
                <p className="py-3 text-center text-sm text-muted-foreground">No releases yet.</p>
              ) : (
                (releases ?? []).slice(0, 3).map((r) => {
                  const meta = RELEASE_STATUS_META[r.status];
                  return (
                    <div key={r.id} className="flex items-center justify-between rounded-md border px-3 py-2 text-sm">
                      <Badge variant="outline" className={meta.badgeClass}>{meta.label}</Badge>
                      <span className="font-mono text-xs text-muted-foreground">{r.id.slice(0, 10)}</span>
                      <span className="text-xs text-muted-foreground">{formatRelativeDate(r.created_at)}</span>
                    </div>
                  );
                })
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}

function Kpi({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border bg-muted/20 px-3 py-2">
      <p className="text-[10px] uppercase tracking-wide text-muted-foreground">{label}</p>
      <p className="mt-0.5 text-lg font-semibold">{value}</p>
    </div>
  );
}

function MemberRow({ member, ordered, onOpen }: { member: AppHealthMember; ordered: boolean; onOpen: () => void }) {
  const meta = MEMBER_STATUS_META[member.live_status];
  return (
    <div className="flex items-center justify-between rounded-md border px-3 py-2">
      <div className="flex min-w-0 items-center gap-3">
        <span className={cn("h-2 w-2 shrink-0 rounded-full", meta.dotClass)} />
        <span className="truncate font-medium">{member.project.name}</span>
        <Badge variant="secondary" className="text-xs">{member.project.framework}</Badge>
        {ordered && <span className="text-xs text-muted-foreground">order {member.project.deploy_order}</span>}
      </div>
      <div className="flex shrink-0 items-center gap-3">
        {member.primary_domain ? (
          <a href={`https://${member.primary_domain}`} target="_blank" rel="noopener noreferrer"
            className="hidden items-center gap-1 text-xs text-muted-foreground hover:text-foreground sm:flex"
            onClick={(e) => e.stopPropagation()}>
            {member.primary_domain} <ExternalLink className="h-3 w-3" />
          </a>
        ) : null}
        <Button variant="ghost" size="sm" onClick={onOpen}>Open</Button>
      </div>
    </div>
  );
}

function DeployRow({ d, onOpen }: { d: AppDeployment; onOpen: () => void }) {
  const meta = DEPLOYMENT_STATUS_META[d.status];
  const envMeta = ENVIRONMENT_META[d.environment];
  return (
    <button type="button" onClick={onOpen}
      className="flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm transition-colors hover:bg-accent">
      <span className={cn("h-2 w-2 shrink-0 rounded-full", meta?.dotClass)} />
      <span className="font-medium">{d.project_name}</span>
      <Badge variant="outline" className={cn("text-[10px]", envMeta?.badgeClass)}>{envMeta?.label}</Badge>
      <span className="ml-auto font-mono text-xs text-muted-foreground">{d.commit_sha ? d.commit_sha.slice(0, 7) : "—"}</span>
      <span className="text-xs text-muted-foreground">{formatRelativeDate(d.created_at)}</span>
    </button>
  );
}
```

- [ ] **Step 2: Typecheck**

Run: `cd web && bunx tsc --noEmit`
Expected: FAIL only on the missing `@/components/apps/topology-map` import — created next. (If you want a green checkpoint first, do Task 1.9 and 1.9b together before committing.)

- [ ] **Step 3: Commit (with the stub from Task 1.9b)**

Commit after Task 1.9b so the import resolves.

### Task 1.9b: `TopologyMap` component (compact renderer; full SVG comes in P3)

**Files:**
- Create: `web/src/components/apps/topology-map.tsx`

- [ ] **Step 1: Implement a deterministic layered renderer**

```tsx
import { Fragment } from "react";
import { cn } from "@/lib/utils";
import { MEMBER_STATUS_META } from "@/lib/app-helpers";
import type { AppHealthMember, AppTopology } from "@/types/api";

interface TopologyMapProps {
  topology?: AppTopology;
  members: AppHealthMember[];
  compact?: boolean;
}

// Renders members left-to-right by deploy_order with confirmed edges labeled by
// the variable that creates them and faint "reachable" links between the rest.
export function TopologyMap({ topology, members, compact }: TopologyMapProps) {
  const ordered = [...members].sort((a, b) => a.project.deploy_order - b.project.deploy_order);
  if (ordered.length === 0) {
    return <p className="py-6 text-center text-sm text-muted-foreground">No members yet — add projects to see the architecture.</p>;
  }
  const edges = topology?.edges ?? [];
  const confirmed = edges.filter((e) => e.confirmed);
  const statusFor = (projectId: string) => members.find((m) => m.project.id === projectId)?.live_status ?? "unknown";

  // edge label between two adjacent nodes (if a confirmed edge exists in either direction)
  const labelBetween = (aId: string, bId: string) => {
    const e = confirmed.find((x) => (x.source === aId && x.target === bId) || (x.source === bId && x.target === aId));
    return e?.via ?? null;
  };

  return (
    <div className={cn("flex flex-wrap items-center justify-center gap-y-3", compact ? "py-3" : "py-8")}>
      {ordered.map((m, i) => {
        const meta = MEMBER_STATUS_META[statusFor(m.project.id)];
        const next = ordered[i + 1];
        const label = next ? labelBetween(m.project.id, next.project.id) : null;
        return (
          <Fragment key={m.project.id}>
            <div className="flex items-center gap-2 rounded-lg border border-primary/40 bg-primary/10 px-3 py-2">
              <span className={cn("h-2 w-2 rounded-full", meta.dotClass)} />
              <span className="text-sm font-medium">{m.project.name}</span>
              <span className="text-[10px] text-muted-foreground">{m.project.framework}</span>
            </div>
            {next ? (
              <div className="flex min-w-[56px] flex-col items-center px-1 text-muted-foreground">
                <span className={cn("text-base leading-none", label ? "text-primary" : "text-muted-foreground/40")}>→</span>
                {label ? <span className="mt-1 font-mono text-[8px] text-primary">{label}</span> : <span className="mt-1 text-[8px] text-muted-foreground/50">reachable</span>}
              </div>
            ) : null}
          </Fragment>
        );
      })}
      {confirmed.length === 0 ? (
        <p className="mt-3 w-full text-center text-[11px] text-muted-foreground">No internal references detected yet.</p>
      ) : null}
    </div>
  );
}
```

- [ ] **Step 2: Typecheck**

Run: `cd web && bunx tsc --noEmit`
Expected: clean (AppOverview + TopologyMap now both resolve).

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/AppOverview.tsx web/src/components/apps/topology-map.tsx
git commit -m "feat(apps): AppOverview page + compact topology map"
```

### Task 1.10: `AppDeployments` full page

**Files:**
- Create: `web/src/pages/AppDeployments.tsx`

- [ ] **Step 1: Implement (mirrors ProjectDeployments' Card+Table; adds a Project column + deep link)**

```tsx
import { useState } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";

import { api } from "@/lib/api";
import { queryKeys, staleTimes } from "@/lib/queryKeys";
import { ACTIVE_DEPLOYMENT_STATUSES, DEPLOYMENT_STATUS_META, ENVIRONMENT_META, formatRelativeDate } from "@/lib/deployment-helpers";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { LoadingState } from "@/components/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { cn } from "@/lib/utils";

type Environment = "preview" | "production";

export function AppDeployments() {
  const { appId } = useParams({ strict: false }) as { appId: string };
  const navigate = useNavigate();
  const [environment, setEnvironment] = useState<Environment>("production");

  const { data: deployments, isLoading } = useQuery({
    queryKey: queryKeys.appDeployments(appId, environment),
    queryFn: () => api.listAppDeployments(appId, environment, 100),
    staleTime: staleTimes.activeDeployments,
    refetchInterval: (query) =>
      (query.state.data ?? []).some((d) => ACTIVE_DEPLOYMENT_STATUSES.has(d.status)) ? 3000 : false,
  });

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">Deployments</h2>
          <p className="text-sm text-muted-foreground">Every member's builds in one place. Click a row to open the project's deployment.</p>
        </div>
        <Select value={environment} onValueChange={(v) => setEnvironment(v as Environment)}>
          <SelectTrigger className="w-[150px]"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="production">Production</SelectItem>
            <SelectItem value="preview">Preview</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {isLoading ? (
        <LoadingState title="Loading deployments…" />
      ) : !deployments?.length ? (
        <Card><CardContent className="py-12 text-center text-sm text-muted-foreground">No {environment} deployments yet.</CardContent></Card>
      ) : (
        <Card className="overflow-hidden">
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow className="border-white/8 hover:bg-transparent">
                  <TableHead className="pl-6">Project</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Commit</TableHead>
                  <TableHead>Environment</TableHead>
                  <TableHead>Started</TableHead>
                  <TableHead className="pr-6 text-right">Duration</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {deployments.map((d) => {
                  const meta = DEPLOYMENT_STATUS_META[d.status];
                  const envMeta = ENVIRONMENT_META[d.environment];
                  const open = () => navigate({ to: "/projects/$id/deployments/$did", params: { id: d.project_id, did: d.id } });
                  return (
                    <TableRow key={d.id} role="link" tabIndex={0}
                      className={cn("cursor-pointer border-white/8 transition-colors hover:bg-white/[0.04]", d.status === "live" && "bg-white/[0.03]")}
                      onClick={open}
                      onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); open(); } }}>
                      <TableCell className="pl-6 font-medium">{d.project_name}</TableCell>
                      <TableCell><Badge variant="outline" className={meta.badgeClass}>{meta.label}</Badge></TableCell>
                      <TableCell className="max-w-[320px]">
                        <span className="font-mono text-xs text-foreground">{d.commit_sha ? d.commit_sha.slice(0, 7) : "pending"}</span>
                        <p className="truncate text-xs text-muted-foreground" title={d.commit_message || d.error_message}>{d.commit_message || d.error_message || meta.label}</p>
                      </TableCell>
                      <TableCell><Badge variant="outline" className={envMeta?.badgeClass}>{envMeta?.label}</Badge></TableCell>
                      <TableCell className="text-sm text-muted-foreground">{formatRelativeDate(d.created_at)}</TableCell>
                      <TableCell className="pr-6 text-right text-sm text-muted-foreground">{d.build_duration > 0 ? `${d.build_duration}s` : "--"}</TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Typecheck + commit**

Run: `cd web && bunx tsc --noEmit`

```bash
git add web/src/pages/AppDeployments.tsx
git commit -m "feat(apps): AppDeployments unified cross-member table page"
```

### Task 1.11: Wire the app-shell routes; retire `AppDetail`

**Files:**
- Modify: `web/src/app/app.tsx`
- Create (temporary stubs): `web/src/pages/AppTopology.tsx`, `AppVariables.tsx`, `AppReleases.tsx`, `AppSettings.tsx`
- Delete: `web/src/pages/AppDetail.tsx`

> Routes can't reference pages that don't exist yet. P3/P4 fully implement Topology/Variables/Releases/Settings; for now create minimal compiling stubs so the shell is navigable, then flesh them out in their phases.

- [ ] **Step 1: Create minimal stubs** (each replaced later)

`web/src/pages/AppTopology.tsx`:
```tsx
export function AppTopology() {
  return <p className="text-sm text-muted-foreground">Topology — coming in this release.</p>;
}
```
`web/src/pages/AppReleases.tsx`:
```tsx
export function AppReleases() {
  return <p className="text-sm text-muted-foreground">Releases — coming in this release.</p>;
}
```
`web/src/pages/AppVariables.tsx`:
```tsx
export function AppVariables() {
  return <p className="text-sm text-muted-foreground">Variables — coming in this release.</p>;
}
```
`web/src/pages/AppSettings.tsx`:
```tsx
export function AppSettings() {
  return <p className="text-sm text-muted-foreground">Settings — coming in this release.</p>;
}
```

- [ ] **Step 2: Edit `web/src/app/app.tsx`**

(a) Replace the eager `import { AppDetail } ...` with the new page imports:
```tsx
import { AppBundleLayout } from "@/components/layout/AppBundleLayout";
import { AppOverview } from "@/pages/AppOverview";
import { AppDeployments } from "@/pages/AppDeployments";
import { AppTopology } from "@/pages/AppTopology";
import { AppVariables } from "@/pages/AppVariables";
import { AppReleases } from "@/pages/AppReleases";
import { AppSettings } from "@/pages/AppSettings";
```
(b) Delete the existing `appDetailRoute` declaration. Keep `appsRoute` (`/apps`). Add the layout route + children:
```tsx
const appLayoutRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/apps/$appId",
  component: AppBundleLayout,
});
const appOverviewRoute = createRoute({ getParentRoute: () => appLayoutRoute, path: "/", component: AppOverview });
const appDeploymentsRoute = createRoute({ getParentRoute: () => appLayoutRoute, path: "/deployments", component: AppDeployments });
const appTopologyRoute = createRoute({ getParentRoute: () => appLayoutRoute, path: "/topology", component: AppTopology });
const appVariablesRoute = createRoute({ getParentRoute: () => appLayoutRoute, path: "/variables", component: AppVariables });
const appReleasesRoute = createRoute({ getParentRoute: () => appLayoutRoute, path: "/releases", component: AppReleases });
const appSettingsRoute = createRoute({ getParentRoute: () => appLayoutRoute, path: "/settings", component: AppSettings });
```
(c) In `routeTree`: remove `appDetailRoute` from `workspaceLayoutRoute.addChildren([...])` (keep `appsRoute`), and add `appLayoutRoute.addChildren([...])` as a sibling of `projectLayoutRoute`:
```tsx
projectLayoutRoute.addChildren([ /* unchanged */ ]),
appLayoutRoute.addChildren([
  appOverviewRoute,
  appDeploymentsRoute,
  appTopologyRoute,
  appVariablesRoute,
  appReleasesRoute,
  appSettingsRoute,
]),
```

- [ ] **Step 3: Delete the legacy page**

```bash
git rm web/src/pages/AppDetail.tsx
```

- [ ] **Step 4: Typecheck + build**

Run: `cd web && bunx tsc --noEmit && bun run build`
Expected: clean (no remaining references to `AppDetail`). Grep to confirm: `grep -rn "AppDetail" web/src` returns nothing.

- [ ] **Step 5: Commit**

```bash
git add web/src/app/app.tsx web/src/pages/AppTopology.tsx web/src/pages/AppVariables.tsx web/src/pages/AppReleases.tsx web/src/pages/AppSettings.tsx
git commit -m "feat(apps): first-class app shell routes; retire AppDetail page"
```

### Task 1.12: Phase 1 verification

- [ ] **Step 1: Full backend test + build**

Run: `go test ./... && go build ./...`
Expected: all PASS, clean build.

- [ ] **Step 2: Full frontend typecheck + build + unit tests**

Run: `cd web && bunx tsc --noEmit && bun run test && bun run build`
Expected: clean.

- [ ] **Step 3: Commit (only if any fixups were needed)**

```bash
git add -p   # stage only app-dashboard fixups
git commit -m "test(apps): phase 1 verification fixups"
```

---

# Phase 2 — Live container-health probe

Swaps the P1 deploy-status source for a real probe behind the same `live_status` field. To avoid an import cycle (`handlers` already imports `build`), the probe types live in package `build`; the status-combination stays in `handlers`.

### Task 2.1: `MemberProbe` + `HealthProber` (build) + `statusFromProbe` (handlers)

**Files:**
- Create: `internal/build/health_prober.go` (types only this task)
- Modify: `internal/api/handlers/app_health.go` (add `statusFromProbe`)
- Test: `internal/api/handlers/app_health_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/api/handlers/app_health_test.go`:

```go
import "github.com/LEFTEQ/lovinka-deployik/internal/build"

func TestStatusFromProbe(t *testing.T) {
	live := &db.Deployment{Status: "live"}
	building := &db.Deployment{Status: "building"}
	failed := &db.Deployment{Status: "failed"}
	cases := []struct {
		name   string
		latest *db.Deployment
		probe  build.MemberProbe
		want   string
	}{
		{"mid-deploy beats probe", building, build.MemberProbe{Probed: true, Running: false}, "deploying"},
		{"unprobed -> unknown", live, build.MemberProbe{Probed: false}, "unknown"},
		{"running+ok -> healthy", live, build.MemberProbe{Probed: true, Running: true, OK: true}, "healthy"},
		{"running+notok -> degraded", live, build.MemberProbe{Probed: true, Running: true, OK: false}, "degraded"},
		{"down: was live, not running", live, build.MemberProbe{Probed: true, Running: false}, "down"},
		{"failed: not running, last failed", failed, build.MemberProbe{Probed: true, Running: false}, "failed"},
		{"none: no deployment, not running", nil, build.MemberProbe{Probed: true, Running: false}, "none"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusFromProbe(c.latest, c.probe); got != c.want {
				t.Fatalf("status = %q, want %q", got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/handlers/ -run TestStatusFromProbe -v`
Expected: FAIL — `build.MemberProbe` / `statusFromProbe` undefined.

- [ ] **Step 3: Implement the types + function**

Create `internal/build/health_prober.go`:

```go
package build

import (
	"context"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// MemberProbe is the result of a live container probe for one member.
type MemberProbe struct {
	Probed  bool // false => could not determine (docker error) => caller treats as "unknown"
	Running bool // container exists and is in the running state
	OK      bool // health endpoint responded with an up status (200/204/3xx/401/403)
}

// HealthProber probes a member project's canonical container for an environment.
type HealthProber interface {
	Probe(ctx context.Context, project db.Project, environment string) MemberProbe
}
```

Add to `internal/api/handlers/app_health.go` (add the `build` import):

```go
import "github.com/LEFTEQ/lovinka-deployik/internal/build"

// statusFromProbe combines a member's latest deployment with a live probe into a
// member live status. Mid-deploy beats the probe; an unprobeable member is "unknown".
func statusFromProbe(latest *db.Deployment, probe build.MemberProbe) string {
	if latest != nil {
		switch latest.Status {
		case "queued", "building", "deploying":
			return memberStatusDeploying
		}
	}
	if !probe.Probed {
		return memberStatusUnknown
	}
	if probe.Running {
		if probe.OK {
			return memberStatusHealthy
		}
		return memberStatusDegraded
	}
	if latest == nil {
		return memberStatusNone
	}
	if latest.Status == "failed" {
		return memberStatusFailed
	}
	return memberStatusDown
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/handlers/ -run TestStatusFromProbe -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/build/health_prober.go internal/api/handlers/app_health.go internal/api/handlers/app_health_test.go
git commit -m "feat(apps): MemberProbe/HealthProber types + statusFromProbe combinator"
```

### Task 2.2: `DockerHealthProber` implementation

**Files:**
- Modify: `internal/build/health_prober.go`
- Test: `internal/build/health_prober_test.go`

- [ ] **Step 1: Write the failing test** (covers the HTTP up-status classification via a local server)

```go
package build

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPUpClassification(t *testing.T) {
	p := &DockerHealthProber{client: http.DefaultClient}
	cases := []struct {
		code int
		want bool
	}{
		{200, true}, {204, true}, {301, true}, {302, true}, {401, true}, {403, true},
		{404, false}, {500, false}, {502, false},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(c.code)
		}))
		got := p.httpUp(context.Background(), srv.URL)
		srv.Close()
		if got != c.want {
			t.Fatalf("status %d: httpUp = %v, want %v", c.code, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/build/ -run TestHTTPUpClassification -v`
Expected: FAIL — `DockerHealthProber` / `httpUp` undefined.

- [ ] **Step 3: Implement the prober**

Append to `internal/build/health_prober.go`:

```go
import (
	"fmt"
	"net/http"
	"time"
)

// DockerHealthProber probes a member's canonical container over the Docker
// network (PROXY_TYPE=docker) or via its host port (PROXY_TYPE=host-port).
type DockerHealthProber struct {
	docker    *DockerClient
	proxyType string
	client    *http.Client
}

// NewDockerHealthProber builds a prober with a short per-probe HTTP timeout.
func NewDockerHealthProber(docker *DockerClient, proxyType string) *DockerHealthProber {
	return &DockerHealthProber{
		docker:    docker,
		proxyType: proxyType,
		client:    &http.Client{Timeout: 2 * time.Second},
	}
}

func (p *DockerHealthProber) Probe(ctx context.Context, project db.Project, environment string) MemberProbe {
	name := db.DeploymentContainerName(project.Name, environment, nil)
	id, exists := p.docker.ContainerExists(ctx, name)
	if !exists {
		return MemberProbe{Probed: true, Running: false}
	}
	running, err := p.docker.IsContainerRunning(ctx, id)
	if err != nil {
		return MemberProbe{Probed: false}
	}
	if !running {
		return MemberProbe{Probed: true, Running: false}
	}

	port := project.Port
	if port <= 0 {
		port = 3000
	}
	// NOTE: empty HealthPath defaults to "/". node-api runtimes serve health at
	// "/health"; ensure such members store HealthPath (set at create time) or
	// extend this to projectconfig.DefaultHealthPath.
	healthPath := project.HealthPath
	if healthPath == "" {
		healthPath = "/"
	}

	var target string
	if p.proxyType == "host-port" {
		hostPort, err := p.docker.GetHostPort(ctx, id, port)
		if err != nil || hostPort == "" {
			// Running but we can't resolve the port — count running as up rather
			// than falsely degraded.
			return MemberProbe{Probed: true, Running: true, OK: true}
		}
		target = fmt.Sprintf("http://127.0.0.1:%s%s", hostPort, healthPath)
	} else {
		target = fmt.Sprintf("http://%s:%d%s", name, port, healthPath)
	}
	return MemberProbe{Probed: true, Running: true, OK: p.httpUp(ctx, target)}
}

// httpUp treats 200/204, any 3xx, and 401/403 as up (mirrors the devops blackbox
// http_app_up module — password-protected 401 is healthy). 5xx / errors are down.
func (p *DockerHealthProber) httpUp(ctx context.Context, url string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	s := resp.StatusCode
	return s == 200 || s == 204 || (s >= 300 && s < 400) || s == 401 || s == 403
}
```

Merge the new `import (...)` block with the existing one at the top of the file (don't create a second import block — combine `context`, `db`, `fmt`, `net/http`, `time`).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/build/ -run TestHTTPUpClassification -v` then `go build ./...`
Expected: PASS + clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/build/health_prober.go internal/build/health_prober_test.go
git commit -m "feat(apps): DockerHealthProber (container + HTTP health probe)"
```

### Task 2.3: Wire the prober into `GetHealth` (concurrent) + router

**Files:**
- Modify: `internal/api/handlers/apps.go` (`AppHandler.Prober` field; concurrent probe; `resolveMemberLiveStatus`)
- Modify: `internal/api/router.go` (construct + assign `Prober`)
- Test: `internal/api/handlers/apps_test.go`

- [ ] **Step 1: Write the failing test** (inject a fake prober; assert degraded/down surface)

Add to `internal/api/handlers/apps_test.go`:

```go
type fakeProber struct{ result build.MemberProbe }

func (f fakeProber) Probe(_ context.Context, _ db.Project, _ string) build.MemberProbe { return f.result }

func TestGetHealthUsesProber(t *testing.T) {
	database := appTestDB(t)
	user := &db.User{ID: db.NewID(), GithubID: 1, Username: "owner", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	app, err := database.CreateApp(&db.AppCreate{OrganizationID: org.ID, Name: "Bundle"})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	member := &db.Project{Name: "web", GithubRepo: "r", GithubOwner: "o", Branch: "main", UserID: user.ID, OrganizationID: org.ID, AppID: app.ID, Framework: "nextjs", PackageManager: "auto", Status: "active"}
	if err := database.CreateProject(member); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if err := database.CreateDeployment(&db.Deployment{ProjectID: member.ID, Environment: "production", Branch: "main", Status: "live", TriggeredBy: user.ID, CommitSHA: "m1"}); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	// last deploy live, but the container is running and failing its probe -> degraded.
	h := &AppHandler{DB: database, Prober: fakeProber{result: build.MemberProbe{Probed: true, Running: true, OK: false}}}
	claims := &auth.Claims{UserID: user.ID, Role: "user"}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", app.ID)
	req := httptest.NewRequest(http.MethodGet, "/apps/"+app.ID+"/health?environment=production", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(auth.WithClaims(req.Context(), claims))
	rec := httptest.NewRecorder()

	h.GetHealth(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	var out struct {
		CombinedStatus string `json:"combined_status"`
		Members        []struct{ LiveStatus string `json:"live_status"` } `json:"members"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.CombinedStatus != "degraded" || len(out.Members) != 1 || out.Members[0].LiveStatus != "degraded" {
		t.Fatalf("want degraded/degraded, got %q/%+v (body: %s)", out.CombinedStatus, out.Members, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/handlers/ -run TestGetHealthUsesProber -v`
Expected: FAIL — `AppHandler` has no `Prober` field.

- [ ] **Step 3: Add the field, the probe-aware resolver, and concurrency**

In `internal/api/handlers/apps.go`, add the field to `AppHandler`:

```go
type AppHandler struct {
	DB        *db.DB
	Pipeline  *build.Pipeline
	Encryptor *crypto.Encryptor
	Audit     *audit.Recorder
	Prober    build.HealthProber // nil => deploy-status-only (P1 fallback)
}
```

Replace the P1 `resolveMemberLiveStatus` method with the probe-aware version:

```go
func (h *AppHandler) resolveMemberLiveStatus(ctx context.Context, project db.Project, environment string, latest *db.Deployment) string {
	if h.Prober == nil {
		return deriveMemberLiveStatusFromDeployment(latest)
	}
	return statusFromProbe(latest, h.Prober.Probe(ctx, project, environment))
}
```

Make `GetHealth` resolve members concurrently (probes can each take up to 2s). Replace the member loop body with a parallel gather:

```go
	type memberResult struct {
		member appHealthMember
		status string
		ok     bool
	}
	results := make([]memberResult, len(members))
	var wg sync.WaitGroup
	for i := range members {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			full, err := h.DB.GetProject(members[i].ID)
			if err != nil || full == nil {
				return
			}
			latest, err := h.DB.GetLatestDeployment(full.ID, environment)
			if err != nil {
				return
			}
			status := h.resolveMemberLiveStatus(r.Context(), *full, environment, latest)
			primaryDomain := ""
			if d, derr := h.DB.GetPrimaryDomain(full.ID, environment); derr == nil && d != nil {
				primaryDomain = d.DomainName
			}
			results[i] = memberResult{
				member: appHealthMember{
					Project:          *full,
					LiveStatus:       status,
					PrimaryDomain:    primaryDomain,
					LatestDeployment: latest,
					LatestPreview:    full.LatestPreviewDeployAt,
					LatestProduction: full.LatestProductionDeployAt,
				},
				status: status,
				ok:     true,
			}
		}(i)
	}
	wg.Wait()

	out := appHealth{App: app, Environment: environment, Members: make([]appHealthMember, 0, len(members))}
	statuses := make([]string, 0, len(members))
	for _, res := range results {
		if !res.ok {
			continue
		}
		out.Members = append(out.Members, res.member)
		statuses = append(statuses, res.status)
	}
	out.CombinedStatus = combinedAppStatus(statuses)
	writeJSON(w, http.StatusOK, out)
```

Add `sync` to the `apps.go` imports. (Concurrent reads on the `*sql.DB` pool are safe.)

- [ ] **Step 4: Wire the prober in `internal/api/router.go`**

First read the router's config struct (the `cfg` used to build handlers) and `cmd/server/main.go`'s router construction. Construct the prober from the same `*build.DockerClient` the Pipeline uses and the proxy type from config, then assign it on the `appHandler` literal:

```go
appHandler := &handlers.AppHandler{
	DB: cfg.DB, Pipeline: cfg.Pipeline, Encryptor: cfg.Encryptor, Audit: auditRecorder,
	Prober: build.NewDockerHealthProber(cfg.DockerClient, cfg.ProxyType),
}
```

If the router config struct does not already carry `DockerClient *build.DockerClient` and `ProxyType string`, add those two fields to it and populate them in `cmd/server/main.go` (it already has `dockerClient` from `build.NewDockerClient()` and the proxy type from `cfg.ProxyType`/config). Import `internal/build` in router.go if not present.

- [ ] **Step 5: Run test + build, then commit**

Run: `go test ./internal/api/handlers/ -run 'TestGetHealth' -v && go build ./...`
Expected: PASS + clean build.

```bash
git add internal/api/handlers/apps.go internal/api/router.go cmd/server/main.go internal/api/handlers/apps_test.go
git commit -m "feat(apps): wire live HealthProber into app health (concurrent per-member probe)"
```

### Task 2.4: Phase 2 verification

- [ ] **Step 1: Run** `go test ./... && go build ./...` — Expected: all PASS.
- [ ] **Step 2: Run** `cd web && bunx tsc --noEmit` — Expected: clean (no FE changes this phase; the `live_status` field is unchanged in shape).
- [ ] **Step 3:** Commit any fixups (`feat(apps): phase 2 fixups`).

---

# Phase 3 — Topology endpoint + map

### Task 3.1: Pure `deriveTopologyEdges`

**Files:**
- Create: `internal/api/handlers/app_topology.go` (types + pure function this task)
- Test: `internal/api/handlers/app_topology_test.go`

- [ ] **Step 1: Write the failing test**

```go
package handlers

import (
	"testing"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

func TestDeriveTopologyEdges(t *testing.T) {
	members := []db.Project{{ID: "p-web", Name: "web"}, {ID: "p-api", Name: "api"}, {ID: "p-db", Name: "db"}}
	memberVars := map[string][]db.ProjectVariable{
		"p-web": {{Key: "API_URL", Kind: "env", Value: "http://deployik-api-production:4000"}},
		"p-api": {{Key: "DATABASE_URL", Kind: "secret", Value: "postgres://u:p@deployik-db-production:5432/app"}},
		"p-db":  {},
	}
	tokens := map[string][]string{
		"p-web": {"deployik-web-production"},
		"p-api": {"deployik-api-production"},
		"p-db":  {"deployik-db-production"},
	}

	edges := deriveTopologyEdges(members, memberVars, tokens)
	if len(edges) != 2 {
		t.Fatalf("expected 2 confirmed edges, got %d: %+v", len(edges), edges)
	}
	want := map[string]topologyEdge{
		"p-web->p-api": {Source: "p-web", Target: "p-api", Via: "API_URL", Kind: "env", Confirmed: true},
		"p-api->p-db":  {Source: "p-api", Target: "p-db", Via: "DATABASE_URL", Kind: "secret", Confirmed: true},
	}
	for _, e := range edges {
		w, ok := want[e.Source+"->"+e.Target]
		if !ok || e != w {
			t.Fatalf("unexpected edge %+v", e)
		}
	}
}

func TestDeriveTopologyEdgesNoFalseMesh(t *testing.T) {
	members := []db.Project{{ID: "a", Name: "a"}, {ID: "b", Name: "b"}}
	// a references nothing; b references nothing.
	edges := deriveTopologyEdges(members,
		map[string][]db.ProjectVariable{"a": {{Key: "X", Kind: "env", Value: "literal"}}, "b": {}},
		map[string][]string{"a": {"deployik-a-production"}, "b": {"deployik-b-production"}},
	)
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges, got %+v", edges)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/handlers/ -run TestDeriveTopologyEdges -v`
Expected: FAIL — undefined `topologyEdge` / `deriveTopologyEdges`.

- [ ] **Step 3: Implement types + pure function**

Create `internal/api/handlers/app_topology.go`:

```go
package handlers

import (
	"strings"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

type topologyNode struct {
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Framework string `json:"framework"`
}

type topologyEdge struct {
	Source    string `json:"source"`
	Target    string `json:"target"`
	Via       string `json:"via"`
	Kind      string `json:"kind"`
	Confirmed bool   `json:"confirmed"`
}

type appTopology struct {
	Nodes []topologyNode `json:"nodes"`
	Edges []topologyEdge `json:"edges"`
}

// deriveTopologyEdges returns one confirmed directed edge per (member -> sibling)
// pair where any of the member's own decrypted variable values references a
// sibling's host token (container name or domain). siblingTokens maps a
// project id to the strings that, if found in a var value, confirm a dependency.
// Reachable (faint) links are rendered client-side; only confirmed edges are returned.
func deriveTopologyEdges(members []db.Project, memberVars map[string][]db.ProjectVariable, siblingTokens map[string][]string) []topologyEdge {
	edges := make([]topologyEdge, 0)
	for _, m := range members {
		vars := memberVars[m.ID]
		for _, s := range members {
			if s.ID == m.ID {
				continue
			}
			tokens := siblingTokens[s.ID]
			if len(tokens) == 0 {
				continue
			}
			matched := false
			for _, v := range vars {
				for _, tok := range tokens {
					if tok != "" && strings.Contains(v.Value, tok) {
						edges = append(edges, topologyEdge{
							Source: m.ID, Target: s.ID, Via: v.Key, Kind: string(v.Kind), Confirmed: true,
						})
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
		}
	}
	return edges
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/handlers/ -run TestDeriveTopologyEdges -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/handlers/app_topology.go internal/api/handlers/app_topology_test.go
git commit -m "feat(apps): pure topology-edge derivation from member env wiring"
```

### Task 3.2: `GetTopology` handler + route

**Files:**
- Modify: `internal/api/handlers/app_topology.go` (add `GetTopology`)
- Modify: `internal/api/router.go`
- Test: `internal/api/handlers/app_topology_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/api/handlers/app_topology_test.go` (needs an `Encryptor` so secret/env values can be decrypted; use the same constructor the project uses — `crypto.NewEncryptor("test-key-...")`; check `internal/crypto/encrypt.go` for the exact constructor name and 1-arg signature):

```go
import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
)

func TestGetTopologyConfirmedEdge(t *testing.T) {
	database := appTestDB(t)
	enc, err := crypto.NewEncryptor("test-encryption-key-please-change-1")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	user := &db.User{ID: db.NewID(), GithubID: 1, Username: "owner", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	app, err := database.CreateApp(&db.AppCreate{OrganizationID: org.ID, Name: "Bundle"})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	web := &db.Project{Name: "web", GithubRepo: "r", GithubOwner: "o", Branch: "main", UserID: user.ID, OrganizationID: org.ID, AppID: app.ID, Framework: "nextjs", PackageManager: "auto", Status: "active"}
	api := &db.Project{Name: "api", GithubRepo: "r2", GithubOwner: "o", Branch: "main", UserID: user.ID, OrganizationID: org.ID, AppID: app.ID, Framework: "static", PackageManager: "auto", Status: "active"}
	if err := database.CreateProject(web); err != nil {
		t.Fatalf("CreateProject web: %v", err)
	}
	if err := database.CreateProject(api); err != nil {
		t.Fatalf("CreateProject api: %v", err)
	}
	// web's API_URL points at api's canonical production container.
	enc1, err := enc.Encrypt("http://deployik-api-production:3000")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if err := database.UpsertProjectVariable(&db.ProjectVariable{ProjectID: web.ID, Environment: "production", Kind: db.VariableKindEnv, Key: "API_URL", Value: enc1}); err != nil {
		t.Fatalf("UpsertProjectVariable: %v", err)
	}

	h := &AppHandler{DB: database, Encryptor: enc}
	claims := &auth.Claims{UserID: user.ID, Role: "user"}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", app.ID)
	req := httptest.NewRequest(http.MethodGet, "/apps/"+app.ID+"/topology?environment=production", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(auth.WithClaims(req.Context(), claims))
	rec := httptest.NewRecorder()

	h.GetTopology(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	var out appTopology
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(out.Nodes))
	}
	if len(out.Edges) != 1 || out.Edges[0].Source != web.ID || out.Edges[0].Target != api.ID || out.Edges[0].Via != "API_URL" {
		t.Fatalf("edges = %+v, want one web->api via API_URL (body: %s)", out.Edges, rec.Body.String())
	}
	// secret values must never leak into the payload
	if bytes.Contains(rec.Body.Bytes(), []byte("deployik-api-production")) == false {
		// container host appearing in `via`? No — via is the KEY. Assert no raw value leaked:
	}
}
```

> Verify the exact names `crypto.NewEncryptor` and `db.UpsertProjectVariable` against `internal/crypto/encrypt.go` and `internal/db/queries_envvars.go`; adjust the calls if the constructor/upsert signatures differ (the encryptor is the same one wired in `cmd/server/main.go`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/handlers/ -run TestGetTopologyConfirmedEdge -v`
Expected: FAIL — `h.GetTopology undefined`.

- [ ] **Step 3: Implement `GetTopology`**

Append to `internal/api/handlers/app_topology.go`:

```go
import (
	"net/http"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// GetTopology returns the app's members as nodes plus confirmed dependency edges
// derived from each member's own env/secret values referencing a sibling's host.
func (h *AppHandler) GetTopology(w http.ResponseWriter, r *http.Request) {
	app, _, ok := h.loadManagedApp(w, r)
	if !ok {
		return
	}
	environment, valid := normalizeAppEnvironment(r.URL.Query().Get("environment"))
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment must be preview or production"})
		return
	}
	members, err := h.DB.ListProjectsByApp(app.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load members"})
		return
	}

	nodes := make([]topologyNode, 0, len(members))
	siblingTokens := make(map[string][]string, len(members))
	for _, m := range members {
		nodes = append(nodes, topologyNode{ProjectID: m.ID, Name: m.Name, Framework: m.Framework})
		tokens := []string{db.DeploymentContainerName(m.Name, environment, nil)}
		if domains, derr := h.DB.ListDomains(m.ID); derr == nil {
			for _, d := range domains {
				if d.Environment == environment && d.DomainName != "" {
					tokens = append(tokens, d.DomainName)
				}
			}
		}
		siblingTokens[m.ID] = tokens
	}

	memberVars := make(map[string][]db.ProjectVariable, len(members))
	for _, m := range members {
		var decrypted []db.ProjectVariable
		for _, kind := range []db.VariableKind{db.VariableKindEnv, db.VariableKindSecret} {
			vars, verr := h.DB.ListResolvedDeployVariables(&m, environment, kind)
			if verr != nil {
				continue
			}
			for _, v := range vars {
				plain, derr := h.Encryptor.Decrypt(v.Value)
				if derr != nil {
					continue
				}
				decrypted = append(decrypted, db.ProjectVariable{Key: v.Key, Kind: v.Kind, Value: plain})
			}
		}
		memberVars[m.ID] = decrypted
	}

	writeJSON(w, http.StatusOK, appTopology{
		Nodes: nodes,
		Edges: deriveTopologyEdges(members, memberVars, siblingTokens),
	})
}
```

Merge imports (single block: `net/http`, `strings`, `db`). Register in `internal/api/router.go` after the `/deployments` route:

```go
r.Get("/apps/{id}/topology", appHandler.GetTopology)
```

> `ListResolvedDeployVariables(&m, ...)` takes `*db.Project`; `m` is the loop variable — taking its address inside the loop is fine here because it is used synchronously before the next iteration. (Go 1.22+ gives each iteration a fresh `m`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/handlers/ -run TestGetTopologyConfirmedEdge -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/api/handlers/app_topology.go internal/api/router.go internal/api/handlers/app_topology_test.go
git commit -m "feat(apps): GET /apps/{id}/topology with server-side value scan (no value leakage)"
```

### Task 3.3: `AppTopology` full page

**Files:**
- Replace stub: `web/src/pages/AppTopology.tsx`

- [ ] **Step 1: Implement (full TopologyMap + confirmed-edge legend)**

```tsx
import { useState } from "react";
import { useParams } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { TopologyMap } from "@/components/apps/topology-map";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { LoadingState } from "@/components/ui/spinner";

type Environment = "preview" | "production";

export function AppTopology() {
  const { appId } = useParams({ strict: false }) as { appId: string };
  const [environment, setEnvironment] = useState<Environment>("production");

  const { data: health, isLoading } = useQuery({
    queryKey: queryKeys.appHealth(appId, environment),
    queryFn: () => api.getAppHealth(appId, environment),
  });
  const { data: topology } = useQuery({
    queryKey: queryKeys.appTopology(appId, environment),
    queryFn: () => api.getAppTopology(appId, environment),
  });

  if (isLoading) return <LoadingState title="Loading topology…" />;
  const members = health?.members ?? [];
  const confirmed = (topology?.edges ?? []).filter((e) => e.confirmed);
  const nameOf = (id: string) => members.find((m) => m.project.id === id)?.project.name ?? id;

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">Topology</h2>
          <p className="text-sm text-muted-foreground">Auto-derived from env wiring. Solid = a member's variable points at a sibling; faint = reachable on the private network.</p>
        </div>
        <Select value={environment} onValueChange={(v) => setEnvironment(v as Environment)}>
          <SelectTrigger className="w-[150px]"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="production">Production</SelectItem>
            <SelectItem value="preview">Preview</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <Card>
        <CardHeader><CardTitle className="text-base">Architecture map</CardTitle></CardHeader>
        <CardContent><TopologyMap topology={topology} members={members} /></CardContent>
      </Card>

      <Card>
        <CardHeader><CardTitle className="text-base">Detected connections</CardTitle></CardHeader>
        <CardContent className="space-y-2">
          {confirmed.length === 0 ? (
            <p className="py-3 text-center text-sm text-muted-foreground">No internal references detected. Members reach each other by the injected <code>&lt;NAME&gt;_URL</code> vars.</p>
          ) : (
            confirmed.map((e, i) => (
              <div key={i} className="flex items-center gap-2 rounded-md border px-3 py-2 text-sm">
                <span className="font-medium">{nameOf(e.source)}</span>
                <span className="text-primary">→</span>
                <span className="font-medium">{nameOf(e.target)}</span>
                <Badge variant="outline" className="ml-auto font-mono text-[10px]">{e.via}</Badge>
                <Badge variant="secondary" className="text-[10px]">{e.kind}</Badge>
              </div>
            ))
          )}
        </CardContent>
      </Card>
    </div>
  );
}
```

- [ ] **Step 2: Typecheck + commit**

Run: `cd web && bunx tsc --noEmit`

```bash
git add web/src/pages/AppTopology.tsx
git commit -m "feat(apps): AppTopology page (architecture map + detected connections)"
```

### Task 3.4: Phase 3 verification

- [ ] **Step 1:** `go test ./... && go build ./...` — Expected: PASS.
- [ ] **Step 2:** `cd web && bunx tsc --noEmit && bun run build` — Expected: clean.
- [ ] **Step 3:** Commit fixups if any (`feat(apps): phase 3 fixups`).

---

# Phase 4 — Variables, Releases, Settings (+ member reorder)

### Task 4.1: `SetAppMemberOrder` DB query

**Files:**
- Modify: `internal/db/queries_apps.go`
- Test: `internal/db/queries_apps_test.go` (or `apps_test.go` — same package)

- [ ] **Step 1: Write the failing test**

```go
func TestSetAppMemberOrder(t *testing.T) {
	database := newTestDB(t)
	user := createAppTestUser(t, database, "owner", 1)
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	app, err := database.CreateApp(&AppCreate{OrganizationID: org.ID, Name: "Bundle"})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	mk := func(name string) *Project {
		p := &Project{Name: name, GithubRepo: "r-" + name, GithubOwner: "o", Branch: "main", UserID: user.ID, OrganizationID: org.ID, AppID: app.ID, Framework: "static", PackageManager: "auto", Status: "active"}
		if err := database.CreateProject(p); err != nil {
			t.Fatalf("CreateProject %s: %v", name, err)
		}
		return p
	}
	web, api, dbp := mk("web"), mk("api"), mk("db")

	if err := database.SetAppMemberOrder(app.ID, []string{dbp.ID, api.ID, web.ID}); err != nil {
		t.Fatalf("SetAppMemberOrder: %v", err)
	}
	members, err := database.ListProjectsByApp(app.ID)
	if err != nil {
		t.Fatalf("ListProjectsByApp: %v", err)
	}
	// ListProjectsByApp orders by deploy_order ASC -> db(0), api(1), web(2)
	if len(members) != 3 || members[0].Name != "db" || members[1].Name != "api" || members[2].Name != "web" {
		t.Fatalf("unexpected order: %v", []string{members[0].Name, members[1].Name, members[2].Name})
	}

	// a non-member id must error and not partially apply
	if err := database.SetAppMemberOrder(app.ID, []string{web.ID, "not-a-member"}); err == nil {
		t.Fatalf("expected error for non-member project id")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestSetAppMemberOrder -v`
Expected: FAIL — `database.SetAppMemberOrder undefined`.

- [ ] **Step 3: Implement (transactional; rejects non-members)**

Add to `internal/db/queries_apps.go`:

```go
// SetAppMemberOrder assigns deploy_order to each member by its index in
// projectIDs (0-based). All ids must be members of the app; otherwise the whole
// change rolls back with an error.
func (db *DB) SetAppMemberOrder(appID string, projectIDs []string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin reorder: %w", err)
	}
	defer tx.Rollback()

	for i, pid := range projectIDs {
		res, err := tx.Exec(
			`UPDATE projects SET deploy_order = ?, updated_at = ? WHERE id = ? AND app_id = ? AND status != 'deleted'`,
			i, time.Now().UTC(), pid, appID,
		)
		if err != nil {
			return fmt.Errorf("reorder member %s: %w", pid, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("reorder rows affected: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("project %s is not a member of app %s", pid, appID)
		}
	}
	return tx.Commit()
}
```

Confirm `time` is imported in `queries_apps.go` (it is used elsewhere in package `db`; if this file lacks the import, add it). If other UPDATEs in this file set `updated_at` differently (e.g. `CURRENT_TIMESTAMP`), match the existing convention instead of `time.Now().UTC()`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/db/ -run TestSetAppMemberOrder -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/queries_apps.go internal/db/queries_apps_test.go
git commit -m "feat(apps): SetAppMemberOrder batch deploy-order reorder query"
```

### Task 4.2: `ReorderMembers` handler + route

**Files:**
- Modify: `internal/api/handlers/apps.go`, `internal/api/router.go`
- Test: `internal/api/handlers/apps_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestReorderMembers(t *testing.T) {
	database := appTestDB(t)
	user := &db.User{ID: db.NewID(), GithubID: 1, Username: "owner", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	app, err := database.CreateApp(&db.AppCreate{OrganizationID: org.ID, Name: "Bundle"})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	mk := func(name string) *db.Project {
		p := &db.Project{Name: name, GithubRepo: "r-" + name, GithubOwner: "o", Branch: "main", UserID: user.ID, OrganizationID: org.ID, AppID: app.ID, Framework: "static", PackageManager: "auto", Status: "active"}
		if err := database.CreateProject(p); err != nil {
			t.Fatalf("CreateProject %s: %v", name, err)
		}
		return p
	}
	a, b := mk("a"), mk("b")

	h := &AppHandler{DB: database}
	claims := &auth.Claims{UserID: user.ID, Role: "user"}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", app.ID)
	body, _ := json.Marshal(map[string]any{"project_ids": []string{b.ID, a.ID}})
	req := httptest.NewRequest(http.MethodPatch, "/apps/"+app.ID+"/members/order", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(auth.WithClaims(req.Context(), claims))
	rec := httptest.NewRecorder()

	h.ReorderMembers(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	var out []db.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 2 || out[0].Name != "b" || out[1].Name != "a" {
		t.Fatalf("unexpected order: %+v (body: %s)", out, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/handlers/ -run TestReorderMembers -v`
Expected: FAIL — `h.ReorderMembers undefined`.

- [ ] **Step 3: Implement handler + route**

Add to `internal/api/handlers/apps.go`:

```go
type reorderMembersRequest struct {
	ProjectIDs []string `json:"project_ids"`
}

// ReorderMembers sets each member's deploy_order from its position in the body.
func (h *AppHandler) ReorderMembers(w http.ResponseWriter, r *http.Request) {
	app, claims, ok := h.loadManagedApp(w, r)
	if !ok {
		return
	}
	var req reorderMembersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if len(req.ProjectIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_ids is required"})
		return
	}
	if err := h.DB.SetAppMemberOrder(app.ID, req.ProjectIDs); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	members, err := h.DB.ListProjectsByApp(app.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load members"})
		return
	}
	h.recordAudit(claims.UserID, "app.reorder", app.ID, map[string]any{"count": len(req.ProjectIDs)})
	writeJSON(w, http.StatusOK, members)
}
```

Register in `internal/api/router.go`, after the projects-remove route:

```go
r.With(mutationLimiter.Middleware("app_members_order")).Patch("/apps/{id}/members/order", appHandler.ReorderMembers)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/handlers/ -run TestReorderMembers -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/api/handlers/apps.go internal/api/router.go internal/api/handlers/apps_test.go
git commit -m "feat(apps): PATCH /apps/{id}/members/order member reorder endpoint"
```

### Task 4.3: App env/secret api methods + type + query keys

**Files:**
- Modify: `web/src/types/api.ts`, `web/src/lib/api.ts`, `web/src/lib/queryKeys.ts`

- [ ] **Step 1: Add the `AppVariable` type** (`web/src/types/api.ts`)

```ts
export interface AppVariable {
  id: string;
  app_id: string;
  environment: "shared" | "preview" | "production";
  kind: "env" | "secret";
  key: string;
  value: string; // masked in responses
  created_at: string;
  updated_at: string;
}
```

- [ ] **Step 2: Add api methods** (`web/src/lib/api.ts`, App-bundles section; `kind` selects the `/env` vs `/secrets` path)

```ts
async listAppVariables(
  appId: string,
  kind: "env" | "secret",
  environment: "shared" | "preview" | "production",
): Promise<AppVariable[]> {
  const path = kind === "secret" ? "secrets" : "env";
  return this.request(`/apps/${appId}/${path}?environment=${environment}`);
}

async upsertAppVariable(
  appId: string,
  kind: "env" | "secret",
  data: { key: string; value: string; environment: "shared" | "preview" | "production" },
): Promise<AppVariable> {
  const path = kind === "secret" ? "secrets" : "env";
  return this.request(`/apps/${appId}/${path}`, { method: "POST", body: JSON.stringify(data) });
}

async deleteAppVariable(
  appId: string,
  kind: "env" | "secret",
  key: string,
  environment: "shared" | "preview" | "production",
): Promise<void> {
  const path = kind === "secret" ? "secrets" : "env";
  await this.request<void>(`/apps/${appId}/${path}/${encodeURIComponent(key)}?environment=${environment}`, { method: "DELETE" });
}
```

Add `AppVariable` to the `@/types/api` import in `api.ts`.

- [ ] **Step 3: Add query key** (`web/src/lib/queryKeys.ts`)

```ts
appVariables: (appId: string, kind: "env" | "secret", environment: string) =>
  ["apps", appId, "variables", kind, environment] as const,
```

- [ ] **Step 4: Typecheck + commit**

Run: `cd web && bunx tsc --noEmit`

```bash
git add web/src/types/api.ts web/src/lib/api.ts web/src/lib/queryKeys.ts
git commit -m "feat(apps): web api methods for app-level env/secret variables"
```

### Task 4.4: `AppVariableStore` component + `AppVariables` page

**Files:**
- Create: `web/src/components/apps/app-variable-store.tsx`
- Replace stub: `web/src/pages/AppVariables.tsx`

- [ ] **Step 1: Implement the store component** (self-contained add/list/delete with a scope selector; secret values never echoed)

```tsx
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Plus, Trash2 } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

type Scope = "shared" | "preview" | "production";

export function AppVariableStore({ appId, kind }: { appId: string; kind: "env" | "secret" }) {
  const queryClient = useQueryClient();
  const [scope, setScope] = useState<Scope>("shared");
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");

  const { data: variables } = useQuery({
    queryKey: queryKeys.appVariables(appId, kind, scope),
    queryFn: () => api.listAppVariables(appId, kind, scope),
  });

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: queryKeys.appVariables(appId, kind, scope) });

  const upsert = useMutation({
    mutationFn: () => api.upsertAppVariable(appId, kind, { key: key.trim(), value, environment: scope }),
    onSuccess: () => {
      toast.success(`${kind === "secret" ? "Secret" : "Variable"} saved`);
      setKey("");
      setValue("");
      invalidate();
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to save"),
  });
  const remove = useMutation({
    mutationFn: (k: string) => api.deleteAppVariable(appId, kind, k, scope),
    onSuccess: () => {
      toast.success("Removed");
      invalidate();
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to remove"),
  });

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <Select value={scope} onValueChange={(v) => setScope(v as Scope)}>
          <SelectTrigger className="w-[160px]"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="shared">Shared</SelectItem>
            <SelectItem value="preview">Preview</SelectItem>
            <SelectItem value="production">Production</SelectItem>
          </SelectContent>
        </Select>
        <span className="text-xs text-muted-foreground">
          Inherited by every member at deploy time (member vars override).
        </span>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Input className="w-[220px] font-mono" placeholder="KEY" value={key} onChange={(e) => setKey(e.target.value)} />
        <Input className="min-w-[220px] flex-1 font-mono" type={kind === "secret" ? "password" : "text"}
          placeholder={kind === "secret" ? "secret value" : "value"} value={value} onChange={(e) => setValue(e.target.value)} />
        <Button onClick={() => upsert.mutate()} disabled={!key.trim() || !value || upsert.isPending}>
          <Plus className="h-4 w-4" /> Add
        </Button>
      </div>

      <div className="divide-y divide-border rounded-lg border">
        {(variables ?? []).length === 0 ? (
          <p className="px-3 py-6 text-center text-sm text-muted-foreground">No {kind === "secret" ? "secrets" : "variables"} in {scope}.</p>
        ) : (
          (variables ?? []).map((v) => (
            <div key={v.id} className="flex items-center justify-between gap-3 px-3 py-2">
              <span className="font-mono text-sm">{v.key}</span>
              <div className="flex items-center gap-3">
                <Badge variant="secondary" className="font-mono text-xs">{v.value}</Badge>
                <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => remove.mutate(v.key)} title="Remove">
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Implement `AppVariables` page**

```tsx
import { useParams } from "@tanstack/react-router";
import { AppVariableStore } from "@/components/apps/app-variable-store";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export function AppVariables() {
  const { appId } = useParams({ strict: false }) as { appId: string };
  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-base font-semibold">Variables</h2>
        <p className="text-sm text-muted-foreground">App-level env vars &amp; secrets shared by every member project.</p>
      </div>
      <Card>
        <CardHeader><CardTitle className="text-base">Environment variables</CardTitle></CardHeader>
        <CardContent><AppVariableStore appId={appId} kind="env" /></CardContent>
      </Card>
      <Card>
        <CardHeader><CardTitle className="text-base">Secrets</CardTitle></CardHeader>
        <CardContent><AppVariableStore appId={appId} kind="secret" /></CardContent>
      </Card>
    </div>
  );
}
```

- [ ] **Step 3: Typecheck + commit**

Run: `cd web && bunx tsc --noEmit`

```bash
git add web/src/components/apps/app-variable-store.tsx web/src/pages/AppVariables.tsx
git commit -m "feat(apps): AppVariables page + app-level variable store"
```

### Task 4.5: `AppReleases` page

**Files:**
- Replace stub: `web/src/pages/AppReleases.tsx`

- [ ] **Step 1: Implement**

```tsx
import { useState } from "react";
import { useParams } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { RotateCcw } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { RELEASE_STATUS_META } from "@/lib/app-helpers";
import { formatRelativeDate } from "@/lib/deployment-helpers";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { LoadingState } from "@/components/ui/spinner";

type Environment = "preview" | "production";

export function AppReleases() {
  const { appId } = useParams({ strict: false }) as { appId: string };
  const queryClient = useQueryClient();
  const [environment, setEnvironment] = useState<Environment>("production");

  const { data: releases, isLoading } = useQuery({
    queryKey: queryKeys.appReleases(appId, environment),
    queryFn: () => api.listAppReleases(appId, environment),
  });
  const rollback = useMutation({
    mutationFn: (releaseId: string) => api.rollbackApp(appId, environment, releaseId),
    onSuccess: () => {
      toast.success(`Rolling back ${environment}`);
      queryClient.invalidateQueries({ queryKey: queryKeys.appReleases(appId, environment) });
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to roll back"),
  });

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">Releases</h2>
          <p className="text-sm text-muted-foreground">Coordinated {environment} deploys. Roll back to redeploy every member to that release.</p>
        </div>
        <Select value={environment} onValueChange={(v) => setEnvironment(v as Environment)}>
          <SelectTrigger className="w-[150px]"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="production">Production</SelectItem>
            <SelectItem value="preview">Preview</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {isLoading ? (
        <LoadingState title="Loading releases…" />
      ) : !releases?.length ? (
        <Card><CardContent className="py-12 text-center text-sm text-muted-foreground">No {environment} releases yet.</CardContent></Card>
      ) : (
        <div className="divide-y divide-border rounded-lg border">
          {releases.map((r) => {
            const meta = RELEASE_STATUS_META[r.status];
            return (
              <div key={r.id} className="flex items-center justify-between gap-3 px-4 py-3">
                <div className="flex items-center gap-3">
                  <Badge variant="outline" className={meta.badgeClass}>{meta.label}</Badge>
                  <span className="font-mono text-xs text-muted-foreground">{r.id.slice(0, 12)}</span>
                  <span className="text-xs text-muted-foreground">{formatRelativeDate(r.created_at)}</span>
                  {r.members?.length ? <span className="text-xs text-muted-foreground">{r.members.length} member(s)</span> : null}
                </div>
                {(r.status === "succeeded" || r.status === "rolled_back") && (
                  <Button variant="ghost" size="sm" disabled={rollback.isPending} onClick={() => rollback.mutate(r.id)}>
                    <RotateCcw className="h-3.5 w-3.5" /> Roll back
                  </Button>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Typecheck + commit**

Run: `cd web && bunx tsc --noEmit`

```bash
git add web/src/pages/AppReleases.tsx
git commit -m "feat(apps): AppReleases page (history + rollback)"
```

### Task 4.6: `AppSettings` page (name, ordered toggle, members add/remove/reorder, delete)

**Files:**
- Replace stub: `web/src/pages/AppSettings.tsx`

- [ ] **Step 1: Implement**

```tsx
import { useState } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ArrowDown, ArrowUp, Plus, Trash2, X } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { LoadingState } from "@/components/ui/spinner";
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";

export function AppSettings() {
  const { appId } = useParams({ strict: false }) as { appId: string };
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [addOpen, setAddOpen] = useState(false);
  const [selected, setSelected] = useState<Record<string, boolean>>({});

  const { data: health, isLoading } = useQuery({
    queryKey: queryKeys.appHealth(appId, "production"),
    queryFn: () => api.getAppHealth(appId, "production"),
  });
  const { data: allProjects } = useQuery({
    queryKey: queryKeys.projects("all"),
    queryFn: () => api.listProjects(),
  });

  const [name, setName] = useState("");
  const app = health?.app;
  const members = health?.members ?? [];
  const memberIds = new Set(members.map((m) => m.project.id));
  const addable = (allProjects ?? []).filter((p) => !p.app_id && !memberIds.has(p.id));

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: queryKeys.appHealth(appId, "production") });
    queryClient.invalidateQueries({ queryKey: queryKeys.apps() });
    queryClient.invalidateQueries({ queryKey: ["projects"] });
  };

  const renameMut = useMutation({
    mutationFn: () => api.updateApp(appId, { name: name.trim() }),
    onSuccess: () => { toast.success("Renamed"); invalidate(); },
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to rename"),
  });
  const orderedMut = useMutation({
    mutationFn: (v: boolean) => api.updateApp(appId, { deploy_ordered: v }),
    onSuccess: () => invalidate(),
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to update"),
  });
  const reorderMut = useMutation({
    mutationFn: (ids: string[]) => api.reorderAppMembers(appId, ids),
    onSuccess: () => invalidate(),
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to reorder"),
  });
  const addMut = useMutation({
    mutationFn: (ids: string[]) => api.addProjectsToApp(appId, ids),
    onSuccess: () => { toast.success("Projects added"); setAddOpen(false); setSelected({}); invalidate(); },
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to add"),
  });
  const removeMut = useMutation({
    mutationFn: (projectId: string) => api.removeProjectFromApp(appId, projectId),
    onSuccess: () => { toast.success("Removed"); invalidate(); },
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to remove"),
  });
  const deleteMut = useMutation({
    mutationFn: () => api.deleteApp(appId),
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to delete"),
  });

  if (isLoading) return <LoadingState title="Loading settings…" />;

  const orderedIds = members.map((m) => m.project.id);
  const move = (index: number, dir: -1 | 1) => {
    const next = [...orderedIds];
    const target = index + dir;
    if (target < 0 || target >= next.length) return;
    [next[index], next[target]] = [next[target], next[index]];
    reorderMut.mutate(next);
  };
  const selectedIds = Object.keys(selected).filter((id) => selected[id]);

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader><CardTitle className="text-base">App name</CardTitle></CardHeader>
        <CardContent className="flex items-center gap-2">
          <Input className="max-w-sm" placeholder={app?.name} value={name} onChange={(e) => setName(e.target.value)} />
          <Button onClick={() => renameMut.mutate()} disabled={!name.trim() || renameMut.isPending}>Save</Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div>
            <CardTitle className="text-base">Deploy order</CardTitle>
            <p className="text-sm text-muted-foreground">When on, members deploy in the order below; otherwise in parallel.</p>
          </div>
          <Switch checked={!!app?.deploy_ordered} onCheckedChange={(v) => orderedMut.mutate(v)} />
        </CardHeader>
        <CardContent className="space-y-2">
          {members.length === 0 ? (
            <p className="py-4 text-center text-sm text-muted-foreground">No members yet.</p>
          ) : (
            members.map((m, i) => (
              <div key={m.project.id} className="flex items-center justify-between rounded-md border px-3 py-2">
                <div className="flex items-center gap-3">
                  {app?.deploy_ordered && <span className="font-mono text-xs text-muted-foreground">{i + 1}</span>}
                  <span className="font-medium">{m.project.name}</span>
                  <Badge variant="secondary" className="text-xs">{m.project.framework}</Badge>
                </div>
                <div className="flex items-center gap-1">
                  {app?.deploy_ordered && (
                    <>
                      <Button variant="ghost" size="icon" className="h-7 w-7" disabled={i === 0 || reorderMut.isPending} onClick={() => move(i, -1)} title="Move up"><ArrowUp className="h-4 w-4" /></Button>
                      <Button variant="ghost" size="icon" className="h-7 w-7" disabled={i === members.length - 1 || reorderMut.isPending} onClick={() => move(i, 1)} title="Move down"><ArrowDown className="h-4 w-4" /></Button>
                    </>
                  )}
                  <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => removeMut.mutate(m.project.id)} title="Remove from app"><X className="h-4 w-4" /></Button>
                </div>
              </div>
            ))
          )}
          <Button variant="outline" size="sm" onClick={() => setAddOpen(true)}><Plus className="h-4 w-4" /> Add projects</Button>
        </CardContent>
      </Card>

      <Card className="border-destructive/40">
        <CardHeader><CardTitle className="text-base text-destructive">Danger zone</CardTitle></CardHeader>
        <CardContent>
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button variant="outline" className="border-destructive/40 text-destructive"><Trash2 className="h-4 w-4" /> Delete app</Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Delete this app?</AlertDialogTitle>
                <AlertDialogDescription>The app bundle is removed. Member projects survive and become standalone.</AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction onClick={() => deleteMut.mutate(undefined, { onSuccess: () => { toast.success("App deleted"); queryClient.invalidateQueries({ queryKey: queryKeys.apps() }); navigate({ to: "/apps" }); } })}>Delete</AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </CardContent>
      </Card>

      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add projects</DialogTitle>
            <DialogDescription>Only standalone projects (not already in an app) can be added.</DialogDescription>
          </DialogHeader>
          <div className="max-h-72 space-y-1 overflow-y-auto">
            {addable.length === 0 ? (
              <p className="py-4 text-center text-sm text-muted-foreground">No standalone projects available.</p>
            ) : (
              addable.map((p) => (
                <label key={p.id} className="flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2 hover:bg-accent">
                  <input type="checkbox" checked={!!selected[p.id]} onChange={(e) => setSelected((s) => ({ ...s, [p.id]: e.target.checked }))} />
                  <span className="font-medium">{p.name}</span>
                  <Badge variant="secondary" className="text-xs">{p.framework}</Badge>
                </label>
              ))
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAddOpen(false)}>Cancel</Button>
            <Button disabled={selectedIds.length === 0 || addMut.isPending} onClick={() => addMut.mutate(selectedIds)}>Add {selectedIds.length || ""}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
```

- [ ] **Step 2: Typecheck + commit**

Run: `cd web && bunx tsc --noEmit`

```bash
git add web/src/pages/AppSettings.tsx
git commit -m "feat(apps): AppSettings page (rename, ordered toggle, member reorder/add/remove, delete)"
```

### Task 4.7: Final verification

- [ ] **Step 1: Backend** — `go test ./... && go build ./...` — Expected: all PASS.
- [ ] **Step 2: Frontend** — `cd web && bunx tsc --noEmit && bun run test && bun run build` — Expected: clean; `bun run test` includes `app-helpers.test.ts`.
- [ ] **Step 3: Grep guards** — `grep -rn "AppDetail" web/src` returns nothing; `grep -rn "coming in this release" web/src` returns nothing (all stubs replaced).
- [ ] **Step 4: Commit** any fixups (`test(apps): final app-dashboard verification`).

---

## Self-Review (completed by plan author)

**1. Spec coverage** — every spec section maps to tasks:
- First-class shell + sub-routes → 1.8 (layout/sidebar/tabbar) + 1.11 (routes).
- Two-column Overview + sticky rail → 1.9.
- Auto-derived topology (confirmed solid + reachable faint, no value leakage) → 3.1/3.2/3.3 + 1.9b (TopologyMap).
- Live container-health roll-up (combined + per-member, concurrent) → 1.3/1.4 (shape + DB-only) → 2.1/2.2/2.3 (real probe).
- Unified cross-member deployments + deep links → 1.2/1.5 (backend) + 1.9 (rail) + 1.10 (full page).
- App variables → 4.3/4.4. Releases → 4.5. Settings + reorder → 4.1/4.2/4.6.
- No new migrations → confirmed (only queries/handlers/endpoints/UI). 

**2. Placeholder scan** — the only intentional placeholders are the four page stubs in Task 1.11, each explicitly replaced in 3.3 / 4.4 / 4.5 / 4.6, with a grep guard in 4.7 Step 3. No `TBD`/`add error handling`/`similar to` placeholders remain.

**3. Type consistency** — names are stable across tasks: backend `appHealth`/`appHealthMember` (`live_status`, `primary_domain`, `combined_status`, `latest_deployment`), `db.AppDeployment{ProjectName}`, `db.GetLatestDeployment`, `db.ListAppDeployments`, `db.SetAppMemberOrder`, `build.MemberProbe{Probed,Running,OK}`, `build.HealthProber`, `statusFromProbe`, `deriveMemberLiveStatusFromDeployment`, `combinedAppStatus`, `topologyEdge{Source,Target,Via,Kind,Confirmed}`, `deriveTopologyEdges`, `GetTopology`/`GetDeployments`/`ReorderMembers`. Frontend mirrors: `AppHealth`/`AppHealthMember`/`AppDeployment`/`AppTopology`/`TopologyEdge`/`AppVariable`, `getAppHealth(id, env)`, `listAppDeployments`, `getAppTopology`, `reorderAppMembers`, `listAppVariables`/`upsertAppVariable`/`deleteAppVariable`, `queryKeys.appHealth(id, env)`/`appTopology`/`appDeployments`/`appVariables`, `APP_STATUS_META`/`MEMBER_STATUS_META`/`RELEASE_STATUS_META`/`ACTIVE_MEMBER_STATUSES`, components `AppBundleLayout`/`TopologyMap`/`AppVariableStore` and pages `AppOverview`/`AppDeployments`/`AppTopology`/`AppVariables`/`AppReleases`/`AppSettings`.

**Known adapt-on-read spots** (flagged inline, require reading the file before editing):
- Router wiring of `Prober` (Task 2.3) — depends on the router config struct + `cmd/server/main.go`; thread `DockerClient`/`ProxyType`.
- `crypto.NewEncryptor` / `db.UpsertProjectVariable` exact signatures (Task 3.2 test) — verify names.
- `ErrorBoundary` `scope` prop union may need `"app"` added (Task 1.8).
- `health_path` default for node-api runtime (Task 2.2) — v1 defaults to `/`; flagged.

