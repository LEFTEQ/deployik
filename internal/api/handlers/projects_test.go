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
	"github.com/LEFTEQ/lovinka-deployik/internal/projectconfig"
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
		t.Errorf("expected preview_branches=* (deploy all branches by default), got %q", config.PreviewBranches)
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

func TestProjectCreate_ProductionAutoDeployOptInSkipsProductionWhenAutoBuildSetupFails(t *testing.T) {
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
	pipeline := &build.Pipeline{DB: database, Encryptor: enc, EnqueueOnly: true}
	h := &ProjectHandler{
		DB:         database,
		Encryptor:  enc,
		Audit:      &audit.Recorder{DB: database},
		Pipeline:   pipeline,
		WebhookURL: ts.URL + "/webhook",
	}

	req := makeCreateProjectReq(t, user.ID, map[string]interface{}{
		"name":                    "prod-setup-fail-app",
		"github_repo":             "my-repo",
		"github_owner":            "testuser",
		"branch":                  "main",
		"framework":               "vite",
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
	if config != nil {
		t.Fatalf("expected no auto-build config, got %+v", config)
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
	if counts["production"] != 0 {
		t.Fatalf("production deployments = %d, want 0", counts["production"])
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
		"name":                    "manual-auto-app",
		"github_repo":             "my-repo",
		"github_owner":            "testuser",
		"branch":                  "main",
		"framework":               "static",
		"auto_build_enabled":      disabled,
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

// TestCreateProjectAcceptsNodeAPIFields verifies that POST /api/projects accepts
// `start_command` and `health_path` for the node-api framework and persists them
// verbatim (rather than letting the projectconfig defaults overwrite them).
func TestCreateProjectAcceptsNodeAPIFields(t *testing.T) {
	// Serve a fake GitHub API that accepts webhook creation.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     67890,
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
		"name":          "node-api-app",
		"github_repo":   "my-repo",
		"github_owner":  "testuser",
		"branch":        "main",
		"framework":     projectconfig.FrameworkNodeAPI,
		"start_command": "bun run dist/server.js",
		"health_path":   "/api/health",
		"port":          4321,
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

	if project.Framework != projectconfig.FrameworkNodeAPI {
		t.Errorf("framework = %q, want %q", project.Framework, projectconfig.FrameworkNodeAPI)
	}
	if project.StartCommand != "bun run dist/server.js" {
		t.Errorf("start_command = %q, want %q", project.StartCommand, "bun run dist/server.js")
	}
	if project.HealthPath != "/api/health" {
		t.Errorf("health_path = %q, want %q", project.HealthPath, "/api/health")
	}
	if project.Port != 4321 {
		t.Errorf("port = %d, want 4321", project.Port)
	}

	// And the values must be persisted, not just echoed in the response body.
	stored, err := database.GetProject(project.ID)
	if err != nil {
		t.Fatalf("load persisted project: %v", err)
	}
	if stored == nil {
		t.Fatal("expected persisted project, got nil")
	}
	if stored.StartCommand != "bun run dist/server.js" {
		t.Errorf("persisted start_command = %q, want %q", stored.StartCommand, "bun run dist/server.js")
	}
	if stored.HealthPath != "/api/health" {
		t.Errorf("persisted health_path = %q, want %q", stored.HealthPath, "/api/health")
	}
}

// TestUpdateProjectAcceptsNodeAPIFields verifies that PATCH /api/projects/{id}
// accepts and persists `start_command` and `health_path` on an existing project.
func TestUpdateProjectAcceptsNodeAPIFields(t *testing.T) {
	database, _, user := setupProjectTestDB(t)

	// Seed a project owned by the authed user, with node-api framework and
	// initial start_command/health_path values that we expect the PATCH to
	// overwrite.
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("ensure personal org: %v", err)
	}
	project := &db.Project{
		OrganizationID: org.ID,
		Name:           "existing-node-api",
		GithubRepo:     "my-repo",
		GithubOwner:    "testuser",
		Branch:         "main",
		UserID:         user.ID,
		Framework:      projectconfig.FrameworkNodeAPI,
		StartCommand:   "node dist/main.js",
		HealthPath:     "/health",
		Status:         "active",
	}
	if err := projectconfig.ApplyProjectDefaults(project); err != nil {
		t.Fatalf("apply defaults: %v", err)
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("create project: %v", err)
	}

	h := &ProjectHandler{
		DB:    database,
		Audit: &audit.Recorder{DB: database},
	}

	body, err := json.Marshal(map[string]interface{}{
		"start_command": "node bin/server.js",
		"health_path":   "/healthz",
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/projects/"+project.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := auth.WithClaims(req.Context(), &auth.Claims{UserID: user.ID})
	ctx = withChiID(ctx, "id", project.ID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Update(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var updated db.Project
	if err := json.NewDecoder(w.Body).Decode(&updated); err != nil {
		t.Fatalf("decode updated project: %v", err)
	}
	if updated.StartCommand != "node bin/server.js" {
		t.Errorf("response start_command = %q, want %q", updated.StartCommand, "node bin/server.js")
	}
	if updated.HealthPath != "/healthz" {
		t.Errorf("response health_path = %q, want %q", updated.HealthPath, "/healthz")
	}

	stored, err := database.GetProject(project.ID)
	if err != nil {
		t.Fatalf("load persisted project: %v", err)
	}
	if stored == nil {
		t.Fatal("expected persisted project, got nil")
	}
	if stored.StartCommand != "node bin/server.js" {
		t.Errorf("persisted start_command = %q, want %q", stored.StartCommand, "node bin/server.js")
	}
	if stored.HealthPath != "/healthz" {
		t.Errorf("persisted health_path = %q, want %q", stored.HealthPath, "/healthz")
	}
}

// TestUpdateProjectBlocksRenameWhenServicesExist ensures that PATCH /api/projects/{id}
// rejects a rename when one or more attached services exist for the project.
// Renaming would silently orphan the service container/volume, which is keyed on
// project.Name today.
func TestUpdateProjectBlocksRenameWhenServicesExist(t *testing.T) {
	database, _, user := setupProjectTestDB(t)

	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("ensure personal org: %v", err)
	}
	project := &db.Project{
		OrganizationID: org.ID,
		Name:           "renamable",
		GithubRepo:     "sample",
		GithubOwner:    "testuser",
		Branch:         "main",
		UserID:         user.ID,
		Framework:      projectconfig.FrameworkNodeAPI,
		Status:         "active",
	}
	if err := projectconfig.ApplyProjectDefaults(project); err != nil {
		t.Fatalf("apply defaults: %v", err)
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("create project: %v", err)
	}

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
	if err := database.CreateService(svc); err != nil {
		t.Fatalf("CreateService: %v", err)
	}

	h := &ProjectHandler{
		DB:    database,
		Audit: &audit.Recorder{DB: database},
	}

	body, err := json.Marshal(map[string]any{"name": "renamed"})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/projects/"+project.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := auth.WithClaims(req.Context(), &auth.Claims{UserID: user.ID})
	ctx = withChiID(ctx, "id", project.ID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Update(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d body=%s", w.Code, w.Body.String())
	}

	// And the project name must not have been mutated.
	stored, err := database.GetProject(project.ID)
	if err != nil {
		t.Fatalf("load persisted project: %v", err)
	}
	if stored == nil {
		t.Fatal("expected persisted project, got nil")
	}
	if stored.Name != "renamable" {
		t.Errorf("project name mutated: got %q, want %q", stored.Name, "renamable")
	}
}
