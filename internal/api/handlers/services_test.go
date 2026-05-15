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

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/projectconfig"
	"github.com/LEFTEQ/lovinka-deployik/internal/services"
)

// withChiParams adds (or replaces) URL params on the chi RouteContext stored in
// ctx, preserving any params already present. The shared withChiID helper in
// tokens_test.go creates a fresh RouteContext on each call, which overwrites
// earlier params — Detach/Credentials/RegeneratePassword tests need BOTH "id"
// and "env" on the same context, so they route through this helper instead.
func withChiParams(ctx context.Context, params map[string]string) context.Context {
	var rc *chi.Context
	if existing, ok := ctx.Value(chi.RouteCtxKey).(*chi.Context); ok && existing != nil {
		rc = existing
	} else {
		rc = chi.NewRouteContext()
	}
	for k, v := range params {
		rc.URLParams.Add(k, v)
	}
	return context.WithValue(ctx, chi.RouteCtxKey, rc)
}

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

func TestServicesDetach(t *testing.T) {
	database, encryptor, user := setupProjectTestDB(t)
	project := seedServicesProject(t, database, user, "svc-detach")
	h := newServiceHandler(t, database, encryptor)

	// Attach first.
	attach := func() *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"environment": "preview", "type": "postgres"})
		r := httptest.NewRequest(http.MethodPost, "/api/projects/"+project.ID+"/services", strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		r = r.WithContext(auth.WithClaims(r.Context(), &auth.Claims{UserID: user.ID, Role: "user"}))
		r = r.WithContext(withChiID(r.Context(), "id", project.ID))
		rec := httptest.NewRecorder()
		h.Attach(rec, r)
		return rec
	}
	if rec := attach(); rec.Code != http.StatusCreated {
		t.Fatalf("attach status %d body=%s", rec.Code, rec.Body.String())
	}

	// Detach preview.
	detachReq := httptest.NewRequest(http.MethodDelete, "/api/projects/"+project.ID+"/services/preview", nil)
	detachReq = detachReq.WithContext(auth.WithClaims(detachReq.Context(), &auth.Claims{UserID: user.ID, Role: "user"}))
	detachReq = detachReq.WithContext(withChiParams(detachReq.Context(), map[string]string{
		"id":  project.ID,
		"env": "preview",
	}))
	detachRec := httptest.NewRecorder()
	h.Detach(detachRec, detachReq)

	if detachRec.Code != http.StatusNoContent {
		t.Fatalf("detach status = %d body = %s", detachRec.Code, detachRec.Body.String())
	}

	// List should now be empty.
	listReq := httptest.NewRequest(http.MethodGet, "/api/projects/"+project.ID+"/services", nil)
	listReq = listReq.WithContext(auth.WithClaims(listReq.Context(), &auth.Claims{UserID: user.ID, Role: "user"}))
	listReq = listReq.WithContext(withChiID(listReq.Context(), "id", project.ID))
	listRec := httptest.NewRecorder()
	h.List(listRec, listReq)
	var list []serviceResponse
	_ = json.Unmarshal(listRec.Body.Bytes(), &list)
	if len(list) != 0 {
		t.Errorf("expected empty after detach, got %d", len(list))
	}
}

func TestServicesCredentialsRevealAndRegenerate(t *testing.T) {
	database, encryptor, user := setupProjectTestDB(t)
	project := seedServicesProject(t, database, user, "svc-creds")
	h := newServiceHandler(t, database, encryptor)

	// Attach production.
	body, _ := json.Marshal(map[string]any{"environment": "production", "type": "postgres"})
	r := httptest.NewRequest(http.MethodPost, "/api/projects/"+project.ID+"/services", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(auth.WithClaims(r.Context(), &auth.Claims{UserID: user.ID, Role: "user"}))
	r = r.WithContext(withChiID(r.Context(), "id", project.ID))
	if rec := httptest.NewRecorder(); true {
		h.Attach(rec, r)
		if rec.Code != http.StatusCreated {
			t.Fatalf("attach status %d body=%s", rec.Code, rec.Body.String())
		}
	}

	// Read credentials.
	credReq := httptest.NewRequest(http.MethodGet, "/api/projects/"+project.ID+"/services/production/credentials", nil)
	credReq = credReq.WithContext(auth.WithClaims(credReq.Context(), &auth.Claims{UserID: user.ID, Role: "user"}))
	credReq = credReq.WithContext(withChiParams(credReq.Context(), map[string]string{
		"id":  project.ID,
		"env": "production",
	}))
	credRec := httptest.NewRecorder()
	h.Credentials(credRec, credReq)

	if credRec.Code != http.StatusOK {
		t.Fatalf("credentials status = %d body = %s", credRec.Code, credRec.Body.String())
	}
	var creds map[string]any
	_ = json.Unmarshal(credRec.Body.Bytes(), &creds)
	if creds["db_name"] != "app" || creds["db_user"] != "app" {
		t.Errorf("missing db_name/db_user: %v", creds)
	}
	pwd, _ := creds["password"].(string)
	if pwd == "" {
		t.Error("credentials response missing plaintext password")
	}

	// Regenerate must change the password.
	regenReq := httptest.NewRequest(http.MethodPost, "/api/projects/"+project.ID+"/services/production/regenerate-password", nil)
	regenReq = regenReq.WithContext(auth.WithClaims(regenReq.Context(), &auth.Claims{UserID: user.ID, Role: "user"}))
	regenReq = regenReq.WithContext(withChiParams(regenReq.Context(), map[string]string{
		"id":  project.ID,
		"env": "production",
	}))
	regenRec := httptest.NewRecorder()
	h.RegeneratePassword(regenRec, regenReq)

	if regenRec.Code != http.StatusOK {
		t.Fatalf("regenerate status = %d body = %s", regenRec.Code, regenRec.Body.String())
	}
	var newCreds map[string]any
	_ = json.Unmarshal(regenRec.Body.Bytes(), &newCreds)
	newPwd, _ := newCreds["password"].(string)
	if newPwd == "" || newPwd == pwd {
		t.Errorf("regenerate should change password; old=%q new=%q", pwd, newPwd)
	}
}
