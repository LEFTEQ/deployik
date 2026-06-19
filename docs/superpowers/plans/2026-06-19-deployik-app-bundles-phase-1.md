# Deployik App Bundles — Phase 1 Implementation Plan (Data Model + App CRUD + Unified View)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce a first-class **App** entity that bundles projects within a workspace, with full CRUD and a composite "unified view" read — entirely inert (no runtime/deploy behavior change; every existing project keeps `app_id = NULL`).

**Architecture:** A new `apps` table (owned by an `organizations` row = workspace) plus a nullable `projects.app_id` FK. DB queries clone the existing groups CRUD shape; HTTP handlers clone the groups handler shape; MCP tools clone the TypeScript `groups.ts`/`workflows.ts` shape. No changes to build, deploy, network, ingress, or env resolution in this phase.

**Tech Stack:** Go 1.26 (control plane), SQLite (`internal/db`, ULID ids via `NewID()`, in-memory test DB via `newTestDB(t)`), chi router + JWT claims middleware (`internal/api`), TypeScript MCP server (`mcp/src/tools`, `@modelcontextprotocol/sdk`, Zod).

**Spec:** `docs/superpowers/specs/2026-06-19-deployik-app-bundles-design.md` (this implements **Phase P1** only; P2 path-filtering, P3 network+app-env, P4 app-DB+deploy-together+rollback are separate plans, each authored against P1's real code once it lands).

**Scope guard for this phase:** the migration adds ONLY `apps` + `projects.app_id`. The `app_variables`, `project_services.app_id`, `deploy_order`, `build_filter_enabled`, `watch_paths`, and `app_releases` columns/tables belong to later phases and are NOT created here. `apps.deploy_ordered` IS created now (it is an attribute of the App entity) but is not acted upon until P4.

---

## File Structure

| File | Create/Modify | Responsibility |
|------|---------------|----------------|
| `internal/db/migrations/026_apps.sql` | Create | `apps` table + `projects.app_id` column + indexes |
| `internal/db/models.go` | Modify | Add `App` struct + `AppCreate`; add `AppID` field to `Project` |
| `internal/db/queries_apps.go` | Create | App CRUD + membership-scoped reads + project↔app moves |
| `internal/db/queries_projects.go` | Modify | Add `app_id` to `GetProject`/`ListProjects` SELECT + scan |
| `internal/db/apps_test.go` | Create | DB-layer tests (TDD) |
| `internal/api/handlers/apps.go` | Create | HTTP handlers (List/Create/Get-health/Update/Delete/AddProjects/RemoveProject) |
| `internal/api/handlers/apps_test.go` | Create | Handler tests (TDD) |
| `internal/api/router.go` | Modify | Register `/apps` routes |
| `mcp/src/tools/apps.ts` | Create | MCP tools (create/list/get/update/delete/add/remove) |
| `mcp/src/tools/index.ts` | Modify | Register `registerAppTools` |

---

## Task 1: Migration — `apps` table + `projects.app_id`

**Files:**
- Create: `internal/db/migrations/026_apps.sql`
- Test: `internal/db/apps_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/db/apps_test.go`:

```go
package db

import "testing"

func TestMigration026CreatesAppsSchema(t *testing.T) {
	database := newTestDB(t)

	// apps table exists
	var tableName string
	err := database.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='apps'`,
	).Scan(&tableName)
	if err != nil {
		t.Fatalf("apps table not found: %v", err)
	}

	// projects.app_id column exists and defaults to NULL
	var appIDColumns int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('projects') WHERE name='app_id'`,
	).Scan(&appIDColumns); err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	if appIDColumns != 1 {
		t.Fatalf("expected projects.app_id column, found %d", appIDColumns)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db -run TestMigration026CreatesAppsSchema -v`
Expected: FAIL — `apps table not found: sql: no rows in result set`.

- [ ] **Step 3: Write the migration**

Create `internal/db/migrations/026_apps.sql` (mirrors the style of `023_project_services.sql` and the `006_organizations.sql` nullable-FK ALTER):

```sql
-- Migration 026: first-class "apps" — a bundle of projects inside a workspace
-- (organizations row). An app groups several independently-deployed projects so
-- they can later share a network, env, and a coordinated deploy (P3/P4). This
-- phase is inert: only the entity + the nullable projects.app_id link exist.
--
-- deploy_ordered is created now (an attribute of the entity) but is not acted
-- upon until the coordinated-deploy phase.
CREATE TABLE apps (
  id              TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name            TEXT NOT NULL,
  slug            TEXT NOT NULL,
  deploy_ordered  INTEGER NOT NULL DEFAULT 0,
  display_order   INTEGER NOT NULL DEFAULT 0,
  created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
  updated_at      DATETIME NOT NULL DEFAULT (datetime('now')),
  UNIQUE(organization_id, slug)
);
CREATE INDEX idx_apps_organization ON apps(organization_id);

-- Nullable FK; SET NULL on app delete so a project survives its app's removal.
-- (Same shape as 006_organizations.sql's organization_id ALTER, which SQLite
-- allows because the added column defaults to NULL.)
ALTER TABLE projects ADD COLUMN app_id TEXT REFERENCES apps(id) ON DELETE SET NULL;
CREATE INDEX idx_projects_app ON projects(app_id);
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/db -run TestMigration026CreatesAppsSchema -v`
Expected: PASS.

> If it still fails with "no such table: apps", the migration runner does not auto-discover new files. Check the embed directive in `internal/db/sqlite.go` (or `migrate.go`) — it should be `//go:embed migrations/*.sql`. If migrations are listed explicitly, append `"026_apps.sql"` to that list. Re-run.

