# Auto Deploy Production Opt-In Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Deployik auto-build preview by default and add an explicit opt-in that also auto-deploys production from the same branch push.

**Architecture:** Keep the existing webhook and deployment pipeline. Add one auto-build config boolean, let one webhook event produce one preview deployment and optionally one production deployment, and keep the two deployments as separate image builds. Rework webhook event idempotency from delivery-level to project/environment-level so one GitHub delivery can record multiple outcomes.

**Tech Stack:** Go 1.25, SQLite migrations, chi HTTP handlers, React 19, TanStack Query, shadcn UI, Bun.

---

## File Structure

- Create `internal/db/migrations/018_auto_production_auto_deploy.sql`: add `auto_production_enabled` and rebuild `webhook_events` with environment-aware uniqueness.
- Modify `internal/db/models.go`: add `AutoProductionEnabled` to `AutoBuildConfig` and `Environment` to `WebhookEvent`.
- Modify `internal/db/queries_autobuild.go`: select, insert, and update the new boolean.
- Modify `internal/db/queries_webhook_events.go`: insert environment and check idempotency per delivery/project/environment.
- Modify `internal/db/db_test.go`: add migration coverage for legacy rows.
- Create `internal/api/handlers/webhooks_test.go`: cover preview-only, preview-plus-production, and ignored branch webhook outcomes.
- Modify `internal/api/handlers/webhooks.go`: replace mutually exclusive environment selection with fan-out.
- Modify `internal/api/handlers/autobuild.go`: expose `auto_production_enabled` through the settings API.
- Modify `internal/api/handlers/projects.go`: accept project-create auto-build flags and enqueue initial production when opted in.
- Modify `internal/api/handlers/projects_test.go`: cover project-create defaults and production opt-in.
- Modify `web/src/types/api.ts`: add `auto_production_enabled` to `AutoBuildConfig`.
- Modify `web/src/lib/api.ts`: add `auto_production_enabled` to auto-build updates and create-project payloads.
- Modify `web/src/pages/ProjectSettings.tsx`: add the production auto-release switch and sync loaded config into local state.
- Modify `web/src/pages/NewProject.tsx`: add the Auto-Deploy section to the wizard and send the new flags.

## Task 1: Database Migration And Query Support

**Files:**
- Create: `internal/db/migrations/018_auto_production_auto_deploy.sql`
- Modify: `internal/db/models.go`
- Modify: `internal/db/queries_autobuild.go`
- Modify: `internal/db/queries_webhook_events.go`
- Test: `internal/db/db_test.go`

- [ ] **Step 1: Write the failing migration test**

Append this test to `internal/db/db_test.go`:

