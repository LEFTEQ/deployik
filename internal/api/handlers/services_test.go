package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/projectconfig"
	"github.com/LEFTEQ/lovinka-deployik/internal/services"
)

// seedServicesProject creates a project owned by user.ID inside their personal
// org and returns it. Used by the ServiceHandler tests so each test gets a
// fresh project to attach services to.
func seedServicesProject(t *testing.T, database *db.DB, user *db.User, name string) *db.Project {
	t.Helper()
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("ensure personal org: %v", err)
	}
	project := &db.Project{
		OrganizationID: org.ID,
		Name:           name,
		GithubRepo:     "svc-repo",
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
	return project
}

// newServiceHandler builds a ServiceHandler wired against the in-memory DB +
// test encryptor. Manager.Docker is left nil — Attach + List exercise only the
// DB + crypto code paths (container lifecycle is covered in services pkg
// tests).
func newServiceHandler(t *testing.T, database *db.DB, encryptor *crypto.Encryptor) *ServiceHandler {
	t.Helper()
	return &ServiceHandler{
		DB: database,
		Manager: &services.Manager{
			DB:        database,
			Encryptor: encryptor,
		},
		Encryptor: encryptor,
		Audit:     &audit.Recorder{DB: database},
	}
}

func TestServicesListEmpty(t *testing.T) {
	database, encryptor, user := setupProjectTestDB(t)
	project := seedServicesProject(t, database, user, "svc-list")
	h := newServiceHandler(t, database, encryptor)

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+project.ID+"/services", nil)
	req = req.WithContext(auth.WithClaims(req.Context(), &auth.Claims{UserID: user.ID, Role: "user"}))
	req = req.WithContext(withChiID(req.Context(), "id", project.ID))
	rec := httptest.NewRecorder()

	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d entries", len(list))
	}
}

func TestServicesAttachAndList(t *testing.T) {
	database, encryptor, user := setupProjectTestDB(t)
	project := seedServicesProject(t, database, user, "svc-attach")
	h := newServiceHandler(t, database, encryptor)

	body, _ := json.Marshal(map[string]any{"environment": "preview", "type": "postgres"})
	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+project.ID+"/services", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithClaims(req.Context(), &auth.Claims{UserID: user.ID, Role: "user"}))
	req = req.WithContext(withChiID(req.Context(), "id", project.ID))
	rec := httptest.NewRecorder()

	h.Attach(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("attach status = %d body = %s", rec.Code, rec.Body.String())
	}
	// The encrypted password column must never escape into the JSON response.
	respBody := rec.Body.String()
	if strings.Contains(respBody, "db_password_encrypted") {
		t.Error("attach response leaked db_password_encrypted field")
	}
	// And no plaintext password key should be present either — credentials are
	// only revealed by the dedicated /credentials endpoint (Task 12).
	if strings.Contains(respBody, `"db_password"`) || strings.Contains(respBody, `"password"`) {
		t.Errorf("attach response should not contain a plaintext password field: %s", respBody)
	}

	// Re-attach to same env must conflict.
	body2, _ := json.Marshal(map[string]any{"environment": "preview", "type": "postgres"})
	req2 := httptest.NewRequest(http.MethodPost, "/api/projects/"+project.ID+"/services", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	req2 = req2.WithContext(auth.WithClaims(req2.Context(), &auth.Claims{UserID: user.ID, Role: "user"}))
	req2 = req2.WithContext(withChiID(req2.Context(), "id", project.ID))
	rec2 := httptest.NewRecorder()
	h.Attach(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Errorf("re-attach should 409, got %d body=%s", rec2.Code, rec2.Body.String())
	}

	// List now returns one entry.
	listReq := httptest.NewRequest(http.MethodGet, "/api/projects/"+project.ID+"/services", nil)
	listReq = listReq.WithContext(auth.WithClaims(listReq.Context(), &auth.Claims{UserID: user.ID, Role: "user"}))
	listReq = listReq.WithContext(withChiID(listReq.Context(), "id", project.ID))
	listRec := httptest.NewRecorder()
	h.List(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d body = %s", listRec.Code, listRec.Body.String())
	}
	var list []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 service, got %d", len(list))
	}
	if list[0]["environment"] != "preview" {
		t.Errorf("environment = %v, want preview", list[0]["environment"])
	}
	if list[0]["type"] != "postgres" {
		t.Errorf("type = %v, want postgres", list[0]["type"])
	}
	if _, ok := list[0]["db_password_encrypted"]; ok {
		t.Error("list response leaked db_password_encrypted field")
	}
}
