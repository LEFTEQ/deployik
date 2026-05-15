# Postgres Sidecar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users attach a per-environment Postgres 16 sidecar to a Deployik project with one click, getting an auto-injected `DATABASE_URL`, lifecycle controls (Reset / Restart / Logs / Connect), and SSH-tunnel-only external access — never publicly exposed.

**Architecture:** New `internal/services` Go package owns the Postgres container + named volume per `(project, environment)`. A new `project_services` table tracks the spec + encrypted password + assigned loopback port. The deploy pipeline gains Step 4b that ensures the pg container is healthy before building the app image and auto-injects env vars (user-set keys win). A new `Services` sidebar item under Settings exposes lifecycle ops, and the New Project flow has a single "Attach Postgres database" toggle that fires the same provisioning path.

**Tech Stack:** Go 1.25 + Docker SDK + chi router + SQLite + AES-256-GCM (existing `internal/crypto`); React 19 + TanStack Router/Query + shadcn/ui + Tailwind 4 + Bun.

**Source design doc:** `~/.claude/plans/i-would-like-to-crystalline-trinket.md` (decisions #1, #2, #4, #5, #6, #8 of the questionnaire).

**Out of scope (Phase 3 plan):** Manual backup / restore-from-upload / scheduled backups. The `services` package's `Dump`/`Restore` functions live in Phase 3 — this plan ships Reset (drop volume → recreate empty) as the only data-destructive op.

**Out of scope (defer to a separate cleanup pass):**
- Extracting `projectColumns` const + `scanProject` helper from `queries_projects.go` (carry-over from Phase 1 review O1).
- Adding non-root `USER` step to `generateStaticDockerfile` / `generateNodeAPIDockerfile` (Phase 1 review M2).
- Promoting the `requireAll` test helper to a shared `testutil_test.go` (Phase 1 review O5).

---

## File Structure

| Path | Role | Status |
|---|---|---|
| `internal/db/migrations/023_project_services.sql` | New table + indexes | Create |
| `internal/db/models.go` | `ProjectService` struct + `ServiceType` constants | Modify |
| `internal/db/queries_services.go` | CRUD: `CreateService`, `GetService`, `ListServicesByProject`, `UpdateServiceHostPort`, `UpdateServiceStatus`, `UpdateServicePassword`, `DeleteService`, `ServicesExist` | Create |
| `internal/db/queries_services_test.go` | DB roundtrip + uniqueness tests | Create |
| `internal/db/queries_projects.go:287-292` | Extend rename guard to reject when `ServicesExist(projectID)` | Modify |
| `internal/services/types.go` | `ServiceSpec`, `EnvInjection`, container/volume naming helpers | Create |
| `internal/services/env_merge.go` | `MergeWithUserOverride` pure function | Create |
| `internal/services/env_merge_test.go` | User-override precedence tests | Create |
| `internal/services/postgres.go` | `EnsureRunning`, `Stop`, `Restart`, `Reset`, `Logs`, `WaitReady`, `BuildEnvInjection` | Create |
| `internal/services/postgres_test.go` | docker-gated integration tests (`t.Skip` when no docker) | Create |
| `internal/services/manager.go` | DB-aware facade: `GetSpec`, `Provision`, `Delete`, `EnsureForDeployment` | Create |
| `internal/build/pipeline.go` | Add `Services *services.Manager` field + Step 4b before image build | Modify |
| `internal/api/handlers/services.go` | `ServiceHandler` with List / Attach / Detach / Credentials / Regenerate / Restart / Reset | Create |
| `internal/api/handlers/services_test.go` | Authz + typed-confirm + audit tests | Create |
| `internal/api/handlers/projects.go` | Extend `DeleteProject` cleanup path to stop/remove pg | Modify |
| `internal/api/router.go` | Wire `ServiceHandler` routes + WS logs route | Modify |
| `internal/ws/services_logs.go` | WS handler streaming `docker logs --follow` of the pg container | Create |
| `cmd/server/main.go` | Instantiate `services.Manager` + thread into Pipeline + router config | Modify |
| `web/src/types/api.ts` | `ProjectService`, `ServiceCredentials`, `ServiceType`, request/response shapes | Modify |
| `web/src/lib/api.ts` | `services.list / attach / detach / credentials / regeneratePassword / restart / reset` | Modify |
| `web/src/lib/queryKeys.ts` | `projectServices(projectId)` query key | Modify |
| `web/src/components/layout/AppSidebar.tsx` | Add `Services` nav item under Settings (route `/projects/$id/settings/services`) | Modify |
| `web/src/app/app.tsx` | Register the new route in the TanStack route tree | Modify |
| `web/src/pages/ProjectSettingsServices.tsx` | New page rendering per-env service cards | Create |
| `web/src/components/projects/services/ServicesPanel.tsx` | Per-env card grid with status pill + action buttons | Create |
| `web/src/components/projects/services/CredentialsDialog.tsx` | Connect dialog with DSN copy + SSH tunnel command + Regenerate Password | Create |
| `web/src/components/projects/services/ServiceLogsDrawer.tsx` | WebSocket consumer for pg container logs | Create |
| `web/src/pages/NewProject.tsx` | `Attach Postgres database` switch + post-create POST to /services | Modify |

---

## Task 1: Migration 023 — `project_services` table

**Files:**
- Create: `internal/db/migrations/023_project_services.sql`

- [ ] **Step 1: Create the migration file**

Write to `internal/db/migrations/023_project_services.sql`:

```sql
-- Migration 023: per-project service sidecars (Postgres in v1; reserved for
-- redis/mysql later via service_type discriminator).
--
-- One row per (project, environment, service_type). The Postgres container
-- and its named data volume are deterministic from those keys
-- ("deployik-<project>-<env>-pg" / "deployik-<project>-<env>-pg-data") so the
-- row doesn't need to store them — only the credentials and assigned host
-- loopback port. Passwords are AES-256-GCM encrypted via internal/crypto,
-- never logged, never returned by list endpoints — only by an explicit
-- credentials reveal.
--
-- host_port is the random :0 binding Docker assigned on first start. It's
-- restored on container restart by re-running services.EnsureRunning, which
-- reads the live port via DockerClient.GetHostPort and updates this row.
-- Stored as int (0 = "not started yet") rather than nullable to keep the
-- queries simple.
--
-- config_json is reserved for future Postgres knobs (shared_buffers,
-- max_connections) and Redis settings — empty {} on insert so the column
-- never has to be nullable.
CREATE TABLE project_services (
  id                    TEXT PRIMARY KEY,
  project_id            TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  environment           TEXT NOT NULL CHECK (environment IN ('preview','production')),
  service_type          TEXT NOT NULL CHECK (service_type IN ('postgres')),
  image                 TEXT NOT NULL DEFAULT 'postgres:16',
  db_name               TEXT NOT NULL,
  db_user               TEXT NOT NULL,
  db_password_encrypted TEXT NOT NULL,
  host_port             INTEGER NOT NULL DEFAULT 0,
  config_json           TEXT NOT NULL DEFAULT '{}',
  status                TEXT NOT NULL DEFAULT 'pending'
                          CHECK (status IN ('pending','running','stopped','failed')),
  last_started_at       DATETIME,
  created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(project_id, environment, service_type)
);
CREATE INDEX idx_project_services_project ON project_services(project_id);
```

- [ ] **Step 2: Verify the migration runs**

Run: `cd /Users/your-github-username/Documents/Work/lovinka-deployik && go test ./internal/db/ -run TestMigrations -v`

Expected: PASS — migration applies cleanly after 022.

- [ ] **Step 3: Commit**

```bash
git add internal/db/migrations/023_project_services.sql
git commit -m "feat(db): add project_services table for postgres sidecar"
```

---

## Task 2: `ProjectService` struct + constants

**Files:**
- Modify: `internal/db/models.go`

- [ ] **Step 1: Add type definitions**

Append to `internal/db/models.go` (after the `Project` struct + helpers, somewhere in the middle of the file — match the existing pattern of one struct per "concept"):

```go
// ServiceType identifies a sidecar service kind. v1 ships postgres only;
// "redis", "mysql" etc. are reserved for follow-up plans.
type ServiceType string

const (
	ServiceTypePostgres ServiceType = "postgres"
)

// ServiceStatus mirrors the CHECK constraint in migration 023.
type ServiceStatus string

const (
	ServiceStatusPending ServiceStatus = "pending"
	ServiceStatusRunning ServiceStatus = "running"
	ServiceStatusStopped ServiceStatus = "stopped"
	ServiceStatusFailed  ServiceStatus = "failed"
)

// ProjectService is one row of project_services. db_password_encrypted is
// AES-256-GCM ciphertext (decrypt via crypto.Encryptor); never expose in JSON.
type ProjectService struct {
	ID                  string         `json:"id"`
	ProjectID           string         `json:"project_id"`
	Environment         string         `json:"environment"`
	ServiceType         ServiceType    `json:"service_type"`
	Image               string         `json:"image"`
	DBName              string         `json:"db_name"`
	DBUser              string         `json:"db_user"`
	DBPasswordEncrypted string         `json:"-"` // ciphertext, never in JSON
	HostPort            int            `json:"host_port"`
	ConfigJSON          string         `json:"config_json,omitempty"`
	Status              ServiceStatus  `json:"status"`
	LastStartedAt       NullableTime   `json:"last_started_at"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}
```

- [ ] **Step 2: Compile**

Run: `go build ./...`

Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add internal/db/models.go
git commit -m "feat(db): add ProjectService struct + ServiceType/Status enums"
```

---

## Task 3: `queries_services.go` CRUD + roundtrip test

**Files:**
- Create: `internal/db/queries_services.go`
- Create: `internal/db/queries_services_test.go`

- [ ] **Step 1: Write the failing roundtrip test**

Create `internal/db/queries_services_test.go`:

```go
package db

import (
	"testing"
)

func TestProjectServiceRoundtrip(t *testing.T) {
	t.Parallel()

	database := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 1, Username: "tester", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	project := &Project{
		Name:        "pg-roundtrip",
		GithubRepo:  "sample",
		GithubOwner: "tester",
		Branch:      "main",
		UserID:      user.ID,
		Framework:   "node-api",
		Port:        3000,
		Status:      "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	svc := &ProjectService{
		ProjectID:           project.ID,
		Environment:         "preview",
		ServiceType:         ServiceTypePostgres,
		Image:               "postgres:16",
		DBName:              "app",
		DBUser:              "app",
		DBPasswordEncrypted: "ciphertext-placeholder",
		HostPort:            0,
		ConfigJSON:          "{}",
		Status:              ServiceStatusPending,
	}
	if err := database.CreateService(svc); err != nil {
		t.Fatalf("CreateService: %v", err)
	}
	if svc.ID == "" {
		t.Fatal("CreateService did not assign ID")
	}

	got, err := database.GetService(svc.ID)
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}
	if got == nil {
		t.Fatal("GetService returned nil")
	}
	if got.Environment != "preview" || got.ServiceType != ServiceTypePostgres {
		t.Errorf("roundtrip mismatch: %+v", got)
	}

	// UNIQUE(project_id, environment, service_type) must reject a duplicate.
	dup := *svc
	dup.ID = ""
	if err := database.CreateService(&dup); err == nil {
		t.Fatal("expected UNIQUE constraint to reject duplicate (project, env, type)")
	}

	// List should return exactly one row.
	list, err := database.ListServicesByProject(project.ID)
	if err != nil {
		t.Fatalf("ListServicesByProject: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 service, got %d", len(list))
	}

	// Update host port + status.
	if err := database.UpdateServiceHostPort(svc.ID, 54321); err != nil {
		t.Fatalf("UpdateServiceHostPort: %v", err)
	}
	if err := database.UpdateServiceStatus(svc.ID, ServiceStatusRunning); err != nil {
		t.Fatalf("UpdateServiceStatus: %v", err)
	}
	got, _ = database.GetService(svc.ID)
	if got.HostPort != 54321 {
		t.Errorf("HostPort after update = %d", got.HostPort)
	}
	if got.Status != ServiceStatusRunning {
		t.Errorf("Status after update = %s", got.Status)
	}

	// Update password.
	if err := database.UpdateServicePassword(svc.ID, "new-ciphertext"); err != nil {
		t.Fatalf("UpdateServicePassword: %v", err)
	}
	got, _ = database.GetService(svc.ID)
	if got.DBPasswordEncrypted != "new-ciphertext" {
		t.Errorf("password not updated")
	}

	// ServicesExist must report true.
	exists, err := database.ServicesExist(project.ID)
	if err != nil {
		t.Fatalf("ServicesExist: %v", err)
	}
	if !exists {
		t.Fatal("ServicesExist should be true after CreateService")
	}

	// Delete + verify gone.
	if err := database.DeleteService(svc.ID); err != nil {
		t.Fatalf("DeleteService: %v", err)
	}
	got, _ = database.GetService(svc.ID)
	if got != nil {
		t.Fatal("GetService should return nil after DeleteService")
	}
	exists, _ = database.ServicesExist(project.ID)
	if exists {
		t.Fatal("ServicesExist should be false after DeleteService")
	}
}
```

- [ ] **Step 2: Run the test — confirm FAIL**

