package ws

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

func TestLogsHandlerRejectsForeignDeploymentAccess(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	owner := &db.User{ID: db.NewID(), GithubID: 1, Username: "owner", Role: "admin"}
	if err := database.UpsertUser(owner); err != nil {
		t.Fatalf("UpsertUser(owner): %v", err)
	}
	other := &db.User{ID: db.NewID(), GithubID: 2, Username: "other", Role: "user"}
	if err := database.UpsertUser(other); err != nil {
		t.Fatalf("UpsertUser(other): %v", err)
	}

	project := &db.Project{
		Name:            "proj",
		GithubRepo:      "repo",
		GithubOwner:     "owner",
		Branch:          "main",
		UserID:          owner.ID,
		Framework:       "nextjs",
		OutputDirectory: ".next",
		BuildCommand:    "bun run build",
		InstallCommand:  "bun install",
		NodeVersion:     "22",
		Status:          "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	deployment := &db.Deployment{
		ProjectID:   project.ID,
		Environment: "preview",
		Branch:      "main",
		Status:      "queued",
		TriggeredBy: owner.ID,
	}
	if err := database.CreateDeployment(deployment); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	token, err := auth.GenerateAccessToken("secret", other.ID, other.Username, other.Role)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ws/deployments/"+deployment.ID+"/logs", nil)
	req.AddCookie(&http.Cookie{Name: auth.AccessCookieName, Value: token})
	req.SetPathValue("did", deployment.ID)
	rec := httptest.NewRecorder()

	LogsHandler(NewHub(), database, "secret", []string{"http://localhost:5173"})(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
