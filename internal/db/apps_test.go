package db

import (
	"database/sql"
	"testing"
)

func TestMigration026CreatesAppsSchema(t *testing.T) {
	database := newTestDB(t)

	// apps table exists
	var tableName string
	err := database.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='apps'`,
	).Scan(&tableName)
	if err != nil {
		t.Fatalf("apps table not found: %v", err)
	}

	// projects.app_id column exists and defaults to NULL
	var appIDColumns int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('projects') WHERE name='app_id'`,
	).Scan(&appIDColumns); err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	if appIDColumns != 1 {
		t.Fatalf("expected projects.app_id column, found %d", appIDColumns)
	}
}

func createAppTestUser(t *testing.T, database *DB, username string, githubID int64) *User {
	t.Helper()
	user := &User{ID: NewID(), GithubID: githubID, Username: username, Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser(%s): %v", username, err)
	}
	return user
}

func TestCreateAppAndGetForUser(t *testing.T) {
	database := newTestDB(t)
	user := createAppTestUser(t, database, "owner", 1)
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}

	app, err := database.CreateApp(&AppCreate{OrganizationID: org.ID, Name: "Forge acme"})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	if app.ID == "" {
		t.Fatal("expected generated app id")
	}
	if app.Name != "Forge acme" || app.Slug == "" {
		t.Fatalf("unexpected app name/slug: %q / %q", app.Name, app.Slug)
	}
	if app.OrganizationID != org.ID {
		t.Fatalf("app org = %q, want %q", app.OrganizationID, org.ID)
	}

	got, err := database.GetAppForUser(app.ID, user.ID)
	if err != nil {
		t.Fatalf("GetAppForUser: %v", err)
	}
	if got == nil || got.ID != app.ID {
		t.Fatalf("GetAppForUser returned %v, want app %s", got, app.ID)
	}

	// A non-member cannot see it.
	stranger := createAppTestUser(t, database, "stranger", 2)
	hidden, err := database.GetAppForUser(app.ID, stranger.ID)
	if err != nil {
		t.Fatalf("GetAppForUser(stranger): %v", err)
	}
	if hidden != nil {
		t.Fatal("expected non-member to get nil app")
	}
}

func TestListAppsForUserAndUpdateDelete(t *testing.T) {
	database := newTestDB(t)
	user := createAppTestUser(t, database, "owner", 1)
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	app, err := database.CreateApp(&AppCreate{OrganizationID: org.ID, Name: "Alpha"})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}

	apps, err := database.ListAppsForUser(user.ID)
	if err != nil {
		t.Fatalf("ListAppsForUser: %v", err)
	}
	if len(apps) != 1 || apps[0].ID != app.ID {
		t.Fatalf("ListAppsForUser = %+v, want 1 app %s", apps, app.ID)
	}

	renamed, err := database.UpdateAppName(app.ID, "Beta")
	if err != nil {
		t.Fatalf("UpdateAppName: %v", err)
	}
	if renamed.Name != "Beta" {
		t.Fatalf("rename = %q, want Beta", renamed.Name)
	}

	if err := database.DeleteApp(app.ID); err != nil {
		t.Fatalf("DeleteApp: %v", err)
	}
	gone, err := database.GetApp(app.ID)
	if err != nil {
		t.Fatalf("GetApp after delete: %v", err)
	}
	if gone != nil {
		t.Fatal("expected app to be deleted")
	}
}

