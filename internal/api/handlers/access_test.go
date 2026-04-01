package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/ws"
)

func addClaims(req *http.Request, userID, role string) *http.Request {
	claims := &auth.Claims{UserID: userID, Username: userID, Role: role}
	return req.WithContext(auth.WithClaims(req.Context(), claims))
}

func routeRequest(req *http.Request, params map[string]string) *http.Request {
	routeCtx := chi.NewRouteContext()
	for key, value := range params {
		routeCtx.URLParams.Add(key, value)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}

func TestProjectGetRejectsForeignUser(t *testing.T) {
	database := newVariableHandlerTestDB(t)
	project := newTestProject(t, database)

	handler := &ProjectHandler{DB: database}
	req := routeRequest(httptest.NewRequest(http.MethodGet, "/projects/"+project.ID, nil), map[string]string{"id": project.ID})
	req = addClaims(req, db.NewID(), "user")

	rec := httptest.NewRecorder()
	handler.Get(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeploymentTriggerRejectsForeignUser(t *testing.T) {
	database := newVariableHandlerTestDB(t)
	project := newTestProject(t, database)
	encryptor, err := crypto.NewEncryptor("test-key")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	otherUser := &db.User{ID: db.NewID(), GithubID: 2, Username: "other", GithubToken: "token", Role: "user"}
	if err := database.UpsertUser(otherUser); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	handler := &DeploymentHandler{
		DB:        database,
		Encryptor: encryptor,
		Pipeline:  &build.Pipeline{DB: database, Hub: ws.NewHub()},
	}
	req := routeRequest(httptest.NewRequest(http.MethodPost, "/projects/"+project.ID+"/deployments", nil), map[string]string{"id": project.ID})
	req = addClaims(req, otherUser.ID, otherUser.Role)

	rec := httptest.NewRecorder()
	handler.Trigger(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestEnvVarListRejectsForeignUser(t *testing.T) {
	database := newVariableHandlerTestDB(t)
	project := newTestProject(t, database)
	encryptor, err := crypto.NewEncryptor("test-key")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	handler := &VariableHandler{DB: database, Encryptor: encryptor, Kind: db.VariableKindEnv}
	req := routeRequest(httptest.NewRequest(http.MethodGet, "/projects/"+project.ID+"/env?environment=shared", nil), map[string]string{"id": project.ID})
	req = addClaims(req, db.NewID(), "user")

	rec := httptest.NewRecorder()
	handler.List(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestProjectGetAllowsAdminCrossTenantAccess(t *testing.T) {
	database := newVariableHandlerTestDB(t)
	project := newTestProject(t, database)

	handler := &ProjectHandler{DB: database}
	req := routeRequest(httptest.NewRequest(http.MethodGet, "/projects/"+project.ID, nil), map[string]string{"id": project.ID})
	req = addClaims(req, db.NewID(), "admin")

	rec := httptest.NewRecorder()
	handler.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload db.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload.ID != project.ID {
		t.Fatalf("project id = %q, want %q", payload.ID, project.ID)
	}
}
