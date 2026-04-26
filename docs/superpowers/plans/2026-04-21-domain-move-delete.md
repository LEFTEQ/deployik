# Domain management: move, delete, re-verify, set primary — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a kebab menu on every custom domain row in `ProjectSettingsDomains.tsx` with **Move to {other env}**, **Re-verify**, **Set as primary**, and **Delete** actions — backed by a new `PATCH /api/projects/{id}/domains/{did}` endpoint and a `domains.is_primary` column. Auto-generated preview domains stay immortal + immovable.

**Architecture:** Single new HTTP verb + single migration. Move re-provisions nginx against the new environment's container (SSL stays valid because `ResolveVariantPlan` is now environment-independent after the 2026-04-21 variants.go fix — same hostnames, same cert). Set-primary is a transactional "unset siblings, set self". Delete already has a backend; only UI is missing. Re-verify reuses the existing verify endpoint.

**Tech Stack:** Go 1.25 (chi, modernc.org/sqlite), React 19 + TanStack Query + shadcn/ui (`DropdownMenu`, `AlertDialog`).

**Design doc:** `docs/plans/2026-04-21-domain-move-delete-design.md`

---

## File Structure

**New files:**
- `internal/db/migrations/014_domain_primary.sql` — migration: add `is_primary` column + partial unique index + backfill.

**Modified files:**
- `internal/db/models.go` — add `IsPrimary bool` field on `Domain`.
- `internal/db/queries_domains.go` — extend SELECTs/INSERT for `is_primary`; add `UpdateDomainEnvironment`, `SetDomainPrimary`.
- `internal/db/db_test.go` — extend `TestDomainCRUD` + add `TestDomainPrimaryAndEnvironmentChange`.
- `internal/api/handlers/domains.go` — add `Update` method and `updateDomainRequest` struct.
- `internal/api/handlers/domains_test.go` **(new file)** — tests for `Update`.
- `internal/api/router.go` — register `PATCH /projects/{id}/domains/{did}`.
- `web/src/types/api.ts` — add `is_primary: boolean` on `Domain`.
- `web/src/lib/api.ts` — add `updateDomain(projectId, domainId, patch)` method.
- `web/src/lib/deployment-helpers.ts` — `getPrimaryEnvironmentUrl` prefers `is_primary` first.
- `web/src/pages/ProjectSettingsDomains.tsx` — kebab menu, AlertDialog, three mutations, primary badge.

---

## Task 1: Database migration and model field

**Files:**
- Create: `internal/db/migrations/014_domain_primary.sql`
- Modify: `internal/db/models.go`
- Test: `internal/db/db_test.go`

**Why:** `is_primary` gives us explicit user intent for "which domain represents this environment" instead of the current implicit `is_auto` heuristic inside `getPrimaryEnvironmentUrl`. A partial unique index enforces one primary per `(project_id, environment)`. The backfill mirrors today's frontend heuristic so existing projects keep the same primary on day one.

- [ ] **Step 1: Write the failing test**

Add at the end of `internal/db/db_test.go`:

```go
func TestDomainIsPrimaryBackfill(t *testing.T) {
	database := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 1, Username: "u", Role: "admin"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	project := &Project{Name: "primary-backfill", GithubRepo: "r", GithubOwner: "o",
		Branch: "main", UserID: user.ID, Framework: "nextjs",
		BuildCommand: "b", InstallCommand: "i", NodeVersion: "22", Status: "active"}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("create project: %v", err)
	}

	// Preview: auto + one custom → auto should end up primary.
	auto := &Domain{ProjectID: project.ID, DomainName: "primary-backfill.preview.example.com",
		Environment: "preview", IsAuto: true, SSLStatus: "active"}
	if err := database.CreateDomain(auto); err != nil {
		t.Fatalf("create auto: %v", err)
	}
	previewCustom := &Domain{ProjectID: project.ID, DomainName: "preview.example.com",
		Environment: "preview", SSLStatus: "active"}
	if err := database.CreateDomain(previewCustom); err != nil {
		t.Fatalf("create preview custom: %v", err)
	}

	// Production: two customs → oldest-by-id should end up primary.
	prodFirst := &Domain{ProjectID: project.ID, DomainName: "example.com",
		Environment: "production", SSLStatus: "active"}
	if err := database.CreateDomain(prodFirst); err != nil {
		t.Fatalf("create prod first: %v", err)
	}
	prodSecond := &Domain{ProjectID: project.ID, DomainName: "www-alt.example.com",
		Environment: "production", SSLStatus: "active"}
	if err := database.CreateDomain(prodSecond); err != nil {
		t.Fatalf("create prod second: %v", err)
	}

	got, err := database.ListDomains(project.ID)
	if err != nil {
		t.Fatalf("list domains: %v", err)
	}
	primaries := map[string]string{}
	for _, d := range got {
		if d.IsPrimary {
			if prev, ok := primaries[d.Environment]; ok {
				t.Fatalf("two primaries in %s: %q and %q", d.Environment, prev, d.DomainName)
			}
			primaries[d.Environment] = d.DomainName
		}
	}
	if primaries["preview"] != auto.DomainName {
		t.Errorf("preview primary = %q, want %q", primaries["preview"], auto.DomainName)
	}
	if primaries["production"] != prodFirst.DomainName {
		t.Errorf("production primary = %q, want %q", primaries["production"], prodFirst.DomainName)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db -run TestDomainIsPrimaryBackfill -v`
