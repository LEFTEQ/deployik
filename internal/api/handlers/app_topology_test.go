package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

func TestDeriveTopologyEdges(t *testing.T) {
	members := []db.Project{{ID: "p-web", Name: "web"}, {ID: "p-api", Name: "api"}, {ID: "p-db", Name: "db"}}
	memberVars := map[string][]db.ProjectVariable{
		"p-web": {{Key: "API_URL", Kind: "env", Value: "http://deployik-api-production:4000"}},
		"p-api": {{Key: "DATABASE_URL", Kind: "secret", Value: "postgres://u:p@deployik-db-production:5432/app"}},
		"p-db":  {},
	}
	tokens := map[string][]string{
		"p-web": {"deployik-web-production"},
		"p-api": {"deployik-api-production"},
		"p-db":  {"deployik-db-production"},
	}

	edges := deriveTopologyEdges(members, memberVars, tokens)
	if len(edges) != 2 {
		t.Fatalf("expected 2 confirmed edges, got %d: %+v", len(edges), edges)
	}
	want := map[string]topologyEdge{
		"p-web->p-api": {Source: "p-web", Target: "p-api", Via: "API_URL", Kind: "env", Confirmed: true},
		"p-api->p-db":  {Source: "p-api", Target: "p-db", Via: "DATABASE_URL", Kind: "secret", Confirmed: true},
	}
	for _, e := range edges {
		w, ok := want[e.Source+"->"+e.Target]
		if !ok || e != w {
			t.Fatalf("unexpected edge %+v", e)
		}
	}
}

func TestDeriveTopologyEdgesNoFalseMesh(t *testing.T) {
	members := []db.Project{{ID: "a", Name: "a"}, {ID: "b", Name: "b"}}
	edges := deriveTopologyEdges(members,
		map[string][]db.ProjectVariable{"a": {{Key: "X", Kind: "env", Value: "literal"}}, "b": {}},
		map[string][]string{"a": {"deployik-a-production"}, "b": {"deployik-b-production"}},
	)
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges, got %+v", edges)
	}
}

func TestGetTopologyConfirmedEdge(t *testing.T) {
	database := appTestDB(t)
	enc, err := crypto.NewEncryptor("test-encryption-key-please-change-1")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
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
	web := &db.Project{Name: "web", GithubRepo: "r", GithubOwner: "o", Branch: "main", UserID: user.ID, OrganizationID: org.ID, Framework: "nextjs", PackageManager: "auto", Status: "active"}
	api := &db.Project{Name: "api", GithubRepo: "r2", GithubOwner: "o", Branch: "main", UserID: user.ID, OrganizationID: org.ID, Framework: "static", PackageManager: "auto", Status: "active"}
	if err := database.CreateProject(web); err != nil {
		t.Fatalf("CreateProject web: %v", err)
	}
	if err := database.CreateProject(api); err != nil {
		t.Fatalf("CreateProject api: %v", err)
	}
	if err := database.AddProjectsToApp(app.ID, []string{web.ID, api.ID}); err != nil {
		t.Fatalf("AddProjectsToApp: %v", err)
	}
	enc1, err := enc.Encrypt("http://deployik-api-production:3000")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if err := database.UpsertProjectVariable(&db.ProjectVariable{ProjectID: web.ID, Environment: "production", Kind: db.VariableKindEnv, Key: "API_URL", Value: enc1}); err != nil {
		t.Fatalf("UpsertProjectVariable: %v", err)
	}

	h := &AppHandler{DB: database, Encryptor: enc}
	claims := &auth.Claims{UserID: user.ID, Role: "user"}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", app.ID)
	req := httptest.NewRequest(http.MethodGet, "/apps/"+app.ID+"/topology?environment=production", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(auth.WithClaims(req.Context(), claims))
	rec := httptest.NewRecorder()

	h.GetTopology(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	var out appTopology
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(out.Nodes))
	}
	if len(out.Edges) != 1 || out.Edges[0].Source != web.ID || out.Edges[0].Target != api.ID || out.Edges[0].Via != "API_URL" {
		t.Fatalf("edges = %+v, want one web->api via API_URL (body: %s)", out.Edges, rec.Body.String())
	}
	// secret/var values must never leak into the payload
	if strings.Contains(rec.Body.String(), "deployik-api-production") {
		t.Fatalf("topology payload leaked a variable value (body: %s)", rec.Body.String())
	}
}