Run: `go test ./internal/db/ -run TestProjectServiceRoundtrip -v`

Expected: FAIL — `CreateService`, `GetService`, etc. are undefined.

- [ ] **Step 3: Implement `queries_services.go`**

Create `internal/db/queries_services.go`:

```go
package db

import (
	"database/sql"
	"fmt"
)

// CreateService inserts a new project_services row. Assigns p.ID if empty.
func (db *DB) CreateService(s *ProjectService) error {
	if s.ID == "" {
		s.ID = NewID()
	}
	if s.ConfigJSON == "" {
		s.ConfigJSON = "{}"
	}
	if s.Status == "" {
		s.Status = ServiceStatusPending
	}
	_, err := db.Exec(
		`INSERT INTO project_services (id, project_id, environment, service_type, image,
			db_name, db_user, db_password_encrypted, host_port, config_json, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.ProjectID, s.Environment, string(s.ServiceType), s.Image,
		s.DBName, s.DBUser, s.DBPasswordEncrypted, s.HostPort, s.ConfigJSON, string(s.Status),
	)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	return nil
}

// GetService returns a single service by id, or (nil, nil) when absent.
func (db *DB) GetService(id string) (*ProjectService, error) {
	s := &ProjectService{}
	var lastStarted sql.NullString
	err := db.QueryRow(
		`SELECT id, project_id, environment, service_type, image,
		        db_name, db_user, db_password_encrypted, host_port, config_json, status,
		        last_started_at, created_at, updated_at
		 FROM project_services WHERE id = ?`, id,
	).Scan(
		&s.ID, &s.ProjectID, &s.Environment, &s.ServiceType, &s.Image,
		&s.DBName, &s.DBUser, &s.DBPasswordEncrypted, &s.HostPort, &s.ConfigJSON, &s.Status,
		&lastStarted, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get service: %w", err)
	}
	if lastStarted.Valid {
		if t := parseSQLiteDateTime(lastStarted.String); t != nil {
			s.LastStartedAt = NullableTime{Time: *t, Valid: true}
		}
	}
	return s, nil
}

// GetServiceByProjectEnv looks up a service by its natural key.
func (db *DB) GetServiceByProjectEnv(projectID, environment string, svcType ServiceType) (*ProjectService, error) {
	s := &ProjectService{}
	var lastStarted sql.NullString
	err := db.QueryRow(
		`SELECT id, project_id, environment, service_type, image,
		        db_name, db_user, db_password_encrypted, host_port, config_json, status,
		        last_started_at, created_at, updated_at
		 FROM project_services
		 WHERE project_id = ? AND environment = ? AND service_type = ?`,
		projectID, environment, string(svcType),
	).Scan(
		&s.ID, &s.ProjectID, &s.Environment, &s.ServiceType, &s.Image,
		&s.DBName, &s.DBUser, &s.DBPasswordEncrypted, &s.HostPort, &s.ConfigJSON, &s.Status,
		&lastStarted, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get service by project+env: %w", err)
	}
	if lastStarted.Valid {
		if t := parseSQLiteDateTime(lastStarted.String); t != nil {
			s.LastStartedAt = NullableTime{Time: *t, Valid: true}
		}
	}
	return s, nil
}

