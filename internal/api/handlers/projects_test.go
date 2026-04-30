package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/github"
)

// setupProjectTestDB creates an in-memory DB, migrates it, creates a test
// encryptor and a user with an encrypted GitHub token, returning all three.
func setupProjectTestDB(t *testing.T) (*db.DB, *crypto.Encryptor, *db.User) {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	enc, err := crypto.NewEncryptor("test-encryption-key-32bytes-xxxx")
	if err != nil {
		t.Fatalf("new encryptor: %v", err)
	}
	encryptedToken, err := enc.Encrypt("fake-github-token")
	if err != nil {
		t.Fatalf("encrypt token: %v", err)
	}
	user := &db.User{
		ID:          db.NewID(),
		GithubID:    99001,
		Username:    "testuser",
		AvatarURL:   "",
		GithubToken: encryptedToken,
		Role:        "user",
	}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	// Ensure personal organization (required by resolveCreateOrganizationID).
	if _, err := database.EnsurePersonalOrganization(user); err != nil {
		t.Fatalf("ensure personal org: %v", err)
	}
	return database, enc, user
}

// makeCreateProjectReq builds a POST /api/projects HTTP request with chi auth claims injected.
func makeCreateProjectReq(t *testing.T, userID string, body map[string]interface{}) *http.Request {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/projects", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	ctx := auth.WithClaims(req.Context(), &auth.Claims{UserID: userID})
	return req.WithContext(ctx)
}

// TestProjectCreate_AutoDeploysAndProvisionsWebhook verifies the happy path:
// project creation triggers both webhook provisioning and an initial deployment.
func TestProjectCreate_AutoDeploysAndProvisionsWebhook(t *testing.T) {
	// Serve a fake GitHub API that accepts webhook creation.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     12345,
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

	// A nil-safe Pipeline (Wg and Ctx are nil — Dispatch guards both).
	pipeline := &build.Pipeline{
		DB:          database,
		Encryptor:   enc,
		EnqueueOnly: true,
	}

	h := &ProjectHandler{
		DB:         database,
		Encryptor:  enc,
		Audit:      &audit.Recorder{DB: database},
		Pipeline:   pipeline,
		WebhookURL: ts.URL + "/webhook",
	}

	req := makeCreateProjectReq(t, user.ID, map[string]interface{}{
		"name":         "my-app",
		"github_repo":  "my-repo",
		"github_owner": "testuser",
		"branch":       "main",
		"framework":    "nextjs",
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
	if project.Name != "my-app" {
		t.Errorf("unexpected project name: %q", project.Name)
	}

	// Assert auto_build_configs row was created with expected values.
	config, err := database.GetAutoBuildConfig(project.ID)
	if err != nil {
		t.Fatalf("get auto build config: %v", err)
	}
	if config == nil {
		t.Fatal("expected auto_build_configs row to be created, got nil")
	}
	if config.WebhookID == nil || *config.WebhookID != 12345 {
		t.Errorf("expected webhook_id=12345, got %v", config.WebhookID)
	}
	if config.ProductionBranch != "main" {
		t.Errorf("expected production_branch=main, got %q", config.ProductionBranch)
	}
	if config.PreviewBranches != "*" {
		t.Errorf("expected preview_branches=*, got %q", config.PreviewBranches)
	}
	if !config.Enabled {
		t.Error("expected auto-build config to be enabled")
	}
	if config.AutoProductionEnabled {
		t.Error("expected auto_production_enabled=false by default")
	}

	// Assert a queued preview deployment was created.
	deployments, err := database.ListDeployments(project.ID, 10)
	if err != nil {
		t.Fatalf("list deployments: %v", err)
	}
	if len(deployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deployments))
	}
	d := deployments[0]
	if d.Environment != "preview" {
		t.Errorf("expected environment=preview, got %q", d.Environment)
	}
	if d.TriggerSource != "api" {
		t.Errorf("expected trigger_source=api, got %q", d.TriggerSource)
	}
	if d.Status != "queued" {
		t.Errorf("expected status=queued, got %q", d.Status)
	}
}

// TestProjectCreate_AutoBuildScopeFailureDoesNotBlockCreation verifies that a
// 403 from the GitHub webhook API does NOT prevent project creation.
func TestProjectCreate_AutoBuildScopeFailureDoesNotBlockCreation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"message": "Resource not accessible by personal access token",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	restore := github.SetTestAPIBase(ts.URL)
	defer restore()

	database, enc, user := setupProjectTestDB(t)

	pipeline := &build.Pipeline{
		DB:          database,
		Encryptor:   enc,
		EnqueueOnly: true,
	}

	h := &ProjectHandler{
		DB:         database,
		Encryptor:  enc,
		Audit:      &audit.Recorder{DB: database},
		Pipeline:   pipeline,
		WebhookURL: ts.URL + "/webhook",
	}

	req := makeCreateProjectReq(t, user.ID, map[string]interface{}{
		"name":         "scope-fail-app",
		"github_repo":  "my-repo",
		"github_owner": "testuser",
		"branch":       "main",
		"framework":    "vite",
	})
	w := httptest.NewRecorder()
	h.Create(w, req)

	// Must still return 201.
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var project db.Project
	if err := json.NewDecoder(w.Body).Decode(&project); err != nil {
		t.Fatalf("decode project response: %v", err)
	}

	// No auto_build_configs row should exist (webhook failed).
	config, err := database.GetAutoBuildConfig(project.ID)
	if err != nil {
		t.Fatalf("get auto build config: %v", err)
	}
	if config != nil {
		t.Errorf("expected no auto_build_configs row when webhook failed, got %+v", config)
	}

	// But the initial deployment should still be created (pipeline != nil).
	deployments, err := database.ListDeployments(project.ID, 10)
	if err != nil {
		t.Fatalf("list deployments: %v", err)
	}
	if len(deployments) != 1 {
		t.Fatalf("expected 1 deployment even when webhook failed, got %d", len(deployments))
	}
}

// TestProjectCreate_NilPipelineDoesNotBlockCreation verifies that a nil Pipeline
// (no build server configured) does NOT prevent project creation.
func TestProjectCreate_NilPipelineDoesNotBlockCreation(t *testing.T) {
	// Serve a GitHub API stub that accepts webhook creation.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     99999,
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

	h := &ProjectHandler{
		DB:         database,
		Encryptor:  enc,
		Audit:      &audit.Recorder{DB: database},
		Pipeline:   nil, // intentionally nil
		WebhookURL: ts.URL + "/webhook",
	}

	req := makeCreateProjectReq(t, user.ID, map[string]interface{}{
		"name":         "nil-pipeline-app",
		"github_repo":  "my-repo",
		"github_owner": "testuser",
		"branch":       "develop",
		"framework":    "static",
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

	// auto_build_configs should have been created (webhook succeeded).
	config, err := database.GetAutoBuildConfig(project.ID)
	if err != nil {
		t.Fatalf("get auto build config: %v", err)
	}
	if config == nil {
		t.Fatal("expected auto_build_configs row when webhook succeeded, got nil")
	}

	// No deployment should exist (pipeline is nil).
	deployments, err := database.ListDeployments(project.ID, 10)
	if err != nil {
		t.Fatalf("list deployments: %v", err)
	}
	if len(deployments) != 0 {
		t.Errorf("expected 0 deployments when pipeline=nil, got %d", len(deployments))
	}
}