```go
func TestMigration018AddsProductionOptInAndWebhookEventOutcomes(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if _, err := database.Exec(`
		CREATE TABLE IF NOT EXISTS _migrations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)
	`); err != nil {
		t.Fatalf("create migrations table: %v", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("ReadDir migrations: %v", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() >= "018_auto_production_auto_deploy.sql" {
			continue
		}
		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", entry.Name(), err)
		}
		if _, err := database.Exec(string(content)); err != nil {
			t.Fatalf("exec migration %s: %v", entry.Name(), err)
		}
		if _, err := database.Exec("INSERT INTO _migrations (name) VALUES (?)", entry.Name()); err != nil {
			t.Fatalf("record migration %s: %v", entry.Name(), err)
		}
	}

	user := &User{ID: NewID(), GithubID: 42, Username: "migration-user", Role: "admin"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	project := &Project{
		Name:           "migration-app",
		GithubRepo:     "repo",
		GithubOwner:    "owner",
		Branch:         "main",
		UserID:         user.ID,
		Framework:      "nextjs",
		BuildCommand:   "bun run build",
		InstallCommand: "bun install",
		NodeVersion:    "22",
		Status:         "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	deployment := &Deployment{
		ProjectID:     project.ID,
		Environment:   "preview",
		Branch:        "main",
		Status:        "queued",
		TriggerSource: "webhook",
		TriggeredBy:   user.ID,
	}
	if err := database.CreateDeployment(deployment); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO auto_build_configs (id, project_id, enabled, production_branch, preview_branches, webhook_id, webhook_secret)
		 VALUES (?, ?, 1, 'main', '*', 123, 'encrypted-secret')`,
		NewID(), project.ID,
	); err != nil {
		t.Fatalf("insert legacy auto_build_config: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO webhook_events (project_id, github_delivery_id, event_type, branch, commit_sha, commit_message, pusher, deployment_id, status)
		 VALUES (?, 'delivery-1', 'push', 'main', 'sha1', 'message', 'pusher', ?, 'processed')`,
		project.ID, deployment.ID,
	); err != nil {
		t.Fatalf("insert legacy webhook_event: %v", err)
	}

	if err := database.Migrate(); err != nil {
		t.Fatalf("Migrate with legacy webhook events: %v", err)
	}

	config, err := database.GetAutoBuildConfig(project.ID)
	if err != nil {
		t.Fatalf("GetAutoBuildConfig: %v", err)
	}
	if config == nil {
		t.Fatal("expected auto build config")
	}
	if config.AutoProductionEnabled {
		t.Fatal("auto_production_enabled = true, want false for migrated configs")
	}

	var environment string
	if err := database.QueryRow(
		`SELECT environment FROM webhook_events WHERE github_delivery_id = 'delivery-1' AND project_id = ?`,
		project.ID,
	).Scan(&environment); err != nil {
		t.Fatalf("select migrated webhook event: %v", err)
	}
	if environment != "preview" {
		t.Fatalf("environment = %q, want preview", environment)
	}

	if err := database.CreateWebhookEvent(&WebhookEvent{
		ProjectID:        project.ID,
		GithubDeliveryID: "delivery-2",
		EventType:        "push",
		Environment:      "preview",
		Branch:           "main",
		CommitSHA:        "sha2",
		Status:           "processed",
	}); err != nil {
		t.Fatalf("CreateWebhookEvent preview: %v", err)
	}
	if err := database.CreateWebhookEvent(&WebhookEvent{
		ProjectID:        project.ID,
		GithubDeliveryID: "delivery-2",
		EventType:        "push",
		Environment:      "production",
		Branch:           "main",
		CommitSHA:        "sha2",
		Status:           "processed",
	}); err != nil {
		t.Fatalf("CreateWebhookEvent production: %v", err)
	}
	if err := database.CreateWebhookEvent(&WebhookEvent{
		ProjectID:        project.ID,
		GithubDeliveryID: "delivery-2",
		EventType:        "push",
		Environment:      "preview",
		Branch:           "main",
		CommitSHA:        "sha2",
		Status:           "processed",
	}); err == nil {
		t.Fatal("expected duplicate delivery/project/environment insert to fail")
	}
}
```

- [ ] **Step 2: Run the migration test to verify it fails**

Run:

```bash
go test ./internal/db -run TestMigration018AddsProductionOptInAndWebhookEventOutcomes -count=1
```

Expected: FAIL because `018_auto_production_auto_deploy.sql` does not exist and `AutoProductionEnabled` / `Environment` fields are not defined.

- [ ] **Step 3: Add the migration**

Create `internal/db/migrations/018_auto_production_auto_deploy.sql`:

```sql
ALTER TABLE auto_build_configs
ADD COLUMN auto_production_enabled INTEGER NOT NULL DEFAULT 0;

ALTER TABLE webhook_events RENAME TO webhook_events_old;

CREATE TABLE webhook_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    github_delivery_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    environment TEXT NOT NULL DEFAULT 'ignored'
        CHECK (environment IN ('preview', 'production', 'ignored')),
    branch TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    commit_message TEXT NOT NULL DEFAULT '',
    pusher TEXT NOT NULL DEFAULT '',
    deployment_id TEXT REFERENCES deployments(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'received'
        CHECK (status IN ('received', 'processed', 'ignored', 'failed')),
    error_message TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO webhook_events (
    id,
    project_id,
    github_delivery_id,
    event_type,
    environment,
    branch,
    commit_sha,
    commit_message,
    pusher,
    deployment_id,
    status,
    error_message,
    created_at
)
SELECT
    old.id,
    old.project_id,
    old.github_delivery_id,
    old.event_type,
    CASE
        WHEN old.status = 'ignored' THEN 'ignored'
        WHEN old.deployment_id IS NOT NULL THEN COALESCE(
            (SELECT d.environment FROM deployments d WHERE d.id = old.deployment_id),
            'preview'
        )
        ELSE 'ignored'
    END,
    old.branch,
    old.commit_sha,
    old.commit_message,
    old.pusher,
    old.deployment_id,
    old.status,
    old.error_message,
    old.created_at
FROM webhook_events_old old;

DROP TABLE webhook_events_old;

CREATE INDEX IF NOT EXISTS idx_webhook_events_project_id
ON webhook_events(project_id);

CREATE INDEX IF NOT EXISTS idx_webhook_events_delivery_id
ON webhook_events(github_delivery_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_webhook_events_delivery_project_env
ON webhook_events(github_delivery_id, project_id, environment);
```

- [ ] **Step 4: Update database models**

In `internal/db/models.go`, replace the existing `AutoBuildConfig` and `WebhookEvent` structs with:

```go
type AutoBuildConfig struct {
	ID                    string    `json:"id"`
	ProjectID             string    `json:"project_id"`
	Enabled               bool      `json:"enabled"`
	ProductionBranch      string    `json:"production_branch"`
	PreviewBranches       string    `json:"preview_branches"`
	AutoProductionEnabled bool      `json:"auto_production_enabled"`
	WebhookID             *int64    `json:"webhook_id,omitempty"`
	WebhookSecret         string    `json:"-"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type WebhookEvent struct {
	ID               int64     `json:"id"`
	ProjectID        string    `json:"project_id"`
	GithubDeliveryID string    `json:"github_delivery_id"`
	EventType        string    `json:"event_type"`
	Environment      string    `json:"environment"`
	Branch           string    `json:"branch"`
	CommitSHA        string    `json:"commit_sha"`
	CommitMessage    string    `json:"commit_message"`
	Pusher           string    `json:"pusher"`
	DeploymentID     string    `json:"deployment_id,omitempty"`
	Status           string    `json:"status"`
	ErrorMessage     *string   `json:"error_message,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}
```

- [ ] **Step 5: Update auto-build queries**

In `internal/db/queries_autobuild.go`, include `auto_production_enabled` in every select, insert, and upsert. The final functions should use these SQL fragments:

```go
`SELECT id, project_id, enabled, production_branch, preview_branches,
        auto_production_enabled, webhook_id, webhook_secret, created_at, updated_at
 FROM auto_build_configs
 WHERE project_id = ?`
```

```go
).Scan(
	&c.ID,
	&c.ProjectID,
	&c.Enabled,
	&c.ProductionBranch,
	&c.PreviewBranches,
	&c.AutoProductionEnabled,
	&c.WebhookID,
	&c.WebhookSecret,
	&c.CreatedAt,
	&c.UpdatedAt,
)
```

```go
`INSERT INTO auto_build_configs (
     id, project_id, enabled, production_branch, preview_branches,
     auto_production_enabled, webhook_id, webhook_secret
 )
 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
 ON CONFLICT(project_id) DO UPDATE SET
     enabled = excluded.enabled,
     production_branch = excluded.production_branch,
     preview_branches = excluded.preview_branches,
     auto_production_enabled = excluded.auto_production_enabled,
     webhook_id = excluded.webhook_id,
     webhook_secret = CASE WHEN excluded.webhook_secret = '' THEN webhook_secret ELSE excluded.webhook_secret END,
     updated_at = datetime('now')`,
c.ID, c.ProjectID, c.Enabled, c.ProductionBranch, c.PreviewBranches,
c.AutoProductionEnabled, c.WebhookID, c.WebhookSecret,
```

Also update `ListActiveAutoBuildConfigsByRepo` to select and scan `auto_production_enabled` with the same order.

- [ ] **Step 6: Update webhook event queries**

Replace `internal/db/queries_webhook_events.go` with:

```go
package db

import (
	"fmt"
)

func (db *DB) CreateWebhookEvent(e *WebhookEvent) error {
	environment := e.Environment
	if environment == "" {
		environment = "ignored"
	}
	_, err := db.Exec(
		`INSERT INTO webhook_events (project_id, github_delivery_id, event_type, environment, branch,
		                             commit_sha, commit_message, pusher, deployment_id, status, error_message)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ProjectID, e.GithubDeliveryID, e.EventType, environment, e.Branch,
		e.CommitSHA, e.CommitMessage, e.Pusher, nullableString(e.DeploymentID), e.Status, e.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("create webhook event: %w", err)
	}
	return nil
}

func (db *DB) WebhookEventExists(deliveryID, projectID, environment string) (bool, error) {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM webhook_events
		 WHERE github_delivery_id = ? AND project_id = ? AND environment = ?`,
		deliveryID, projectID, environment,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check webhook event exists: %w", err)
	}
	return count > 0, nil
}

func (db *DB) UpdateWebhookEventStatus(deliveryID, projectID, environment, status, deploymentID string, errorMsg *string) error {
	_, err := db.Exec(
		`UPDATE webhook_events
		 SET status = ?, deployment_id = ?, error_message = ?
		 WHERE github_delivery_id = ? AND project_id = ? AND environment = ?`,
		status, nullableString(deploymentID), errorMsg, deliveryID, projectID, environment,
	)
	if err != nil {
		return fmt.Errorf("update webhook event status: %w", err)
	}
	return nil
}
```

- [ ] **Step 7: Run database tests**

Run:

```bash
go test ./internal/db -run 'TestMigration018AddsProductionOptInAndWebhookEventOutcomes|TestMigrations' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit database support**

Run:

```bash
git add internal/db/migrations/018_auto_production_auto_deploy.sql internal/db/models.go internal/db/queries_autobuild.go internal/db/queries_webhook_events.go internal/db/db_test.go
git commit -m "feat: add auto production deploy config"
```

## Task 2: Webhook Fan-Out

**Files:**
- Create: `internal/api/handlers/webhooks_test.go`
- Modify: `internal/api/handlers/webhooks.go`

- [ ] **Step 1: Write failing webhook fan-out tests**

Create `internal/api/handlers/webhooks_test.go`:

```go
package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

const webhookSecretForTests = "webhook-secret"

func setupWebhookProject(t *testing.T, autoProductionEnabled bool, previewBranches string) (*db.DB, *WebhookHandler, *db.Project) {
	t.Helper()
	database, enc, user := setupProjectTestDB(t)
	project := &db.Project{
		Name:           "webhook-app",
		GithubRepo:     "repo",
		GithubOwner:    "owner",
		Branch:         "main",
		UserID:         user.ID,
		Framework:      "nextjs",
		BuildCommand:   "bun run build",
		InstallCommand: "bun install",
		NodeVersion:    "22",
		Status:         "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	encryptedSecret, err := enc.Encrypt(webhookSecretForTests)
	if err != nil {
		t.Fatalf("encrypt webhook secret: %v", err)
	}
	webhookID := int64(123)
	if err := database.UpsertAutoBuildConfig(&db.AutoBuildConfig{
		ProjectID:             project.ID,
		Enabled:               true,
		ProductionBranch:      "main",
		PreviewBranches:       previewBranches,
		AutoProductionEnabled: autoProductionEnabled,
		WebhookID:             &webhookID,
		WebhookSecret:         encryptedSecret,
	}); err != nil {
		t.Fatalf("UpsertAutoBuildConfig: %v", err)
	}
	handler := &WebhookHandler{
		DB:        database,
		Encryptor: enc,
		Pipeline: &build.Pipeline{DB: database, Encryptor: enc, EnqueueOnly: true},
	}
	return database, handler, project
}

func sendGithubPush(t *testing.T, handler *WebhookHandler, deliveryID string, branch string) *httptest.ResponseRecorder {
	t.Helper()
	payload := map[string]any{
		"ref":     "refs/heads/" + branch,
		"after":   "abcdef1234567890",
		"deleted": false,
		"repository": map[string]any{
			"full_name": "owner/repo",
		},
		"pusher": map[string]any{
			"name": "alice",
		},
		"head_commit": map[string]any{
			"message": "Ship homepage",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	mac := hmac.New(sha256.New, []byte(webhookSecretForTests))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", deliveryID)
	req.Header.Set("X-Hub-Signature-256", signature)
	rr := httptest.NewRecorder()
	handler.HandleGithub(rr, req)
	return rr
}

func deploymentEnvironmentCounts(t *testing.T, database *db.DB, projectID string) map[string]int {
	t.Helper()
	deployments, err := database.ListDeployments(projectID, 10)
	if err != nil {
		t.Fatalf("ListDeployments: %v", err)
	}
	counts := map[string]int{}
	for _, deployment := range deployments {
		counts[deployment.Environment]++
		if deployment.Branch != "main" {
			t.Fatalf("deployment branch = %q, want main", deployment.Branch)
		}
		if deployment.CommitSHA != "abcdef1234567890" {
			t.Fatalf("deployment commit = %q, want webhook SHA", deployment.CommitSHA)
		}
		if deployment.TriggerSource != "webhook" {
			t.Fatalf("trigger_source = %q, want webhook", deployment.TriggerSource)
		}
	}
	return counts
}

func webhookEventCount(t *testing.T, database *db.DB, projectID, environment string) int {
	t.Helper()
	var count int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM webhook_events WHERE project_id = ? AND environment = ?`,
		projectID, environment,
	).Scan(&count); err != nil {
		t.Fatalf("count webhook events: %v", err)
	}
	return count
}

func TestWebhookMainBranchPreviewOnlyByDefault(t *testing.T) {
	database, handler, project := setupWebhookProject(t, false, "*")

	rr := sendGithubPush(t, handler, "delivery-preview-only", "main")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rr.Code, rr.Body.String())
	}

	counts := deploymentEnvironmentCounts(t, database, project.ID)
	if counts["preview"] != 1 {
		t.Fatalf("preview deployments = %d, want 1", counts["preview"])
	}
	if counts["production"] != 0 {
		t.Fatalf("production deployments = %d, want 0", counts["production"])
	}
	if webhookEventCount(t, database, project.ID, "preview") != 1 {
		t.Fatal("expected one preview webhook event")
	}
}

func TestWebhookMainBranchPreviewAndProductionWhenOptedIn(t *testing.T) {
	database, handler, project := setupWebhookProject(t, true, "*")

	rr := sendGithubPush(t, handler, "delivery-preview-production", "main")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rr.Code, rr.Body.String())
	}

	counts := deploymentEnvironmentCounts(t, database, project.ID)
	if counts["preview"] != 1 {
		t.Fatalf("preview deployments = %d, want 1", counts["preview"])
	}
	if counts["production"] != 1 {
		t.Fatalf("production deployments = %d, want 1", counts["production"])
	}
	if webhookEventCount(t, database, project.ID, "preview") != 1 {
		t.Fatal("expected one preview webhook event")
	}
	if webhookEventCount(t, database, project.ID, "production") != 1 {
		t.Fatal("expected one production webhook event")
	}
}

func TestWebhookNonMatchingBranchRecordsIgnored(t *testing.T) {
	database, handler, project := setupWebhookProject(t, true, "develop")

	rr := sendGithubPush(t, handler, "delivery-ignored", "feature")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rr.Code, rr.Body.String())
	}

	counts := deploymentEnvironmentCounts(t, database, project.ID)
	if counts["preview"] != 0 || counts["production"] != 0 {
		t.Fatalf("deployment counts = %#v, want no deployments", counts)
	}
	if webhookEventCount(t, database, project.ID, "ignored") != 1 {
		t.Fatal("expected one ignored webhook event")
	}
}

func TestWebhookRedeliveryDoesNotDuplicateDeployments(t *testing.T) {
	database, handler, project := setupWebhookProject(t, true, "*")

	sendGithubPush(t, handler, "delivery-redelivered", "main")
	sendGithubPush(t, handler, "delivery-redelivered", "main")

	counts := deploymentEnvironmentCounts(t, database, project.ID)
	if counts["preview"] != 1 {
		t.Fatalf("preview deployments = %d, want 1", counts["preview"])
	}
	if counts["production"] != 1 {
		t.Fatalf("production deployments = %d, want 1", counts["production"])
	}
}
```

- [ ] **Step 2: Run webhook tests to verify they fail**

Run:

```bash
go test ./internal/api/handlers -run 'TestWebhookMainBranch|TestWebhookNonMatchingBranch|TestWebhookRedelivery' -count=1
```

Expected: FAIL because the handler still chooses one environment and still calls `WebhookEventExists` with only the delivery ID.

- [ ] **Step 3: Add webhook target helpers**

In `internal/api/handlers/webhooks.go`, add these helpers below `matchesPreviewBranches`:

```go
func webhookTargetEnvironments(branch string, config db.AutoBuildConfig) []string {
	environments := make([]string, 0, 2)
	if matchesPreviewBranches(branch, config.PreviewBranches) {
		environments = append(environments, "preview")
	}
	if config.AutoProductionEnabled && branch == config.ProductionBranch {
		environments = append(environments, "production")
	}
	return environments
}

func webhookEventPayload(config db.AutoBuildConfig, deliveryID, environment, branch string, payload githubPushPayload, status string, deploymentID string, errorMsg *string) *db.WebhookEvent {
	return &db.WebhookEvent{
		ProjectID:        config.ProjectID,
		GithubDeliveryID: deliveryID,
		EventType:        "push",
		Environment:      environment,
		Branch:           branch,
		CommitSHA:        payload.After,
		CommitMessage:    commitMessageFromPayload(payload),
		Pusher:           payload.Pusher.Name,
		DeploymentID:     deploymentID,
		Status:           status,
		ErrorMessage:     errorMsg,
	}
}

func commitMessageFromPayload(payload githubPushPayload) string {
	if payload.HeadCommit == nil {
		return ""
	}
	return payload.HeadCommit.Message
}
```

- [ ] **Step 4: Replace global idempotency in `HandleGithub`**

Remove this block from `HandleGithub`:

```go
exists, err := h.DB.WebhookEventExists(deliveryID)
if err != nil {
	log.Printf("Webhook: failed to check idempotency for %s: %v", deliveryID, err)
}
if exists {
	w.WriteHeader(http.StatusOK)
	return
}
```

Also remove the local `commitMessage := ""` block because `commitMessageFromPayload` now centralizes that logic.

- [ ] **Step 5: Replace the environment decision and deployment creation loop**

Inside the `for _, config := range configs` loop, replace the code from `secret, err := h.Encryptor.Decrypt(config.WebhookSecret)` through `h.Pipeline.Dispatch(...)` with:

```go
	targets := webhookTargetEnvironments(branch, config)
	if len(targets) == 0 {
		exists, err := h.DB.WebhookEventExists(deliveryID, config.ProjectID, "ignored")
		if err != nil {
			log.Printf("Webhook: failed to check ignored idempotency for %s/%s: %v", deliveryID, config.ProjectID, err)
			continue
		}
		if !exists {
			if err := h.DB.CreateWebhookEvent(webhookEventPayload(config, deliveryID, "ignored", branch, payload, "ignored", "", nil)); err != nil {
				log.Printf("Webhook: failed to record ignored event for project %s: %v", config.ProjectID, err)
			}
		}
		continue
	}

	secret, err := h.Encryptor.Decrypt(config.WebhookSecret)
	if err != nil {
		log.Printf("Webhook: failed to decrypt secret for project %s: %v", config.ProjectID, err)
		errMsg := "failed to decrypt webhook secret"
		for _, environment := range targets {
			exists, existsErr := h.DB.WebhookEventExists(deliveryID, config.ProjectID, environment)
			if existsErr != nil || exists {
				continue
			}
			_ = h.DB.CreateWebhookEvent(webhookEventPayload(config, deliveryID, environment, branch, payload, "failed", "", &errMsg))
		}
		continue
	}

	if !validateGithubSignature(secret, body, signatureHeader) {
		log.Printf("Webhook: invalid signature for project %s", config.ProjectID)
		errMsg := "invalid HMAC signature"
		for _, environment := range targets {
			exists, existsErr := h.DB.WebhookEventExists(deliveryID, config.ProjectID, environment)
			if existsErr != nil || exists {
				continue
			}
			_ = h.DB.CreateWebhookEvent(webhookEventPayload(config, deliveryID, environment, branch, payload, "failed", "", &errMsg))
		}
		continue
	}

	project, err := h.DB.GetProject(config.ProjectID)
	if err != nil || project == nil {
		log.Printf("Webhook: project %s not found: %v", config.ProjectID, err)
		continue
	}

	user, err := h.DB.GetUserByID(project.UserID)
	if err != nil || user == nil {
		log.Printf("Webhook: project owner not found for %s: %v", config.ProjectID, err)
		continue
	}
	githubToken, err := h.Encryptor.Decrypt(user.GithubToken)
	if err != nil {
		log.Printf("Webhook: failed to decrypt token for project %s: %v", config.ProjectID, err)
		continue
	}

	for _, environment := range targets {
		exists, err := h.DB.WebhookEventExists(deliveryID, config.ProjectID, environment)
		if err != nil {
			log.Printf("Webhook: failed to check idempotency for %s/%s/%s: %v", deliveryID, config.ProjectID, environment, err)
			continue
		}
		if exists {
			continue
		}

		deployment := &db.Deployment{
			ProjectID:           project.ID,
			Environment:         environment,
			Branch:              branch,
			CommitSHA:           payload.After,
			CommitMessage:       commitMessageFromPayload(payload),
			Status:              "queued",
			TriggerSource:       "webhook",
			TriggeredByUsername: payload.Pusher.Name,
			TriggeredBy:         project.UserID,
		}
		if err := h.DB.CreateDeployment(deployment); err != nil {
			log.Printf("Webhook: failed to create %s deployment for project %s: %v", environment, config.ProjectID, err)
			continue
		}

		if err := h.DB.CreateWebhookEvent(webhookEventPayload(config, deliveryID, environment, branch, payload, "processed", deployment.ID, nil)); err != nil {
			log.Printf("Webhook: failed to create %s webhook event for project %s: %v", environment, config.ProjectID, err)
			continue
		}

		h.Pipeline.Dispatch(project, deployment, githubToken, func(line string, stream string) {
			log.Printf("[webhook-deploy:%s] %s", deployment.ID[:8], line)
		})
	}
```

- [ ] **Step 6: Run webhook tests**

Run:

```bash
go test ./internal/api/handlers -run 'TestWebhookMainBranch|TestWebhookNonMatchingBranch|TestWebhookRedelivery' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit webhook fan-out**

Run:

```bash
git add internal/api/handlers/webhooks.go internal/api/handlers/webhooks_test.go
git commit -m "feat: fan out auto deploy webhooks"
```

## Task 3: Auto-Build Settings API

**Files:**
- Modify: `internal/api/handlers/autobuild.go`

- [ ] **Step 1: Write the failing expectation in existing project-create tests**

In `internal/api/handlers/projects_test.go`, inside `TestProjectCreate_AutoDeploysAndProvisionsWebhook`, after the existing `if !config.Enabled` assertion, add:

```go
	if config.AutoProductionEnabled {
		t.Error("expected auto_production_enabled=false by default")
	}
```

This compiles after Task 1 and should pass once project creation still defaults the field to false.

- [ ] **Step 2: Update auto-build request and response structs**

In `internal/api/handlers/autobuild.go`, replace the request and response structs with:

```go
type autoBuildRequest struct {
	Enabled               bool   `json:"enabled"`
	ProductionBranch      string `json:"production_branch"`
	PreviewBranches       string `json:"preview_branches"`
	AutoProductionEnabled bool   `json:"auto_production_enabled"`
}

type autoBuildResponse struct {
	Enabled               bool   `json:"enabled"`
	ProductionBranch      string `json:"production_branch"`
	PreviewBranches       string `json:"preview_branches"`
	AutoProductionEnabled bool   `json:"auto_production_enabled"`
	WebhookActive          bool   `json:"webhook_active"`
	CreatedAt             string `json:"created_at"`
	UpdatedAt             string `json:"updated_at"`
}
```

- [ ] **Step 3: Thread the flag through `provisionWebhook`**

Update the `provisionWebhook` signature:

```go
func provisionWebhook(
	_ context.Context,
	database *db.DB,
	encryptor *crypto.Encryptor,
	project *db.Project,
	githubToken, webhookURL, productionBranch, previewBranches string,
	autoProductionEnabled bool,
) (*db.AutoBuildConfig, error) {
```

Inside its `db.AutoBuildConfig` literal, add:

```go
		AutoProductionEnabled: autoProductionEnabled,
```

Update all `provisionWebhook` callers to pass the boolean.

- [ ] **Step 4: Include the flag in GET and PUT responses**

In `Get`, the missing-config response should include:

```go
			AutoProductionEnabled: false,
```

The existing-config response should include:

```go
		AutoProductionEnabled: config.AutoProductionEnabled,
```

In `Put`, add `AutoProductionEnabled: req.AutoProductionEnabled` to both `db.AutoBuildConfig` literals and to the final JSON response. Add this audit metadata entry:

```go
			"auto_production_enabled": req.AutoProductionEnabled,
```

- [ ] **Step 5: Run auto-build related Go tests**

Run:

```bash
go test ./internal/api/handlers -run 'TestProjectCreate_AutoDeploysAndProvisionsWebhook|TestProjectCreate_NilPipelineDoesNotBlockCreation' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit settings API support**

Run:

```bash
git add internal/api/handlers/autobuild.go internal/api/handlers/projects_test.go
git commit -m "feat: expose production auto deploy setting"
```

## Task 4: Project Creation Auto-Deploy Flags

**Files:**
- Modify: `internal/api/handlers/projects.go`
- Modify: `internal/api/handlers/projects_test.go`

- [ ] **Step 1: Add failing project creation opt-in tests**

Append these tests to `internal/api/handlers/projects_test.go`:

```go
func TestProjectCreate_ProductionAutoDeployOptInCreatesInitialProductionDeployment(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     54321,
				"active": true,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	restore := github.SetTestAPIBase(ts.URL)
	defer restore()

	database, enc, user := setupProjectTestDB(t)
	pipeline := &build.Pipeline{DB: database, Encryptor: enc, EnqueueOnly: true}
	h := &ProjectHandler{
		DB:         database,
		Encryptor:  enc,
		Audit:      &audit.Recorder{DB: database},
		Pipeline:   pipeline,
		WebhookURL: ts.URL + "/webhook",
	}

	req := makeCreateProjectReq(t, user.ID, map[string]interface{}{
		"name":                    "prod-auto-app",
		"github_repo":             "my-repo",
		"github_owner":            "testuser",
		"branch":                  "main",
		"framework":               "nextjs",
		"auto_production_enabled": true,
	})
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var project db.Project
	if err := json.NewDecoder(w.Body).Decode(&project); err != nil {
		t.Fatalf("decode project response: %v", err)
	}
	config, err := database.GetAutoBuildConfig(project.ID)
	if err != nil {
		t.Fatalf("get auto build config: %v", err)
	}
	if config == nil {
		t.Fatal("expected auto-build config")
	}
	if !config.AutoProductionEnabled {
		t.Fatal("expected auto_production_enabled=true")
	}

	deployments, err := database.ListDeployments(project.ID, 10)
	if err != nil {
		t.Fatalf("list deployments: %v", err)
	}
	counts := map[string]int{}
	for _, deployment := range deployments {
		counts[deployment.Environment]++
	}
	if counts["preview"] != 1 {
		t.Fatalf("preview deployments = %d, want 1", counts["preview"])
	}
	if counts["production"] != 1 {
		t.Fatalf("production deployments = %d, want 1", counts["production"])
	}
}

func TestProjectCreate_CanDisableAutoBuildProvisioning(t *testing.T) {
	database, enc, user := setupProjectTestDB(t)
	pipeline := &build.Pipeline{DB: database, Encryptor: enc, EnqueueOnly: true}
	h := &ProjectHandler{
		DB:         database,
		Encryptor:  enc,
		Audit:      &audit.Recorder{DB: database},
		Pipeline:   pipeline,
		WebhookURL: "https://deployik.example.test/api/webhooks/github",
	}

	disabled := false
	req := makeCreateProjectReq(t, user.ID, map[string]interface{}{
		"name":               "manual-auto-app",
		"github_repo":        "my-repo",
		"github_owner":       "testuser",
		"branch":             "main",
		"framework":          "static",
		"auto_build_enabled": disabled,
	})
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var project db.Project
	if err := json.NewDecoder(w.Body).Decode(&project); err != nil {
		t.Fatalf("decode project response: %v", err)
	}
	config, err := database.GetAutoBuildConfig(project.ID)
	if err != nil {
		t.Fatalf("get auto build config: %v", err)
	}
	if config != nil {
		t.Fatalf("expected no auto-build config, got %+v", config)
	}
	deployments, err := database.ListDeployments(project.ID, 10)
	if err != nil {
		t.Fatalf("list deployments: %v", err)
	}
	if len(deployments) != 1 {
		t.Fatalf("expected initial preview deployment, got %d", len(deployments))
	}
	if deployments[0].Environment != "preview" {
		t.Fatalf("initial deployment environment = %q, want preview", deployments[0].Environment)
	}
}
```

- [ ] **Step 2: Run project creation tests to verify they fail**

Run:

```bash
go test ./internal/api/handlers -run 'TestProjectCreate_ProductionAutoDeployOptInCreatesInitialProductionDeployment|TestProjectCreate_CanDisableAutoBuildProvisioning' -count=1
```

Expected: FAIL because `createProjectRequest` ignores both new request fields.

- [ ] **Step 3: Extend `createProjectRequest`**

In `internal/api/handlers/projects.go`, add these fields to `createProjectRequest`:

```go
	AutoBuildEnabled     *bool `json:"auto_build_enabled"`
	AutoProductionEnabled bool  `json:"auto_production_enabled"`
```

- [ ] **Step 4: Apply project creation behavior**

In `Create`, replace the post-create setup block with:

```go
	autoBuildEnabled := true
	if req.AutoBuildEnabled != nil {
		autoBuildEnabled = *req.AutoBuildEnabled
	}

	// Best-effort: provision GitHub webhook + auto-build config.
	user, err := h.DB.GetUserByID(claims.UserID)
	if err != nil || user == nil {
		log.Printf("Warning: could not load user for post-create setup (project %s): %v", project.ID, err)
	} else {
		if autoBuildEnabled {
			h.setupAutoBuildBestEffort(r.Context(), project, user, req.AutoProductionEnabled)
		}
		h.dispatchInitialDeployBestEffort(r.Context(), project, user, "preview")
		if autoBuildEnabled && req.AutoProductionEnabled {
			h.dispatchInitialDeployBestEffort(r.Context(), project, user, "production")
		}
	}
```

- [ ] **Step 5: Update setup and initial deploy helpers**

Change `setupAutoBuildBestEffort` to accept and pass the new flag:

```go
func (h *ProjectHandler) setupAutoBuildBestEffort(ctx context.Context, project *db.Project, user *db.User, autoProductionEnabled bool) {
	if h.WebhookURL == "" {
		log.Printf("Warning: WebhookURL not configured; skipping auto-build setup for project %s", project.ID)
		return
	}
	token, err := h.Encryptor.Decrypt(user.GithubToken)
	if err != nil {
		log.Printf("Warning: failed to decrypt token for auto-build setup (project %s): %v", project.ID, err)
		return
	}
	config, err := provisionWebhook(ctx, h.DB, h.Encryptor, project, token, h.WebhookURL, project.Branch, "*", autoProductionEnabled)
	if err != nil {
		log.Printf("Warning: auto-build webhook setup failed for project %s: %v", project.ID, err)
		return
	}
	h.Audit.Record(audit.Entry{
		UserID:       user.ID,
		Action:       "auto_build_create",
		ResourceType: "auto_build_config",
		ResourceID:   config.ID,
		ProjectID:    project.ID,
		Metadata: map[string]any{
			"source":                  "project_create",
			"auto_production_enabled": autoProductionEnabled,
		},
	})
}
```

Change `dispatchInitialDeployBestEffort` to accept an environment:

```go
func (h *ProjectHandler) dispatchInitialDeployBestEffort(ctx context.Context, project *db.Project, user *db.User, environment string) {
	if h.Pipeline == nil {
		log.Printf("Warning: Pipeline not configured; skipping initial %s deploy for project %s", environment, project.ID)
		return
	}
	token, err := h.Encryptor.Decrypt(user.GithubToken)
	if err != nil {
		log.Printf("Warning: failed to decrypt token for initial deploy (project %s): %v", project.ID, err)
		return
	}
	deployment := &db.Deployment{
		ProjectID:           project.ID,
		Environment:         environment,
		Branch:              project.Branch,
		Status:              "queued",
		TriggerSource:       "api",
		TriggeredBy:         user.ID,
		TriggeredByUsername: user.Username,
	}
	if err := h.DB.CreateDeployment(deployment); err != nil {
		log.Printf("Warning: failed to create initial %s deployment for project %s: %v", environment, project.ID, err)
		return
	}
	h.Pipeline.Dispatch(project, deployment, token, func(line, stream string) {
		log.Printf("[deploy:%s] %s", deployment.ID[:8], line)
	})
	h.Audit.Record(audit.Entry{
		UserID:       user.ID,
		Action:       "deployment_create",
		ResourceType: "deployment",
		ResourceID:   deployment.ID,
		ProjectID:    project.ID,
		DeploymentID: deployment.ID,
		Metadata: map[string]any{
			"source":      "project_create",
			"environment": environment,
		},
	})
	_ = ctx
}
```

- [ ] **Step 6: Run project creation tests**

Run:

```bash
go test ./internal/api/handlers -run 'TestProjectCreate_' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit project creation behavior**

Run:

```bash
git add internal/api/handlers/projects.go internal/api/handlers/projects_test.go
git commit -m "feat: configure auto deploy during project import"
```

## Task 5: Frontend API Types

**Files:**
- Modify: `web/src/types/api.ts`
- Modify: `web/src/lib/api.ts`

- [ ] **Step 1: Add frontend type fields**

In `web/src/types/api.ts`, replace `AutoBuildConfig` with:

```ts
export interface AutoBuildConfig {
  enabled: boolean;
  production_branch: string;
  preview_branches: string;
  auto_production_enabled: boolean;
  webhook_active: boolean;
  created_at: string;
  updated_at: string;
}
```

- [ ] **Step 2: Update API method payload types**

In `web/src/lib/api.ts`, update `createProject` to accept the wizard flags:

```ts
  async createProject(
    data: Partial<Project> & {
      name: string;
      github_repo: string;
      github_owner: string;
      auto_build_enabled?: boolean;
      auto_production_enabled?: boolean;
    },
  ): Promise<Project> {
```

Update `updateAutoBuildConfig` to accept the new setting:

```ts
  async updateAutoBuildConfig(
    projectId: string,
    data: {
      enabled: boolean;
      production_branch: string;
      preview_branches: string;
      auto_production_enabled: boolean;
    },
  ): Promise<AutoBuildConfig> {
```

- [ ] **Step 3: Run frontend typecheck to verify remaining UI compile errors**

Run:

```bash
cd web && bunx tsc --noEmit
```

Expected: FAIL because `ProjectSettings.tsx` still calls `updateAutoBuildConfig` without `auto_production_enabled`.

- [ ] **Step 4: Commit frontend type plumbing after Task 6 passes**

Do not commit in this task yet. Commit these type changes with the settings UI in Task 6 so the frontend does not have an intentionally broken intermediate commit.

## Task 6: Project Settings UI

**Files:**
- Modify: `web/src/pages/ProjectSettings.tsx`

- [ ] **Step 1: Update React import**

Change the first line in `web/src/pages/ProjectSettings.tsx` to:

```ts
import { useEffect, useState } from "react";
```

- [ ] **Step 2: Add local state and config synchronization**

Inside `AutoBuildSection`, after `previewBranches` state, add:

```ts
  const [autoProductionEnabled, setAutoProductionEnabled] = useState(
    config?.auto_production_enabled ?? false,
  );

  useEffect(() => {
    if (!config) return;
    setProductionBranch(config.production_branch || defaultBranch || "main");
    setPreviewBranches(config.preview_branches || "*");
    setAutoProductionEnabled(config.auto_production_enabled ?? false);
  }, [config, defaultBranch]);
```

- [ ] **Step 3: Update mutation payload type and calls**

Change the mutation type to:

```ts
    mutationFn: (data: {
      enabled: boolean;
      production_branch: string;
      preview_branches: string;
      auto_production_enabled: boolean;
    }) => api.updateAutoBuildConfig(projectId, data),
```

Change `handleToggle` to:

```ts
  const handleToggle = (checked: boolean) => {
    if (!checked) {
      setAutoProductionEnabled(false);
    }
    updateMutation.mutate({
      enabled: checked,
      production_branch: productionBranch,
      preview_branches: previewBranches,
      auto_production_enabled: checked ? autoProductionEnabled : false,
    });
  };
```

Add this handler below `handleToggle`:

```ts
  const handleProductionToggle = (checked: boolean) => {
    setAutoProductionEnabled(checked);
    updateMutation.mutate({
      enabled: true,
      production_branch: productionBranch,
      preview_branches: previewBranches,
      auto_production_enabled: checked,
    });
  };
```

Change `handleSave` to:

```ts
  const handleSave = () => {
    updateMutation.mutate({
      enabled: true,
      production_branch: productionBranch,
      preview_branches: previewBranches,
      auto_production_enabled: autoProductionEnabled,
    });
  };
```

- [ ] **Step 4: Update section copy**

In the section intro, replace:

```tsx
            Automatically deploy when you push to GitHub. Configure which
            branches trigger preview and production deployments.
```

with:

```tsx
            Automatically deploy preview builds when you push to GitHub.
            Production deploys are an explicit opt-in for simple sites that
            keep preview and production on the same commit.
```

- [ ] **Step 5: Add the production auto-release switch**

Inside the `{enabled && (...)}` config panel, after the branch grid and before the Save button, insert:

```tsx
          <div className="flex items-start justify-between gap-4 rounded-lg border bg-muted/20 px-4 py-3">
            <div className="space-y-1">
              <Label htmlFor="auto-production-release">
                Auto-release production from production branch
              </Label>
              <p className="text-xs text-muted-foreground">
                When enabled, pushes to{" "}
                <span className="font-mono">{productionBranch || "main"}</span>{" "}
                create both preview and production deployments from the same
                commit.
              </p>
            </div>
            <Switch
              id="auto-production-release"
              checked={autoProductionEnabled}
              onCheckedChange={handleProductionToggle}
              disabled={updateMutation.isPending}
            />
          </div>
```

- [ ] **Step 6: Run frontend typecheck**

Run:

```bash
cd web && bunx tsc --noEmit
```

Expected: PASS.

- [ ] **Step 7: Commit frontend settings support**

Run:

```bash
git add web/src/types/api.ts web/src/lib/api.ts web/src/pages/ProjectSettings.tsx
git commit -m "feat: add production auto deploy setting UI"
```

## Task 7: New Project Wizard UI

**Files:**
- Modify: `web/src/pages/NewProject.tsx`

- [ ] **Step 1: Update imports**

Change the lucide import to:

```ts
import { Search, Lock, Globe, GitBranch, ArrowLeft, Webhook } from "lucide-react";
```

Add the switch import with the other UI imports:

```ts
import { Switch } from "@/components/ui/switch";
```

- [ ] **Step 2: Add wizard state**

After the `buildSettings` state, add:

```ts
  const [autoDeployPreview, setAutoDeployPreview] = useState(true);
  const [autoProductionEnabled, setAutoProductionEnabled] = useState(false);
```

- [ ] **Step 3: Send create-project flags**

In the `api.createProject` payload, after `port: buildSettings.port,` add:

```ts
        auto_build_enabled: autoDeployPreview,
        auto_production_enabled: autoDeployPreview && autoProductionEnabled,
```

- [ ] **Step 4: Reset flags when selecting a repo**

Inside the repo click handler, after `setBuildSettings(getFrameworkDefaults("nextjs", "auto"));`, add:

```ts
                    setAutoDeployPreview(true);
                    setAutoProductionEnabled(false);
```

- [ ] **Step 5: Add the Auto-Deploy section**

In the Configure Project form, insert this block after the branch select field and before `<BuildSettingsFields ... />`:

```tsx
        <div className="space-y-4 rounded-lg border p-4">
          <div className="flex items-start justify-between gap-4">
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <Webhook className="h-4 w-4 text-muted-foreground" />
                <h3 className="text-sm font-semibold">Auto-Deploy</h3>
              </div>
              <p className="text-sm text-muted-foreground">
                Keep the preview deployment updated from{" "}
                <span className="font-mono">
                  {branch || selectedRepo.default_branch}
                </span>
                .
              </p>
            </div>
            <Switch
              checked={autoDeployPreview}
              onCheckedChange={(checked) => {
                setAutoDeployPreview(checked);
                if (!checked) setAutoProductionEnabled(false);
              }}
            />
          </div>

          <div className="flex items-start justify-between gap-4 rounded-md border bg-muted/20 px-3 py-3">
            <div className="space-y-1">
              <Label htmlFor="new-project-auto-production">
                Also deploy production on every push to this branch
              </Label>
              <p className="text-xs text-muted-foreground">
                Use this for simple sites where preview and production should
                track the same commit.
              </p>
            </div>
            <Switch
              id="new-project-auto-production"
              checked={autoDeployPreview && autoProductionEnabled}
              onCheckedChange={setAutoProductionEnabled}
              disabled={!autoDeployPreview}
            />
          </div>
        </div>
```

- [ ] **Step 6: Run frontend typecheck**

Run:

```bash
cd web && bunx tsc --noEmit
```

Expected: PASS.

- [ ] **Step 7: Commit wizard support**

Run:

```bash
git add web/src/pages/NewProject.tsx
git commit -m "feat: add auto deploy options to project wizard"
```

## Task 8: Full Verification

**Files:**
- No file edits.

- [ ] **Step 1: Run Go tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 2: Run frontend build**

Run:

```bash
cd web && bun run build
```

Expected: PASS with TypeScript build and Vite build completing successfully.

- [ ] **Step 3: Review git diff for unrelated changes**

Run:

```bash
git status --short
git diff --stat HEAD
```

Expected: only files from this plan should be modified by the implementation branch. Existing unrelated worktree files may still appear if they were present before this work; do not stage or revert them.

- [ ] **Step 4: Rollout safety note**

Before deploying this migration to production, run the production backup command in the Deployik deployment environment. For this repository, do not run a production migration without first creating a backup of Deployik's SQLite database.

- [ ] **Step 5: Final commit if verification required fixes**

If verification required fixes after Task 7, commit the fixes:

```bash
git add internal web
git commit -m "fix: finalize production auto deploy opt-in"
```

If no files changed during verification, do not create an empty commit.

## Self-Review

- Spec coverage: The plan covers the new boolean, preview-first behavior, dual preview/production deployments from one push, no tag creation, settings UI, new project wizard, migration defaults, and tests.
- Placeholder scan: The plan contains no placeholder markers or open-ended implementation steps.
- Type consistency: The field name is `auto_production_enabled` in JSON, `AutoProductionEnabled` in Go, and `autoProductionEnabled` in React state.