// ListServicesByProject returns all services across both environments.
func (db *DB) ListServicesByProject(projectID string) ([]ProjectService, error) {
	rows, err := db.Query(
		`SELECT id, project_id, environment, service_type, image,
		        db_name, db_user, db_password_encrypted, host_port, config_json, status,
		        last_started_at, created_at, updated_at
		 FROM project_services
		 WHERE project_id = ?
		 ORDER BY environment, service_type`, projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	defer rows.Close()

	var out []ProjectService
	for rows.Next() {
		var s ProjectService
		var lastStarted sql.NullString
		if err := rows.Scan(
			&s.ID, &s.ProjectID, &s.Environment, &s.ServiceType, &s.Image,
			&s.DBName, &s.DBUser, &s.DBPasswordEncrypted, &s.HostPort, &s.ConfigJSON, &s.Status,
			&lastStarted, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan service: %w", err)
		}
		if lastStarted.Valid {
			if t := parseSQLiteDateTime(lastStarted.String); t != nil {
				s.LastStartedAt = NullableTime{Time: *t, Valid: true}
			}
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// UpdateServiceHostPort persists the loopback host port assigned by Docker
// after EnsureRunning. last_started_at is bumped to NOW so the UI can show
// "last restarted X ago".
func (db *DB) UpdateServiceHostPort(id string, hostPort int) error {
	_, err := db.Exec(
		`UPDATE project_services
		 SET host_port = ?, last_started_at = datetime('now'), updated_at = datetime('now')
		 WHERE id = ?`, hostPort, id,
	)
	if err != nil {
		return fmt.Errorf("update service host_port: %w", err)
	}
	return nil
}

// UpdateServiceStatus persists a status transition.
func (db *DB) UpdateServiceStatus(id string, status ServiceStatus) error {
	_, err := db.Exec(
		`UPDATE project_services
		 SET status = ?, updated_at = datetime('now')
		 WHERE id = ?`, string(status), id,
	)
	if err != nil {
		return fmt.Errorf("update service status: %w", err)
	}
	return nil
}

// UpdateServicePassword replaces the encrypted password (used by regenerate).
func (db *DB) UpdateServicePassword(id, encrypted string) error {
	_, err := db.Exec(
		`UPDATE project_services
		 SET db_password_encrypted = ?, updated_at = datetime('now')
		 WHERE id = ?`, encrypted, id,
	)
	if err != nil {
		return fmt.Errorf("update service password: %w", err)
	}
	return nil
}

// DeleteService removes the row. The container + volume are cleaned up
// separately by the services.Manager before this is called.
func (db *DB) DeleteService(id string) error {
	_, err := db.Exec(`DELETE FROM project_services WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	return nil
}

// ServicesExist reports whether the project has ANY service rows. Used by the
// rename guard — renaming a project would orphan its container + volume names.
func (db *DB) ServicesExist(projectID string) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM project_services WHERE project_id = ?`, projectID,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("services exist: %w", err)
	}
	return n > 0, nil
}
```

- [ ] **Step 4: Run the test — confirm PASS**

Run: `go test ./internal/db/ -run TestProjectServiceRoundtrip -v`

Expected: PASS.

- [ ] **Step 5: Full DB suite stays green**

Run: `go test ./internal/db/...`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/db/queries_services.go internal/db/queries_services_test.go
git commit -m "feat(db): CRUD queries for project_services"
```

---

## Task 4: `internal/services` package skeleton + types

**Files:**
- Create: `internal/services/types.go`

- [ ] **Step 1: Create the package + types**

Create `internal/services/types.go`:

```go
// Package services manages per-project sidecar containers (Postgres in v1).
// Lifecycle ops (EnsureRunning, Stop, Restart, Reset, Logs) live here rather
// than internal/build so handlers can call them without importing the deploy
// pipeline.
package services

import (
	"fmt"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// ServiceSpec is a fully-resolved (plaintext-password) sidecar specification.
// Built by Manager methods from a db.ProjectService row + crypto.Encryptor.
type ServiceSpec struct {
	ServiceID         string
	ProjectID         string
	ProjectName       string
	Environment       string // "preview" or "production"
	Type              db.ServiceType
	Image             string
	DBName            string
	DBUser            string
	DBPasswordPlain   string // decrypted; never persist back to disk as-is
	HostPort          int    // loopback host port; 0 means "not yet running"
	ContainerName     string // deployik-<project>-<env>-pg
	VolumeName        string // deployik-<project>-<env>-pg-data
}

// EnvInjection holds the env vars that get merged into the app container's
// runtime environment. POSTGRES_PASSWORD is split out so handlers can decide
// whether to expose it as a secret-store variable; the rest are plain env.
type EnvInjection struct {
	Env     map[string]string // DATABASE_URL, POSTGRES_HOST/PORT/DB/USER
	Secrets map[string]string // POSTGRES_PASSWORD
}

// PostgresContainerName returns the deterministic container name for the
// Postgres sidecar of a (project, environment) pair.
func PostgresContainerName(projectName, environment string) string {
	return fmt.Sprintf("deployik-%s-%s-pg", projectName, environment)
}

// PostgresVolumeName returns the deterministic Docker volume name for the
// Postgres data directory of a (project, environment) pair.
func PostgresVolumeName(projectName, environment string) string {
	return fmt.Sprintf("deployik-%s-%s-pg-data", projectName, environment)
}
```

- [ ] **Step 2: Compile**

Run: `go build ./...`

Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add internal/services/types.go
git commit -m "feat(services): package skeleton + ServiceSpec/EnvInjection types"
```

---

## Task 5: `MergeWithUserOverride` pure function + test

**Files:**
- Create: `internal/services/env_merge.go`
- Create: `internal/services/env_merge_test.go`

- [ ] **Step 1: Failing test**

Create `internal/services/env_merge_test.go`:

```go
package services

import (
	"strings"
	"testing"
)

func TestMergeWithUserOverrideInjectsAllKeysWhenUserMapEmpty(t *testing.T) {
	t.Parallel()

	inj := EnvInjection{
		Env:     map[string]string{"DATABASE_URL": "postgres://app:pwd@host:5432/app", "POSTGRES_HOST": "host"},
		Secrets: map[string]string{"POSTGRES_PASSWORD": "pwd"},
	}
	merged := MergeWithUserOverride(nil, inj)
	if len(merged) != 3 {
		t.Fatalf("expected 3 vars, got %d (%v)", len(merged), merged)
	}
	if findVar(merged, "DATABASE_URL") != "postgres://app:pwd@host:5432/app" {
		t.Errorf("DATABASE_URL missing or wrong")
	}
	if findVar(merged, "POSTGRES_PASSWORD") != "pwd" {
		t.Errorf("POSTGRES_PASSWORD missing or wrong")
	}
}

func TestMergeWithUserOverrideUserKeysWin(t *testing.T) {
	t.Parallel()

	inj := EnvInjection{
		Env:     map[string]string{"DATABASE_URL": "postgres://injected"},
		Secrets: map[string]string{"POSTGRES_PASSWORD": "injected"},
	}
	// User has DATABASE_URL set to a managed Neon DB; injection must NOT clobber it.
	userVars := []string{"DATABASE_URL=postgres://user-managed", "OTHER_VAR=keep"}
	merged := MergeWithUserOverride(userVars, inj)

	if findVar(merged, "DATABASE_URL") != "postgres://user-managed" {
		t.Errorf("user DATABASE_URL should win, got %q", findVar(merged, "DATABASE_URL"))
	}
	if findVar(merged, "POSTGRES_PASSWORD") != "injected" {
		t.Errorf("POSTGRES_PASSWORD (not user-set) should be injected, got %q", findVar(merged, "POSTGRES_PASSWORD"))
	}
	if findVar(merged, "OTHER_VAR") != "keep" {
		t.Errorf("unrelated user var lost: %v", merged)
	}
}

func TestMergeWithUserOverrideSecretsObeyUserOverride(t *testing.T) {
	t.Parallel()

	inj := EnvInjection{
		Secrets: map[string]string{"POSTGRES_PASSWORD": "injected"},
	}
	merged := MergeWithUserOverride([]string{"POSTGRES_PASSWORD=user-set"}, inj)
	if findVar(merged, "POSTGRES_PASSWORD") != "user-set" {
		t.Errorf("user POSTGRES_PASSWORD should win, got %q", findVar(merged, "POSTGRES_PASSWORD"))
	}
}

// findVar returns the VALUE of NAME from a slice of "KEY=VAL" strings, or ""
// if missing. Mirrors how Docker accepts envVars in RunContainer.
func findVar(envVars []string, name string) string {
	prefix := name + "="
	for _, v := range envVars {
		if strings.HasPrefix(v, prefix) {
			return v[len(prefix):]
		}
	}
	return ""
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `go test ./internal/services/ -run TestMergeWithUserOverride -v`

Expected: FAIL — `MergeWithUserOverride` undefined.

- [ ] **Step 3: Implement**

Create `internal/services/env_merge.go`:

```go
package services

import "strings"

// MergeWithUserOverride composes the final runtime env-var slice for the app
// container. User-set keys (already in userVars, "KEY=VAL" shape) ALWAYS win
// over injected values — both for plain env and secrets. Injected keys not
// already present in userVars are appended.
//
// The returned slice has the user's vars first (preserving their relative
// order), followed by the injected non-conflicting keys in alphabetical order
// so the result is deterministic for tests + audit diffing.
func MergeWithUserOverride(userVars []string, inj EnvInjection) []string {
	seen := make(map[string]struct{}, len(userVars))
	for _, v := range userVars {
		if i := strings.IndexByte(v, '='); i > 0 {
			seen[v[:i]] = struct{}{}
		}
	}

	result := make([]string, 0, len(userVars)+len(inj.Env)+len(inj.Secrets))
	result = append(result, userVars...)

	// Walk injection in deterministic order so tests + audit logs are stable.
	for _, key := range sortedKeys(inj.Env) {
		if _, exists := seen[key]; exists {
			continue
		}
		result = append(result, key+"="+inj.Env[key])
	}
	for _, key := range sortedKeys(inj.Secrets) {
		if _, exists := seen[key]; exists {
			continue
		}
		result = append(result, key+"="+inj.Secrets[key])
	}
	return result
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// stdlib sort.Strings would do, but we keep zero external deps in this file.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}
```

- [ ] **Step 4: Run — confirm PASS**

Run: `go test ./internal/services/ -run TestMergeWithUserOverride -v`

Expected: PASS (3 cases).

- [ ] **Step 5: Commit**

```bash
git add internal/services/env_merge.go internal/services/env_merge_test.go
git commit -m "feat(services): MergeWithUserOverride with user-key precedence"
```

---

## Task 6: `BuildEnvInjection` for Postgres

**Files:**
- Create: `internal/services/postgres.go` (skeleton + this function)
- Modify: `internal/services/env_merge_test.go` (add a Postgres-specific test)

- [ ] **Step 1: Add the BuildEnvInjection test**

Append to `internal/services/env_merge_test.go`:

```go
func TestBuildPostgresEnvInjection(t *testing.T) {
	t.Parallel()

	spec := ServiceSpec{
		ContainerName:   "deployik-myapp-preview-pg",
		DBName:          "app",
		DBUser:          "app",
		DBPasswordPlain: "s3cr3t",
	}
	inj := BuildPostgresEnvInjection(spec)

	if got := inj.Env["DATABASE_URL"]; got != "postgresql://app:s3cr3t@deployik-myapp-preview-pg:5432/app" {
		t.Errorf("DATABASE_URL = %q", got)
	}
	if got := inj.Env["POSTGRES_HOST"]; got != "deployik-myapp-preview-pg" {
		t.Errorf("POSTGRES_HOST = %q", got)
	}
	if got := inj.Env["POSTGRES_PORT"]; got != "5432" {
		t.Errorf("POSTGRES_PORT = %q", got)
	}
	if got := inj.Env["POSTGRES_DB"]; got != "app" {
		t.Errorf("POSTGRES_DB = %q", got)
	}
	if got := inj.Env["POSTGRES_USER"]; got != "app" {
		t.Errorf("POSTGRES_USER = %q", got)
	}
	if _, present := inj.Env["POSTGRES_PASSWORD"]; present {
		t.Error("POSTGRES_PASSWORD should be in Secrets, not Env")
	}
	if got := inj.Secrets["POSTGRES_PASSWORD"]; got != "s3cr3t" {
		t.Errorf("POSTGRES_PASSWORD secret = %q", got)
	}
}

func TestBuildPostgresEnvInjectionEscapesPasswordInDSN(t *testing.T) {
	t.Parallel()

	spec := ServiceSpec{
		ContainerName:   "deployik-myapp-preview-pg",
		DBName:          "app",
		DBUser:          "app",
		DBPasswordPlain: "p@ss word/with:colon",
	}
	inj := BuildPostgresEnvInjection(spec)
	got := inj.Env["DATABASE_URL"]
	// The plaintext characters @, /, :, and space MUST be percent-encoded.
	// If they appear unencoded, parsers like pgx or libpq will misread the DSN.
	if strings.Contains(got, "@ss word/with:colon") {
		t.Fatalf("DATABASE_URL leaks unencoded password chars: %q", got)
	}
	if !strings.Contains(got, "p%40ss%20word%2Fwith%3Acolon") {
		t.Errorf("expected percent-encoded password in DSN, got %q", got)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `go test ./internal/services/ -run TestBuildPostgresEnvInjection -v`

Expected: FAIL — `BuildPostgresEnvInjection` undefined.

- [ ] **Step 3: Implement**

Create `internal/services/postgres.go`:

```go
package services

import (
	"fmt"
	"net/url"
)

// PostgresPort is the in-network TCP port Postgres listens on. The same value
// goes into DATABASE_URL (the in-network DSN); the loopback host_port
// recorded on the row is only used for SSH-tunnel external access.
const PostgresPort = 5432

// BuildPostgresEnvInjection composes the env vars that get merged into the
// app container's runtime environment when this spec's sidecar is attached.
// DATABASE_URL uses url.UserPassword so the password is percent-encoded for
// special chars (@, /, :, space, etc) — the spec's plaintext password is
// random base64url today (see Manager.Provision), so this is belt-and-suspenders
// against future password schemes.
func BuildPostgresEnvInjection(spec ServiceSpec) EnvInjection {
	dsn := (&url.URL{
		Scheme: "postgresql",
		User:   url.UserPassword(spec.DBUser, spec.DBPasswordPlain),
		Host:   fmt.Sprintf("%s:%d", spec.ContainerName, PostgresPort),
		Path:   "/" + spec.DBName,
	}).String()

	return EnvInjection{
		Env: map[string]string{
			"DATABASE_URL":  dsn,
			"POSTGRES_HOST": spec.ContainerName,
			"POSTGRES_PORT": fmt.Sprintf("%d", PostgresPort),
			"POSTGRES_DB":   spec.DBName,
			"POSTGRES_USER": spec.DBUser,
		},
		Secrets: map[string]string{
			"POSTGRES_PASSWORD": spec.DBPasswordPlain,
		},
	}
}
```

- [ ] **Step 4: Run — confirm PASS**

Run: `go test ./internal/services/ -run TestBuildPostgresEnvInjection -v`

Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/services/postgres.go internal/services/env_merge_test.go
git commit -m "feat(services): BuildPostgresEnvInjection composes DSN + discrete vars"
```

---

## Task 7: `EnsureRunning` and `WaitReady`

**Files:**
- Modify: `internal/services/postgres.go`
- Create: `internal/services/postgres_docker_test.go` (docker-gated)

- [ ] **Step 1: Implement `EnsureRunning`**

Append to `internal/services/postgres.go`:

```go
import (
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/errdefs"

	"github.com/LEFTEQ/lovinka-deployik/internal/build"
)
```

(Replace the existing `import "fmt"` / `import "net/url"` block with the wider import set; keep `PostgresPort` and `BuildPostgresEnvInjection` exactly as Task 6 wrote them.)

Then add:

```go
// PostgresImage is the pinned image tag — v1 uses postgres:16. Changing this
// across an existing fleet would require a pg_upgrade strategy; out of scope.
const PostgresImage = "postgres:16"

// waitReadyTimeout caps how long EnsureRunning waits for pg_isready before
// declaring the sidecar failed. 60s is generous on cold-start; warm restarts
// usually pass on the second poll (~2s).
const waitReadyTimeout = 60 * time.Second

// EnsureRunning brings the Postgres sidecar for spec to a healthy state.
// Idempotent: if the container exists and is running it returns the live
// host port; if the container exists but is stopped it starts it; if it
// doesn't exist it creates the volume, pulls the image lazily, runs the
// container, and waits for pg_isready.
//
// On success the live host_port (from docker inspect) is set on spec.HostPort.
// Callers should persist it via db.UpdateServiceHostPort.
func EnsureRunning(ctx context.Context, docker *build.DockerClient, proxyNetwork string, spec *ServiceSpec) error {
	if id, running := docker.ContainerExists(ctx, spec.ContainerName); running {
		port, err := docker.GetHostPort(ctx, id, PostgresPort)
		if err != nil {
			return fmt.Errorf("inspect host port for running pg: %w", err)
		}
		hp, _ := strconv.Atoi(port)
		spec.HostPort = hp
		return nil
	}

	if err := docker.EnsureVolume(ctx, spec.VolumeName); err != nil {
		return fmt.Errorf("ensure pg volume: %w", err)
	}

	if err := ensurePostgresImage(ctx, docker); err != nil {
		return fmt.Errorf("pull postgres image: %w", err)
	}

	envVars := []string{
		"POSTGRES_DB=" + spec.DBName,
		"POSTGRES_USER=" + spec.DBUser,
		"POSTGRES_PASSWORD=" + spec.DBPasswordPlain,
		"PGDATA=/var/lib/postgresql/data/pgdata",
	}
	mountSpec := spec.VolumeName + ":/var/lib/postgresql/data"

	containerID, err := docker.RunContainer(ctx, spec.ContainerName, PostgresImage, envVars, proxyNetwork, build.RunContainerOptions{
		VolumeBinds:  []string{mountSpec},
		Port:         PostgresPort,
		BindHostPort: true, // bind to 127.0.0.1:<random> for SSH-tunnel access
	})
	if err != nil {
		return fmt.Errorf("run pg container: %w", err)
	}

	if err := WaitReady(ctx, spec, waitReadyTimeout); err != nil {
		return fmt.Errorf("pg not ready in %s: %w", waitReadyTimeout, err)
	}

	port, err := docker.GetHostPort(ctx, containerID, PostgresPort)
	if err != nil {
		return fmt.Errorf("inspect host port after start: %w", err)
	}
	hp, _ := strconv.Atoi(port)
	spec.HostPort = hp
	return nil
}

// ensurePostgresImage lazily pulls postgres:16 if it isn't already present.
// Uses `docker pull` via exec rather than ImagePull's stream-decode loop
// because we don't need to surface progress — the deploy log shows a single
// "Pulling postgres:16..." line.
func ensurePostgresImage(ctx context.Context, docker *build.DockerClient) error {
	pullCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(pullCtx, "docker", "pull", PostgresImage)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// WaitReady polls pg_isready inside the container until it succeeds or the
// timeout elapses. Uses `docker exec` rather than connecting through the
// network because the loopback host_port may not be assigned yet at this
// point — and we don't want to depend on the proxy network being reachable
// from the deployik process itself.
func WaitReady(ctx context.Context, spec *ServiceSpec, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		cmd := exec.CommandContext(probeCtx, "docker", "exec", spec.ContainerName,
			"pg_isready", "-U", spec.DBUser, "-d", spec.DBName)
		out, err := cmd.CombinedOutput()
		cancel()
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
		_ = out // discarded; tail is in the next iteration's error if it persists
	}
	return fmt.Errorf("pg_isready did not succeed within %s", timeout)
}

// Stop halts and removes the Postgres container. The named volume is NOT
// removed — call ResetData to also wipe data.
func Stop(ctx context.Context, docker *build.DockerClient, spec *ServiceSpec) error {
	id, _ := docker.ContainerExists(ctx, spec.ContainerName)
	if id == "" {
		return nil // already gone
	}
	return docker.StopContainer(ctx, id)
}

// Restart stops and re-runs the container, preserving volume data. Used by
// the [Restart] button.
func Restart(ctx context.Context, docker *build.DockerClient, proxyNetwork string, spec *ServiceSpec) error {
	if err := Stop(ctx, docker, spec); err != nil {
		return fmt.Errorf("stop before restart: %w", err)
	}
	return EnsureRunning(ctx, docker, proxyNetwork, spec)
}

// ResetData removes the container AND the named volume, then recreates an
// empty Postgres instance. Used by [Reset] (typed-confirm in the UI).
func ResetData(ctx context.Context, docker *build.DockerClient, proxyNetwork string, spec *ServiceSpec) error {
	if err := Stop(ctx, docker, spec); err != nil {
		return fmt.Errorf("stop before reset: %w", err)
	}
	if err := docker.RemoveVolume(ctx, spec.VolumeName); err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("remove pg volume: %w", err)
	}
	return EnsureRunning(ctx, docker, proxyNetwork, spec)
}

// Logs streams `docker logs --follow` of the pg container into w until ctx is
// cancelled or the container exits. Used by the WS handler at
// /ws/projects/{id}/services/{env}/logs.
func Logs(ctx context.Context, spec *ServiceSpec, w io.Writer) error {
	cmd := exec.CommandContext(ctx, "docker", "logs", "--follow", "--tail", "200", spec.ContainerName)
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil && ctx.Err() == nil {
		log.Printf("services.Logs: docker logs %s ended: %v", spec.ContainerName, err)
		return err
	}
	return nil
}
```

- [ ] **Step 2: Add a docker-gated integration test**

Create `internal/services/postgres_docker_test.go`:

```go
package services

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// requireDocker skips the test when `docker` isn't on PATH or the daemon
// isn't reachable. Matches how internal/build skips its docker-touching tests.
func requireDocker(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("short mode skips docker tests")
	}
	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		t.Skipf("docker not available: %v", err)
	}
}

func TestEnsureRunningIdempotent(t *testing.T) {
	requireDocker(t)
	t.Parallel()

	docker, err := build.NewDockerClient()
	if err != nil {
		t.Fatalf("NewDockerClient: %v", err)
	}
	defer docker.Close()

	spec := &ServiceSpec{
		ProjectName:     "pgtest-" + db.NewID()[:8],
		Environment:     "preview",
		Type:            db.ServiceTypePostgres,
		Image:           PostgresImage,
		DBName:          "app",
		DBUser:          "app",
		DBPasswordPlain: "test-" + db.NewID()[:8],
	}
	spec.ContainerName = PostgresContainerName(spec.ProjectName, spec.Environment)
	spec.VolumeName = PostgresVolumeName(spec.ProjectName, spec.Environment)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = Stop(ctx, docker, spec)
		_ = docker.RemoveVolume(ctx, spec.VolumeName)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := EnsureRunning(ctx, docker, "", spec); err != nil {
		t.Fatalf("first EnsureRunning: %v", err)
	}
	if spec.HostPort <= 0 {
		t.Fatalf("HostPort not set after EnsureRunning: %d", spec.HostPort)
	}
	port1 := spec.HostPort

	// Second call should be a no-op and preserve the host port.
	if err := EnsureRunning(ctx, docker, "", spec); err != nil {
		t.Fatalf("second EnsureRunning: %v", err)
	}
	if spec.HostPort != port1 {
		t.Errorf("HostPort changed across idempotent calls: %d → %d", port1, spec.HostPort)
	}
}

func TestResetDataWipesVolume(t *testing.T) {
	requireDocker(t)
	t.Parallel()

	docker, err := build.NewDockerClient()
	if err != nil {
		t.Fatalf("NewDockerClient: %v", err)
	}
	defer docker.Close()

	spec := &ServiceSpec{
		ProjectName:     "pgreset-" + db.NewID()[:8],
		Environment:     "preview",
		Type:            db.ServiceTypePostgres,
		Image:           PostgresImage,
		DBName:          "app",
		DBUser:          "app",
		DBPasswordPlain: "test-" + db.NewID()[:8],
	}
	spec.ContainerName = PostgresContainerName(spec.ProjectName, spec.Environment)
	spec.VolumeName = PostgresVolumeName(spec.ProjectName, spec.Environment)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = Stop(ctx, docker, spec)
		_ = docker.RemoveVolume(ctx, spec.VolumeName)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	if err := EnsureRunning(ctx, docker, "", spec); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}

	// Create a marker table so we can verify reset wiped it.
	exec1 := exec.CommandContext(ctx, "docker", "exec", spec.ContainerName,
		"psql", "-U", spec.DBUser, "-d", spec.DBName, "-c", "CREATE TABLE marker (id INT);")
	if out, err := exec1.CombinedOutput(); err != nil {
		t.Fatalf("create marker: %v\n%s", err, out)
	}

	if err := ResetData(ctx, docker, "", spec); err != nil {
		t.Fatalf("ResetData: %v", err)
	}

	// Marker table must be gone.
	exec2 := exec.CommandContext(ctx, "docker", "exec", spec.ContainerName,
		"psql", "-U", spec.DBUser, "-d", spec.DBName, "-tAc",
		"SELECT to_regclass('marker') IS NULL;")
	out, err := exec2.CombinedOutput()
	if err != nil {
		t.Fatalf("check marker: %v\n%s", err, out)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(out)), "t") {
		t.Errorf("marker table survived ResetData: %s", out)
	}
}
```

- [ ] **Step 3: Add `ProjectName` field to `ServiceSpec`**

Edit `internal/services/types.go`, add `ProjectName string` to `ServiceSpec` (after `ProjectID`):

```go
	ProjectID         string
	ProjectName       string  // used for container/volume naming
	Environment       string
```

- [ ] **Step 4: Build + run unit tests (non-docker)**

Run: `go build ./... && go test -short ./internal/services/...`

Expected: build passes; `-short` skips the docker-gated tests, only `MergeWithUserOverride` + `BuildPostgresEnvInjection` run. All PASS.

- [ ] **Step 5: Run docker-gated tests (optional, slow)**

Run: `go test -timeout 5m ./internal/services/ -run "TestEnsureRunning|TestResetData" -v`

Expected: PASS if Docker is reachable; SKIP if not. The two tests take ~30-60s combined because each starts a real Postgres container.

- [ ] **Step 6: Commit**

```bash
git add internal/services/postgres.go internal/services/types.go internal/services/postgres_docker_test.go
git commit -m "feat(services): EnsureRunning/Restart/ResetData/Logs for postgres sidecar"
```

---

## Task 8: `Manager` facade

**Files:**
- Create: `internal/services/manager.go`
- Create: `internal/services/manager_test.go`

- [ ] **Step 1: Write the failing test (in-memory DB, no docker)**

Create `internal/services/manager_test.go`:

```go
package services

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func TestManagerProvisionPersistsRowWithEncryptedPassword(t *testing.T) {
	t.Parallel()

	database := newTestDB(t)
	encryptor, err := crypto.NewEncryptor("dev-encryption-key-32chars-yesssss")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	user := &db.User{ID: db.NewID(), GithubID: 1, Username: "tester", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	project := &db.Project{
		Name: "mgr-test", GithubRepo: "x", GithubOwner: "y", Branch: "main",
		UserID: user.ID, Framework: "node-api", Port: 3000, Status: "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	mgr := &Manager{DB: database, Encryptor: encryptor, RandReader: rand.Reader}

	spec, err := mgr.Provision(context.Background(), project, "preview", db.ServiceTypePostgres)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if spec.DBPasswordPlain == "" {
		t.Fatal("Provision returned empty plaintext password")
	}

	// Row persisted with encrypted password (not plaintext).
	row, err := database.GetServiceByProjectEnv(project.ID, "preview", db.ServiceTypePostgres)
	if err != nil {
		t.Fatalf("GetServiceByProjectEnv: %v", err)
	}
	if row == nil {
		t.Fatal("Provision did not persist a row")
	}
	if row.DBPasswordEncrypted == spec.DBPasswordPlain {
		t.Error("password stored as plaintext")
	}
	decrypted, err := encryptor.Decrypt(row.DBPasswordEncrypted)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if decrypted != spec.DBPasswordPlain {
		t.Errorf("decrypted password mismatch")
	}

	// Provision is NOT idempotent on conflict — second call returns ErrAlreadyProvisioned.
	_, err = mgr.Provision(context.Background(), project, "preview", db.ServiceTypePostgres)
	if err != ErrAlreadyProvisioned {
		t.Errorf("expected ErrAlreadyProvisioned, got %v", err)
	}
}

func TestManagerGetSpecLoadsAndDecrypts(t *testing.T) {
	t.Parallel()

	database := newTestDB(t)
	encryptor, _ := crypto.NewEncryptor("dev-encryption-key-32chars-yesssss")

	user := &db.User{ID: db.NewID(), GithubID: 2, Username: "u2", Role: "user"}
	_ = database.UpsertUser(user)
	project := &db.Project{Name: "getspec", GithubRepo: "x", GithubOwner: "y", Branch: "main", UserID: user.ID, Framework: "node-api", Port: 3000, Status: "active"}
	_ = database.CreateProject(project)
	mgr := &Manager{DB: database, Encryptor: encryptor, RandReader: rand.Reader}
	provisioned, _ := mgr.Provision(context.Background(), project, "production", db.ServiceTypePostgres)

	loaded, err := mgr.GetSpec(project, "production", db.ServiceTypePostgres)
	if err != nil {
		t.Fatalf("GetSpec: %v", err)
	}
	if loaded == nil {
		t.Fatal("GetSpec returned nil")
	}
	if loaded.DBPasswordPlain != provisioned.DBPasswordPlain {
		t.Errorf("GetSpec password mismatch")
	}
	if loaded.ContainerName != PostgresContainerName(project.Name, "production") {
		t.Errorf("ContainerName wrong: %s", loaded.ContainerName)
	}

	// GetSpec for an attached-but-different env returns nil.
	missing, err := mgr.GetSpec(project, "preview", db.ServiceTypePostgres)
	if err != nil {
		t.Fatalf("GetSpec (missing): %v", err)
	}
	if missing != nil {
		t.Error("GetSpec for missing env should return nil")
	}

	// Just enough delay so the row's updated_at would tick — sanity check this
	// test isn't depending on time precision.
	_ = time.Now()
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `go test ./internal/services/ -run "TestManager" -v`

Expected: FAIL — `Manager`, `Provision`, `GetSpec`, `ErrAlreadyProvisioned` undefined.

- [ ] **Step 3: Implement `manager.go`**

Create `internal/services/manager.go`:

```go
package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// ErrAlreadyProvisioned is returned by Provision when the (project, env,
// service_type) row already exists. Handlers surface as 409 Conflict.
var ErrAlreadyProvisioned = errors.New("service already provisioned for this environment")

// Manager is the DB-aware facade for sidecar lifecycle. Handlers and the
// deploy pipeline construct one of these in cmd/server/main.go and inject it.
type Manager struct {
	DB           *db.DB
	Encryptor    *crypto.Encryptor
	Docker       *build.DockerClient
	ProxyNetwork string

	// RandReader is the source of entropy for password generation. Defaults
	// to crypto/rand.Reader; tests can override for determinism.
	RandReader io.Reader
}

// passwordBytes is the entropy length for generated Postgres passwords.
// 32 bytes → 43-character base64url password (no padding). Sufficient against
// online attacks; the DB is never internet-exposed.
const passwordBytes = 32

// generatePassword returns a base64url-encoded random password of fixed length.
// Using base64url means the raw string is safe in DATABASE_URL without
// percent-encoding (no /, +, or = chars). DSN composition still uses
// url.UserPassword for defense-in-depth (Task 6).
func (m *Manager) generatePassword() (string, error) {
	reader := m.RandReader
	if reader == nil {
		reader = rand.Reader
	}
	buf := make([]byte, passwordBytes)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// Provision inserts a new project_services row for the given (project, env, type)
// with a freshly-generated encrypted password. Does NOT start the container —
// EnsureForDeployment / the API restart endpoint do that. Returns
// ErrAlreadyProvisioned if a row already exists.
func (m *Manager) Provision(ctx context.Context, project *db.Project, environment string, svcType db.ServiceType) (*ServiceSpec, error) {
	existing, err := m.DB.GetServiceByProjectEnv(project.ID, environment, svcType)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrAlreadyProvisioned
	}

	password, err := m.generatePassword()
	if err != nil {
		return nil, err
	}
	encrypted, err := m.Encryptor.Encrypt(password)
	if err != nil {
		return nil, fmt.Errorf("encrypt pg password: %w", err)
	}

	row := &db.ProjectService{
		ProjectID:           project.ID,
		Environment:         environment,
		ServiceType:         svcType,
		Image:               PostgresImage,
		DBName:              "app",
		DBUser:              "app",
		DBPasswordEncrypted: encrypted,
		HostPort:            0,
		ConfigJSON:          "{}",
		Status:              db.ServiceStatusPending,
	}
	if err := m.DB.CreateService(row); err != nil {
		return nil, err
	}

	return m.specFromRow(project, row, password), nil
}

// GetSpec loads the persisted row, decrypts the password, and assembles a
// ready-to-use ServiceSpec. Returns (nil, nil) when no row exists for the
// (project, env, type) tuple.
func (m *Manager) GetSpec(project *db.Project, environment string, svcType db.ServiceType) (*ServiceSpec, error) {
	row, err := m.DB.GetServiceByProjectEnv(project.ID, environment, svcType)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	password, err := m.Encryptor.Decrypt(row.DBPasswordEncrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt pg password: %w", err)
	}
	return m.specFromRow(project, row, password), nil
}

// EnsureForDeployment is called from pipeline.go Step 4b. If a service is
// attached for (project, env), it ensures the container is running and
// returns the EnvInjection. When no service is attached it returns (nil, nil)
// — the deploy proceeds without DB env injection.
//
// On failure the deployment should abort BEFORE building the image so we
// don't waste compute on a broken DB dependency.
func (m *Manager) EnsureForDeployment(ctx context.Context, project *db.Project, environment string) (*EnvInjection, error) {
	spec, err := m.GetSpec(project, environment, db.ServiceTypePostgres)
	if err != nil {
		return nil, err
	}
	if spec == nil {
		return nil, nil
	}
	if err := EnsureRunning(ctx, m.Docker, m.ProxyNetwork, spec); err != nil {
		_ = m.DB.UpdateServiceStatus(spec.ServiceID, db.ServiceStatusFailed)
		return nil, err
	}
	if err := m.DB.UpdateServiceHostPort(spec.ServiceID, spec.HostPort); err != nil {
		return nil, err
	}
	if err := m.DB.UpdateServiceStatus(spec.ServiceID, db.ServiceStatusRunning); err != nil {
		return nil, err
	}
	inj := BuildPostgresEnvInjection(*spec)
	return &inj, nil
}

// Delete stops the container, removes its volume, then deletes the row.
// Safe to call when the container doesn't exist; volume errors other than
// NotFound surface as wrapped errors.
func (m *Manager) Delete(ctx context.Context, project *db.Project, environment string, svcType db.ServiceType) error {
	spec, err := m.GetSpec(project, environment, svcType)
	if err != nil {
		return err
	}
	if spec == nil {
		return nil
	}
	if err := Stop(ctx, m.Docker, spec); err != nil {
		return fmt.Errorf("stop service: %w", err)
	}
	if err := m.Docker.RemoveVolume(ctx, spec.VolumeName); err != nil {
		// errdefs.IsNotFound is OK — volume already gone or never created.
		// All other errors (in-use conflict, etc.) should propagate.
		if !isNotFound(err) {
			return fmt.Errorf("remove volume: %w", err)
		}
	}
	return m.DB.DeleteService(spec.ServiceID)
}

// specFromRow assembles a ServiceSpec from a db row + already-decrypted password.
func (m *Manager) specFromRow(project *db.Project, row *db.ProjectService, password string) *ServiceSpec {
	return &ServiceSpec{
		ServiceID:       row.ID,
		ProjectID:       project.ID,
		ProjectName:     project.Name,
		Environment:     row.Environment,
		Type:            row.ServiceType,
		Image:           row.Image,
		DBName:          row.DBName,
		DBUser:          row.DBUser,
		DBPasswordPlain: password,
		HostPort:        row.HostPort,
		ContainerName:   PostgresContainerName(project.Name, row.Environment),
		VolumeName:      PostgresVolumeName(project.Name, row.Environment),
	}
}

// isNotFound bridges to errdefs without importing it into manager.go's
// imports. Kept tiny so future callers don't have to know the docker types.
func isNotFound(err error) bool {
	type notFounder interface{ NotFound() bool }
	var nf notFounder
	return errors.As(err, &nf) && nf.NotFound()
}
```

- [ ] **Step 4: Run — confirm PASS**

Run: `go test ./internal/services/ -run "TestManager" -v`

Expected: PASS.

- [ ] **Step 5: Full services package green**

Run: `go test -short ./internal/services/...`

Expected: PASS (all non-docker tests).

- [ ] **Step 6: Commit**

```bash
git add internal/services/manager.go internal/services/manager_test.go
git commit -m "feat(services): Manager facade for Provision/GetSpec/Delete/EnsureForDeployment"
```

---

## Task 9: Pipeline Step 4b — ensure Postgres before image build

**Files:**
- Modify: `internal/build/pipeline.go`

- [ ] **Step 1: Add the `Services` field to `Pipeline`**

In `internal/build/pipeline.go`, locate the `Pipeline` struct (around line 24). After the `Hub` field, add:

```go
	// Services manages sidecar lifecycle (postgres in v1). When non-nil the
	// pipeline checks for an attached service per deployment and ensures it's
	// running before the image build. Nil disables the path entirely.
	Services *services.Manager
```

Add the import at the top of the file:

```go
import (
	// ... existing imports ...
	"github.com/LEFTEQ/lovinka-deployik/internal/services"
)
```

- [ ] **Step 2: Insert Step 4b in `Deploy`**

Locate `Deploy` and find the block where env vars are decrypted (around `decryptedSecrets := ...` ending near line 230). Right BEFORE the line `buildEnvVars, runtimeEnvVars := resolveDeploymentVariables(...)`, insert:

```go
	// Step 4b: Ensure sidecar services (postgres) before building the image.
	// If pg fails to come up we want to abort BEFORE the docker build wastes
	// compute. The injection merges into runtimeEnvVars with user-set keys
	// taking precedence (see services.MergeWithUserOverride).
	var serviceInjection *services.EnvInjection
	if p.Services != nil {
		emit("Checking for attached services...")
		inj, err := p.Services.EnsureForDeployment(ctx, project, deployment.Environment)
		if err != nil {
			fail(err, "Service dependency failed")
			return
		}
		if inj != nil {
			emit("Postgres sidecar ready")
			serviceInjection = inj
		}
	}
```

Then, AFTER the existing `buildEnvVars, runtimeEnvVars := resolveDeploymentVariables(decryptedEnvVars, decryptedSecrets)` line, append:

```go
	if serviceInjection != nil {
		runtimeEnvVars = services.MergeWithUserOverride(runtimeEnvVars, *serviceInjection)
	}
```

- [ ] **Step 3: Compile**

Run: `go build ./...`

Expected: clean.

- [ ] **Step 4: Run the build suite (regression gate)**

Run: `go test ./internal/build/...`

Expected: PASS. Existing pipeline tests construct Pipeline with `Services: nil` implicitly, so the new path is a no-op for them.

- [ ] **Step 5: Commit**

```bash
git add internal/build/pipeline.go
git commit -m "feat(build): pipeline Step 4b ensures postgres sidecar before image build"
```

---

## Task 10: Project rename guard extension

**Files:**
- Modify: `internal/api/handlers/projects.go`

- [ ] **Step 1: Failing test**

Append to `internal/api/handlers/projects_test.go`:

```go
func TestUpdateProjectBlocksRenameWhenServicesExist(t *testing.T) {
	t.Parallel()

	env := setupProjectTestDB(t)
	project := seedProject(t, env, "renamable", "node-api")

	// Manually insert a service row so ServicesExist returns true.
	svc := &db.ProjectService{
		ProjectID:           project.ID,
		Environment:         "preview",
		ServiceType:         db.ServiceTypePostgres,
		Image:               "postgres:16",
		DBName:              "app",
		DBUser:              "app",
		DBPasswordEncrypted: "ciphertext",
		HostPort:            0,
		ConfigJSON:          "{}",
		Status:              db.ServiceStatusPending,
	}
	if err := env.DB.CreateService(svc); err != nil {
		t.Fatalf("CreateService: %v", err)
	}

	body := map[string]any{"name": "renamed"}
	req := env.PatchProjectJSON(t, project.ID, body)
	if req.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d body=%s", req.Code, req.Body.String())
	}
}
```

(Reuse the same `setupProjectTestDB` / `seedProject` / `PatchProjectJSON` helpers from existing handler tests; adapt names if the actual helpers differ.)

- [ ] **Step 2: Run — expect FAIL**

Run: `go test ./internal/api/handlers/ -run TestUpdateProjectBlocksRenameWhenServicesExist -v`

Expected: FAIL — rename succeeds because the guard only checks `DataVolumeEnabled`.

- [ ] **Step 3: Extend the rename guard**

In `internal/api/handlers/projects.go`, locate the rename guard inside `Update` (around line 287-292, the `if name != project.Name && project.DataVolumeEnabled` block).

Replace it with a single combined guard:

```go
		if name != project.Name {
			servicesAttached, err := h.DB.ServicesExist(project.ID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to check attached services"})
				return
			}
			if project.DataVolumeEnabled || servicesAttached {
				writeJSON(w, http.StatusConflict, map[string]string{
					"error": "cannot rename project while a persistent data volume or service is attached — detach them first",
				})
				return
			}
			project.Name = name
		}
```

- [ ] **Step 4: Run — confirm PASS**

Run: `go test ./internal/api/handlers/ -run TestUpdateProjectBlocksRenameWhenServicesExist -v`

Expected: PASS.

- [ ] **Step 5: Full handlers suite**

Run: `go test ./internal/api/handlers/...`

Expected: PASS — the existing data-volume rename-guard test should still pass.

- [ ] **Step 6: Commit**

```bash
git add internal/api/handlers/projects.go internal/api/handlers/projects_test.go
git commit -m "feat(api): block project rename when services are attached"
```

---

## Task 11: `ServiceHandler` — List + Attach endpoints

**Files:**
- Create: `internal/api/handlers/services.go`
- Create: `internal/api/handlers/services_test.go`

- [ ] **Step 1: Failing test (List empty + Attach + List one)**

Create `internal/api/handlers/services_test.go`:

```go
package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

func TestServicesListEmpty(t *testing.T) {
	t.Parallel()

	env := setupServicesTestEnv(t)
	project := seedProject(t, env.ProjectEnv, "svc-list", "node-api")

	resp := env.GetServices(t, project.ID)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	var list []db.ProjectService
	if err := json.Unmarshal(resp.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d entries", len(list))
	}
}

func TestServicesAttachAndList(t *testing.T) {
	t.Parallel()

	env := setupServicesTestEnv(t)
	project := seedProject(t, env.ProjectEnv, "svc-attach", "node-api")

	resp := env.PostJSON(t, "/api/projects/"+project.ID+"/services",
		map[string]any{"environment": "preview", "type": "postgres"})
	if resp.Code != http.StatusCreated {
		t.Fatalf("attach status = %d body = %s", resp.Code, resp.Body.String())
	}
	var created db.ProjectService
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ID == "" || created.Environment != "preview" {
		t.Errorf("bad created row: %+v", created)
	}
	// Password ciphertext must NEVER appear in the attach response.
	if strings.Contains(resp.Body.String(), "db_password_encrypted") {
		t.Error("attach response leaked db_password_encrypted field")
	}

	// Re-attach to same env must conflict.
	resp = env.PostJSON(t, "/api/projects/"+project.ID+"/services",
		map[string]any{"environment": "preview", "type": "postgres"})
	if resp.Code != http.StatusConflict {
		t.Errorf("re-attach should 409, got %d", resp.Code)
	}

	// List now returns one entry.
	resp = env.GetServices(t, project.ID)
	var list []db.ProjectService
	_ = json.Unmarshal(resp.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Errorf("expected 1 service, got %d", len(list))
	}
}
```

- [ ] **Step 2: Add the test harness helper `setupServicesTestEnv`**

In `internal/api/handlers/services_test.go`, add the harness (style matches existing helper-funcs in `projects_test.go`):

```go
import (
	// ... existing test-imports ...
	"github.com/LEFTEQ/lovinka-deployik/internal/services"
)

type servicesTestEnv struct {
	*projectTestEnv
	ServiceHandler *ServiceHandler
}

// Replace projectTestEnv with whatever the existing helper actually returns.
// The key requirement: same auth-claim injection + DB ownership the project
// handler tests use.

func setupServicesTestEnv(t *testing.T) *servicesTestEnv {
	t.Helper()
	base := setupProjectTestDB(t)
	mgr := &services.Manager{
		DB:        base.DB,
		Encryptor: base.Encryptor,
		// Docker + ProxyNetwork left nil — handler tests do NOT touch
		// EnsureForDeployment / EnsureRunning. Provision-only path is
		// fully DB+crypto.
	}
	return &servicesTestEnv{
		projectTestEnv: base,
		ServiceHandler: &ServiceHandler{
			DB:        base.DB,
			Manager:   mgr,
			Encryptor: base.Encryptor,
			Audit:     base.Audit,
		},
	}
}

func (env *servicesTestEnv) GetServices(t *testing.T, projectID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID+"/services", nil)
	req = injectAuth(req, env.User.ID)
	req = injectChiID(req, "id", projectID)
	rec := httptest.NewRecorder()
	env.ServiceHandler.List(rec, req)
	return rec
}
```

(If the existing `setupProjectTestDB` doesn't expose `Encryptor` / `Audit` fields by name, plumb them through — this is the kind of small test-harness extension the plan calls for.)

- [ ] **Step 3: Run — expect FAIL**

Run: `go test ./internal/api/handlers/ -run "TestServicesListEmpty|TestServicesAttachAndList" -v`

Expected: FAIL — `ServiceHandler` undefined.

- [ ] **Step 4: Implement `ServiceHandler` with List + Attach**

Create `internal/api/handlers/services.go`:

```go
package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/services"
)

type ServiceHandler struct {
	DB        *db.DB
	Manager   *services.Manager
	Encryptor *crypto.Encryptor
	Audit     *audit.Recorder
}

type attachServiceRequest struct {
	Environment string         `json:"environment"`
	Type        db.ServiceType `json:"type"`
}

// serviceResponse is the JSON shape returned by list + attach. Excludes the
// encrypted password column.
type serviceResponse struct {
	ID            string             `json:"id"`
	ProjectID     string             `json:"project_id"`
	Environment   string             `json:"environment"`
	Type          db.ServiceType     `json:"type"`
	Image         string             `json:"image"`
	DBName        string             `json:"db_name"`
	DBUser        string             `json:"db_user"`
	HostPort      int                `json:"host_port"`
	Status        db.ServiceStatus   `json:"status"`
	LastStartedAt db.NullableTime    `json:"last_started_at"`
	CreatedAt     string             `json:"created_at"`
	UpdatedAt     string             `json:"updated_at"`
}

func toServiceResponse(s db.ProjectService) serviceResponse {
	return serviceResponse{
		ID:            s.ID,
		ProjectID:     s.ProjectID,
		Environment:   s.Environment,
		Type:          s.ServiceType,
		Image:         s.Image,
		DBName:        s.DBName,
		DBUser:        s.DBUser,
		HostPort:      s.HostPort,
		Status:        s.Status,
		LastStartedAt: s.LastStartedAt,
		CreatedAt:     s.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     s.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (h *ServiceHandler) List(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, _, ok := loadAuthorizedProject(w, r, h.DB, id)
	if !ok {
		return
	}

	rows, err := h.DB.ListServicesByProject(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]serviceResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, toServiceResponse(r))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ServiceHandler) Attach(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, _, ok := loadAuthorizedProject(w, r, h.DB, id)
	if !ok {
		return
	}

	var req attachServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Environment != "preview" && req.Environment != "production" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment must be 'preview' or 'production'"})
		return
	}
	if req.Type != db.ServiceTypePostgres {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "v1 only supports type=postgres"})
		return
	}

	spec, err := h.Manager.Provision(r.Context(), project, req.Environment, req.Type)
	if err != nil {
		if errors.Is(err, services.ErrAlreadyProvisioned) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "service already attached for this environment"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	row, _ := h.DB.GetServiceByProjectEnv(project.ID, req.Environment, req.Type)
	writeJSON(w, http.StatusCreated, toServiceResponse(*row))

	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "service.attach",
		ResourceType: "service",
		ResourceID:   spec.ServiceID,
		ProjectID:    project.ID,
		Metadata:     map[string]any{"environment": req.Environment, "type": string(req.Type)},
	})
}
```

- [ ] **Step 5: Run — confirm PASS**

Run: `go test ./internal/api/handlers/ -run "TestServicesListEmpty|TestServicesAttachAndList" -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/handlers/services.go internal/api/handlers/services_test.go
git commit -m "feat(api): ServiceHandler List + Attach"
```

---

## Task 12: ServiceHandler — Detach + Credentials + Regenerate

**Files:**
- Modify: `internal/api/handlers/services.go`
- Modify: `internal/api/handlers/services_test.go`

- [ ] **Step 1: Failing tests**

Append to `services_test.go`:

```go
func TestServicesDetach(t *testing.T) {
	t.Parallel()

	env := setupServicesTestEnv(t)
	project := seedProject(t, env.ProjectEnv, "svc-detach", "node-api")
	_ = env.PostJSON(t, "/api/projects/"+project.ID+"/services",
		map[string]any{"environment": "preview", "type": "postgres"})

	resp := env.DeleteServiceEnv(t, project.ID, "preview")
	if resp.Code != http.StatusNoContent {
		t.Fatalf("detach status = %d body = %s", resp.Code, resp.Body.String())
	}

	listResp := env.GetServices(t, project.ID)
	var list []serviceResponse
	_ = json.Unmarshal(listResp.Body.Bytes(), &list)
	if len(list) != 0 {
		t.Errorf("expected empty after detach, got %d", len(list))
	}
}