- [ ] **Step 5: Commit**

```bash
git -C /Users/your-github-username/Documents/Work/lovinka-deployik add internal/db/migrations/026_apps.sql internal/db/apps_test.go
git -C /Users/your-github-username/Documents/Work/lovinka-deployik commit -m "feat(apps): migration 026 — apps table + projects.app_id"
```

---

## Task 2: Models — `App` struct, `AppCreate`, and `Project.AppID`

**Files:**
- Modify: `internal/db/models.go`

- [ ] **Step 1: Add the `App` and `AppCreate` types**

In `internal/db/models.go`, add near the `Group` struct (after line 56):

```go
// App is a bundle of projects inside a workspace (organization). Projects link
// to it via projects.app_id (nullable; NULL = standalone). DeployOrdered is an
// attribute of the entity, consumed only by the coordinated-deploy phase.
type App struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	DeployOrdered  bool      `json:"deploy_ordered"`
	DisplayOrder   int       `json:"display_order"`
	ProjectCount   int       `json:"project_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// AppCreate is the input to CreateApp.
type AppCreate struct {
	OrganizationID string
	Name           string
	ProjectIDs     []string
}
```

- [ ] **Step 2: Add `AppID` to the `Project` struct**

In `internal/db/models.go`, in the `Project` struct (lines 143-179), add the field immediately after `OrganizationName` (line 151) so it sits with the other ownership fields:

```go
	OrganizationID    string `json:"organization_id"`
	OrganizationName  string `json:"organization_name,omitempty"`
	AppID             string `json:"app_id,omitempty"` // empty = not in an app
	Framework         string `json:"framework"`
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./...`
Expected: builds clean (no usages yet — fields are inert).

- [ ] **Step 4: Commit**

```bash
git -C /Users/your-github-username/Documents/Work/lovinka-deployik add internal/db/models.go
git -C /Users/your-github-username/Documents/Work/lovinka-deployik commit -m "feat(apps): App + AppCreate models; Project.AppID field"
```

---

## Task 3: DB queries — App CRUD

**Files:**
- Create: `internal/db/queries_apps.go`
- Test: `internal/db/apps_test.go` (append)

This clones the shape of `queries_groups.go` (CreateGroup/GetGroup/ListGroupsForUser/UpdateGroupName/DeleteGroupMovingProjects), scoping reads by org membership and reusing the package-private `slugifyOrganizationName` for the slug base.

- [ ] **Step 1: Write the failing tests**

Append to `internal/db/apps_test.go`:

```go
func createAppTestUser(t *testing.T, database *DB, username string, githubID int64) *User {
	t.Helper()
	user := &User{ID: NewID(), GithubID: githubID, Username: username, Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser(%s): %v", username, err)
	}
	return user
}

func TestCreateAppAndGetForUser(t *testing.T) {
	database := newTestDB(t)
	user := createAppTestUser(t, database, "owner", 1)
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}

	app, err := database.CreateApp(&AppCreate{OrganizationID: org.ID, Name: "Forge acme"})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	if app.ID == "" {
		t.Fatal("expected generated app id")
	}
	if app.Name != "Forge acme" || app.Slug == "" {
		t.Fatalf("unexpected app name/slug: %q / %q", app.Name, app.Slug)
	}
	if app.OrganizationID != org.ID {
		t.Fatalf("app org = %q, want %q", app.OrganizationID, org.ID)
	}

	got, err := database.GetAppForUser(app.ID, user.ID)
	if err != nil {
		t.Fatalf("GetAppForUser: %v", err)
	}
	if got == nil || got.ID != app.ID {
		t.Fatalf("GetAppForUser returned %v, want app %s", got, app.ID)
	}

	// A non-member cannot see it.
	stranger := createAppTestUser(t, database, "stranger", 2)
	hidden, err := database.GetAppForUser(app.ID, stranger.ID)
	if err != nil {
		t.Fatalf("GetAppForUser(stranger): %v", err)
	}
	if hidden != nil {
		t.Fatal("expected non-member to get nil app")
	}
}

