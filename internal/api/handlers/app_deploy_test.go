package handlers

import (
	"testing"

	"github.com/lefteq/lovinka-deployik/internal/db"
)

// TestAppMemberTriggerSourceIsDBValid guards the regression where coordinated
// app deploys/rollbacks recorded trigger_source values ("app_deploy" /
// "app_rollback") that the deployments CHECK constraint (migration 008) rejects.
// That made CreateDeployment fail on member 0, aborting the whole rollout before
// any member built. The value the app deploy/rollback paths write MUST be
// accepted by CreateDeployment.
func TestAppMemberTriggerSourceIsDBValid(t *testing.T) {
	database := appTestDB(t)
	user := &db.User{ID: db.NewID(), GithubID: 1, Username: "owner", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	project := &db.Project{
		Name: "api", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, OrganizationID: org.ID, Framework: "static",
		PackageManager: "auto", Status: "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	dep := &db.Deployment{
		ProjectID:     project.ID,
		Environment:   "preview",
		Branch:        "main",
		Status:        "queued",
		TriggeredBy:   user.ID,
		TriggerSource: appMemberTriggerSource,
	}
	if err := database.CreateDeployment(dep); err != nil {
		t.Fatalf("CreateDeployment with app-member trigger source %q failed (CHECK constraint?): %v", appMemberTriggerSource, err)
	}
}
