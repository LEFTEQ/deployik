package db

import "testing"

func TestListAppDeployments(t *testing.T) {
	database := newTestDB(t)
	user := createAppTestUser(t, database, "owner", 1)
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	app, err := database.CreateApp(&AppCreate{OrganizationID: org.ID, Name: "Bundle"})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}

	member := &Project{
		Name: "web", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, OrganizationID: org.ID, Framework: "nextjs",
		PackageManager: "auto", Status: "active",
	}
	if err := database.CreateProject(member); err != nil {
		t.Fatalf("CreateProject member: %v", err)
	}
	if err := database.AddProjectsToApp(app.ID, []string{member.ID}); err != nil {
		t.Fatalf("AddProjectsToApp: %v", err)
	}
	standalone := &Project{
		Name: "lonely", GithubRepo: "r2", GithubOwner: "o", Branch: "main",
		UserID: user.ID, OrganizationID: org.ID, Framework: "static",
		PackageManager: "auto", Status: "active",
	}
	if err := database.CreateProject(standalone); err != nil {
		t.Fatalf("CreateProject standalone: %v", err)
	}

	if err := database.CreateDeployment(&Deployment{ProjectID: member.ID, Environment: "production", Branch: "main", Status: "live", TriggeredBy: user.ID, CommitSHA: "m1"}); err != nil {
		t.Fatalf("CreateDeployment member: %v", err)
	}
	if err := database.CreateDeployment(&Deployment{ProjectID: standalone.ID, Environment: "production", Branch: "main", Status: "live", TriggeredBy: user.ID, CommitSHA: "s1"}); err != nil {
		t.Fatalf("CreateDeployment standalone: %v", err)
	}

	got, err := database.ListAppDeployments(app.ID, "production", 10)
	if err != nil {
		t.Fatalf("ListAppDeployments: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected only the app member's deployment, got %d", len(got))
	}
	if got[0].ProjectName != "web" || got[0].CommitSHA != "m1" {
		t.Fatalf("unexpected row: %+v", got[0])
	}
}