func TestServicesCredentialsRevealAndRegenerate(t *testing.T) {
	t.Parallel()

	env := setupServicesTestEnv(t)
	project := seedProject(t, env.ProjectEnv, "svc-creds", "node-api")
	_ = env.PostJSON(t, "/api/projects/"+project.ID+"/services",
		map[string]any{"environment": "production", "type": "postgres"})

	resp := env.GetCredentials(t, project.ID, "production")
	if resp.Code != http.StatusOK {
		t.Fatalf("credentials status = %d body = %s", resp.Code, resp.Body.String())
	}
	var creds map[string]any
	_ = json.Unmarshal(resp.Body.Bytes(), &creds)
	if creds["db_name"] != "app" || creds["db_user"] != "app" {
		t.Errorf("missing db_name/db_user: %v", creds)
	}
	pwd, _ := creds["password"].(string)
	if pwd == "" {
		t.Error("credentials response missing plaintext password")
	}

	// Regenerate must change the password.
	regenResp := env.PostJSON(t, "/api/projects/"+project.ID+"/services/production/regenerate-password", nil)
	if regenResp.Code != http.StatusOK {
		t.Fatalf("regenerate status = %d", regenResp.Code)
	}
	var newCreds map[string]any
	_ = json.Unmarshal(regenResp.Body.Bytes(), &newCreds)
	newPwd, _ := newCreds["password"].(string)
	if newPwd == "" || newPwd == pwd {
		t.Errorf("regenerate should change password; old=%q new=%q", pwd, newPwd)
	}
}
```

(Add the missing helpers `DeleteServiceEnv` / `GetCredentials` on `servicesTestEnv` — wrappers that build the right httptest request with chi URL params.)

- [ ] **Step 2: Run — expect FAIL**

Run: `go test ./internal/api/handlers/ -run "TestServicesDetach|TestServicesCredentials" -v`

Expected: FAIL — methods not implemented.

- [ ] **Step 3: Add the handler methods**

Append to `internal/api/handlers/services.go`:

```go
type credentialsResponse struct {
	DBName        string `json:"db_name"`
	DBUser        string `json:"db_user"`
	Password      string `json:"password"`
	InternalHost  string `json:"internal_host"`
	InternalPort  int    `json:"internal_port"`
	VPSLoopbackPort int  `json:"vps_loopback_port"`
	SSHTunnelCmd  string `json:"ssh_tunnel_cmd"`
}