func TestListAppsForUserAndUpdateDelete(t *testing.T) {
	database := newTestDB(t)
	user := createAppTestUser(t, database, "owner", 1)
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	app, err := database.CreateApp(&AppCreate{OrganizationID: org.ID, Name: "Alpha"})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}

	apps, err := database.ListAppsForUser(user.ID)
	if err != nil {
		t.Fatalf("ListAppsForUser: %v", err)
	}
	if len(apps) != 1 || apps[0].ID != app.ID {
		t.Fatalf("ListAppsForUser = %+v, want 1 app %s", apps, app.ID)
	}

	renamed, err := database.UpdateAppName(app.ID, "Beta")
	if err != nil {
		t.Fatalf("UpdateAppName: %v", err)
	}
	if renamed.Name != "Beta" {
		t.Fatalf("rename = %q, want Beta", renamed.Name)
	}

	if err := database.DeleteApp(app.ID); err != nil {
		t.Fatalf("DeleteApp: %v", err)
	}
	gone, err := database.GetApp(app.ID)
	if err != nil {
		t.Fatalf("GetApp after delete: %v", err)
	}
	if gone != nil {
		t.Fatal("expected app to be deleted")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/db -run 'TestCreateAppAndGetForUser|TestListAppsForUserAndUpdateDelete' -v`
Expected: FAIL — `database.CreateApp undefined`.

- [ ] **Step 3: Write the implementation**

Create `internal/db/queries_apps.go`:

```go
package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// CreateApp creates an app inside an organization and optionally moves the given
// projects into it. Mirrors CreateGroup. The caller is responsible for verifying
// the user may write to the organization + projects (the HTTP handler does this).
func (db *DB) CreateApp(input *AppCreate) (*App, error) {
	if input == nil {
		return nil, fmt.Errorf("create app: input is nil")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("create app: name is required")
	}
	orgID := strings.TrimSpace(input.OrganizationID)
	if orgID == "" {
		return nil, fmt.Errorf("create app: organization_id is required")
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("create app: begin: %w", err)
	}
	defer tx.Rollback()

	slug, err := reserveUniqueAppSlug(tx, orgID, slugifyOrganizationName(name))
	if err != nil {
		return nil, fmt.Errorf("create app: %w", err)
	}

	var displayOrder int
	if err := tx.QueryRow(
		`SELECT COALESCE(MAX(display_order), 0) + 1 FROM apps WHERE organization_id = ?`,
		orgID,
	).Scan(&displayOrder); err != nil {
		return nil, fmt.Errorf("create app: next display order: %w", err)
	}

	appID := NewID()
	if _, err := tx.Exec(
		`INSERT INTO apps (id, organization_id, name, slug, display_order)
		 VALUES (?, ?, ?, ?, ?)`,
		appID, orgID, name, slug, displayOrder,
	); err != nil {
		return nil, fmt.Errorf("create app: insert: %w", err)
	}
	for _, projectID := range uniqueNonEmpty(input.ProjectIDs) {
		if err := moveProjectToAppTx(tx, appID, projectID); err != nil {
			return nil, fmt.Errorf("create app: move project: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create app: commit: %w", err)
	}

	app, err := db.GetApp(appID)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, fmt.Errorf("create app: created app not found")
	}
	return app, nil
}

// reserveUniqueAppSlug returns base, or base-2, base-3, … unique within the org.
func reserveUniqueAppSlug(tx *sql.Tx, orgID, base string) (string, error) {
	if base == "" {
		base = "app"
	}
	candidate := base
	for n := 2; ; n++ {
		var exists int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM apps WHERE organization_id = ? AND slug = ?`,
			orgID, candidate,
		).Scan(&exists); err != nil {
			return "", fmt.Errorf("reserve slug: %w", err)
		}
		if exists == 0 {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", base, n)
	}
}

// moveProjectToAppTx sets a project's app_id within a transaction.
func moveProjectToAppTx(tx *sql.Tx, appID, projectID string) error {
	res, err := tx.Exec(
		`UPDATE projects SET app_id = ?, updated_at = datetime('now')
		 WHERE id = ? AND status != 'deleted'`,
		appID, projectID,
	)
	if err != nil {
		return fmt.Errorf("move project to app: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("move project to app: rows affected: %w", err)
	}
	if affected == 0 {
		return ErrProjectNotMovable
	}
	return nil
}

func (db *DB) appSelect(where string, args ...any) (*App, error) {
	app := &App{}
	query := `SELECT a.id, a.organization_id, a.name, a.slug, a.deploy_ordered, a.display_order,
	                 COALESCE((SELECT COUNT(*) FROM projects p
	                           WHERE p.app_id = a.id AND p.status != 'deleted'), 0),
	                 a.created_at, a.updated_at
	          FROM apps a ` + where
	err := db.QueryRow(query, args...).Scan(
		&app.ID, &app.OrganizationID, &app.Name, &app.Slug, &app.DeployOrdered,
		&app.DisplayOrder, &app.ProjectCount, &app.CreatedAt, &app.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}
	return app, nil
}

// GetApp fetches an app by id (no access check).
func (db *DB) GetApp(appID string) (*App, error) {
	return db.appSelect(`WHERE a.id = ?`, appID)
}

// GetAppForUser fetches an app only if the user is a member of its organization.
func (db *DB) GetAppForUser(appID, userID string) (*App, error) {
	return db.appSelect(
		`WHERE a.id = ? AND EXISTS (
		   SELECT 1 FROM organization_memberships om
		   WHERE om.organization_id = a.organization_id AND om.user_id = ?
		 )`,
		appID, userID,
	)
}

// ListAppsForUser lists apps across every organization the user belongs to.
func (db *DB) ListAppsForUser(userID string) ([]App, error) {
	rows, err := db.Query(
		`SELECT a.id, a.organization_id, a.name, a.slug, a.deploy_ordered, a.display_order,
		        COALESCE((SELECT COUNT(*) FROM projects p
		                  WHERE p.app_id = a.id AND p.status != 'deleted'), 0),
		        a.created_at, a.updated_at
		 FROM apps a
		 JOIN organization_memberships om ON om.organization_id = a.organization_id
		 WHERE om.user_id = ?
		 ORDER BY a.display_order ASC, lower(a.name) ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list apps for user: %w", err)
	}
	defer rows.Close()

	var apps []App
	for rows.Next() {
		var app App
		if err := rows.Scan(
			&app.ID, &app.OrganizationID, &app.Name, &app.Slug, &app.DeployOrdered,
			&app.DisplayOrder, &app.ProjectCount, &app.CreatedAt, &app.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan app: %w", err)
		}
		apps = append(apps, app)
	}
	return apps, rows.Err()
}

// UpdateAppName renames an app.
func (db *DB) UpdateAppName(appID, name string) (*App, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("update app name: name is required")
	}
	if _, err := db.Exec(
		`UPDATE apps SET name = ?, updated_at = datetime('now') WHERE id = ?`,
		name, appID,
	); err != nil {
		return nil, fmt.Errorf("update app name: %w", err)
	}
	return db.GetApp(appID)
}

