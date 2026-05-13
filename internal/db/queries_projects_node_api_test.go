package db

import "testing"

func TestProjectStartCommandAndHealthPathRoundtrip(t *testing.T) {
	t.Parallel()

	db, err := OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	userID := NewID()
	if _, err := db.Exec(`INSERT INTO users (id, github_id, username, github_token, role)
		VALUES (?, 1, 'tester', 'tok', 'user')`, userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	project := &Project{
		Name:         "node-api-sample",
		GithubRepo:   "sample",
		GithubOwner:  "tester",
		Branch:       "main",
		UserID:       userID,
		Framework:    "node-api",
		Status:       "active",
		StartCommand: "node bin/server.js",
		HealthPath:   "/api/health",
		Port:         3000,
	}
	if err := db.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	got, err := db.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got == nil {
		t.Fatal("GetProject returned nil")
	}
	if got.StartCommand != "node bin/server.js" {
		t.Errorf("StartCommand = %q, want %q", got.StartCommand, "node bin/server.js")
	}
	if got.HealthPath != "/api/health" {
		t.Errorf("HealthPath = %q, want %q", got.HealthPath, "/api/health")
	}

	// Update via the struct-based signature (UpdateProject takes *Project,
	// not a map). The function reads the struct's current values for every
	// updatable column, so set the new values and call it once.
	got.StartCommand = "bun run dist/main.js"
	got.HealthPath = "/healthz"
	if err := db.UpdateProject(got); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}

	fresh, err := db.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject after update: %v", err)
	}
	if fresh.StartCommand != "bun run dist/main.js" {
		t.Errorf("StartCommand after update = %q", fresh.StartCommand)
	}
	if fresh.HealthPath != "/healthz" {
		t.Errorf("HealthPath after update = %q", fresh.HealthPath)
	}

	// Confirm the ListProjects path also returns the new columns
	// (this catches scan-arg mismatch in ListProjects vs. GetProject).
	list, err := db.ListProjects(userID, "")
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	var foundInList *Project
	for i := range list {
		if list[i].ID == project.ID {
			foundInList = &list[i]
			break
		}
	}
	if foundInList == nil {
		t.Fatal("project missing from ListProjects")
	}
	if foundInList.StartCommand != "bun run dist/main.js" {
		t.Errorf("ListProjects StartCommand = %q", foundInList.StartCommand)
	}
	if foundInList.HealthPath != "/healthz" {
		t.Errorf("ListProjects HealthPath = %q", foundInList.HealthPath)
	}
}