func (h *ServiceHandler) Detach(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	environment := chi.URLParam(r, "env")
	project, _, ok := loadAuthorizedProject(w, r, h.DB, id)
	if !ok {
		return
	}

	if err := h.Manager.Delete(r.Context(), project, environment, db.ServiceTypePostgres); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)

	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "service.detach",
		ResourceType: "service",
		ResourceID:   services.PostgresContainerName(project.Name, environment),
		ProjectID:    project.ID,
		Metadata:     map[string]any{"environment": environment},
	})
}

func (h *ServiceHandler) Credentials(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	environment := chi.URLParam(r, "env")
	project, _, ok := loadAuthorizedProject(w, r, h.DB, id)
	if !ok {
		return
	}

	spec, err := h.Manager.GetSpec(project, environment, db.ServiceTypePostgres)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if spec == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no service attached for this environment"})
		return
	}

	tunnelCmd := ""
	if spec.HostPort > 0 {
		tunnelCmd = "ssh -L 15432:127.0.0.1:" + itoa(spec.HostPort) + " deploy@<your-vps>"
	}

	writeJSON(w, http.StatusOK, credentialsResponse{
		DBName:           spec.DBName,
		DBUser:           spec.DBUser,
		Password:         spec.DBPasswordPlain,
		InternalHost:     spec.ContainerName,
		InternalPort:     services.PostgresPort,
		VPSLoopbackPort:  spec.HostPort,
		SSHTunnelCmd:     tunnelCmd,
	})

	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "service.credentials.reveal",
		ResourceType: "service",
		ResourceID:   spec.ServiceID,
		ProjectID:    project.ID,
		Metadata:     map[string]any{"environment": environment},
	})
}