func TestAddRemoveProjectAndListByApp(t *testing.T) {
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

	project := &Project{
		Name: "acme-api", GithubRepo: "forge", GithubOwner: "owner", Branch: "main",
		UserID: user.ID, OrganizationID: org.ID, Framework: "static",
		PackageManager: "auto", OutputDirectory: "dist", BuildCommand: "bun run build",
		InstallCommand: "bun install", NodeVersion: "22", Status: "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	if err := database.AddProjectsToApp(app.ID, []string{project.ID}); err != nil {
		t.Fatalf("AddProjectsToApp: %v", err)
	}

	members, err := database.ListProjectsByApp(app.ID)
	if err != nil {
		t.Fatalf("ListProjectsByApp: %v", err)
	}
	if len(members) != 1 || members[0].ID != project.ID {
		t.Fatalf("members = %+v, want [%s]", members, project.ID)
	}

	// GetProject now reflects the app membership.
	got, err := database.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.AppID != app.ID {
		t.Fatalf("project app_id = %q, want %q", got.AppID, app.ID)
	}

	if err := database.RemoveProjectFromApp(project.ID); err != nil {
		t.Fatalf("RemoveProjectFromApp: %v", err)
	}
	after, err := database.ListProjectsByApp(app.ID)
	if err != nil {
		t.Fatalf("ListProjectsByApp after remove: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("expected 0 members after remove, got %d", len(after))
	}
}

func TestDeleteAppDetachesServicesNotDestroys(t *testing.T) {
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
	project := &Project{
		Name: "api", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, OrganizationID: org.ID, Framework: "static",
		PackageManager: "auto", Status: "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	// A project-owned service associated with the app (app_id set).
	if _, err := database.Exec(
		`INSERT INTO project_services
		   (id, project_id, app_id, environment, service_type, db_name, db_user, db_password_encrypted)
		 VALUES (?, ?, ?, 'production', 'postgres', 'db', 'u', 'enc')`,
		NewID(), project.ID, app.ID,
	); err != nil {
		t.Fatalf("insert service: %v", err)
	}

	if err := database.DeleteApp(app.ID); err != nil {
		t.Fatalf("DeleteApp: %v", err)
	}

	// Service must SURVIVE the app deletion (detached, not destroyed).
	var count int
	var appID sql.NullString
	if err := database.QueryRow(
		`SELECT COUNT(*), MAX(app_id) FROM project_services WHERE project_id = ?`,
		project.ID,
	).Scan(&count, &appID); err != nil {
		t.Fatalf("query service: %v", err)
	}
	if count != 1 {
		t.Fatalf("service count = %d, want 1 (app delete must not destroy services)", count)
	}
	if appID.Valid {
		t.Fatalf("service app_id = %q, want NULL after app delete", appID.String)
	}
}

func TestAppReleaseSnapshotRoundTrip(t *testing.T) {
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
	project := &Project{
		Name: "api", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, OrganizationID: org.ID, Framework: "static",
		PackageManager: "auto", Status: "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	deployment := &Deployment{ProjectID: project.ID, Environment: "production", Branch: "main", Status: "live", TriggeredBy: user.ID}
	if err := database.CreateDeployment(deployment); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	release, err := database.CreateAppRelease(
		&AppRelease{AppID: app.ID, Environment: "production", Status: "succeeded"},
		[]AppReleaseMember{{ProjectID: project.ID, DeploymentID: deployment.ID}},
	)
	if err != nil {
		t.Fatalf("CreateAppRelease: %v", err)
	}
	if release.ID == "" || len(release.Members) != 1 {
		t.Fatalf("unexpected release: %+v", release)
	}
	if release.Members[0].DeploymentID != deployment.ID {
		t.Fatalf("member deployment = %q, want %q", release.Members[0].DeploymentID, deployment.ID)
	}

	list, err := database.ListAppReleases(app.ID, "production")
	if err != nil {
		t.Fatalf("ListAppReleases: %v", err)
	}
	if len(list) != 1 || list[0].ID != release.ID || list[0].Status != "succeeded" {
		t.Fatalf("ListAppReleases = %+v, want one succeeded release %s", list, release.ID)
	}

	// deploy_order persists + drives member ordering.
	project.DeployOrder = 3
	if err := database.UpdateProject(project); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	reloaded, err := database.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if reloaded.DeployOrder != 3 {
		t.Fatalf("deploy_order = %d, want 3", reloaded.DeployOrder)
	}
}
