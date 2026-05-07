package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/github"
)

func newAutoBuildRequest(t *testing.T, userID, projectID string, body any) *http.Request {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/projects/"+projectID+"/autobuild", bytes.NewReader(payload))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", projectID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	return req.WithContext(auth.WithClaims(req.Context(), &auth.Claims{UserID: userID, Role: "admin"}))
}

func newAutoBuildGetRequest(userID, projectID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/projects/"+projectID+"/autobuild", nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", projectID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	return req.WithContext(auth.WithClaims(req.Context(), &auth.Claims{UserID: userID, Role: "admin"}))
}

func createAutoBuildTestProject(t *testing.T, database *db.DB, user *db.User, name string) *db.Project {
	t.Helper()

	project := &db.Project{
		Name:           name,
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
	return project
}

func TestAutoBuildGetMissingConfigReturnsAutoProductionDisabled(t *testing.T) {
	database, encryptor, user := setupProjectTestDB(t)
	t.Cleanup(func() { database.Close() })

	project := createAutoBuildTestProject(t, database, user, "autobuild-get-missing")
	handler := &AutoBuildHandler{
		DB:        database,
		Encryptor: encryptor,
		Audit:     &audit.Recorder{DB: database},
	}
	req := newAutoBuildGetRequest(user.ID, project.ID)
	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	got, ok := body["auto_production_enabled"].(bool)
	if !ok {
		t.Fatalf("auto_production_enabled missing or not bool in response: %#v", body)
	}
	if got {
		t.Fatal("auto_production_enabled = true, want false")
	}
}

func TestAutoBuildGetMissingConfigDefaultsProductionBranchToProjectBranchAndPreviewBranchesToWildcard(t *testing.T) {
	database, encryptor, user := setupProjectTestDB(t)
	t.Cleanup(func() { database.Close() })

	project := createAutoBuildTestProject(t, database, user, "autobuild-get-project-branch")
	project.Branch = "develop"
	if err := database.UpdateProject(project); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	handler := &AutoBuildHandler{
		DB:        database,
		Encryptor: encryptor,
		Audit:     &audit.Recorder{DB: database},
	}
	req := newAutoBuildGetRequest(user.ID, project.ID)
	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if got := body["production_branch"]; got != "develop" {
		t.Fatalf("production_branch = %#v, want develop", got)
	}
	if got := body["preview_branches"]; got != "*" {
		t.Fatalf("preview_branches = %#v, want * (default = all branches)", got)
	}
}

func TestAutoBuildPutDefaultsProductionBranchToProjectBranchAndPreviewBranchesToWildcard(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     98765,
			"active": true,
		})
	}))
	defer ts.Close()

	restore := github.SetTestAPIBase(ts.URL)
	defer restore()

	database, encryptor, user := setupProjectTestDB(t)
	t.Cleanup(func() { database.Close() })

	project := createAutoBuildTestProject(t, database, user, "autobuild-put-project-branch")
	project.Branch = "develop"
	if err := database.UpdateProject(project); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	handler := &AutoBuildHandler{
		DB:         database,
		Encryptor:  encryptor,
		Audit:      &audit.Recorder{DB: database},
		WebhookURL: ts.URL + "/webhook",
	}
	req := newAutoBuildRequest(t, user.ID, project.ID, map[string]any{
		"enabled": true,
	})
	rec := httptest.NewRecorder()

	handler.Put(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	config, err := database.GetAutoBuildConfig(project.ID)
	if err != nil {
		t.Fatalf("GetAutoBuildConfig: %v", err)
	}
	if config == nil {
		t.Fatal("expected auto-build config")
	}
	if config.ProductionBranch != "develop" {
		t.Fatalf("production_branch = %q, want develop", config.ProductionBranch)
	}
	if config.PreviewBranches != "*" {
		t.Fatalf("preview_branches = %q, want * (default = all branches)", config.PreviewBranches)
	}
}

