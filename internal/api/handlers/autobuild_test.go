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

func TestAutoBuildPutPreservesExistingAutoProductionOptIn(t *testing.T) {
	database, encryptor, user := setupProjectTestDB(t)
	t.Cleanup(func() { database.Close() })

	project := &db.Project{
		Name:           "autobuild-preserve",
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
		t.Fatal("auto_production_enabled was cleared")
	}
}

func TestAutoBuildPutPreservesAutoProductionOptInWhenReprovisioningWebhook(t *testing.T) {
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

	project := &db.Project{
		Name:           "autobuild-reprovision",
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
		t.Fatal("auto_production_enabled was cleared during webhook reprovisioning")
	}
	if config.WebhookID == nil || *config.WebhookID != 54321 {
		t.Fatalf("webhook_id = %v, want 54321", config.WebhookID)
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