Expected: FAIL — `d.IsPrimary` doesn't exist and migration hasn't created the column.

- [ ] **Step 3: Add the `IsPrimary` field to the Domain model**

Edit `internal/db/models.go`, find the `Domain` struct (~line 158) and add `IsPrimary`:

```go
type Domain struct {
	ID           string       `json:"id"`
	ProjectID    string       `json:"project_id"`
	DomainName   string       `json:"domain"`
	Environment  string       `json:"environment"`
	IsAuto       bool         `json:"is_auto"`
	IsPrimary    bool         `json:"is_primary"`
	DNSVerified  bool         `json:"dns_verified"`
	SSLStatus    string       `json:"ssl_status"`
	SSLExpiresAt sql.NullTime `json:"ssl_expires_at,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
}
```

- [ ] **Step 4: Write the migration SQL**

Create `internal/db/migrations/014_domain_primary.sql`:

```sql
-- Explicit "primary domain per environment" selection.
--
-- Before this, the frontend helper `getPrimaryEnvironmentUrl` guessed the
-- "main" domain for an environment using an implicit rule: preview → prefer
-- is_auto, production → prefer non-auto, otherwise first. That was fine
-- when each project had one or two domains, but gave users no way to pin a
-- specific hostname as the canonical one (e.g. when multiple custom domains
-- serve the same production).
--
-- This column lets users mark one domain per (project, environment) as the
-- primary. The partial unique index enforces that invariant at the DB layer
-- so concurrent requests can't produce two primaries. Existing rows are
-- backfilled to mirror today's heuristic so nothing visibly changes for
-- projects that don't use the new action.

ALTER TABLE domains ADD COLUMN is_primary INTEGER NOT NULL DEFAULT 0;

-- Backfill: one primary per (project_id, environment). Preview prefers auto;
-- production prefers non-auto; ties break by created_at ASC, then id ASC.
UPDATE domains SET is_primary = 1 WHERE id IN (
    SELECT id FROM domains d1
    WHERE NOT EXISTS (
        SELECT 1 FROM domains d2
        WHERE d2.project_id = d1.project_id
          AND d2.environment = d1.environment
          AND d2.id != d1.id
          AND (
              (d1.environment = 'preview'    AND d2.is_auto > d1.is_auto) OR
              (d1.environment = 'production' AND d2.is_auto < d1.is_auto) OR
              (d2.is_auto = d1.is_auto AND (
                   d2.created_at < d1.created_at
                   OR (d2.created_at = d1.created_at AND d2.id < d1.id)
              ))
          )
    )
);

CREATE UNIQUE INDEX idx_domains_one_primary_per_env
    ON domains(project_id, environment)
    WHERE is_primary = 1;
