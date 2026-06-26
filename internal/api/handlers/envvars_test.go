package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/lefteq/lovinka-deployik/internal/auth"
	"github.com/lefteq/lovinka-deployik/internal/crypto"
	"github.com/lefteq/lovinka-deployik/internal/db"
)

func newVariableHandlerTestDB(t *testing.T) *db.DB {
	t.Helper()

	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	t.Cleanup(func() {
		database.Close()
	})

	return database
}

func newTestProject(t *testing.T, database *db.DB) *db.Project {
	t.Helper()

	user := &db.User{ID: db.NewID(), GithubID: 1, Username: "tester", Role: "admin"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	project := &db.Project{
		Name:           "proj",
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

func newVariableHandlerRequest(t *testing.T, method, path, projectID string, body any) *http.Request {
	t.Helper()

	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", projectID)
	if strings.Contains(path, "/env/") || strings.Contains(path, "/secrets/") {
		parts := strings.Split(path, "/")
		routeCtx.URLParams.Add("key", parts[len(parts)-1])
	}
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	return req.WithContext(auth.WithClaims(req.Context(), &auth.Claims{UserID: "admin", Role: "admin"}))
}

func TestVariableHandlerBulkSetRejectsPublicSecrets(t *testing.T) {
	database := newVariableHandlerTestDB(t)
	project := newTestProject(t, database)
	encryptor, err := crypto.NewEncryptor("test-key")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	handler := &VariableHandler{DB: database, Encryptor: encryptor, Kind: db.VariableKindSecret}
	req := newVariableHandlerRequest(t, http.MethodPut, "/projects/"+project.ID+"/secrets", project.ID, map[string]any{
		"environment": "shared",
		"variables": []map[string]string{
			{"key": "NEXT_PUBLIC_API_URL", "value": "https://api.example.com"},
		},
	})

	rec := httptest.NewRecorder()
	handler.BulkSet(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "NEXT_PUBLIC_") {
		t.Fatalf("expected NEXT_PUBLIC_ validation error, got %s", rec.Body.String())
	}
}

func TestVariableHandlerBulkSetRejectsInvalidKeys(t *testing.T) {
	database := newVariableHandlerTestDB(t)
	project := newTestProject(t, database)
	encryptor, err := crypto.NewEncryptor("test-key")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	handler := &VariableHandler{DB: database, Encryptor: encryptor, Kind: db.VariableKindEnv}
	req := newVariableHandlerRequest(t, http.MethodPut, "/projects/"+project.ID+"/env", project.ID, map[string]any{
		"environment": "shared",
		"variables": []map[string]string{
			{"key": "bad-key", "value": "https://api.example.com"},
		},
	})

	req = req.WithContext(auth.WithClaims(req.Context(), &auth.Claims{UserID: project.UserID, Role: "admin"}))
	rec := httptest.NewRecorder()
	handler.BulkSet(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "letters, numbers, and underscores") {
		t.Fatalf("expected invalid key error, got %s", rec.Body.String())
	}
}

func TestVariableHandlerBulkSetRejectsCrossStoreConflicts(t *testing.T) {
	database := newVariableHandlerTestDB(t)
	project := newTestProject(t, database)
	encryptor, err := crypto.NewEncryptor("test-key")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	encryptedValue, err := encryptor.Encrypt("https://preview.example.com")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if err := database.BulkSetEnvVars(project.ID, "preview", []db.ProjectVariable{
		{Key: "API_URL", Value: encryptedValue},
	}); err != nil {
		t.Fatalf("BulkSetEnvVars: %v", err)
	}

	handler := &VariableHandler{DB: database, Encryptor: encryptor, Kind: db.VariableKindSecret}
	req := newVariableHandlerRequest(t, http.MethodPut, "/projects/"+project.ID+"/secrets", project.ID, map[string]any{
		"environment": "production",
		"variables": []map[string]string{
			{"key": "API_URL", "value": "super-secret"},
		},
	})
	rec := httptest.NewRecorder()
	handler.BulkSet(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	if !strings.Contains(rec.Body.String(), "only one store per project") {
		t.Fatalf("expected cross-store conflict, got %s", rec.Body.String())
	}
}

func TestVariableHandlerListMasksValues(t *testing.T) {
	database := newVariableHandlerTestDB(t)
	project := newTestProject(t, database)
	encryptor, err := crypto.NewEncryptor("test-key")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	encryptedValue, err := encryptor.Encrypt("super-secret-value")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if err := database.BulkSetSecrets(project.ID, "shared", []db.ProjectVariable{
		{Key: "DATABASE_URL", Value: encryptedValue},
	}); err != nil {
		t.Fatalf("BulkSetSecrets: %v", err)
	}

	handler := &VariableHandler{DB: database, Encryptor: encryptor, Kind: db.VariableKindSecret}
	req := newVariableHandlerRequest(t, http.MethodGet, "/projects/"+project.ID+"/secrets?environment=shared", project.ID, nil)
	rec := httptest.NewRecorder()
	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload []variableResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("len(payload) = %d, want 1", len(payload))
	}
	if payload[0].Value == "super-secret-value" {
		t.Fatalf("value should be masked, got %q", payload[0].Value)
	}
	if payload[0].Kind != db.VariableKindSecret {
		t.Fatalf("kind = %q, want %q", payload[0].Kind, db.VariableKindSecret)
	}
	if payload[0].Environment != "shared" {
		t.Fatalf("environment = %q, want shared", payload[0].Environment)
	}
}