func (h *ServiceHandler) RegeneratePassword(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	environment := chi.URLParam(r, "env")
	project, _, ok := loadAuthorizedProject(w, r, h.DB, id)
	if !ok {
		return
	}

	spec, err := h.Manager.GetSpec(project, environment, db.ServiceTypePostgres)
	if err != nil || spec == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "service not found"})
		return
	}

	newPassword, err := h.Manager.GeneratePasswordForReset()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	encrypted, err := h.Encryptor.Encrypt(newPassword)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := h.DB.UpdateServicePassword(spec.ServiceID, encrypted); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	spec.DBPasswordPlain = newPassword

	// Note: the running container still has the OLD password until the next
	// deploy restarts it. The UI dialog mentions this; we don't force-restart
	// the pg container here to avoid surprise downtime mid-session.

	writeJSON(w, http.StatusOK, credentialsResponse{
		DBName:          spec.DBName,
		DBUser:          spec.DBUser,
		Password:        newPassword,
		InternalHost:    spec.ContainerName,
		InternalPort:    services.PostgresPort,
		VPSLoopbackPort: spec.HostPort,
	})

	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "service.password.regenerate",
		ResourceType: "service",
		ResourceID:   spec.ServiceID,
		ProjectID:    project.ID,
		Metadata:     map[string]any{"environment": environment},
	})
}

