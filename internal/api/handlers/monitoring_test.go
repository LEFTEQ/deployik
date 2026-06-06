package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

func seedProdTarget(t *testing.T, database *db.DB, userID string) {
	t.Helper()
	p := &db.Project{
		Name: "app1", GithubRepo: "app1", GithubOwner: "o", Branch: "main",
		UserID: userID, Framework: "nextjs", PackageManager: "auto",
		BuildCommand: "bun run build", InstallCommand: "bun install",
		NodeVersion: "22", Status: "active",
	}
	if err := database.CreateProject(p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if err := database.CreateDomain(&db.Domain{
		ProjectID: p.ID, DomainName: "app1.example.com",
		Environment: "production", IsPrimary: true, SSLStatus: "active",
	}); err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}
}

func TestMonitoringTargets_DisabledWhenNoToken(t *testing.T) {
	database, _, _ := setupProjectTestDB(t)
	h := &MonitoringHandler{DB: database, Token: ""}

	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/targets", nil)
	req.Header.Set("Authorization", "Bearer anything")
	rec := httptest.NewRecorder()
	h.Targets(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404 when MONITORING_TOKEN unset", rec.Code)
	}
}

func TestMonitoringTargets_Unauthorized(t *testing.T) {
	database, _, _ := setupProjectTestDB(t)
	h := &MonitoringHandler{DB: database, Token: "sekret"}

	cases := []struct {
		name string
		auth string
	}{
		{"missing", ""},
		{"wrong-token", "Bearer nope"},
		{"no-bearer-prefix", "sekret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/monitoring/targets", nil)
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			rec := httptest.NewRecorder()
			h.Targets(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status: got %d, want 401", rec.Code)
			}
		})
	}
}

func TestMonitoringTargets_OK(t *testing.T) {
	database, _, user := setupProjectTestDB(t)
	seedProdTarget(t, database, user.ID)

	h := &MonitoringHandler{DB: database, Token: "sekret"}
	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/targets", nil)
	req.Header.Set("Authorization", "Bearer sekret")
	rec := httptest.NewRecorder()
	h.Targets(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type: got %q, want application/json", ct)
	}

	var body []struct {
		Targets []string          `json:"targets"`
		Labels  map[string]string `json:"labels"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("got %d targets, want 1: %+v", len(body), body)
	}
	if len(body[0].Targets) != 1 || body[0].Targets[0] != "https://app1.example.com" {
		t.Errorf("targets: got %+v, want [https://app1.example.com]", body[0].Targets)
	}
	if got := body[0].Labels["project"]; got != "app1" {
		t.Errorf("label project: got %q, want app1", got)
	}
	if got := body[0].Labels["deployik_env"]; got != "production" {
		t.Errorf("label deployik_env: got %q, want production", got)
	}
	if got := body[0].Labels["protected"]; got != "false" {
		t.Errorf("label protected: got %q, want false", got)
	}
	if got := body[0].Labels["health_path"]; got != "/" {
		t.Errorf("label health_path: got %q, want / (default when unset)", got)
	}
}