// DeleteApp deletes an app. Member projects survive: projects.app_id is set NULL
// by the ON DELETE SET NULL foreign key.
func (db *DB) DeleteApp(appID string) error {
	if _, err := db.Exec(`DELETE FROM apps WHERE id = ?`, appID); err != nil {
		return fmt.Errorf("delete app: %w", err)
	}
	return nil
}
```

> `ErrProjectNotMovable`, `uniqueNonEmpty`, `slugifyOrganizationName`, and `EnsurePersonalOrganization` already exist in package `db` (used by `queries_groups.go`). Reuse them; do not redefine.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/db -run 'TestCreateAppAndGetForUser|TestListAppsForUserAndUpdateDelete' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git -C /Users/your-github-username/Documents/Work/lovinka-deployik add internal/db/queries_apps.go internal/db/apps_test.go
git -C /Users/your-github-username/Documents/Work/lovinka-deployik commit -m "feat(apps): App CRUD queries (create/get/list/update/delete)"
```

---

## Task 4: DB queries — project↔app membership + `ListProjectsByApp` + project scan wiring

**Files:**
- Modify: `internal/db/queries_apps.go` (append)
- Modify: `internal/db/queries_projects.go` (`GetProject` + `ListProjects` SELECT/scan)
- Test: `internal/db/apps_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append to `internal/db/apps_test.go`:

```go
func TestAddRemoveProjectAndListByApp(t *testing.T) {
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

	project := &Project{
		Name: "acme-api", GithubRepo: "forge", GithubOwner: "owner", Branch: "main",
		UserID: user.ID, OrganizationID: org.ID, Framework: "static",
		PackageManager: "auto", OutputDirectory: "dist", BuildCommand: "bun run build",
		InstallCommand: "bun install", NodeVersion: "22", Status: "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	if err := database.AddProjectsToApp(app.ID, []string{project.ID}); err != nil {
		t.Fatalf("AddProjectsToApp: %v", err)
	}

	members, err := database.ListProjectsByApp(app.ID)
	if err != nil {
		t.Fatalf("ListProjectsByApp: %v", err)
	}
	if len(members) != 1 || members[0].ID != project.ID {
		t.Fatalf("members = %+v, want [%s]", members, project.ID)
	}

	// GetProject now reflects the app membership.
	got, err := database.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.AppID != app.ID {
		t.Fatalf("project app_id = %q, want %q", got.AppID, app.ID)
	}

	if err := database.RemoveProjectFromApp(project.ID); err != nil {
		t.Fatalf("RemoveProjectFromApp: %v", err)
	}
	after, err := database.ListProjectsByApp(app.ID)
	if err != nil {
		t.Fatalf("ListProjectsByApp after remove: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("expected 0 members after remove, got %d", len(after))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db -run TestAddRemoveProjectAndListByApp -v`
Expected: FAIL — `database.AddProjectsToApp undefined`.

- [ ] **Step 3a: Append membership + list functions to `internal/db/queries_apps.go`**

```go
// AddProjectsToApp moves projects into an app (sets projects.app_id).
func (db *DB) AddProjectsToApp(appID string, projectIDs []string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("add projects to app: begin: %w", err)
	}
	defer tx.Rollback()
	for _, projectID := range uniqueNonEmpty(projectIDs) {
		if err := moveProjectToAppTx(tx, appID, projectID); err != nil {
			return fmt.Errorf("add projects to app: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("add projects to app: commit: %w", err)
	}
	return nil
}

// RemoveProjectFromApp detaches a project from its app (app_id = NULL).
func (db *DB) RemoveProjectFromApp(projectID string) error {
	if _, err := db.Exec(
		`UPDATE projects SET app_id = NULL, updated_at = datetime('now')
		 WHERE id = ? AND status != 'deleted'`,
		projectID,
	); err != nil {
		return fmt.Errorf("remove project from app: %w", err)
	}
	return nil
}

// ListProjectsByApp returns the member projects of an app (for the unified view).
func (db *DB) ListProjectsByApp(appID string) ([]Project, error) {
	rows, err := db.Query(
		`SELECT p.id, p.name, p.github_repo, p.github_owner, p.branch, p.user_id,
		        COALESCE(p.organization_id, ''), COALESCE(o.name, ''), COALESCE(p.app_id, ''),
		        p.framework, p.package_manager, p.root_directory, p.output_directory,
		        p.build_command, p.install_command, p.node_version, p.status,
		        p.created_at, p.updated_at,
		        p.host_network_access, p.data_volume_enabled, COALESCE(p.data_mount_path, '/app/data'),
		        p.port, COALESCE(p.resource_tier, 'small'), p.start_command, p.health_path
		 FROM projects p
		 LEFT JOIN organizations o ON o.id = p.organization_id
		 WHERE p.app_id = ? AND p.status != 'deleted'
		 ORDER BY p.name ASC`,
		appID,
	)
	if err != nil {
		return nil, fmt.Errorf("list projects by app: %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.GithubRepo, &p.GithubOwner, &p.Branch,
			&p.UserID, &p.OrganizationID, &p.OrganizationName, &p.AppID,
			&p.Framework, &p.PackageManager, &p.RootDirectory, &p.OutputDirectory,
			&p.BuildCommand, &p.InstallCommand, &p.NodeVersion, &p.Status,
			&p.CreatedAt, &p.UpdatedAt,
			&p.HostNetworkAccess, &p.DataVolumeEnabled, &p.DataMountPath,
			&p.Port, &p.ResourceTier, &p.StartCommand, &p.HealthPath); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}
```

- [ ] **Step 3b: Wire `app_id` into `GetProject` (`internal/db/queries_projects.go:85-123`)**

In the `GetProject` SELECT, add `COALESCE(p.app_id, '')` immediately after `COALESCE(o.name, '')`:

```go
		`SELECT p.id, p.name, p.github_repo, p.github_owner, p.branch, p.user_id,
		        COALESCE(p.organization_id, ''), COALESCE(o.name, ''), COALESCE(p.app_id, ''), p.framework, p.package_manager,
```

and add `&p.AppID` to the `.Scan(...)` immediately after `&p.OrganizationName`:

```go
	).Scan(&p.ID, &p.Name, &p.GithubRepo, &p.GithubOwner, &p.Branch,
		&p.UserID, &p.OrganizationID, &p.OrganizationName, &p.AppID, &p.Framework, &p.PackageManager, &p.RootDirectory, &p.OutputDirectory, &p.BuildCommand, &p.InstallCommand, &p.NodeVersion,
```

- [ ] **Step 3c: Wire `app_id` into `ListProjects` (`internal/db/queries_projects.go:35-83`)**

In the `baseQuery` SELECT, add `COALESCE(p.app_id, '')` immediately after `COALESCE(o.name, '')`:

```go
		SELECT p.id, p.name, p.github_repo, p.github_owner, p.branch, p.user_id,
		       COALESCE(p.organization_id, ''), COALESCE(o.name, ''), COALESCE(p.app_id, ''), p.framework, p.package_manager,
```

and add `&p.AppID` to the `rows.Scan(...)` immediately after `&p.OrganizationName`:

```go
		if err := rows.Scan(&p.ID, &p.Name, &p.GithubRepo, &p.GithubOwner, &p.Branch,
			&p.UserID, &p.OrganizationID, &p.OrganizationName, &p.AppID, &p.Framework, &p.PackageManager, &p.RootDirectory, &p.OutputDirectory, &p.BuildCommand, &p.InstallCommand, &p.NodeVersion,
```

- [ ] **Step 4: Run the full db suite to verify pass + no regression**

Run: `go test ./internal/db -v`
Expected: PASS — the new app tests pass AND every existing project/group test still passes (the added SELECT column + scan target are aligned).

- [ ] **Step 5: Commit**

```bash
git -C /Users/your-github-username/Documents/Work/lovinka-deployik add internal/db/queries_apps.go internal/db/queries_projects.go internal/db/apps_test.go
git -C /Users/your-github-username/Documents/Work/lovinka-deployik commit -m "feat(apps): project<->app moves, ListProjectsByApp, app_id scan wiring"
```

---

## Task 5: HTTP handlers + router

**Files:**
- Create: `internal/api/handlers/apps.go`
- Modify: `internal/api/router.go`
- Test: `internal/api/handlers/apps_test.go`

Clones the `GroupHandler` shape (`internal/api/handlers/groups.go`): `auth.GetClaims`, `writeJSON`, `json.NewDecoder`. The composite `GetHealth` mirrors the MCP `get_project_health` composite read, but assembled server-side from `ListProjectsByApp` + each member's latest deploys.

- [ ] **Step 1: Write the failing test**

Create `internal/api/handlers/apps_test.go` (mirrors the `auth.WithClaims` + `chi.RouteCtxKey` idiom from `services_test.go`/`preview_instances_test.go`):

```go
package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

func appTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestAppHandlerCreateAndList(t *testing.T) {
	database := appTestDB(t)
	user := &db.User{ID: db.NewID(), GithubID: 1, Username: "owner", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}

	h := &AppHandler{DB: database}
	claims := &auth.Claims{UserID: user.ID, Role: "user"}

	// Create
	body, _ := json.Marshal(map[string]any{"name": "Forge acme", "organization_id": org.ID})
	req := httptest.NewRequest(http.MethodPost, "/apps", bytes.NewReader(body))
	req = req.WithContext(auth.WithClaims(req.Context(), claims))
	rec := httptest.NewRecorder()
	h.Create(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Create status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	var created db.App
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if created.ID == "" || created.Name != "Forge acme" {
		t.Fatalf("unexpected created app: %+v", created)
	}

	// List
	listReq := httptest.NewRequest(http.MethodGet, "/apps", nil)
	listReq = listReq.WithContext(auth.WithClaims(listReq.Context(), claims))
	listRec := httptest.NewRecorder()
	h.List(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("List status = %d, want 200", listRec.Code)
	}
	var apps []db.App
	if err := json.Unmarshal(listRec.Body.Bytes(), &apps); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(apps) != 1 || apps[0].ID != created.ID {
		t.Fatalf("List = %+v, want [%s]", apps, created.ID)
	}

	// GetHealth (composite)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", created.ID)
	healthReq := httptest.NewRequest(http.MethodGet, "/apps/"+created.ID+"/health", nil)
	healthReq = healthReq.WithContext(context.WithValue(healthReq.Context(), chi.RouteCtxKey, rctx))
	healthReq = healthReq.WithContext(auth.WithClaims(healthReq.Context(), claims))
	healthRec := httptest.NewRecorder()
	h.GetHealth(healthRec, healthReq)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("GetHealth status = %d, want 200 (body: %s)", healthRec.Code, healthRec.Body.String())
	}
}
```

> Confirm the module path prefix by checking `go.mod` (the import paths above assume `github.com/LEFTEQ/lovinka-deployik`). If it differs, adjust the three internal imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/handlers -run TestAppHandlerCreateAndList -v`
Expected: FAIL — `undefined: AppHandler`.

- [ ] **Step 3: Write the handler**

Create `internal/api/handlers/apps.go`:

```go
package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// AppHandler serves /apps — bundles of projects within a workspace.
type AppHandler struct {
	DB *db.DB
}

type createAppRequest struct {
	Name           string   `json:"name"`
	OrganizationID string   `json:"organization_id"`
	ProjectIDs     []string `json:"project_ids"`
}

type updateAppRequest struct {
	Name string `json:"name"`
}

type appProjectsRequest struct {
	ProjectIDs []string `json:"project_ids"`
}

// loadManagedApp loads the app by URL id and verifies the caller is a member of
// its organization. Writes the error response + returns ok=false on failure.
func (h *AppHandler) loadManagedApp(w http.ResponseWriter, r *http.Request) (*db.App, *auth.Claims, bool) {
	claims := auth.GetClaims(r.Context())
	appID := chi.URLParam(r, "id")
	app, err := h.DB.GetAppForUser(appID, claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load app"})
		return nil, nil, false
	}
	if app == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "app not found"})
		return nil, nil, false
	}
	return app, claims, true
}

func (h *AppHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	apps, err := h.DB.ListAppsForUser(claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list apps"})
		return
	}
	if apps == nil {
		apps = []db.App{}
	}
	writeJSON(w, http.StatusOK, apps)
}

func (h *AppHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	var req createAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if strings.TrimSpace(req.OrganizationID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "organization_id is required"})
		return
	}
	if !h.canManageOrg(claims, req.OrganizationID) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "organization not found"})
		return
	}
	if !h.canAttachProjects(claims, req.OrganizationID, req.ProjectIDs) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	app, err := h.DB.CreateApp(&db.AppCreate{
		OrganizationID: req.OrganizationID,
		Name:           req.Name,
		ProjectIDs:     req.ProjectIDs,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create app"})
		return
	}
	writeJSON(w, http.StatusCreated, app)
}

func (h *AppHandler) Update(w http.ResponseWriter, r *http.Request) {
	app, _, ok := h.loadManagedApp(w, r)
	if !ok {
		return
	}
	var req updateAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	updated, err := h.DB.UpdateAppName(app.ID, req.Name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update app"})
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *AppHandler) Delete(w http.ResponseWriter, r *http.Request) {
	app, _, ok := h.loadManagedApp(w, r)
	if !ok {
		return
	}
	if err := h.DB.DeleteApp(app.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete app"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AppHandler) AddProjects(w http.ResponseWriter, r *http.Request) {
	app, claims, ok := h.loadManagedApp(w, r)
	if !ok {
		return
	}
	var req appProjectsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if !h.canAttachProjects(claims, app.OrganizationID, req.ProjectIDs) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	if err := h.DB.AddProjectsToApp(app.ID, req.ProjectIDs); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to add projects"})
		return
	}
	writeJSON(w, http.StatusOK, app)
}

func (h *AppHandler) RemoveProject(w http.ResponseWriter, r *http.Request) {
	app, claims, ok := h.loadManagedApp(w, r)
	if !ok {
		return
	}
	projectID := chi.URLParam(r, "pid")
	project, err := h.DB.GetProject(projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load project"})
		return
	}
	if project == nil || project.AppID != app.ID || !h.canManageOrg(claims, project.OrganizationID) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found in app"})
		return
	}
	if err := h.DB.RemoveProjectFromApp(projectID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to remove project"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// appHealth is the composite "unified view" payload.
type appHealth struct {
	App     *db.App          `json:"app"`
	Members []appHealthMember `json:"members"`
}

type appHealthMember struct {
	Project           db.Project `json:"project"`
	LatestPreview     *time.Time `json:"latest_preview_deploy_at,omitempty"`
	LatestProduction  *time.Time `json:"latest_production_deploy_at,omitempty"`
}

func (h *AppHandler) GetHealth(w http.ResponseWriter, r *http.Request) {
	app, _, ok := h.loadManagedApp(w, r)
	if !ok {
		return
	}
	members, err := h.DB.ListProjectsByApp(app.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load members"})
		return
	}
	out := appHealth{App: app, Members: make([]appHealthMember, 0, len(members))}
	for i := range members {
		// GetProject populates the latest-deploy timestamps; ListProjectsByApp
		// returns the lighter row, so re-fetch each member for its deploy state.
		full, err := h.DB.GetProject(members[i].ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load member"})
			return
		}
		if full == nil {
			continue
		}
		out.Members = append(out.Members, appHealthMember{
			Project:          *full,
			LatestPreview:    full.LatestPreviewDeployAt,
			LatestProduction: full.LatestProductionDeployAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// canManageOrg reports whether the caller is a member of the organization.
func (h *AppHandler) canManageOrg(claims *auth.Claims, orgID string) bool {
	ok, err := h.DB.IsOrganizationMember(orgID, claims.UserID)
	if err != nil {
		return false
	}
	return ok
}

// canAttachProjects checks every project exists, is in the target org, and the
// caller can access it. Empty list = ok.
func (h *AppHandler) canAttachProjects(claims *auth.Claims, orgID string, projectIDs []string) bool {
	for _, id := range projectIDs {
		if strings.TrimSpace(id) == "" {
			continue
		}
		project, err := h.DB.GetProject(id)
		if err != nil || project == nil {
			return false
		}
		if project.OrganizationID != orgID {
			return false
		}
		if project.UserID != claims.UserID && !h.canManageOrg(claims, orgID) {
			return false
		}
	}
	return true
}
```

Add the `time` import to the file's import block (used by `appHealthMember`):

```go
import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)
```

- [ ] **Step 3b: Add the `IsOrganizationMember` query if it does not already exist**

Check first: `grep -rn "func (db \*DB) IsOrganizationMember" internal/db`. If absent, append to `internal/db/queries_apps.go`:

```go
// IsOrganizationMember reports whether the user belongs to the organization.
func (db *DB) IsOrganizationMember(orgID, userID string) (bool, error) {
	var exists int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM organization_memberships WHERE organization_id = ? AND user_id = ?`,
		orgID, userID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("is organization member: %w", err)
	}
	return exists > 0, nil
}
```

(If an equivalent already exists under a different name, use that instead and drop this addition.)

- [ ] **Step 4: Register the routes**

In `internal/api/router.go`, immediately after the group-routes block (ends ~line 195), add:

```go
	appHandler := &handlers.AppHandler{DB: cfg.DB}
	r.Get("/apps", appHandler.List)
	r.With(mutationLimiter.Middleware("app_create")).Post("/apps", appHandler.Create)
	r.Get("/apps/{id}/health", appHandler.GetHealth)
	r.With(mutationLimiter.Middleware("app_update")).Patch("/apps/{id}", appHandler.Update)
	r.With(mutationLimiter.Middleware("app_delete")).Delete("/apps/{id}", appHandler.Delete)
	r.With(mutationLimiter.Middleware("app_projects_add")).Post("/apps/{id}/projects", appHandler.AddProjects)
	r.With(mutationLimiter.Middleware("app_projects_remove")).Delete("/apps/{id}/projects/{pid}", appHandler.RemoveProject)
```

- [ ] **Step 5: Run the test + build**

Run: `go test ./internal/api/handlers -run TestAppHandlerCreateAndList -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 6: Commit**

```bash
git -C /Users/your-github-username/Documents/Work/lovinka-deployik add internal/api/handlers/apps.go internal/api/handlers/apps_test.go internal/api/router.go internal/db/queries_apps.go
git -C /Users/your-github-username/Documents/Work/lovinka-deployik commit -m "feat(apps): HTTP handlers + routes (CRUD, add/remove project, unified health)"
```

---

## Task 6: MCP tools

**Files:**
- Create: `mcp/src/tools/apps.ts`
- Modify: `mcp/src/tools/index.ts`

Clones `mcp/src/tools/groups.ts` (write tools via `ctx.client.request`) and `workflows.ts` (the `get_*_health` read). The MCP dispatches to the HTTP API added in Task 5.

- [ ] **Step 1: Write the tool module**

Create `mcp/src/tools/apps.ts`:

```typescript
import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { registerTool, type ToolContext } from "./_helpers.js";

// Minimal shapes for the API responses we render.
interface App {
  id: string;
  organization_id: string;
  name: string;
  slug: string;
  project_count: number;
}

interface AppHealth {
  app: App;
  members: Array<{
    project: { id: string; name: string; status: string };
    latest_preview_deploy_at?: string | null;
    latest_production_deploy_at?: string | null;
  }>;
}

export function registerAppTools(server: McpServer, ctx: ToolContext): void {
  registerTool(server, ctx, {
    name: "list_apps",
    description: "List app bundles (groups of projects deployed as one unit) the caller can access.",
    inputSchema: {},
    annotations: { readOnlyHint: true, title: "List apps" },
    handler: async () => {
      const apps = await ctx.client.request<App[]>("/apps");
      const text = apps.length
        ? apps.map((a) => `${a.name} (id: ${a.id}, ${a.project_count} project(s))`).join("\n")
        : "(no apps)";
      return { text, data: apps };
    },
  });

  registerTool(server, ctx, {
    name: "create_app",
    description: "Create an app bundle inside a workspace. Optionally move existing projects into it by id.",
    inputSchema: {
      name: z.string(),
      organization_id: z.string().describe("Workspace/group id the app belongs to."),
      project_ids: z.array(z.string()).default([]),
    },
    annotations: { title: "Create app bundle" },
    handler: async (args) => {
      const app = await ctx.client.request<App>("/apps", {
        method: "POST",
        body: { name: args.name, organization_id: args.organization_id, project_ids: args.project_ids ?? [] },
      });
      return {
        text: `Created app '${app.name}' (id: ${app.id}).${
          (args.project_ids ?? []).length ? ` Added ${(args.project_ids ?? []).length} project(s).` : ""
        }`,
        data: app,
      };
    },
  });

  registerTool(server, ctx, {
    name: "get_app_health",
    description: "Composite snapshot of an app: the app + its member projects with each one's latest preview/production deploy timestamps.",
    inputSchema: { app_id: z.string() },
    annotations: { readOnlyHint: true, title: "App health" },
    handler: async (args) => {
      const health = await ctx.client.request<AppHealth>(`/apps/${args.app_id}/health`);
      const lines = [
        `App: ${health.app.name} (id: ${health.app.id})`,
        ``,
        `Members:`,
        ...(health.members.length
          ? health.members.map(
              (m) =>
                `  - ${m.project.name} [${m.project.status}] preview=${m.latest_preview_deploy_at ?? "(none)"} prod=${m.latest_production_deploy_at ?? "(none)"}`,
            )
          : ["  (none)"]),
      ];
      return { text: lines.join("\n"), data: health };
    },
  });

  registerTool(server, ctx, {
    name: "update_app",
    description: "Rename an app bundle.",
    inputSchema: { app_id: z.string(), name: z.string() },
    annotations: { title: "Update app" },
    handler: async (args) => {
      const app = await ctx.client.request<App>(`/apps/${args.app_id}`, {
        method: "PATCH",
        body: { name: args.name },
      });
      return { text: `Renamed app to '${app.name}' (id: ${app.id}).`, data: app };
    },
  });

  registerTool(server, ctx, {
    name: "delete_app",
    description: "Delete an app bundle. Member projects survive (they become standalone).",
    inputSchema: { app_id: z.string() },
    annotations: { title: "Delete app" },
    handler: async (args) => {
      await ctx.client.request<void>(`/apps/${args.app_id}`, { method: "DELETE" });
      return { text: `Deleted app ${args.app_id}.`, data: { id: args.app_id, deleted: true } };
    },
  });

  registerTool(server, ctx, {
    name: "add_project_to_app",
    description: "Move one or more projects into an app bundle.",
    inputSchema: { app_id: z.string(), project_ids: z.array(z.string()) },
    annotations: { title: "Add projects to app" },
    handler: async (args) => {
      const app = await ctx.client.request<App>(`/apps/${args.app_id}/projects`, {
        method: "POST",
        body: { project_ids: args.project_ids },
      });
      return { text: `Added ${args.project_ids.length} project(s) to '${app.name}'.`, data: app };
    },
  });

  registerTool(server, ctx, {
    name: "remove_project_from_app",
    description: "Detach a project from its app bundle (it becomes standalone).",
    inputSchema: { app_id: z.string(), project_id: z.string() },
    annotations: { title: "Remove project from app" },
    handler: async (args) => {
      await ctx.client.request<void>(`/apps/${args.app_id}/projects/${args.project_id}`, { method: "DELETE" });
      return {
        text: `Removed project ${args.project_id} from app ${args.app_id}.`,
        data: { app_id: args.app_id, project_id: args.project_id, removed: true },
      };
    },
  });
}
```

> Confirm the import extension + `ToolContext`/`registerTool` export names against `mcp/src/tools/groups.ts` (this mirrors it). If `groups.ts` imports from `"./_helpers"` without `.js`, match that.

- [ ] **Step 2: Register the module**

In `mcp/src/tools/index.ts`, add the import at the top with the other tool imports and call it inside `registerAllTools` next to `registerGroupTools(server, ctx);`:

```typescript
import { registerAppTools } from "./apps.js";
```

```typescript
  registerGroupTools(server, ctx);
  registerAppTools(server, ctx);
```

- [ ] **Step 3: Build the MCP package**

Run (from `mcp/`): `cd /Users/your-github-username/Documents/Work/lovinka-deployik/mcp && npm run build`
Expected: TypeScript compiles clean.

> If the repo uses bun/pnpm, use that package manager's build/typecheck script instead (check `mcp/package.json` scripts). Use `git -C` style or run the build without `cd` if a script exists at the repo root.

- [ ] **Step 4: Commit**

```bash
git -C /Users/your-github-username/Documents/Work/lovinka-deployik add mcp/src/tools/apps.ts mcp/src/tools/index.ts
git -C /Users/your-github-username/Documents/Work/lovinka-deployik commit -m "feat(apps): MCP tools (create/list/get-health/update/delete/add/remove)"
```

---

## Final verification

- [ ] **Whole-repo gates**

```bash
cd /Users/your-github-username/Documents/Work/lovinka-deployik
go build ./... && go vet ./... && go test ./...
gofmt -l internal/db/queries_apps.go internal/db/apps_test.go internal/api/handlers/apps.go internal/api/handlers/apps_test.go
```

Expected: build/vet/test all green; `gofmt -l` prints nothing (no formatting drift) for the new files.

- [ ] **Inert-by-default sanity:** confirm an existing project with no app still round-trips — `go test ./internal/db ./internal/api/handlers` stays green, proving `GetProject`/`ListProjects` behave identically when `app_id` is NULL (`COALESCE(... , '')` → empty string).

---

## Self-review (completed by plan author)

- **Spec coverage (P1 slice):** `apps` table + `projects.app_id` (Task 1) ✓; `App`/`Project.AppID` models (Task 2) ✓; App CRUD (Task 3) ✓; project↔app membership + `ListProjectsByApp` + unified read (Task 4 + Task 5 `GetHealth`) ✓; MCP surface `create_app`/`list_apps`/`get_app_health`/`update_app`/`delete_app`/`add_project_to_app`/`remove_project_from_app` (Task 6) ✓. Deferred to later phases per the scope guard: `app_variables`, `project_services.app_id`, `deploy_order`, `build_filter_enabled`, `watch_paths`, `app_releases`.
- **Placeholder scan:** none — every code step shows full code; the two "confirm X" notes (migration embed discovery, module import path, `IsOrganizationMember` existence) are verification guards, not deferred work, each with a concrete fallback.
- **Type consistency:** `App`/`AppCreate`/`Project.AppID` defined in Task 2 are used consistently in Tasks 3–6; `CreateApp`/`GetApp`/`GetAppForUser`/`ListAppsForUser`/`UpdateAppName`/`DeleteApp`/`AddProjectsToApp`/`RemoveProjectFromApp`/`ListProjectsByApp`/`IsOrganizationMember` names match between definition and call sites; the `GetProject`/`ListProjects` SELECT column additions are mirrored 1:1 by their scan-target additions.

## Open assumptions to verify during execution

1. **Migration discovery** — `db.Migrate()` picks up `026_apps.sql` via an embed glob (Task 1 Step 4 guard).
2. **Module path** — internal import prefix is `github.com/LEFTEQ/lovinka-deployik` (Task 5 Step 1 note); adjust if `go.mod` differs.
3. **`EnsurePersonalOrganization`, `UpsertUser`, `uniqueNonEmpty`, `slugifyOrganizationName`, `ErrProjectNotMovable`** exist in package `db` (all observed in `queries_groups.go`/its tests).
4. **`mutationLimiter`** is in scope at the router insertion point (it wraps every existing group route immediately above).