// itoa is the tiny non-allocating int→string helper without importing strconv
// twice for one call site. (Existing helpers.go has writeJSON; nothing for itoa.)
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
```

- [ ] **Step 4: Add `GeneratePasswordForReset` to Manager**

In `internal/services/manager.go`, add (after `generatePassword`):

```go
// GeneratePasswordForReset is the exported version of generatePassword.
// Used by handlers that need a fresh password without going through the
// full Provision flow (regenerate-password endpoint).
func (m *Manager) GeneratePasswordForReset() (string, error) {
	return m.generatePassword()
}
```

- [ ] **Step 5: Run — confirm PASS**

Run: `go test ./internal/api/handlers/ -run "TestServicesDetach|TestServicesCredentials" -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/handlers/services.go internal/api/handlers/services_test.go internal/services/manager.go
git commit -m "feat(api): ServiceHandler Detach + Credentials + RegeneratePassword"
```

---

## Task 13: ServiceHandler — Restart + Reset

**Files:**
- Modify: `internal/api/handlers/services.go`
- Modify: `internal/api/handlers/services_test.go`

- [ ] **Step 1: Failing test**

Append to `services_test.go`:

```go
func TestServicesResetRequiresTypedConfirm(t *testing.T) {
	t.Parallel()

	env := setupServicesTestEnv(t)
	project := seedProject(t, env.ProjectEnv, "svc-reset", "node-api")
	_ = env.PostJSON(t, "/api/projects/"+project.ID+"/services",
		map[string]any{"environment": "preview", "type": "postgres"})

	// Wrong confirm string.
	bad := env.PostJSON(t, "/api/projects/"+project.ID+"/services/preview/reset",
		map[string]any{"confirm": "wrong"})
	if bad.Code != http.StatusBadRequest {
		t.Errorf("wrong confirm should 400, got %d", bad.Code)
	}

	// Right confirm string is "<project>-<env>".
	good := env.PostJSON(t, "/api/projects/"+project.ID+"/services/preview/reset",
		map[string]any{"confirm": "svc-reset-preview"})
	// Docker isn't wired in test harness so Manager.Docker is nil; we accept
	// either 200 (no-op when Docker is nil — Manager.Reset is a no-op) or 500
	// (whatever error the nil Docker returns). The key assertion is that the
	// confirm GATE passed.
	if good.Code == http.StatusBadRequest {
		t.Errorf("correct confirm should pass the gate, got 400 body=%s", good.Body.String())
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `go test ./internal/api/handlers/ -run TestServicesResetRequiresTypedConfirm -v`

Expected: FAIL — `Reset` handler not implemented.

- [ ] **Step 3: Implement Restart + Reset**

Append to `internal/api/handlers/services.go`:

```go
type resetServiceRequest struct {
	Confirm string `json:"confirm"`
}

func (h *ServiceHandler) Restart(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	environment := chi.URLParam(r, "env")
	project, _, ok := loadAuthorizedProject(w, r, h.DB, id)
	if !ok {
		return
	}
	spec, err := h.Manager.GetSpec(project, environment, db.ServiceTypePostgres)
	if err != nil || spec == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "service not found"})
		return
	}
	if h.Manager.Docker == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if err := services.Restart(r.Context(), h.Manager.Docker, h.Manager.ProxyNetwork, spec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_ = h.DB.UpdateServiceHostPort(spec.ServiceID, spec.HostPort)
	_ = h.DB.UpdateServiceStatus(spec.ServiceID, db.ServiceStatusRunning)
	writeJSON(w, http.StatusOK, map[string]any{"status": "running", "host_port": spec.HostPort})

	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "service.restart",
		ResourceType: "service",
		ResourceID:   spec.ServiceID,
		ProjectID:    project.ID,
		Metadata:     map[string]any{"environment": environment},
	})
}

func (h *ServiceHandler) Reset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	environment := chi.URLParam(r, "env")
	project, _, ok := loadAuthorizedProject(w, r, h.DB, id)
	if !ok {
		return
	}

	var req resetServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	expectedConfirm := project.Name + "-" + environment
	if req.Confirm != expectedConfirm {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "confirm must be '" + expectedConfirm + "' to wipe this environment's database",
		})
		return
	}

	spec, err := h.Manager.GetSpec(project, environment, db.ServiceTypePostgres)
	if err != nil || spec == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "service not found"})
		return
	}
	if h.Manager.Docker == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if err := services.ResetData(r.Context(), h.Manager.Docker, h.Manager.ProxyNetwork, spec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_ = h.DB.UpdateServiceHostPort(spec.ServiceID, spec.HostPort)
	_ = h.DB.UpdateServiceStatus(spec.ServiceID, db.ServiceStatusRunning)
	writeJSON(w, http.StatusOK, map[string]any{"status": "reset", "host_port": spec.HostPort})

	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "service.reset",
		ResourceType: "service",
		ResourceID:   spec.ServiceID,
		ProjectID:    project.ID,
		Metadata:     map[string]any{"environment": environment},
	})
}
```

- [ ] **Step 4: Run — confirm PASS**

Run: `go test ./internal/api/handlers/ -run TestServicesResetRequiresTypedConfirm -v`

Expected: PASS.

- [ ] **Step 5: Full handler suite**

Run: `go test ./internal/api/handlers/...`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/handlers/services.go internal/api/handlers/services_test.go
git commit -m "feat(api): ServiceHandler Restart + Reset with typed-confirm"
```

---

## Task 14: WebSocket logs handler

**Files:**
- Create: `internal/ws/services_logs.go`

- [ ] **Step 1: Implement the WS handler**

Create `internal/ws/services_logs.go`:

```go
package ws

import (
	"context"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"github.com/LEFTEQ/lovinka-deployik/internal/authz"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/services"
)

// ServiceLogsHandler streams `docker logs --follow` of a project service
// container to a WebSocket client. The Hub isn't needed because there's no
// fan-out — one client per stream.
type ServiceLogsHandler struct {
	DB              *db.DB
	Manager         *services.Manager
	AllowedOrigins  []string
}

func (h *ServiceLogsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	environment := chi.URLParam(r, "env")

	project, _, ok := loadAuthorizedWSProject(w, r, h.DB, projectID, h.AllowedOrigins)
	if !ok {
		return
	}

	spec, err := h.Manager.GetSpec(project, environment, db.ServiceTypePostgres)
	if err != nil || spec == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: makeOriginChecker(h.AllowedOrigins),
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("services_logs: upgrade: %v", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// docker logs pipe → websocket writer
	pr, pw := newPipeWriter()
	go func() {
		defer pw.Close()
		_ = services.Logs(ctx, spec, pw)
	}()
	streamPipeToWS(ctx, conn, pr)
}
```

(Reuse `loadAuthorizedWSProject`, `makeOriginChecker`, `newPipeWriter`, `streamPipeToWS` if they exist in the package — otherwise look at `internal/ws/logs.go` and mirror its structure. The key constraint: this file should be ~50 lines, not 200.)

- [ ] **Step 2: Compile**

Run: `go build ./...`

Expected: clean. If helper symbols don't exist with these exact names, adapt to whatever `internal/ws/logs.go` uses — that's the canonical sibling.

- [ ] **Step 3: Commit**

```bash
git add internal/ws/services_logs.go
git commit -m "feat(ws): WebSocket handler for postgres container logs"
```

---

## Task 15: Router wiring + main.go bootstrap

**Files:**
- Modify: `internal/api/router.go`
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Wire `ServiceHandler` into router**

In `internal/api/router.go`:

1. Add a `Services *services.Manager` field to `RouterConfig`.
2. After the existing volume handler instantiation, add:

```go
	serviceHandler := &handlers.ServiceHandler{
		DB:        cfg.DB,
		Manager:   cfg.Services,
		Encryptor: cfg.Encryptor,
		Audit:     auditRecorder,
	}
	serviceLogsHandler := &ws.ServiceLogsHandler{
		DB:             cfg.DB,
		Manager:        cfg.Services,
		AllowedOrigins: cfg.AllowedOrigins,
	}
```

3. Inside the authenticated `/api/projects/{id}/services` group:

```go
	r.Route("/projects/{id}/services", func(r chi.Router) {
		r.Use(mutationLimiter)
		r.Get("/", serviceHandler.List)
		r.Post("/", serviceHandler.Attach)
		r.Route("/{env}", func(r chi.Router) {
			r.Delete("/", serviceHandler.Detach)
			r.Get("/credentials", serviceHandler.Credentials)
			r.Post("/regenerate-password", serviceHandler.RegeneratePassword)
			r.Post("/restart", serviceHandler.Restart)
			r.Post("/reset", serviceHandler.Reset)
		})
	})
```

4. Inside the WS group, add:

```go
	r.Get("/ws/projects/{id}/services/{env}/logs", serviceLogsHandler.Handle)
```

(Exact placement depends on the existing route tree — mirror how the volume + deployment-logs routes are wired.)

- [ ] **Step 2: Bootstrap `services.Manager` in `main.go`**

In `cmd/server/main.go`, locate where the Pipeline is constructed. Add (BEFORE Pipeline creation, since Pipeline gains `Services`):

```go
	servicesMgr := &services.Manager{
		DB:           database,
		Encryptor:    encryptor,
		Docker:       docker,
		ProxyNetwork: cfg.ProxyNetwork,
	}
```

Add the import: `"github.com/LEFTEQ/lovinka-deployik/internal/services"`.

In the `Pipeline` literal, add:

```go
		Services: servicesMgr,
```

In the `RouterConfig` literal, add:

```go
		Services: servicesMgr,
```

- [ ] **Step 3: Compile + run all tests**

Run: `go build ./... && go test -short ./...`

Expected: PASS (short mode skips docker tests).

- [ ] **Step 4: Commit**

```bash
git add internal/api/router.go cmd/server/main.go
git commit -m "feat(api): wire ServiceHandler routes + bootstrap services.Manager"
```

---

## Task 16: Project delete cleanup

**Files:**
- Modify: `internal/api/handlers/projects.go`

- [ ] **Step 1: Extend `DeleteProject`**

In `internal/api/handlers/projects.go`, locate the `Delete` handler (or `DeleteProject`). Before the existing soft-delete call, iterate services and clean up:

```go
	if rows, err := h.DB.ListServicesByProject(project.ID); err == nil {
		for _, row := range rows {
			if err := h.Manager.Delete(r.Context(), project, row.Environment, row.ServiceType); err != nil {
				log.Printf("Warning: failed to delete service %s on project %s: %v",
					row.ID, project.ID, err)
			}
		}
	}
```

Add `Manager *services.Manager` to the `ProjectHandler` struct (and the `services` import to projects.go) so `h.Manager.Delete` resolves. Wire this in `router.go` where `ProjectHandler` is instantiated.

- [ ] **Step 2: Compile**

Run: `go build ./...`

Expected: clean.

- [ ] **Step 3: Full Go suite stays green**

Run: `go test -short ./...`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/api/handlers/projects.go internal/api/router.go
git commit -m "feat(api): clean up services + volumes when project is deleted"
```

---

## Task 17: Frontend types + API client

**Files:**
- Modify: `web/src/types/api.ts`
- Modify: `web/src/lib/api.ts`
- Modify: `web/src/lib/queryKeys.ts`

- [ ] **Step 1: Add type definitions**

Append to `web/src/types/api.ts`:

```ts
export type ServiceType = "postgres";
export type ServiceStatus = "pending" | "running" | "stopped" | "failed";

export interface ProjectService {
  id: string;
  project_id: string;
  environment: "preview" | "production";
  type: ServiceType;
  image: string;
  db_name: string;
  db_user: string;
  host_port: number;
  status: ServiceStatus;
  last_started_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface ServiceCredentials {
  db_name: string;
  db_user: string;
  password: string;
  internal_host: string;
  internal_port: number;
  vps_loopback_port: number;
  ssh_tunnel_cmd: string;
}

export interface AttachServiceRequest {
  environment: "preview" | "production";
  type: ServiceType;
}
```

- [ ] **Step 2: Add API client methods**

In `web/src/lib/api.ts`, find the existing project-scoped method group and append:

```ts
  // ----- Services (postgres sidecar) -----

  async listServices(projectId: string): Promise<ProjectService[]> {
    return this.request(`/projects/${projectId}/services`);
  }

  async attachService(projectId: string, body: AttachServiceRequest): Promise<ProjectService> {
    return this.request(`/projects/${projectId}/services`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  }

  async detachService(projectId: string, environment: string): Promise<void> {
    await this.request(`/projects/${projectId}/services/${environment}`, {
      method: "DELETE",
    });
  }

  async getServiceCredentials(projectId: string, environment: string): Promise<ServiceCredentials> {
    return this.request(`/projects/${projectId}/services/${environment}/credentials`);
  }

  async regenerateServicePassword(projectId: string, environment: string): Promise<ServiceCredentials> {
    return this.request(`/projects/${projectId}/services/${environment}/regenerate-password`, {
      method: "POST",
    });
  }

  async restartService(projectId: string, environment: string): Promise<{ status: string }> {
    return this.request(`/projects/${projectId}/services/${environment}/restart`, {
      method: "POST",
    });
  }

  async resetService(projectId: string, environment: string, confirm: string): Promise<{ status: string }> {
    return this.request(`/projects/${projectId}/services/${environment}/reset`, {
      method: "POST",
      body: JSON.stringify({ confirm }),
    });
  }
```

Add the imports at the top of `api.ts`:

```ts
import type {
  // ... existing ...
  ProjectService,
  ServiceCredentials,
  AttachServiceRequest,
} from "@/types/api";
```

- [ ] **Step 3: Add query key**

In `web/src/lib/queryKeys.ts`, append:

```ts
  projectServices: (projectId: string) => ["projects", projectId, "services"] as const,
```

- [ ] **Step 4: Typecheck**

Run: `cd web && bunx tsc --noEmit`

Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add web/src/types/api.ts web/src/lib/api.ts web/src/lib/queryKeys.ts
git commit -m "feat(web): types + API client + query keys for services"
```

---

## Task 18: Sidebar + Services route registration

**Files:**
- Modify: `web/src/components/layout/AppSidebar.tsx`
- Modify: `web/src/app/app.tsx`
- Create: `web/src/pages/ProjectSettingsServices.tsx` (skeleton)

- [ ] **Step 1: Add the sidebar entry**

In `web/src/components/layout/AppSidebar.tsx`, find the Settings sub-menu (currently lists `Build / Domains / Environments / Protection`). Add a new entry before `Protection`:

```tsx
              <SidebarMenuItem>
                <SidebarMenuButton
                  asChild
                  isActive={isActive("/projects/$id/settings/services")}
                >
                  <Link
                    to="/projects/$id/settings/services"
                    params={{ id: projectId }}
                  >
                    <Database className="size-4" />
                    Services
                  </Link>
                </SidebarMenuButton>
              </SidebarMenuItem>
```

Add `Database` to the lucide-react imports at the top.

- [ ] **Step 2: Skeleton page**

Create `web/src/pages/ProjectSettingsServices.tsx`:

```tsx
import { useParams } from "@tanstack/react-router";

export function ProjectSettingsServices() {
  const { id } = useParams({ from: "/projects/$id/settings/services" });
  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-base font-semibold">Services</h2>
        <p className="text-sm text-muted-foreground">
          Attach a Postgres database to this project. Each environment gets its
          own container + persistent volume. Credentials are revealed on demand;
          external access is via SSH tunnel only.
        </p>
      </div>
      <div className="rounded-2xl border border-dashed border-white/10 p-8 text-center text-sm text-muted-foreground">
        Services panel (project {id}) — wire-up in Task 19.
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Register the route**

In `web/src/app/app.tsx`, find the existing settings route block (the one with `settingsDomains`, `settingsEnv`, etc.). Add:

```ts
const settingsServicesRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: "settings/services",
  component: ProjectSettingsServices,
});
```

Add the import: `import { ProjectSettingsServices } from "@/pages/ProjectSettingsServices";`

Add `settingsServicesRoute` to the route tree's children array (alongside the other settings sub-routes).

- [ ] **Step 4: Typecheck**

Run: `cd web && bunx tsc --noEmit`

Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/AppSidebar.tsx web/src/app/app.tsx web/src/pages/ProjectSettingsServices.tsx
git commit -m "feat(web): Services sidebar item + route stub"
```

---

## Task 19: `ServicesPanel` component

**Files:**
- Create: `web/src/components/projects/services/ServicesPanel.tsx`
- Modify: `web/src/pages/ProjectSettingsServices.tsx`

- [ ] **Step 1: Implement `ServicesPanel`**

Create `web/src/components/projects/services/ServicesPanel.tsx`:

```tsx
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Database, Plus, RefreshCw, RotateCcw, ScrollText, Settings2, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import type { ProjectService, ServiceStatus } from "@/types/api";

import { CredentialsDialog } from "./CredentialsDialog";

const STATUS_TONE: Record<ServiceStatus, string> = {
  pending: "bg-amber-500/20 text-amber-200",
  running: "bg-green-500/20 text-green-200",
  stopped: "bg-zinc-500/20 text-zinc-200",
  failed: "bg-red-500/20 text-red-200",
};

export function ServicesPanel({ projectId, projectName }: { projectId: string; projectName: string }) {
  const queryClient = useQueryClient();
  const [credentialsEnv, setCredentialsEnv] = useState<string | null>(null);

  const services = useQuery({
    queryKey: queryKeys.projectServices(projectId),
    queryFn: () => api.listServices(projectId),
  });

  const attach = useMutation({
    mutationFn: (environment: "preview" | "production") =>
      api.attachService(projectId, { environment, type: "postgres" }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.projectServices(projectId) });
      toast.success("Postgres attached. It'll start on the next deploy.");
    },
    onError: (err) => toast.error(err.message),
  });

  const detach = useMutation({
    mutationFn: (environment: string) => api.detachService(projectId, environment),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.projectServices(projectId) });
      toast.success("Service detached.");
    },
    onError: (err) => toast.error(err.message),
  });

  const restart = useMutation({
    mutationFn: (environment: string) => api.restartService(projectId, environment),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.projectServices(projectId) });
      toast.success("Postgres restarted.");
    },
    onError: (err) => toast.error(err.message),
  });

  const byEnv = new Map(services.data?.map((s) => [s.environment, s]) ?? []);

  return (
    <>
      <div className="grid gap-4 md:grid-cols-2">
        {(["preview", "production"] as const).map((environment) => {
          const svc = byEnv.get(environment);
          return (
            <div key={environment} className="rounded-2xl border border-white/10 bg-black/10 p-4 space-y-3">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Database className="size-4" />
                  <span className="font-medium capitalize">{environment}</span>
                </div>
                {svc ? (
                  <Badge className={STATUS_TONE[svc.status]}>{svc.status}</Badge>
                ) : (
                  <Badge variant="outline">not attached</Badge>
                )}
              </div>

              {svc ? (
                <>
                  <div className="text-xs text-muted-foreground space-y-1">
                    <div>image: <code>{svc.image}</code></div>
                    <div>db: <code>{svc.db_name}</code> · user: <code>{svc.db_user}</code></div>
                    <div>host port: <code>{svc.host_port > 0 ? svc.host_port : "—"}</code></div>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button size="sm" variant="outline" onClick={() => setCredentialsEnv(environment)}>
                      <Settings2 className="mr-1.5 size-3.5" />
                      Connect
                    </Button>
                    <Button size="sm" variant="outline" onClick={() => restart.mutate(environment)} disabled={restart.isPending}>
                      <RotateCcw className="mr-1.5 size-3.5" />
                      Restart
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        const expected = `${projectName}-${environment}`;
                        const got = window.prompt(`Type "${expected}" to wipe ${environment}'s data:`);
                        if (got === expected) {
                          api.resetService(projectId, environment, got).then(
                            () => {
                              toast.success(`${environment} database reset.`);
                              queryClient.invalidateQueries({ queryKey: queryKeys.projectServices(projectId) });
                            },
                            (err) => toast.error(err.message),
                          );
                        }
                      }}
                    >
                      <RefreshCw className="mr-1.5 size-3.5" />
                      Reset
                    </Button>
                    <Button size="sm" variant="outline" disabled title="Logs panel coming in Phase 3">
                      <ScrollText className="mr-1.5 size-3.5" />
                      Logs
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      className="text-red-300"
                      onClick={() => {
                        if (window.confirm(`Detach Postgres from ${environment}? This also deletes the data volume.`)) {
                          detach.mutate(environment);
                        }
                      }}
                    >
                      <Trash2 className="mr-1.5 size-3.5" />
                      Detach
                    </Button>
                  </div>
                </>
              ) : (
                <Button size="sm" onClick={() => attach.mutate(environment)} disabled={attach.isPending}>
                  <Plus className="mr-1.5 size-3.5" />
                  Attach Postgres
                </Button>
              )}
            </div>
          );
        })}
      </div>

      {credentialsEnv && (
        <CredentialsDialog
          projectId={projectId}
          environment={credentialsEnv}
          onClose={() => setCredentialsEnv(null)}
        />
      )}
    </>
  );
}
```

- [ ] **Step 2: Wire into the page**

Edit `web/src/pages/ProjectSettingsServices.tsx`:

```tsx
import { useParams } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { ServicesPanel } from "@/components/projects/services/ServicesPanel";

