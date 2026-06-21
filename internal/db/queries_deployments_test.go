package db

import "testing"

func TestGetLatestDeployment(t *testing.T) {
	database := newTestDB(t)
	user := createAppTestUser(t, database, "owner", 1)
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	project := &Project{
		Name: "api", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, OrganizationID: org.ID, Framework: "static",
		PackageManager: "auto", Status: "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	older := &Deployment{ProjectID: project.ID, Environment: "production", Branch: "main", Status: "replaced", TriggeredBy: user.ID, CommitSHA: "aaa"}
	if err := database.CreateDeployment(older); err != nil {
		t.Fatalf("CreateDeployment older: %v", err)
	}
	newer := &Deployment{ProjectID: project.ID, Environment: "production", Branch: "main", Status: "live", TriggeredBy: user.ID, CommitSHA: "bbb"}
	if err := database.CreateDeployment(newer); err != nil {
		t.Fatalf("CreateDeployment newer: %v", err)
	}
	if err := database.CreateDeployment(&Deployment{ProjectID: project.ID, Environment: "preview", Branch: "dev", Status: "live", TriggeredBy: user.ID, CommitSHA: "ccc"}); err != nil {
		t.Fatalf("CreateDeployment preview: %v", err)
	}

	got, err := database.GetLatestDeployment(project.ID, "production")
	if err != nil {
		t.Fatalf("GetLatestDeployment: %v", err)
	}
	if got == nil || got.CommitSHA != "bbb" || got.Status != "live" {
		t.Fatalf("expected newest production deployment bbb/live, got %+v", got)
	}

	none, err := database.GetLatestDeployment(project.ID, "production-nope")
	if err != nil {
		t.Fatalf("GetLatestDeployment empty env: %v", err)
	}
	if none != nil {
		t.Fatalf("expected nil for env with no deployments, got %+v", none)
	}
}
