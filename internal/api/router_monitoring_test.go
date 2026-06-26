package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lefteq/lovinka-deployik/internal/db"
)

// TestRouterMonitoringTargetsRoute verifies GET /api/monitoring/targets is
// registered in the PUBLIC route group (not behind JWT auth) and wired to the
// configured MONITORING_TOKEN. The 200-with-token case is the decisive check:
// if the route were inside the protected group, the JWT middleware would reject
// a non-JWT Bearer before the handler ran, yielding 401 instead of 200.
func TestRouterMonitoringTargetsRoute(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer database.Close()
	if err := database.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	user := &db.User{ID: db.NewID(), GithubID: 7, Username: "u", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	project := &db.Project{
		Name: "routed", GithubRepo: "routed", GithubOwner: "o", Branch: "main",
		UserID: user.ID, Framework: "nextjs", PackageManager: "auto",
		BuildCommand: "bun run build", InstallCommand: "bun install",
		NodeVersion: "22", Status: "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if err := database.CreateDomain(&db.Domain{
		ProjectID: project.ID, DomainName: "routed.example.com",
		Environment: "production", IsPrimary: true, SSLStatus: "active",
	}); err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}

	router := NewRouter(&RouterConfig{
		DB:              database,
		JWTSecret:       "test-secret",
		AllowedOrigins:  []string{"http://localhost"},
		MonitoringToken: "tok",
	})

	// No credential → the handler's own 401 (the request still reaches the
	// handler because the route is public).
	recNoAuth := httptest.NewRecorder()
	router.ServeHTTP(recNoAuth, httptest.NewRequest(http.MethodGet, "/api/monitoring/targets", nil))
	if recNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("no-token: got %d, want 401", recNoAuth.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/targets", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("with-token: got %d, want 200 (route must be public, not behind JWT auth)", rec.Code)
	}

	var body []struct {
		Targets []string `json:"targets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 1 || len(body[0].Targets) != 1 || body[0].Targets[0] != "https://routed.example.com" {
		t.Fatalf("unexpected body: %+v", body)
	}
}