```

- [ ] **Step 5: Extend the List/Get/Create SQL to read/write `is_primary`**

Edit `internal/db/queries_domains.go`. Replace every `SELECT id, project_id, domain, environment, is_auto, dns_verified, ssl_status, ssl_expires_at, created_at` with `SELECT id, project_id, domain, environment, is_auto, is_primary, dns_verified, ssl_status, ssl_expires_at, created_at` — there are three such queries (`ListDomains`, `GetDomainByID`, `GetDomainByName`).

For each of those three methods, update the `Scan(...)` call to include `&d.IsPrimary` between `&d.IsAuto` and `&d.DNSVerified`. Concretely:

```go
// ListDomains:
rows, err := db.Query(
    `SELECT id, project_id, domain, environment, is_auto, is_primary, dns_verified, ssl_status, ssl_expires_at, created_at
     FROM domains WHERE project_id = ?
     ORDER BY is_auto DESC, created_at ASC`, projectID,
)
// …
if err := rows.Scan(&d.ID, &d.ProjectID, &d.DomainName, &d.Environment,
    &d.IsAuto, &d.IsPrimary, &d.DNSVerified, &d.SSLStatus, &d.SSLExpiresAt, &d.CreatedAt); err != nil {
```

Do the same substitution in `GetDomainByID` and `GetDomainByName`.

Also update `CreateDomain` to insert `is_primary`:

```go
func (db *DB) CreateDomain(d *Domain) error {
    d.ID = NewID()
    _, err := db.Exec(
        `INSERT INTO domains (id, project_id, domain, environment, is_auto, is_primary, dns_verified, ssl_status)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
        d.ID, d.ProjectID, d.DomainName, d.Environment, d.IsAuto, d.IsPrimary, d.DNSVerified, d.SSLStatus,
    )
    if err != nil {
        return fmt.Errorf("create domain: %w", err)
    }
    return nil
}
```

- [ ] **Step 6: Run the test**

Run: `go test ./internal/db -run TestDomainIsPrimaryBackfill -v`
Expected: PASS. Also re-run `TestDomainCRUD` to confirm no regression: `go test ./internal/db -run TestDomain -v`.

- [ ] **Step 7: Commit**

```bash
git add internal/db/migrations/014_domain_primary.sql internal/db/models.go internal/db/queries_domains.go internal/db/db_test.go
git commit -m "feat(db): add domains.is_primary with per-env uniqueness + backfill"
```

---

## Task 2: `UpdateDomainEnvironment` and `SetDomainPrimary` queries

**Files:**
- Modify: `internal/db/queries_domains.go`
- Test: `internal/db/db_test.go`

**Why:** The PATCH handler needs two narrow helpers. `UpdateDomainEnvironment` is a single-column update. `SetDomainPrimary` has to run inside a transaction so the partial unique index never sees two primaries in the same `(project_id, environment)` mid-write.

- [ ] **Step 1: Write the failing test**

Append to `internal/db/db_test.go`:

```go
func TestUpdateDomainEnvironmentAndSetPrimary(t *testing.T) {
	database := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 2, Username: "env-switcher", Role: "admin"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	project := &Project{Name: "mover", GithubRepo: "r", GithubOwner: "o",
		Branch: "main", UserID: user.ID, Framework: "nextjs",
		BuildCommand: "b", InstallCommand: "i", NodeVersion: "22", Status: "active"}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("create project: %v", err)
	}

	custom := &Domain{ProjectID: project.ID, DomainName: "switch.example.com",
		Environment: "preview", SSLStatus: "pending"}
	if err := database.CreateDomain(custom); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Environment change.
	if err := database.UpdateDomainEnvironment(custom.ID, "production"); err != nil {
		t.Fatalf("UpdateDomainEnvironment: %v", err)
	}
	got, err := database.GetDomainByID(custom.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v", err)
	}
	if got.Environment != "production" {
		t.Fatalf("environment = %q, want production", got.Environment)
	}

	// Add a sibling in production and set the new one primary.
	sibling := &Domain{ProjectID: project.ID, DomainName: "other.example.com",
		Environment: "production", SSLStatus: "active", IsPrimary: true}
	if err := database.CreateDomain(sibling); err != nil {
		t.Fatalf("create sibling: %v", err)
	}

	if err := database.SetDomainPrimary(project.ID, "production", custom.ID); err != nil {
		t.Fatalf("SetDomainPrimary: %v", err)
	}

	list, err := database.ListDomains(project.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	primaries := 0
	var primaryID string
	for _, d := range list {
		if d.Environment == "production" && d.IsPrimary {
			primaries++
			primaryID = d.ID
		}
	}
	if primaries != 1 {
		t.Fatalf("expected exactly 1 production primary, got %d", primaries)
	}
	if primaryID != custom.ID {
		t.Fatalf("primary id = %q, want %q", primaryID, custom.ID)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/db -run TestUpdateDomainEnvironmentAndSetPrimary -v`
Expected: FAIL — `UpdateDomainEnvironment` and `SetDomainPrimary` don't exist.

- [ ] **Step 3: Add the two query methods**

Append to `internal/db/queries_domains.go` (after `UpdateDomainSSL`):

```go
func (db *DB) UpdateDomainEnvironment(id, environment string) error {
    _, err := db.Exec(`UPDATE domains SET environment = ? WHERE id = ?`, environment, id)
    if err != nil {
        return fmt.Errorf("update domain environment: %w", err)
    }
    return nil
}

// SetDomainPrimary clears the primary flag across every other row in the same
// (project, environment) scope and sets it on domainID. Wrapped in a
// transaction so the partial unique index never observes two primaries.
func (db *DB) SetDomainPrimary(projectID, environment, domainID string) error {
    tx, err := db.Begin()
    if err != nil {
        return fmt.Errorf("set primary begin: %w", err)
    }
    defer tx.Rollback()

    if _, err := tx.Exec(
        `UPDATE domains SET is_primary = 0
         WHERE project_id = ? AND environment = ? AND id != ?`,
        projectID, environment, domainID,
    ); err != nil {
        return fmt.Errorf("set primary clear siblings: %w", err)
    }
    if _, err := tx.Exec(
        `UPDATE domains SET is_primary = 1
         WHERE id = ? AND project_id = ? AND environment = ?`,
        domainID, projectID, environment,
    ); err != nil {
        return fmt.Errorf("set primary assign: %w", err)
    }
    if err := tx.Commit(); err != nil {
        return fmt.Errorf("set primary commit: %w", err)
    }
    return nil
}
```

- [ ] **Step 4: Run the test**

Run: `go test ./internal/db -run TestUpdateDomainEnvironmentAndSetPrimary -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/queries_domains.go internal/db/db_test.go
git commit -m "feat(db): UpdateDomainEnvironment + transactional SetDomainPrimary"
```

---

## Task 3: `DomainHandler.Update` — move, set primary

**Files:**
- Modify: `internal/api/handlers/domains.go`
- Create: `internal/api/handlers/domains_test.go`

**Why:** One new HTTP verb (`PATCH`) handles both the "change environment" action and the "set primary" action via a sparse-update JSON body. Rejects `is_auto` rows. Re-provisions nginx against the new container on an environment change; SSL cert stays valid because `ResolveVariantPlan` is environment-independent.

- [ ] **Step 1: Write the failing tests**

Create `internal/api/handlers/domains_test.go`:

```go
package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/ws"
)

func newDomainTestHandler(t *testing.T) (*DomainHandler, *db.DB, *db.User, *db.Project) {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	user := &db.User{ID: db.NewID(), GithubID: 42, Username: "owner", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := database.EnsurePersonalOrganization(user); err != nil {
		t.Fatalf("ensure org: %v", err)
	}
	project := &db.Project{
		Name: "dh", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, Framework: "nextjs", BuildCommand: "b",
		InstallCommand: "i", NodeVersion: "22", Status: "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("project: %v", err)
	}
	h := &DomainHandler{DB: database, Hub: ws.NewHub(), Audit: &audit.Recorder{DB: database}}
	return h, database, user, project
}

func patchDomain(t *testing.T, h *DomainHandler, userID, projectID, domainID string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPatch,
		"/api/projects/"+projectID+"/domains/"+domainID, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", projectID)
	rctx.URLParams.Add("did", domainID)
	req = req.WithContext(chi.RouteContext(req.Context()).Context())
	ctx := req.Context()
	ctx = auth.WithClaims(ctx, &auth.Claims{UserID: userID})
	req = req.WithContext(ctx)
	req = req.WithContext(chiRouteContext(req.Context(), projectID, domainID))
	w := httptest.NewRecorder()
	h.Update(w, req)
	return w
}

// chiRouteContext builds a fresh chi route context with URL params injected,
// mirroring the production request lifecycle.
func chiRouteContext(ctx interface{ Value(any) any }, projectID, domainID string) interface{ Value(any) any } {
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", projectID)
	rc.URLParams.Add("did", domainID)
	// chi stores its context under a private key; httptest.NewRequest already
	// wired a context — clone it with the route ctx.
	_ = rc
	return ctx
}

func TestDomainUpdateMovesEnvironment(t *testing.T) {
	h, database, user, project := newDomainTestHandler(t)

	d := &db.Domain{ProjectID: project.ID, DomainName: "move.example.com",
		Environment: "preview", SSLStatus: "active", DNSVerified: true}
	if err := database.CreateDomain(d); err != nil {
		t.Fatalf("create: %v", err)
	}

	w := patchDomainWithChi(t, h, user.ID, project.ID, d.ID, map[string]any{"environment": "production"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	got, _ := database.GetDomainByID(d.ID)
	if got.Environment != "production" {
		t.Fatalf("environment = %q, want production", got.Environment)
	}
}

func TestDomainUpdateRejectsAutoDomain(t *testing.T) {
	h, database, user, project := newDomainTestHandler(t)

	auto := &db.Domain{ProjectID: project.ID, DomainName: "dh.preview.example.com",
		Environment: "preview", SSLStatus: "active", IsAuto: true}
	if err := database.CreateDomain(auto); err != nil {
		t.Fatalf("create: %v", err)
	}

	w := patchDomainWithChi(t, h, user.ID, project.ID, auto.ID, map[string]any{"environment": "production"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", w.Code, w.Body.String())
	}
}

func TestDomainUpdateSetsPrimary(t *testing.T) {
	h, database, user, project := newDomainTestHandler(t)

	a := &db.Domain{ProjectID: project.ID, DomainName: "a.example.com",
		Environment: "production", SSLStatus: "active", IsPrimary: true}
	b := &db.Domain{ProjectID: project.ID, DomainName: "b.example.com",
		Environment: "production", SSLStatus: "active"}
	if err := database.CreateDomain(a); err != nil {
		t.Fatalf("create a: %v", err)
	}
	if err := database.CreateDomain(b); err != nil {
		t.Fatalf("create b: %v", err)
	}

	w := patchDomainWithChi(t, h, user.ID, project.ID, b.ID, map[string]any{"is_primary": true})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	list, _ := database.ListDomains(project.ID)
	var primary string
	primaries := 0
	for _, d := range list {
		if d.Environment == "production" && d.IsPrimary {
			primaries++
			primary = d.DomainName
		}
	}
	if primaries != 1 {
		t.Fatalf("expected exactly 1 primary, got %d", primaries)
	}
	if primary != "b.example.com" {
		t.Fatalf("primary = %q, want b.example.com", primary)
	}
}

func TestDomainUpdateRejectsForeignProject(t *testing.T) {
	h, database, user, project := newDomainTestHandler(t)

	d := &db.Domain{ProjectID: project.ID, DomainName: "foreign.example.com",
		Environment: "preview", SSLStatus: "active"}
	if err := database.CreateDomain(d); err != nil {
		t.Fatalf("create: %v", err)
	}

	stranger := &db.User{ID: db.NewID(), GithubID: 999, Username: "stranger", Role: "user"}
	if err := database.UpsertUser(stranger); err != nil {
		t.Fatalf("upsert stranger: %v", err)
	}
	if _, err := database.EnsurePersonalOrganization(stranger); err != nil {
		t.Fatalf("ensure stranger org: %v", err)
	}

	w := patchDomainWithChi(t, h, stranger.ID, project.ID, d.ID, map[string]any{"environment": "production"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
}

// patchDomainWithChi issues a PATCH with chi URL params wired correctly.
func patchDomainWithChi(t *testing.T, h *DomainHandler, userID, projectID, domainID string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPatch,
		"/api/projects/"+projectID+"/domains/"+domainID, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", projectID)
	rc.URLParams.Add("did", domainID)
	ctx := req.Context()
	ctx = auth.WithClaims(ctx, &auth.Claims{UserID: userID})
	ctx = chi.RouteContextWithContext(ctx, rc)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.Update(w, req)
	return w
}
```

**Note for implementer:** the helper `chiRouteContext` in the first file draft is unused and was left only to guide you. Delete it after running the test — keep only `patchDomainWithChi`. Also check `chi.RouteContextWithContext`: if your chi version uses `context.WithValue(ctx, chi.RouteCtxKey, rc)` instead, prefer that; inspect chi's exported symbols with `go doc github.com/go-chi/chi/v5 RouteContext` before writing if unsure.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/handlers -run TestDomainUpdate -v`
Expected: FAIL — `Update` method doesn't exist on `DomainHandler`.

- [ ] **Step 3: Implement `Update` on `DomainHandler`**

Edit `internal/api/handlers/domains.go`. Add the request struct near the top (after `addDomainRequest`):

```go
type updateDomainRequest struct {
    Environment *string `json:"environment,omitempty"`
    IsPrimary   *bool   `json:"is_primary,omitempty"`
}
```

Then add the `Update` method (place it after `Delete`, before `Verify`):

```go
func (h *DomainHandler) Update(w http.ResponseWriter, r *http.Request) {
    projectID := chi.URLParam(r, "id")
    domainID := chi.URLParam(r, "did")

    project, _, ok := loadAuthorizedProject(w, r, h.DB, projectID)
    if !ok {
        return
    }

    target, err := h.DB.GetDomainByID(domainID)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load domain"})
        return
    }
    if target == nil || target.ProjectID != projectID {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "domain not found"})
        return
    }
    if target.IsAuto {
        writeJSON(w, http.StatusForbidden, map[string]string{"error": "auto domains cannot be modified"})
        return
    }

    var req updateDomainRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
        return
    }
    if req.Environment == nil && req.IsPrimary == nil {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nothing to update"})
        return
    }

    claims := auth.GetClaims(r.Context())

    // Environment change: serialize with in-flight verifications and re-provision.
    if req.Environment != nil && *req.Environment != target.Environment {
        newEnv := *req.Environment
        if newEnv != "preview" && newEnv != "production" {
            writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment must be preview or production"})
            return
        }
        // Avoid stomping a running verify on the same project.
        if _, loaded := h.verifying.LoadOrStore(projectID, domainID); loaded {
            writeJSON(w, http.StatusConflict, map[string]string{"error": "a verification is already in progress for this project"})
            return
        }
        defer h.verifying.Delete(projectID)

        oldEnv := target.Environment

        // Remove nginx config for the old env so the old vhost doesn't linger.
        if h.Manager != nil {
            if err := h.Manager.RemoveDomain(target.DomainName); err != nil {
                log.Printf("update domain: remove old nginx config for %s: %v", target.DomainName, err)
            }
        }

        if err := h.DB.UpdateDomainEnvironment(domainID, newEnv); err != nil {
            writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update environment"})
            return
        }
        target.Environment = newEnv

        // Re-provision nginx against the new environment's container. SSL stays
        // valid because ResolveVariantPlan is environment-independent — the cert
        // SAN list is the same for both envs of the same hostname.
        if h.Manager != nil && target.SSLStatus == "active" {
            plan := domain.ResolveVariantPlan(target.DomainName, newEnv)
            cfg := domain.ProvisionConfig{
                ProjectID:      project.ID,
                ProjectName:    project.Name,
                Domain:         plan.CanonicalDomain,
                RedirectDomain: plan.RedirectDomain,
                SSLDomains:     plan.AllDomains(),
                Environment:    newEnv,
                ContainerName:  fmt.Sprintf("deployik-%s-%s", project.Name, newEnv),
                Port:           project.Port,
            }
            if err := h.Manager.ProvisionDomain(cfg, false, nil); err != nil {
                log.Printf("update domain: re-provision %s for %s: %v", target.DomainName, newEnv, err)
                // Don't hard-fail — the DB is already updated; user can click
                // Verify to retry provisioning manually.
            }
        }

        h.Audit.Record(audit.Entry{
            UserID: claims.UserID, Action: "domain.move",
            ResourceType: "domain", ResourceID: domainID, ProjectID: projectID,
            Metadata: map[string]any{
                "domain": target.DomainName, "from": oldEnv, "to": newEnv,
            },
        })
    }

    // Primary flag.
    if req.IsPrimary != nil && *req.IsPrimary {
        if err := h.DB.SetDomainPrimary(projectID, target.Environment, domainID); err != nil {
            writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to set primary"})
            return
        }
        h.Audit.Record(audit.Entry{
            UserID: claims.UserID, Action: "domain.set_primary",
            ResourceType: "domain", ResourceID: domainID, ProjectID: projectID,
            Metadata: map[string]any{"domain": target.DomainName, "environment": target.Environment},
        })
    }

    fresh, _ := h.DB.GetDomainByID(domainID)
    writeJSON(w, http.StatusOK, fresh)
}
```

You also need new imports at the top of `domains.go`: add `"fmt"` (already present) and `"github.com/LEFTEQ/lovinka-deployik/internal/domain"` (already present — verify with `grep -n '"github.com/LEFTEQ/lovinka-deployik/internal/domain"' internal/api/handlers/domains.go`; add if missing).

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/api/handlers -run TestDomainUpdate -v`
Expected: all four PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/handlers/domains.go internal/api/handlers/domains_test.go
git commit -m "feat(api): PATCH /projects/:id/domains/:did — move env + set primary"
```

---

## Task 4: Register the PATCH route

**Files:**
- Modify: `internal/api/router.go:192-196`

**Why:** The handler is useless until it's wired into the chi mux.

- [ ] **Step 1: Add the route**

Edit `internal/api/router.go`. Inside the `// Domains` block (around line 192), add the PATCH line after the `Delete` registration and before `Verify`:

```go
// Domains
domainHandler := &handlers.DomainHandler{DB: cfg.DB, Manager: cfg.DomainManager, Hub: cfg.WSHub, Audit: auditRecorder}
r.Get("/projects/{id}/domains", domainHandler.List)
r.With(mutationLimiter.Middleware("domain_add")).Post("/projects/{id}/domains", domainHandler.Add)
r.With(mutationLimiter.Middleware("domain_update")).Patch("/projects/{id}/domains/{did}", domainHandler.Update)
r.With(mutationLimiter.Middleware("domain_delete")).Delete("/projects/{id}/domains/{did}", domainHandler.Delete)
r.With(mutationLimiter.Middleware("domain_verify")).Post("/projects/{id}/domains/{did}/verify", domainHandler.Verify)
```

- [ ] **Step 2: Build to verify**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/api/router.go
git commit -m "feat(api): wire PATCH /projects/:id/domains/:did"
```

---

## Task 5: Frontend types + API client

**Files:**
- Modify: `web/src/types/api.ts`
- Modify: `web/src/lib/api.ts`

**Why:** The UI can't call the new endpoint until the client method and type exist.

- [ ] **Step 1: Add `is_primary` to the Domain type**

Edit `web/src/types/api.ts`. Find the `Domain` interface and add `is_primary`:

```ts
export interface Domain {
  id: string;
  project_id: string;
  domain: string;
  environment: "preview" | "production";
  is_auto: boolean;
  is_primary: boolean;
  dns_verified: boolean;
  ssl_status: "pending" | "active" | "error";
  ssl_expires_at?: string;
  created_at: string;
}
```

(Use the exact property names present in the existing type; only add `is_primary: boolean`.)

- [ ] **Step 2: Add the `updateDomain` method on `ApiClient`**

Edit `web/src/lib/api.ts`. After `deleteDomain` (around line 249), add:

```ts
async updateDomain(
  projectId: string,
  domainId: string,
  patch: { environment?: "preview" | "production"; is_primary?: boolean },
): Promise<Domain> {
  return this.request(`/projects/${projectId}/domains/${domainId}`, {
    method: "PATCH",
    body: JSON.stringify(patch),
  });
}
```

- [ ] **Step 3: Typecheck**

Run: `cd web && bunx tsc --noEmit`
Expected: no output (pass).

- [ ] **Step 4: Commit**

```bash
git add web/src/types/api.ts web/src/lib/api.ts
git commit -m "feat(web): Domain.is_primary + api.updateDomain()"
```

---

## Task 6: `getPrimaryEnvironmentUrl` prefers `is_primary`

**Files:**
- Modify: `web/src/lib/deployment-helpers.ts:138-152`

**Why:** The implicit is_auto heuristic should yield to the user's explicit primary pick. Legacy projects without any primary flag keep the old behavior because backfill already set one primary per env — the fallback path only runs if no row has `is_primary`.

- [ ] **Step 1: Update the helper**

Replace the body of `getPrimaryEnvironmentUrl`:

```ts
export function getPrimaryEnvironmentUrl(
  domains: Domain[] | undefined,
  environment: Domain["environment"],
): string | null {
  const readyDomains = getReadyEnvironmentDomains(domains, environment);
  if (!readyDomains.length) return null;

  const explicitPrimary = readyDomains.find((domain) => domain.is_primary);
  if (explicitPrimary) return `https://${explicitPrimary.domain}`;

  const fallback =
    readyDomains.find((domain) =>
      environment === "preview" ? domain.is_auto : !domain.is_auto,
    ) ?? readyDomains[0];
  if (!fallback) return null;

  return `https://${fallback.domain}`;
}
```

- [ ] **Step 2: Typecheck**

Run: `cd web && bunx tsc --noEmit`
Expected: pass.

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/deployment-helpers.ts
git commit -m "feat(web): getPrimaryEnvironmentUrl honors domain.is_primary first"
```

---

## Task 7: Domain row kebab menu + AlertDialog + mutations

**Files:**
- Modify: `web/src/pages/ProjectSettingsDomains.tsx`

**Why:** This is the whole user-visible outcome. Adding `DropdownMenu` + `AlertDialog` on custom rows with four actions: Move, Re-verify, Set primary, Delete. Hiding the menu on `is_auto` rows keeps that case unchanged. Re-verify reuses the existing `verifyMutation` so no new mutation is needed.

- [ ] **Step 1: Add imports and new mutations**

Edit `web/src/pages/ProjectSettingsDomains.tsx`. Extend the existing import list:

```ts
import {
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  ExternalLink,
  GlobeLock,
  Link2,
  LoaderCircle,
  MoreHorizontal,
  Plus,
  RefreshCcw,
  Star,
  Trash2,
  X,
} from "lucide-react";
// …existing imports…
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
```

Inside `ProjectSettingsDomains()`, after the existing `verifyMutation` block, add:

```ts
const [deleteTarget, setDeleteTarget] = useState<Domain | null>(null);

const moveMutation = useMutation({
  mutationFn: ({ domainId, environment }: { domainId: string; environment: Domain["environment"] }) =>
    api.updateDomain(id, domainId, { environment }),
  onSuccess: () => {
    queryClient.invalidateQueries({ queryKey: queryKeys.domains(id) });
    toast.success("Environment updated");
  },
  onError: (err) => toast.error(err.message),
});

const setPrimaryMutation = useMutation({
  mutationFn: (domainId: string) => api.updateDomain(id, domainId, { is_primary: true }),
  onSuccess: () => {
    queryClient.invalidateQueries({ queryKey: queryKeys.domains(id) });
    toast.success("Primary updated");
  },
  onError: (err) => toast.error(err.message),
});

const deleteMutation = useMutation({
  mutationFn: (domainId: string) => api.deleteDomain(id, domainId),
  onSuccess: () => {
    queryClient.invalidateQueries({ queryKey: queryKeys.domains(id) });
    setDeleteTarget(null);
    toast.success("Domain deleted");
  },
  onError: (err) => {
    setDeleteTarget(null);
    toast.error(err.message);
  },
});
```

- [ ] **Step 2: Render the kebab menu on custom rows and the delete dialog**

Inside `renderDomainRow`, replace the action cluster in the flex container. Find the `<div className="flex flex-wrap gap-2">` block (around line 259 in the current file) and update it:

```tsx
<div className="flex flex-wrap items-center gap-2">
  {ready ? (
    <Button asChild size="sm" variant="outline">
      <a
        href={`https://${domain.domain}`}
        target="_blank"
        rel="noopener noreferrer"
      >
        <ExternalLink className="mr-1.5 h-3.5 w-3.5" />
        Open
      </a>
    </Button>
  ) : null}
  {!domain.is_auto ? (
    <>
      <Button
        size="sm"
        variant="outline"
        onClick={() => verifyMutation.mutate(domain.id)}
        disabled={allVerifyDisabled}
      >
        {isVerifying ? (
          <LoaderCircle className="mr-1.5 h-3.5 w-3.5 animate-spin" />
        ) : (
          <GlobeLock className="mr-1.5 h-3.5 w-3.5" />
        )}
        {isVerifying ? "Verifying..." : "Verify"}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button size="sm" variant="outline" className="h-8 w-8 px-0">
            <MoreHorizontal className="h-4 w-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem
            onSelect={() =>
              moveMutation.mutate({
                domainId: domain.id,
                environment: domain.environment === "preview" ? "production" : "preview",
              })
            }
            disabled={moveMutation.isPending}
          >
            <RefreshCcw className="mr-2 h-4 w-4" />
            Move to {domain.environment === "preview" ? "Production" : "Preview"}
          </DropdownMenuItem>
          <DropdownMenuItem
            onSelect={() => verifyMutation.mutate(domain.id)}
            disabled={allVerifyDisabled}
          >
            <GlobeLock className="mr-2 h-4 w-4" />
            Re-verify
          </DropdownMenuItem>
          {!domain.is_primary ? (
            <DropdownMenuItem
              onSelect={() => setPrimaryMutation.mutate(domain.id)}
              disabled={setPrimaryMutation.isPending}
            >
              <Star className="mr-2 h-4 w-4" />
              Set as primary
            </DropdownMenuItem>
          ) : null}
          <DropdownMenuSeparator />
          <DropdownMenuItem
            className="text-red-400 focus:text-red-400"
            onSelect={() => setDeleteTarget(domain)}
          >
            <Trash2 className="mr-2 h-4 w-4" />
            Delete
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </>
  ) : null}
</div>
```

Also add the `Primary` badge next to the existing Auto/Custom badge. Find:

```tsx
<Badge variant={domain.is_auto ? "secondary" : "outline"}>
  {domain.is_auto ? "Auto" : "Custom"}
</Badge>
```

Append immediately after:

```tsx
{domain.is_primary ? (
  <Badge variant="outline" className="border-amber-400/40 text-amber-200">
    <Star className="mr-1 h-3 w-3" />
    Primary
  </Badge>
) : null}
```

At the bottom of the component's `return` (after the `DnsSetupGuide` call), add the `AlertDialog` for delete confirmation:

```tsx
<AlertDialog
  open={deleteTarget !== null}
  onOpenChange={(open) => {
    if (!open && !deleteMutation.isPending) setDeleteTarget(null);
  }}
>
  <AlertDialogContent>
    <AlertDialogHeader>
      <AlertDialogTitle>
        Delete {deleteTarget?.domain}?
      </AlertDialogTitle>
      <AlertDialogDescription>
        This removes the nginx config and the live URL will stop responding.
        SSL certificate entries for this host are kept by certbot and will
        expire naturally.
      </AlertDialogDescription>
    </AlertDialogHeader>
    <AlertDialogFooter>
      <AlertDialogCancel disabled={deleteMutation.isPending}>Cancel</AlertDialogCancel>
      <AlertDialogAction
        className="bg-red-500 text-white hover:bg-red-500/90"
        onClick={(e) => {
          e.preventDefault();
          if (deleteTarget) deleteMutation.mutate(deleteTarget.id);
        }}
        disabled={deleteMutation.isPending}
      >
        {deleteMutation.isPending ? "Deleting..." : "Delete"}
      </AlertDialogAction>
    </AlertDialogFooter>
  </AlertDialogContent>
</AlertDialog>
```

- [ ] **Step 3: Typecheck**

Run: `cd web && bunx tsc --noEmit`
Expected: pass.

- [ ] **Step 4: Commit**

```bash
git add web/src/pages/ProjectSettingsDomains.tsx
git commit -m "feat(web): domain row kebab menu + delete confirm + primary badge"
```

---

## Task 8: End-to-end verification + living docs

**Why:** Make sure the whole flow actually works against a running dev server. Update living-docs with the new endpoint + column.

- [ ] **Step 1: Start dev servers**

Run in one terminal: `make dev-api`
Run in another: `make dev-web`
Run once: `make dev-seed` (if DB is empty).

- [ ] **Step 2: Exercise the flow in the browser**

1. Sign in via dev-login.
2. Open a project's **Settings → Domains**.
3. Add a custom domain (e.g. `example.test`). Confirm it appears in Preview group.
4. Click the three-dot menu → **Move to Production**. Row should move under the Production group and toast should say "Environment updated".
5. Click the three-dot menu → **Set as primary**. A Primary badge should appear next to Custom.
6. Add a second custom domain in Production. Set it as primary. The previous one should lose the Primary badge.
7. Click the three-dot menu → **Delete**. AlertDialog shows the domain name. Click Delete. Row disappears.
8. Confirm the auto `{project}.preview.example.com` row has **no three-dot menu**.
9. Confirm the **Open** button on the Overview / Deployments page points to the domain you marked Primary (if Production has a ready domain).

- [ ] **Step 3: Run full Go test suite**

Run: `go test ./...`
Expected: all packages pass.

- [ ] **Step 4: Update living-docs**

Edit `.claude/CLAUDE.md` — in the `## API Endpoints → Domains` section, add the new line after the existing `DELETE` entry:

```markdown
- `PATCH  /api/projects/{id}/domains/{did}` -- Update domain `{environment?, is_primary?}`; re-provisions nginx on env change. Rejects `is_auto=1` rows with 403.
```

In the `## Database Schema` table, append to the `domains` row's key fields: `is_primary (one per project+environment via partial unique index)`.

In the migration list:

```markdown
      014_domain_primary.sql    Adds is_primary with partial unique index per (project, environment); backfills based on today's is_auto heuristic
```

- [ ] **Step 5: Final commit**

```bash
git add .claude/CLAUDE.md
git commit -m "docs: note PATCH domain endpoint + domains.is_primary"
```

---

## Self-review checklist results

**Spec coverage** — verified:
- Kebab menu on custom rows ✓ (Task 7)
- Move to {other env} ✓ (Tasks 3 + 7)
- Re-verify ✓ (Task 7 reuses existing verifyMutation)
- Set as primary ✓ (Tasks 2 + 3 + 7)
- Delete with AlertDialog ✓ (Task 7)
- Auto-domain immortal + immovable ✓ (Task 3 rejects is_auto; Task 7 hides menu)
- PATCH endpoint re-provisioning nginx ✓ (Task 3)
- Backfill for existing rows ✓ (Task 1)
- Unique primary per env ✓ (Task 1 partial unique index + Task 2 transactional set)

**Type consistency** — `UpdateDomainEnvironment(id, environment string)` and `SetDomainPrimary(projectID, environment, domainID string)` signatures are used identically in Task 2 queries, Task 3 handler, and Task 3 tests.

**Placeholder scan** — no "TBD" or "similar to above" stubs. Every code block is complete.