export function ProjectSettingsServices() {
  const { id } = useParams({ from: "/projects/$id/settings/services" });
  const project = useQuery({ queryKey: queryKeys.project(id), queryFn: () => api.getProject(id) });

  if (project.isLoading) return null;
  if (!project.data) return <div className="text-sm text-muted-foreground">Project not found.</div>;

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-base font-semibold">Services</h2>
        <p className="text-sm text-muted-foreground">
          Attach a Postgres database to this project. Each environment gets its
          own container + persistent volume. Credentials are revealed on demand;
          external access is via SSH tunnel only.
        </p>
      </div>
      <ServicesPanel projectId={id} projectName={project.data.name} />
    </div>
  );
}
```

- [ ] **Step 3: Typecheck**

Run: `cd web && bunx tsc --noEmit`

Expected: clean (after `CredentialsDialog` from Task 20 is also created — if it isn't yet, leave the import + temporarily stub it).

- [ ] **Step 4: Commit**

```bash
git add web/src/components/projects/services/ServicesPanel.tsx web/src/pages/ProjectSettingsServices.tsx
git commit -m "feat(web): ServicesPanel with attach/detach/restart/reset/connect"
```

(If `CredentialsDialog` isn't yet implemented, this commit will fail typecheck — interleave with Task 20 by combining them.)

---

## Task 20: `CredentialsDialog`

**Files:**
- Create: `web/src/components/projects/services/CredentialsDialog.tsx`

- [ ] **Step 1: Implement**

Create `web/src/components/projects/services/CredentialsDialog.tsx`:

```tsx
import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Copy, KeyRound } from "lucide-react";

import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

import { api } from "@/lib/api";

type Props = {
  projectId: string;
  environment: string;
  onClose: () => void;
};

export function CredentialsDialog({ projectId, environment, onClose }: Props) {
  const queryClient = useQueryClient();
  const [showPassword, setShowPassword] = useState(false);

  const creds = useQuery({
    queryKey: ["projects", projectId, "services", environment, "credentials"],
    queryFn: () => api.getServiceCredentials(projectId, environment),
  });

  const regenerate = useMutation({
    mutationFn: () => api.regenerateServicePassword(projectId, environment),
    onSuccess: (newCreds) => {
      queryClient.setQueryData(
        ["projects", projectId, "services", environment, "credentials"],
        newCreds,
      );
      toast.success("Password regenerated. Redeploy to apply the new password to the running container.");
    },
    onError: (err) => toast.error(err.message),
  });

  const copy = (label: string, value: string) => {
    navigator.clipboard.writeText(value);
    toast.success(`${label} copied`);
  };

  const c = creds.data;

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Connect to {environment} Postgres</DialogTitle>
          <DialogDescription>
            The database is reachable only from inside the deploy network and
            via SSH tunnel from your laptop. Credentials are decrypted
            on-demand for this dialog.
          </DialogDescription>
        </DialogHeader>

        {creds.isLoading ? (
          <div className="text-sm text-muted-foreground">Loading…</div>
        ) : !c ? (
          <div className="text-sm text-red-300">Failed to load credentials.</div>
        ) : (
          <div className="space-y-4">
            <Field label="Database" value={c.db_name} onCopy={copy} />
            <Field label="User" value={c.db_user} onCopy={copy} />
            <div className="space-y-1">
              <Label className="text-xs uppercase tracking-wide text-muted-foreground">Password</Label>
              <div className="flex gap-2">
                <Input readOnly type={showPassword ? "text" : "password"} value={c.password} />
                <Button type="button" variant="outline" size="sm" onClick={() => setShowPassword((v) => !v)}>
                  {showPassword ? "Hide" : "Show"}
                </Button>
                <Button type="button" variant="outline" size="sm" onClick={() => copy("Password", c.password)}>
                  <Copy className="size-3.5" />
                </Button>
              </div>
            </div>
            <Field label="Internal host (inside deploy network)" value={`${c.internal_host}:${c.internal_port}`} onCopy={copy} mono />
            {c.vps_loopback_port > 0 ? (
              <Field
                label="SSH tunnel from your laptop"
                value={c.ssh_tunnel_cmd}
                onCopy={copy}
                mono
                hint="Run this in a separate terminal, then `psql postgresql://...@127.0.0.1:15432/...`"
              />
            ) : (
              <div className="text-xs text-muted-foreground">
                Container hasn't started yet — restart it to get a tunnel port.
              </div>
            )}
          </div>
        )}

        <DialogFooter className="gap-2">
          <Button variant="outline" onClick={() => regenerate.mutate()} disabled={regenerate.isPending}>
            <KeyRound className="mr-1.5 size-3.5" />
            Regenerate password
          </Button>
          <Button onClick={onClose}>Close</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Field({
  label,
  value,
  onCopy,
  mono,
  hint,
}: {
  label: string;
  value: string;
  onCopy: (label: string, value: string) => void;
  mono?: boolean;
  hint?: string;
}) {
  return (
    <div className="space-y-1">
      <Label className="text-xs uppercase tracking-wide text-muted-foreground">{label}</Label>
      <div className="flex gap-2">
        <Input readOnly value={value} className={mono ? "font-mono text-xs" : ""} />
        <Button type="button" variant="outline" size="sm" onClick={() => onCopy(label, value)}>
          <Copy className="size-3.5" />
        </Button>
      </div>
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}
```

- [ ] **Step 2: Typecheck**

Run: `cd web && bunx tsc --noEmit`

Expected: clean.

- [ ] **Step 3: Bun tests stay green**

Run: `cd web && bun run test`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add web/src/components/projects/services/CredentialsDialog.tsx
git commit -m "feat(web): CredentialsDialog with SSH-tunnel command + regenerate"
```

---

## Task 21: NewProject — Attach Postgres toggle

**Files:**
- Modify: `web/src/pages/NewProject.tsx`

- [ ] **Step 1: Add a switch + post-create attach**

In `web/src/pages/NewProject.tsx`:

1. Add a `useState` for `attachPostgres`:

```ts
  const [attachPostgres, setAttachPostgres] = useState(false);
```

2. In the JSX, after the build-settings section (right before the submit button), add:

```tsx
            <div className="flex items-start justify-between gap-3 rounded-2xl border border-white/10 bg-black/10 p-4">
              <div className="space-y-1">
                <Label htmlFor="attach-postgres" className="text-sm">Attach Postgres database</Label>
                <p className="text-xs text-muted-foreground">
                  Each environment (preview + production) gets its own postgres:16 container with a persistent volume.
                  <code className="ml-1 rounded bg-white/10 px-1 py-0.5">DATABASE_URL</code> is auto-injected.
                </p>
              </div>
              <Switch
                id="attach-postgres"
                checked={attachPostgres}
                onCheckedChange={setAttachPostgres}
              />
            </div>
```

(Import `Switch` from `@/components/ui/switch` and `Label` from `@/components/ui/label` if not already imported.)

3. In the `createMutation`'s `onSuccess`, after `navigate(...)`, dispatch the attach calls when the toggle is on:

```ts
    onSuccess: async (project) => {
      queryClient.invalidateQueries({ queryKey: ["projects"] });
      if (attachPostgres) {
        try {
          await api.attachService(project.id, { environment: "preview", type: "postgres" });
          await api.attachService(project.id, { environment: "production", type: "postgres" });
        } catch (err) {
          // Best-effort: the project is created, so the user can attach manually
          // from Services if this failed. Surface a toast.
          toast.error("Project created, but Postgres attach failed: " + (err as Error).message);
        }
      }
      toast.success("Project created");
      navigate({ to: "/projects/$id", params: { id: project.id } });
    },
```

- [ ] **Step 2: Typecheck**

Run: `cd web && bunx tsc --noEmit`

Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/NewProject.tsx
git commit -m "feat(web): NewProject 'Attach Postgres database' toggle"
```

---

## Task 22: Final verification + smoke test handoff

**Files:** (none)

- [ ] **Step 1: Full test gate**

Run:

```
cd /Users/your-github-username/Documents/Work/lovinka-deployik
go test -short ./...
go vet ./...
cd web && bunx tsc --noEmit
cd /Users/your-github-username/Documents/Work/lovinka-deployik/web && bun run test
```

Expected: all green. Docker-gated tests are skipped via `-short`.

- [ ] **Step 2: Branch state sanity**

Run: `git status && git log --oneline main..HEAD`

Expected: clean working tree; commit log shows the 21 task commits in order.

- [ ] **Step 3: Manual smoke test (kept for you, not for a subagent)**

Walk through in the browser at `http://localhost:5273` (per the Phase 1 chore commit):

1. **New Project** → pick framework `Node API` → toggle **Attach Postgres database** ON → submit.
2. Confirm the project is created. Check the **Services** sidebar item under Settings — both preview and production show **Postgres pending**.
3. Click **Deploy** on the preview environment. In the deploy log expect a line `Postgres sidecar ready` BEFORE the image build.
4. Open the **Services** page → click **Connect** on preview. Verify the dialog shows the DSN, the internal hostname, and an SSH tunnel command. Click **Regenerate password** and confirm the password changes.
5. Click **Reset** on preview → typed-confirm with `<project>-preview` → expect a toast + the data volume is recreated empty.
6. **From your laptop:** SSH-tunnel using the command in the Connect dialog, then `psql postgresql://app:<password>@127.0.0.1:15432/app -c '\dt'`. Should connect, list zero tables for a fresh DB.

---

## Verification (top-level)

| What | How |
|---|---|
| Migration 023 applies cleanly | `rm -f data/deployik.db && make dev-api`; `sqlite3 data/deployik.db '.schema project_services'` |
| Unit + integration tests green | `go test -short ./...` |
| Docker-gated tests green | `go test -timeout 5m ./internal/services/...` (when Docker is reachable) |
| Frontend type-clean | `cd web && bunx tsc --noEmit` |
| Frontend unit tests | `cd web && bun run test` |
| Real Postgres sidecar boots | Manual: deploy a Node-API project with Postgres attached; verify `DATABASE_URL` reaches the app container |

## Risks (carry-over from design doc)

1. **Project rename guard**: extended in Task 10 to reject when services exist. Long-term fix (re-key volume by `project.ID`) is out of scope.
2. **`/opt/backups`-style disk pressure**: not relevant in v2 — no scheduled backups land here. Phase 3 will need a free-space guard.
3. **Postgres image pull on first attach**: `ensurePostgresImage` lazily pulls `postgres:16` (~140 MB). Surfaced as a single deploy-log line; first attach can take 30-60s on a slow connection.
4. **Password rotation requires redeploy**: the running container still has the OLD password until the next deploy restarts it. The UI dialog mentions this. Worth flagging in the Phase 3 plan as a candidate for a "Restart with new password" affordance.
5. **WebSocket logs handler stub**: Task 14 sketches the file but references helpers (`loadAuthorizedWSProject`, etc.) that may or may not exist by exactly those names in `internal/ws/`. The implementer should mirror `internal/ws/logs.go`'s pattern rather than inventing new helpers.

## Next plans (separately authored)

- `docs/plans/2026-05-13-postgres-backups.md` — Phase 3: manual `pg_dump` backup, restore-from-upload, daily scheduled backups with retention pruning. Also a good landing spot for the deferred Phase 1 carry-overs:
  - Extract `projectColumns` const + `scanProject` helper from `queries_projects.go`.
  - Add non-root `USER` step to `generateStaticDockerfile` + `generateNodeAPIDockerfile`.
  - Promote `requireAll` test helper to a shared `testutil_test.go`.