func TestAutoBuildPutPersistsAutoProductionEnabledTrue(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     54321,
			"active": true,
		})
	}))
	defer ts.Close()

	restore := github.SetTestAPIBase(ts.URL)
	defer restore()

	database, encryptor, user := setupProjectTestDB(t)
	t.Cleanup(func() { database.Close() })

	project := createAutoBuildTestProject(t, database, user, "autobuild-put-enable-production")
	handler := &AutoBuildHandler{
		DB:         database,
		Encryptor:  encryptor,
		Audit:      &audit.Recorder{DB: database},
		WebhookURL: ts.URL + "/webhook",
	}
	req := newAutoBuildRequest(t, user.ID, project.ID, map[string]any{
		"enabled":                 true,
		"production_branch":       "release",
		"preview_branches":        "*",
		"auto_production_enabled": true,
	})
	rec := httptest.NewRecorder()

	handler.Put(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	config, err := database.GetAutoBuildConfig(project.ID)
	if err != nil {
		t.Fatalf("GetAutoBuildConfig: %v", err)
	}
	if config == nil {
		t.Fatal("expected auto-build config")
	}
	if !config.AutoProductionEnabled {
		t.Fatal("auto_production_enabled = false in database, want true")
	}
	if config.WebhookID == nil || *config.WebhookID != 54321 {
		t.Fatalf("webhook_id = %v, want 54321", config.WebhookID)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if got, ok := body["auto_production_enabled"].(bool); !ok || !got {
		t.Fatalf("auto_production_enabled response = %#v, want true", body["auto_production_enabled"])
	}
}

func TestAutoBuildPutOmittingAutoProductionPreservesExistingTrue(t *testing.T) {
	database, encryptor, user := setupProjectTestDB(t)
	t.Cleanup(func() { database.Close() })

	project := createAutoBuildTestProject(t, database, user, "autobuild-put-omit-preserve")
	webhookID := int64(12345)
	if err := database.UpsertAutoBuildConfig(&db.AutoBuildConfig{
		ProjectID:             project.ID,
		Enabled:               true,
		ProductionBranch:      "main",
		PreviewBranches:       "*",
		AutoProductionEnabled: true,
		WebhookID:             &webhookID,
		WebhookSecret:         "encrypted-secret",
	}); err != nil {
		t.Fatalf("UpsertAutoBuildConfig: %v", err)
	}

	handler := &AutoBuildHandler{
		DB:        database,
		Encryptor: encryptor,
		Audit:     &audit.Recorder{DB: database},
	}
	req := newAutoBuildRequest(t, user.ID, project.ID, map[string]any{
		"enabled":           true,
		"production_branch": "release",
		"preview_branches":  "*",
	})
	rec := httptest.NewRecorder()

	handler.Put(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	config, err := database.GetAutoBuildConfig(project.ID)
	if err != nil {
		t.Fatalf("GetAutoBuildConfig: %v", err)
	}
	if config == nil {
		t.Fatal("expected auto-build config")
	}
	if !config.AutoProductionEnabled {
		t.Fatal("auto_production_enabled = false in database, want preserved true")
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if got, ok := body["auto_production_enabled"].(bool); !ok || !got {
		t.Fatalf("auto_production_enabled response = %#v, want true", body["auto_production_enabled"])
	}
}

func TestAutoBuildPutOmittingAutoProductionPreservesExistingTrueWhenReprovisioning(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     54321,
			"active": true,
		})
	}))
	defer ts.Close()

	restore := github.SetTestAPIBase(ts.URL)
	defer restore()

	database, encryptor, user := setupProjectTestDB(t)
	t.Cleanup(func() { database.Close() })

	project := createAutoBuildTestProject(t, database, user, "autobuild-put-omit-reprovision")
	if err := database.UpsertAutoBuildConfig(&db.AutoBuildConfig{
		ProjectID:             project.ID,
		Enabled:               true,
		ProductionBranch:      "main",
		PreviewBranches:       "*",
		AutoProductionEnabled: true,
		WebhookSecret:         "missing-webhook-secret",
	}); err != nil {
		t.Fatalf("UpsertAutoBuildConfig: %v", err)
	}

	handler := &AutoBuildHandler{
		DB:         database,
		Encryptor:  encryptor,
		Audit:      &audit.Recorder{DB: database},
		WebhookURL: ts.URL + "/webhook",
	}
	req := newAutoBuildRequest(t, user.ID, project.ID, map[string]any{
		"enabled":           true,
		"production_branch": "release",
		"preview_branches":  "*",
	})
	rec := httptest.NewRecorder()

	handler.Put(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	config, err := database.GetAutoBuildConfig(project.ID)
	if err != nil {
		t.Fatalf("GetAutoBuildConfig: %v", err)
	}
	if config == nil {
		t.Fatal("expected auto-build config")
	}
	if !config.AutoProductionEnabled {
		t.Fatal("auto_production_enabled = false in database, want preserved true")
	}
	if config.WebhookID == nil || *config.WebhookID != 54321 {
		t.Fatalf("webhook_id = %v, want 54321", config.WebhookID)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if got, ok := body["auto_production_enabled"].(bool); !ok || !got {
		t.Fatalf("auto_production_enabled response = %#v, want true", body["auto_production_enabled"])
	}
}

func TestAutoBuildPutCanDisableAutoProductionEnabled(t *testing.T) {
	database, encryptor, user := setupProjectTestDB(t)
	t.Cleanup(func() { database.Close() })

	project := createAutoBuildTestProject(t, database, user, "autobuild-put-disable-production")
	webhookID := int64(12345)
	if err := database.UpsertAutoBuildConfig(&db.AutoBuildConfig{
		ProjectID:             project.ID,
		Enabled:               true,
		ProductionBranch:      "main",
		PreviewBranches:       "*",
		AutoProductionEnabled: true,
		WebhookID:             &webhookID,
		WebhookSecret:         "encrypted-secret",
	}); err != nil {
		t.Fatalf("UpsertAutoBuildConfig: %v", err)
	}

	handler := &AutoBuildHandler{
		DB:        database,
		Encryptor: encryptor,
		Audit:     &audit.Recorder{DB: database},
	}
	req := newAutoBuildRequest(t, user.ID, project.ID, map[string]any{
		"enabled":                 true,
		"production_branch":       "release",
		"preview_branches":        "*",
		"auto_production_enabled": false,
	})
	rec := httptest.NewRecorder()

	handler.Put(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	config, err := database.GetAutoBuildConfig(project.ID)
	if err != nil {
		t.Fatalf("GetAutoBuildConfig: %v", err)
	}
	if config == nil {
		t.Fatal("expected auto-build config")
	}
	if config.AutoProductionEnabled {
		t.Fatal("auto_production_enabled = true in database, want false")
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if got, ok := body["auto_production_enabled"].(bool); !ok || got {
		t.Fatalf("auto_production_enabled response = %#v, want false", body["auto_production_enabled"])
	}
}

func TestWebhookTargetEnvironmentsRequiresProductionOptIn(t *testing.T) {
	config := db.AutoBuildConfig{
		ProductionBranch:      "main",
		PreviewBranches:       "*",
		AutoProductionEnabled: false,
	}
	if got := webhookTargetEnvironments("main", config); len(got) != 1 || got[0] != "preview" {
		t.Fatalf("webhookTargetEnvironments without production opt-in = %#v, want [preview]", got)
	}

	config.AutoProductionEnabled = true
	if got := webhookTargetEnvironments("main", config); len(got) != 2 || got[0] != "preview" || got[1] != "production" {
		t.Fatalf("webhookTargetEnvironments with production opt-in = %#v, want [preview production]", got)
	}
}
