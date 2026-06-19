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

func TestAppHandlerDeployDisabledWithoutPipeline(t *testing.T) {
	database := appTestDB(t)
	user := &db.User{ID: db.NewID(), GithubID: 1, Username: "owner", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	app, err := database.CreateApp(&db.AppCreate{OrganizationID: org.ID, Name: "Bundle"})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}

	h := &AppHandler{DB: database} // no Pipeline
	claims := &auth.Claims{UserID: user.ID, Role: "user"}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", app.ID)
	req := httptest.NewRequest(http.MethodPost, "/apps/"+app.ID+"/deploy", bytes.NewReader([]byte(`{"environment":"production"}`)))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(auth.WithClaims(req.Context(), claims))
	rec := httptest.NewRecorder()
	h.Deploy(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("Deploy without pipeline = %d, want 503", rec.Code)
	}
}

func TestAppHandlerListReleases(t *testing.T) {
	database := appTestDB(t)
	user := &db.User{ID: db.NewID(), GithubID: 1, Username: "owner", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	app, err := database.CreateApp(&db.AppCreate{OrganizationID: org.ID, Name: "Bundle"})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	if _, err := database.CreateAppRelease(&db.AppRelease{AppID: app.ID, Environment: "production", Status: "succeeded"}, nil); err != nil {
		t.Fatalf("CreateAppRelease: %v", err)
	}

	h := &AppHandler{DB: database}
	claims := &auth.Claims{UserID: user.ID, Role: "user"}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", app.ID)
	req := httptest.NewRequest(http.MethodGet, "/apps/"+app.ID+"/releases?environment=production", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(auth.WithClaims(req.Context(), claims))
	rec := httptest.NewRecorder()
	h.ListReleases(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListReleases status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var releases []db.AppRelease
	if err := json.Unmarshal(rec.Body.Bytes(), &releases); err != nil {
		t.Fatalf("decode releases: %v", err)
	}
	if len(releases) != 1 || releases[0].Status != "succeeded" {
		t.Fatalf("releases = %+v, want one succeeded", releases)
	}
}
