package db

import (
	"sort"
	"strings"
	"testing"
	"time"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMigrations(t *testing.T) {
	db := newTestDB(t)

	// Verify tables exist
	tables := []string{"users", "organizations", "organization_memberships", "projects", "preview_instances", "project_analytics", "project_email_settings", "deployments", "build_logs", "domains", "env_variables", "refresh_tokens", "audit_logs", "api_tokens", "_migrations"}
	for _, table := range tables {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		if err != nil {
			t.Errorf("table %s does not exist: %v", table, err)
		}
	}

	// Running migrations again should be idempotent
	if err := db.Migrate(); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestMigration015HandlesExistingEnvVariables(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if _, err := database.Exec(`
		CREATE TABLE IF NOT EXISTS _migrations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)
	`); err != nil {
		t.Fatalf("create migrations table: %v", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("ReadDir migrations: %v", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() >= "015_env_variable_updated_at.sql" {
			continue
		}
		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", entry.Name(), err)
		}
		if _, err := database.Exec(string(content)); err != nil {
			t.Fatalf("exec migration %s: %v", entry.Name(), err)
		}
		if _, err := database.Exec("INSERT INTO _migrations (name) VALUES (?)", entry.Name()); err != nil {
			t.Fatalf("record migration %s: %v", entry.Name(), err)
		}
	}

	user := &User{ID: NewID(), GithubID: 42, Username: "pre-migration", Role: "admin"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	project := &Project{Name: "pre-migration-project", GithubRepo: "repo", GithubOwner: "owner", Branch: "main",
		UserID: user.ID, Framework: "nextjs", BuildCommand: "build", InstallCommand: "install",
		NodeVersion: "22", Status: "active"}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO env_variables (id, project_id, environment, kind, key, value)
		 VALUES (?, ?, 'preview', 'env', 'LEGACY_KEY', 'legacy-value')`,
		NewID(), project.ID,
	); err != nil {
		t.Fatalf("insert legacy env var: %v", err)
	}

	if err := database.Migrate(); err != nil {
		t.Fatalf("Migrate with existing env vars: %v", err)
	}

	var createdAt, updatedAt time.Time
	if err := database.QueryRow(
		`SELECT created_at, updated_at FROM env_variables WHERE project_id = ? AND key = 'LEGACY_KEY'`,
		project.ID,
	).Scan(&createdAt, &updatedAt); err != nil {
		t.Fatalf("select migrated env var: %v", err)
	}
	if !updatedAt.Equal(createdAt) {
		t.Fatalf("updated_at = %v, want created_at %v", updatedAt, createdAt)
	}

	var applied int
	if err := database.QueryRow(`SELECT COUNT(*) FROM _migrations WHERE name = ?`, "015_env_variable_updated_at.sql").Scan(&applied); err != nil {
		t.Fatalf("count migration 015: %v", err)
	}
	if applied != 1 {
		t.Fatalf("migration 015 applied count = %d, want 1", applied)
	}
}

func TestMigration020RewritesProjectBranchPreviewToWildcard(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if _, err := database.Exec(`
		CREATE TABLE IF NOT EXISTS _migrations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)
	`); err != nil {
		t.Fatalf("create migrations table: %v", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("ReadDir migrations: %v", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		// Apply everything strictly before 020.
		if entry.IsDir() || entry.Name() >= "020_default_preview_all_branches.sql" {
			continue
		}
		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", entry.Name(), err)
		}
		if _, err := database.Exec(string(content)); err != nil {
			t.Fatalf("exec migration %s: %v", entry.Name(), err)
		}
		if _, err := database.Exec("INSERT INTO _migrations (name) VALUES (?)", entry.Name()); err != nil {
			t.Fatalf("record migration %s: %v", entry.Name(), err)
		}
	}

	user := &User{ID: NewID(), GithubID: 99, Username: "owner", Role: "admin"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	// Three projects: bad-default, explicit-list, already-wildcard.
	type seed struct {
		name           string
		branch         string
		previewBranch  string
		wantPostMigr   string
	}
	seeds := []seed{
		{name: "bad-default-main", branch: "main", previewBranch: "main", wantPostMigr: "*"},
		{name: "bad-default-develop", branch: "develop", previewBranch: "develop", wantPostMigr: "*"},
		{name: "explicit-list", branch: "main", previewBranch: "develop,staging", wantPostMigr: "develop,staging"},
		{name: "already-wildcard", branch: "main", previewBranch: "*", wantPostMigr: "*"},
		{name: "explicit-single-not-matching-branch", branch: "main", previewBranch: "release", wantPostMigr: "release"},
	}
	projectIDs := make(map[string]string, len(seeds))
	for _, s := range seeds {
		project := &Project{
			Name: s.name, GithubRepo: "repo", GithubOwner: "owner", Branch: s.branch,
			UserID: user.ID, Framework: "nextjs", BuildCommand: "build", InstallCommand: "install",
			NodeVersion: "22", Status: "active",
		}
		if err := database.CreateProject(project); err != nil {
			t.Fatalf("CreateProject(%s): %v", s.name, err)
		}
		if _, err := database.Exec(
			`INSERT INTO auto_build_configs
			   (id, project_id, enabled, production_branch, preview_branches, auto_production_enabled, webhook_secret)
			 VALUES (?, ?, 1, ?, ?, 0, 'secret')`,
			NewID(), project.ID, s.branch, s.previewBranch,
		); err != nil {
			t.Fatalf("insert auto_build_configs(%s): %v", s.name, err)
		}
		projectIDs[s.name] = project.ID
	}

	if err := database.Migrate(); err != nil {
		t.Fatalf("Migrate (apply 020): %v", err)
	}

	for _, s := range seeds {
		var got string
		if err := database.QueryRow(
			`SELECT preview_branches FROM auto_build_configs WHERE project_id = ?`,
			projectIDs[s.name],
		).Scan(&got); err != nil {
			t.Fatalf("select(%s): %v", s.name, err)
		}
		if got != s.wantPostMigr {
			t.Errorf("%s: preview_branches = %q, want %q", s.name, got, s.wantPostMigr)
		}
	}

	// Migration 020 must be recorded.
	var applied int
	if err := database.QueryRow(`SELECT COUNT(*) FROM _migrations WHERE name = ?`, "020_default_preview_all_branches.sql").Scan(&applied); err != nil {
		t.Fatalf("count migration 020: %v", err)
	}
	if applied != 1 {
		t.Fatalf("migration 020 applied count = %d, want 1", applied)
	}

	// Re-running should be idempotent (no-op, no errors).
	if err := database.Migrate(); err != nil {
		t.Fatalf("Migrate (re-run): %v", err)
	}
}

func TestUserCRUD(t *testing.T) {
	db := newTestDB(t)

	user := &User{
		ID:          NewID(),
		GithubID:    12345,
		Username:    "testuser",
		AvatarURL:   "https://github.com/testuser.png",
		GithubToken: "encrypted-token",
		Role:        "admin",
	}

	// Create
	if err := db.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	// Read by GitHub ID
	got, err := db.GetUserByGithubID(12345)
	if err != nil {
		t.Fatalf("GetUserByGithubID: %v", err)
	}
	if got == nil {
		t.Fatal("user not found")
	}
	if got.Username != "testuser" {
		t.Errorf("username = %q, want %q", got.Username, "testuser")
	}

	// Read by ID
	got, err = db.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got == nil || got.GithubID != 12345 {
		t.Error("GetUserByID returned wrong user")
	}

	// Upsert (update)
	user.Username = "updated"
	if err := db.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser (update): %v", err)
	}
	got, err = db.GetUserByGithubID(12345)
	if err != nil || got.Username != "updated" {
		t.Error("upsert did not update username")
	}
}

func TestEnsurePersonalOrganization(t *testing.T) {
	db := newTestDB(t)

	user := &User{
		ID:        NewID(),
		GithubID:  12345,
		Username:  "testuser",
		AvatarURL: "https://github.com/testuser.png",
		Role:      "admin",
	}
	if err := db.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	organization, err := db.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	if organization == nil {
		t.Fatal("organization should not be nil")
	}
	if !organization.IsPersonal {
		t.Fatal("organization should be personal")
	}
	if organization.MembershipRole != OrganizationRoleOwner {
		t.Fatalf("membership_role = %q, want %q", organization.MembershipRole, OrganizationRoleOwner)
	}

	organizations, err := db.ListOrganizationsForUser(user.ID)
	if err != nil {
		t.Fatalf("ListOrganizationsForUser: %v", err)
	}
	if len(organizations) != 1 {
		t.Fatalf("got %d organizations, want 1", len(organizations))
	}
	if organizations[0].PersonalOwnerUserID != user.ID {
		t.Fatalf("personal_owner_user_id = %q, want %q", organizations[0].PersonalOwnerUserID, user.ID)
	}
}

func TestProjectCRUD(t *testing.T) {
	db := newTestDB(t)

	// Create user first
	user := &User{ID: NewID(), GithubID: 1, Username: "user1", Role: "admin"}
	db.UpsertUser(user)

	project := &Project{
		Name:            "test-project",
		GithubRepo:      "my-app",
		GithubOwner:     "user1",
		Branch:          "main",
		UserID:          user.ID,
		Framework:       "nextjs",
		PackageManager:  "pnpm",
		RootDirectory:   "apps/web",
		OutputDirectory: ".next",
		BuildCommand:    "bun run build",
		InstallCommand:  "bun install",
		NodeVersion:     "22",
		Status:          "active",
	}

	// Create
	if err := db.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if project.ID == "" {
		t.Error("project ID not set")
	}

	// List
	projects, err := db.ListProjects(user.ID, "")
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(projects))
	}

	// Get
	got, err := db.GetProject(project.ID)
	if err != nil || got == nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.Name != "test-project" {
		t.Errorf("name = %q, want %q", got.Name, "test-project")
	}
	if got.RootDirectory != "apps/web" {
		t.Errorf("root_directory = %q, want %q", got.RootDirectory, "apps/web")
	}
	if got.OutputDirectory != ".next" {
		t.Errorf("output_directory = %q, want %q", got.OutputDirectory, ".next")
	}
	if got.PackageManager != "pnpm" {
		t.Errorf("package_manager = %q, want %q", got.PackageManager, "pnpm")
	}

	gotForUser, err := db.GetProjectForUser(project.ID, user.ID)
	if err != nil || gotForUser == nil {
		t.Fatalf("GetProjectForUser: %v", err)
	}
	gotForOtherUser, err := db.GetProjectForUser(project.ID, NewID())
	if err != nil {
		t.Fatalf("GetProjectForUser(other): %v", err)
	}
	if gotForOtherUser != nil {
		t.Fatal("GetProjectForUser should hide projects from other users")
	}

	// Update
	got.Branch = "develop"
	if err := db.UpdateProject(got); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}

	// Delete (soft)
	if err := db.DeleteProject(project.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	projects, _ = db.ListProjects(user.ID, "")
	if len(projects) != 0 {
		t.Error("deleted project still shows in list")
	}

	deletedProject, err := db.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject(deleted): %v", err)
	}
	if deletedProject == nil || !strings.HasPrefix(deletedProject.Name, "test-project--deleted-") {
		t.Fatalf("deleted project name = %q, want deleted suffix", deletedProject.Name)
	}
}

func TestDeleteProjectReleasesNameAndDomains(t *testing.T) {
	db := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 1, Username: "user1", Role: "admin"}
	if err := db.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	project := &Project{
		Name:           "reusable-project",
		GithubRepo:     "my-app",
		GithubOwner:    "user1",
		Branch:         "main",
		UserID:         user.ID,
		Framework:      "nextjs",
		PackageManager: "auto",
		BuildCommand:   "bun run build",
		InstallCommand: "bun install",
		NodeVersion:    "22",
		Status:         "active",
	}
	if err := db.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	domain := &Domain{
		ProjectID:   project.ID,
		DomainName:  "reusable-project.preview.example.com",
		Environment: "preview",
		IsAuto:      true,
		SSLStatus:   "pending",
	}
	if err := db.CreateDomain(domain); err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}

	if err := db.DeleteAllDomainsForProject(project.ID); err != nil {
		t.Fatalf("DeleteAllDomainsForProject: %v", err)
	}
	if err := db.DeleteProject(project.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	recreated := &Project{
		Name:           "reusable-project",
		GithubRepo:     "my-app-2",
		GithubOwner:    "user1",
		Branch:         "main",
		UserID:         user.ID,
		Framework:      "nextjs",
		PackageManager: "auto",
		BuildCommand:   "bun run build",
		InstallCommand: "bun install",
		NodeVersion:    "22",
		Status:         "active",
	}
	if err := db.CreateProject(recreated); err != nil {
		t.Fatalf("CreateProject(recreated): %v", err)
	}

	recreatedDomain := &Domain{
		ProjectID:   recreated.ID,
		DomainName:  "reusable-project.preview.example.com",
		Environment: "preview",
		IsAuto:      true,
		SSLStatus:   "pending",
	}
	if err := db.CreateDomain(recreatedDomain); err != nil {
		t.Fatalf("CreateDomain(recreated): %v", err)
	}
}

func TestProjectAnalyticsCRUD(t *testing.T) {
	db := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 1, Username: "analytics-owner", Role: "admin"}
	if err := db.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	project := &Project{
		Name:           "analytics-project",
		GithubRepo:     "repo",
		GithubOwner:    "owner",
		Branch:         "main",
		UserID:         user.ID,
		Framework:      "nextjs",
		PackageManager: "pnpm",
		BuildCommand:   "pnpm run build",
		InstallCommand: "pnpm install --frozen-lockfile",
		NodeVersion:    "22",
		Status:         "active",
	}
	if err := db.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	record := &ProjectAnalytics{
		ProjectID:        project.ID,
		AudienceEnabled:  true,
		TrackingMode:     AnalyticsTrackingModeAIInstall,
		AudienceStatus:   AnalyticsAudienceStatusReceivingData,
		UmamiWebsiteID:   "website-1",
		UmamiWebsiteName: "analytics-project",
		LastError:        "",
	}
	if err := db.UpsertProjectAnalytics(record); err != nil {
		t.Fatalf("UpsertProjectAnalytics: %v", err)
	}

	stored, err := db.GetProjectAnalytics(project.ID)
	if err != nil {
		t.Fatalf("GetProjectAnalytics: %v", err)
	}
	if stored == nil {
		t.Fatal("expected analytics row")
	}
	if stored.UmamiWebsiteID != "website-1" {
		t.Fatalf("UmamiWebsiteID = %q, want %q", stored.UmamiWebsiteID, "website-1")
	}
	if stored.TrackingMode != AnalyticsTrackingModeAIInstall {
		t.Fatalf("TrackingMode = %q, want %q", stored.TrackingMode, AnalyticsTrackingModeAIInstall)
	}

	record.AudienceStatus = AnalyticsAudienceStatusStale
	record.LastError = "stale"
	if err := db.UpsertProjectAnalytics(record); err != nil {
		t.Fatalf("UpsertProjectAnalytics(update): %v", err)
	}

	stored, err = db.GetProjectAnalytics(project.ID)
	if err != nil {
		t.Fatalf("GetProjectAnalytics(update): %v", err)
	}
	if stored.AudienceStatus != AnalyticsAudienceStatusStale {
		t.Fatalf("AudienceStatus = %q, want %q", stored.AudienceStatus, AnalyticsAudienceStatusStale)
	}
	if stored.LastError != "stale" {
		t.Fatalf("LastError = %q, want %q", stored.LastError, "stale")
	}

	if err := db.DeleteProjectAnalytics(project.ID); err != nil {
		t.Fatalf("DeleteProjectAnalytics: %v", err)
	}

	stored, err = db.GetProjectAnalytics(project.ID)
	if err != nil {
		t.Fatalf("GetProjectAnalytics(deleted): %v", err)
	}
	if stored != nil {
		t.Fatal("expected analytics row to be deleted")
	}
}

func TestOrganizationMembersCanAccessSharedProjects(t *testing.T) {
	db := newTestDB(t)

	owner := &User{ID: NewID(), GithubID: 1, Username: "owner", Role: "admin"}
	member := &User{ID: NewID(), GithubID: 2, Username: "member", Role: "user"}
	if err := db.UpsertUser(owner); err != nil {
		t.Fatalf("UpsertUser(owner): %v", err)
	}
	if err := db.UpsertUser(member); err != nil {
		t.Fatalf("UpsertUser(member): %v", err)
	}

	organization := &Organization{Name: "FixIt Technologies", MembershipRole: OrganizationRoleOwner}
	if err := db.CreateOrganization(organization); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if err := db.AddOrganizationMember(organization.ID, owner.ID, OrganizationRoleOwner); err != nil {
		t.Fatalf("AddOrganizationMember(owner): %v", err)
	}
	if err := db.AddOrganizationMember(organization.ID, member.ID, OrganizationRoleMember); err != nil {
		t.Fatalf("AddOrganizationMember(member): %v", err)
	}

	project := &Project{
		Name:            "shared-project",
		GithubRepo:      "my-app",
		GithubOwner:     "owner",
		Branch:          "main",
		UserID:          owner.ID,
		OrganizationID:  organization.ID,
		Framework:       "nextjs",
		PackageManager:  "auto",
		OutputDirectory: ".next",
		BuildCommand:    "bun run build",
		InstallCommand:  "bun install --frozen-lockfile",
		NodeVersion:     "22",
		Status:          "active",
	}
	if err := db.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	projects, err := db.ListProjects(member.ID, organization.ID)
	if err != nil {
		t.Fatalf("ListProjects(member): %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(projects))
	}
	if projects[0].OrganizationName != "FixIt Technologies" {
		t.Fatalf("organization_name = %q, want %q", projects[0].OrganizationName, "FixIt Technologies")
	}

	gotForMember, err := db.GetProjectForUser(project.ID, member.ID)
	if err != nil {
		t.Fatalf("GetProjectForUser(member): %v", err)
	}
	if gotForMember == nil {
		t.Fatal("member should have access to shared project")
	}
}

func TestDeploymentCRUD(t *testing.T) {
	db := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 1, Username: "u", Role: "admin"}
	db.UpsertUser(user)

	project := &Project{Name: "p1", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, Framework: "nextjs", PackageManager: "auto", RootDirectory: "", OutputDirectory: ".next", BuildCommand: "build", InstallCommand: "install",
		NodeVersion: "22", Status: "active"}
	db.CreateProject(project)

	deploy := &Deployment{
		ProjectID:   project.ID,
		Environment: "preview",
		CommitSHA:   "abc123",
		Branch:      "main",
		Status:      "queued",
		TriggeredBy: user.ID,
	}

	if err := db.CreateDeployment(deploy); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	// Update status
	if err := db.UpdateDeploymentStatus(deploy.ID, "building", ""); err != nil {
		t.Fatalf("UpdateDeploymentStatus: %v", err)
	}

	got, _ := db.GetDeployment(deploy.ID)
	if got.Status != "building" {
		t.Errorf("status = %q, want %q", got.Status, "building")
	}

	gotForUser, err := db.GetDeploymentForUser(deploy.ID, user.ID)
	if err != nil || gotForUser == nil {
		t.Fatalf("GetDeploymentForUser: %v", err)
	}
	gotForOtherUser, err := db.GetDeploymentForUser(deploy.ID, NewID())
	if err != nil {
		t.Fatalf("GetDeploymentForUser(other): %v", err)
	}
	if gotForOtherUser != nil {
		t.Fatal("GetDeploymentForUser should hide deployments from other users")
	}

	// List
	deploys, _ := db.ListDeployments(project.ID, 10)
	if len(deploys) != 1 {
		t.Fatalf("got %d deployments, want 1", len(deploys))
	}
}

func TestOrganizationMembersCanAccessSharedDeployments(t *testing.T) {
	db := newTestDB(t)

	owner := &User{ID: NewID(), GithubID: 1, Username: "owner", Role: "admin"}
	member := &User{ID: NewID(), GithubID: 2, Username: "member", Role: "user"}
	if err := db.UpsertUser(owner); err != nil {
		t.Fatalf("UpsertUser(owner): %v", err)
	}
	if err := db.UpsertUser(member); err != nil {
		t.Fatalf("UpsertUser(member): %v", err)
	}

	organization := &Organization{Name: "FixIt Technologies"}
	if err := db.CreateOrganization(organization); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if err := db.AddOrganizationMember(organization.ID, owner.ID, OrganizationRoleOwner); err != nil {
		t.Fatalf("AddOrganizationMember(owner): %v", err)
	}
	if err := db.AddOrganizationMember(organization.ID, member.ID, OrganizationRoleMember); err != nil {
		t.Fatalf("AddOrganizationMember(member): %v", err)
	}

	project := &Project{Name: "shared-deploy-project", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: owner.ID, OrganizationID: organization.ID, Framework: "nextjs", PackageManager: "auto", RootDirectory: "", OutputDirectory: ".next", BuildCommand: "build", InstallCommand: "install",
		NodeVersion: "22", Status: "active"}
	if err := db.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	deploy := &Deployment{
		ProjectID:   project.ID,
		Environment: "preview",
		CommitSHA:   "abc123",
		Branch:      "main",
		Status:      "queued",
		TriggeredBy: owner.ID,
	}
	if err := db.CreateDeployment(deploy); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	got, err := db.GetDeploymentForUser(deploy.ID, member.ID)
	if err != nil {
		t.Fatalf("GetDeploymentForUser(member): %v", err)
	}
	if got == nil {
		t.Fatal("member should have access to shared deployment")
	}
}

func TestEnvVarCRUD(t *testing.T) {
	db := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 1, Username: "u", Role: "admin"}
	db.UpsertUser(user)

	project := &Project{Name: "p1", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, Framework: "nextjs", PackageManager: "auto", RootDirectory: "", OutputDirectory: ".next", BuildCommand: "b", InstallCommand: "i",
		NodeVersion: "22", Status: "active"}
	db.CreateProject(project)

	// Upsert
	v := &EnvVariable{ProjectID: project.ID, Environment: "shared", Key: "API_KEY", Value: "encrypted-val"}
	if err := db.UpsertEnvVar(v); err != nil {
		t.Fatalf("UpsertEnvVar: %v", err)
	}

	// List
	vars, _ := db.ListEnvVars(project.ID, "shared")
	if len(vars) != 1 || vars[0].Key != "API_KEY" {
		t.Errorf("ListEnvVars: got %v", vars)
	}

	// Bulk set
	if err := db.BulkSetEnvVars(project.ID, "preview", []EnvVariable{
		{Key: "KEY1", Value: "val1"},
		{Key: "KEY2", Value: "val2"},
	}); err != nil {
		t.Fatalf("BulkSetEnvVars: %v", err)
	}
	vars, _ = db.ListEnvVars(project.ID, "preview")
	if len(vars) != 2 {
		t.Errorf("got %d vars after bulk set, want 2", len(vars))
	}

	// Delete
	if err := db.DeleteEnvVar(project.ID, "preview", "KEY1"); err != nil {
		t.Fatalf("DeleteEnvVar: %v", err)
	}
	vars, _ = db.ListEnvVars(project.ID, "preview")
	if len(vars) != 1 {
		t.Errorf("got %d vars after delete, want 1", len(vars))
	}
}

func TestProjectVariableResolution(t *testing.T) {
	db := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 1, Username: "u", Role: "admin"}
	db.UpsertUser(user)

	project := &Project{Name: "p1", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, Framework: "nextjs", PackageManager: "auto", BuildCommand: "b", InstallCommand: "i",
		NodeVersion: "22", Status: "active"}
	db.CreateProject(project)

	if err := db.BulkSetEnvVars(project.ID, "shared", []EnvVariable{
		{Key: "API_BASE_URL", Value: "https://shared.example.com"},
		{Key: "NEXT_PUBLIC_BRAND", Value: "shared-brand"},
	}); err != nil {
		t.Fatalf("BulkSetEnvVars (shared): %v", err)
	}

	if err := db.BulkSetEnvVars(project.ID, "preview", []EnvVariable{
		{Key: "API_BASE_URL", Value: "https://preview.example.com"},
	}); err != nil {
		t.Fatalf("BulkSetEnvVars (preview): %v", err)
	}

	previewVars, err := db.ListResolvedEnvVars(project.ID, "preview")
	if err != nil {
		t.Fatalf("ListResolvedEnvVars (preview): %v", err)
	}
	if len(previewVars) != 2 {
		t.Fatalf("got %d preview vars, want 2", len(previewVars))
	}
	if previewVars[0].Key != "API_BASE_URL" || previewVars[0].Value != "https://preview.example.com" {
		t.Fatalf("preview override = %#v, want preview-scoped value", previewVars[0])
	}
	if previewVars[1].Key != "NEXT_PUBLIC_BRAND" || previewVars[1].Value != "shared-brand" {
		t.Fatalf("preview shared carry-through = %#v, want shared value", previewVars[1])
	}

	productionVars, err := db.ListResolvedEnvVars(project.ID, "production")
	if err != nil {
		t.Fatalf("ListResolvedEnvVars (production): %v", err)
	}
	if len(productionVars) != 2 {
		t.Fatalf("got %d production vars, want 2", len(productionVars))
	}
	if productionVars[0].Key != "API_BASE_URL" || productionVars[0].Value != "https://shared.example.com" {
		t.Fatalf("production shared value = %#v, want shared value", productionVars[0])
	}
}

func TestSecretCRUD(t *testing.T) {
	db := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 1, Username: "u", Role: "admin"}
	db.UpsertUser(user)

	project := &Project{Name: "p1", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, Framework: "nextjs", BuildCommand: "b", InstallCommand: "i",
		NodeVersion: "22", Status: "active"}
	db.CreateProject(project)

	if err := db.BulkSetSecrets(project.ID, "shared", []ProjectVariable{
		{Key: "DATABASE_URL", Value: "encrypted-db"},
	}); err != nil {
		t.Fatalf("BulkSetSecrets (shared): %v", err)
	}

	if err := db.BulkSetEnvVars(project.ID, "shared", []ProjectVariable{
		{Key: "NEXT_PUBLIC_API_URL", Value: "encrypted-api"},
	}); err != nil {
		t.Fatalf("BulkSetEnvVars (shared): %v", err)
	}

	secrets, err := db.ListSecrets(project.ID, "shared")
	if err != nil {
		t.Fatalf("ListSecrets: %v", err)
	}
	if len(secrets) != 1 || secrets[0].Key != "DATABASE_URL" {
		t.Fatalf("ListSecrets = %#v, want DATABASE_URL", secrets)
	}

	envVars, err := db.ListEnvVars(project.ID, "shared")
	if err != nil {
		t.Fatalf("ListEnvVars: %v", err)
	}
	if len(envVars) != 1 || envVars[0].Key != "NEXT_PUBLIC_API_URL" {
		t.Fatalf("ListEnvVars = %#v, want NEXT_PUBLIC_API_URL", envVars)
	}

	if err := db.DeleteSecret(project.ID, "shared", "DATABASE_URL"); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}

	secrets, err = db.ListSecrets(project.ID, "shared")
	if err != nil {
		t.Fatalf("ListSecrets after delete: %v", err)
	}
	if len(secrets) != 0 {
		t.Fatalf("got %d secrets after delete, want 0", len(secrets))
	}

	envVars, err = db.ListEnvVars(project.ID, "shared")
	if err != nil {
		t.Fatalf("ListEnvVars after secret delete: %v", err)
	}
	if len(envVars) != 1 {
		t.Fatalf("got %d env vars after secret delete, want 1", len(envVars))
	}
}

func TestRefreshSessionRotation(t *testing.T) {
	db := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 1, Username: "u", Role: "admin"}
	if err := db.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	oldHash := "old-hash"
	if err := db.CreateRefreshSession(&RefreshSession{
		UserID:    user.ID,
		TokenHash: oldHash,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreateRefreshSession: %v", err)
	}

	session, err := db.GetActiveRefreshSessionByHash(oldHash)
	if err != nil {
		t.Fatalf("GetActiveRefreshSessionByHash(old): %v", err)
	}
	if session == nil {
		t.Fatal("expected active refresh session")
	}

	if err := db.RotateRefreshSession(session.ID, user.ID, "new-hash", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("RotateRefreshSession: %v", err)
	}

	oldSession, err := db.GetActiveRefreshSessionByHash(oldHash)
	if err != nil {
		t.Fatalf("GetActiveRefreshSessionByHash(rotated old): %v", err)
	}
	if oldSession != nil {
		t.Fatal("old refresh session should no longer be active")
	}

	newSession, err := db.GetActiveRefreshSessionByHash("new-hash")
	if err != nil {
		t.Fatalf("GetActiveRefreshSessionByHash(new): %v", err)
	}
	if newSession == nil {
		t.Fatal("expected rotated refresh session")
	}
}

func TestCreateAuditLog(t *testing.T) {
	db := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 1, Username: "u", Role: "admin"}
	if err := db.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	if err := db.CreateAuditLog(&AuditLog{
		UserID:       user.ID,
		Action:       "project.create",
		ResourceType: "project",
		ResourceID:   "project-1",
		Metadata:     `{"name":"demo"}`,
	}); err != nil {
		t.Fatalf("CreateAuditLog: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE action = ?", "project.create").Scan(&count); err != nil {
		t.Fatalf("QueryRow audit_logs: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}

func TestDomainCRUD(t *testing.T) {
	db := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 1, Username: "u", Role: "admin"}
	db.UpsertUser(user)

	project := &Project{Name: "p1", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, Framework: "nextjs", BuildCommand: "b", InstallCommand: "i",
		NodeVersion: "22", Status: "active"}
	db.CreateProject(project)

	// Create auto domain
	auto := &Domain{ProjectID: project.ID, DomainName: "p1.preview.example.com",
		Environment: "preview", IsAuto: true, SSLStatus: "active"}
	if err := db.CreateDomain(auto); err != nil {
		t.Fatalf("CreateDomain (auto): %v", err)
	}

	// Create custom domain
	custom := &Domain{ProjectID: project.ID, DomainName: "vaclav.cz",
		Environment: "production", IsAuto: false, SSLStatus: "pending"}
	if err := db.CreateDomain(custom); err != nil {
		t.Fatalf("CreateDomain (custom): %v", err)
	}

	// List
	domains, _ := db.ListDomains(project.ID)
	if len(domains) != 2 {
		t.Fatalf("got %d domains, want 2", len(domains))
	}

	// Update DNS
	if err := db.UpdateDomainDNS(custom.ID, true); err != nil {
		t.Fatalf("UpdateDomainDNS: %v", err)
	}

	// Get by name
	got, _ := db.GetDomainByName("vaclav.cz")
	if got == nil || !got.DNSVerified {
		t.Error("domain DNS not updated")
	}

	// Delete custom (should work)
	if err := db.DeleteDomain(custom.ID); err != nil {
		t.Fatalf("DeleteDomain: %v", err)
	}

	// Delete custom through scoped helper
	if err := db.CreateDomain(custom); err != nil {
		t.Fatalf("CreateDomain (custom recreate): %v", err)
	}
	if err := db.DeleteDomainForProject(project.ID, custom.ID); err != nil {
		t.Fatalf("DeleteDomainForProject: %v", err)
	}

	// Delete auto (should not delete due to is_auto check)
	db.DeleteDomain(auto.ID)
	domains, _ = db.ListDomains(project.ID)
	if len(domains) != 1 {
		t.Errorf("got %d domains after delete, want 1 (auto should remain)", len(domains))
	}
}

func TestDomainIsPrimaryBackfill(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if _, err := database.Exec(`
		CREATE TABLE IF NOT EXISTS _migrations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)
	`); err != nil {
		t.Fatalf("create _migrations: %v", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("ReadDir migrations: %v", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		name := entry.Name()
		if name >= "014_" {
			break
		}

		content, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}
		if _, err := database.Exec(string(content)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
		if _, err := database.Exec(`INSERT INTO _migrations (name) VALUES (?)`, name); err != nil {
			t.Fatalf("record migration %s: %v", name, err)
		}
	}

	user := &User{ID: NewID(), GithubID: 2, Username: "primary-user", Role: "admin"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	project := &Project{
		Name:           "primary-backfill",
		GithubRepo:     "repo",
		GithubOwner:    "owner",
		Branch:         "main",
		UserID:         user.ID,
		Framework:      "nextjs",
		BuildCommand:   "bun run build",
		InstallCommand: "bun install",
		NodeVersion:    "22",
		Status:         "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	base := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	insert := func(id, domainName, environment string, isAuto bool, createdAt time.Time) {
		t.Helper()
		if _, err := database.Exec(
			`INSERT INTO domains (id, project_id, domain, environment, is_auto, dns_verified, ssl_status, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, project.ID, domainName, environment, isAuto, true, "active", createdAt,
		); err != nil {
			t.Fatalf("insert domain %s: %v", domainName, err)
		}
	}

	autoID := NewID()
	autoDomain := "primary-backfill.preview.example.com"
	insert(autoID, autoDomain, "preview", true, base)

	previewCustomDomain := "preview.example.com"
	insert(NewID(), previewCustomDomain, "preview", false, base.Add(time.Minute))

	prodFirstID := NewID()
	prodFirstDomain := "example.com"
	insert(prodFirstID, prodFirstDomain, "production", false, base.Add(2*time.Minute))

	prodSecondDomain := "www-alt.example.com"
	insert(NewID(), prodSecondDomain, "production", false, base.Add(3*time.Minute))

	if err := database.Migrate(); err != nil {
		t.Fatalf("Migrate(reapply 014): %v", err)
	}

	got, err := database.ListDomains(project.ID)
	if err != nil {
		t.Fatalf("ListDomains: %v", err)
	}

	primaries := map[string]string{}
	for _, d := range got {
		if !d.IsPrimary {
			continue
		}
		if prev, ok := primaries[d.Environment]; ok {
			t.Fatalf("two primaries in %s: %q and %q", d.Environment, prev, d.DomainName)
		}
		primaries[d.Environment] = d.DomainName
	}

	if primaries["preview"] != autoDomain {
		t.Fatalf("preview primary = %q, want %q", primaries["preview"], autoDomain)
	}
	if primaries["production"] != prodFirstDomain {
		t.Fatalf("production primary = %q, want %q", primaries["production"], prodFirstDomain)
	}
}

func TestUpdateDomainEnvironmentAndSetPrimary(t *testing.T) {
	database := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 3, Username: "env-switcher", Role: "admin"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	project := &Project{
		Name:           "domain-mover",
		GithubRepo:     "repo",
		GithubOwner:    "owner",
		Branch:         "main",
		UserID:         user.ID,
		Framework:      "nextjs",
		BuildCommand:   "bun run build",
		InstallCommand: "bun install",
		NodeVersion:    "22",
		Status:         "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	custom := &Domain{
		ProjectID:   project.ID,
		DomainName:  "switch.example.com",
		Environment: "preview",
		IsPrimary:   true,
		SSLStatus:   "pending",
	}
	if err := database.CreateDomain(custom); err != nil {
		t.Fatalf("CreateDomain(custom): %v", err)
	}

	if err := database.UpdateDomainEnvironment(custom.ID, "production", ""); err != nil {
		t.Fatalf("UpdateDomainEnvironment: %v", err)
	}

	got, err := database.GetDomainByID(custom.ID)
	if err != nil || got == nil {
		t.Fatalf("GetDomainByID: %v", err)
	}
	if got.Environment != "production" {
		t.Fatalf("environment = %q, want production", got.Environment)
	}
	if got.IsPrimary {
		t.Fatalf("moved domain should no longer be primary in the new environment")
	}

	sibling := &Domain{
		ProjectID:   project.ID,
		DomainName:  "other.example.com",
		Environment: "production",
		SSLStatus:   "active",
		IsPrimary:   true,
	}
	if err := database.CreateDomain(sibling); err != nil {
		t.Fatalf("CreateDomain(sibling): %v", err)
	}

	if err := database.SetDomainPrimary(project.ID, "production", custom.ID); err != nil {
		t.Fatalf("SetDomainPrimary: %v", err)
	}

	list, err := database.ListDomains(project.ID)
	if err != nil {
		t.Fatalf("ListDomains: %v", err)
	}

	primaries := 0
	var primaryID string
	for _, d := range list {
		if d.Environment == "production" && d.IsPrimary {
			primaries++
			primaryID = d.ID
		}
	}
	if primaries != 1 {
		t.Fatalf("expected exactly 1 production primary, got %d", primaries)
	}
	if primaryID != custom.ID {
		t.Fatalf("primary id = %q, want %q", primaryID, custom.ID)
	}
}

func TestProjectNewFields(t *testing.T) {
	db := newTestDB(t)

	db.Exec(`INSERT INTO users (id, github_id, username) VALUES ('user1', 1, 'tester')`)
	db.Exec(`INSERT INTO organizations (id, name, slug, is_personal, personal_owner_user_id) VALUES ('org1', 'tester', 'tester', 1, 'user1')`)
	db.Exec(`INSERT INTO organization_memberships (organization_id, user_id, role) VALUES ('org1', 'user1', 'owner')`)

	p := &Project{
		Name:              "testapp",
		GithubRepo:        "repo",
		GithubOwner:       "owner",
		Branch:            "main",
		UserID:            "user1",
		OrganizationID:    "org1",
		Framework:         "nextjs",
		PackageManager:    "auto",
		Status:            "active",
		HostNetworkAccess: true,
		DataVolumeEnabled: true,
		DataMountPath:     "/app/storage",
	}
	if err := db.CreateProject(p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	got, err := db.GetProject(p.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if !got.HostNetworkAccess {
		t.Error("expected host_network_access=true")
	}
	if !got.DataVolumeEnabled {
		t.Error("expected data_volume_enabled=true")
	}
	if got.DataMountPath != "/app/storage" {
		t.Errorf("expected data_mount_path=/app/storage, got %s", got.DataMountPath)
	}

	// Also verify via GetProjectForUser
	got2, err := db.GetProjectForUser(p.ID, "user1")
	if err != nil {
		t.Fatalf("GetProjectForUser: %v", err)
	}
	if !got2.HostNetworkAccess || !got2.DataVolumeEnabled {
		t.Error("GetProjectForUser: expected new fields to round-trip")
	}

	// Update
	got.HostNetworkAccess = false
	got.DataVolumeEnabled = false
	got.DataMountPath = "/app/data"
	if err := db.UpdateProject(got); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	got3, _ := db.GetProject(p.ID)
	if got3.HostNetworkAccess || got3.DataVolumeEnabled {
		t.Error("expected both flags false after update")
	}
	if got3.DataMountPath != "/app/data" {
		t.Errorf("expected data_mount_path=/app/data after update, got %s", got3.DataMountPath)
	}

	// Verify ListProjectsWithLatestDeployment includes new fields
	projects, err := db.ListProjectsWithLatestDeployment("user1", "")
	if err != nil {
		t.Fatalf("ListProjectsWithLatestDeployment: %v", err)
	}
	if len(projects) == 0 {
		t.Fatal("expected at least 1 project in list")
	}
	if projects[0].DataMountPath != "/app/data" {
		t.Errorf("list: expected data_mount_path=/app/data, got %s", projects[0].DataMountPath)
	}
}

func TestEnvVarUpdatedAtRefresh(t *testing.T) {
	db := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 1, Username: "u", Role: "admin"}
	db.UpsertUser(user)

	project := &Project{Name: "ts-project", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, Framework: "nextjs", PackageManager: "auto", BuildCommand: "b", InstallCommand: "i",
		NodeVersion: "22", Status: "active"}
	if err := db.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	v := &EnvVariable{ProjectID: project.ID, Environment: "preview", Key: "API_KEY", Value: "v1"}
	if err := db.UpsertEnvVar(v); err != nil {
		t.Fatalf("UpsertEnvVar(create): %v", err)
	}
	first, _ := db.ListEnvVars(project.ID, "preview")
	if len(first) != 1 {
		t.Fatalf("got %d vars, want 1", len(first))
	}
	if first[0].UpdatedAt.IsZero() {
		t.Fatal("updated_at should be set after upsert")
	}
	originalUpdatedAt := first[0].UpdatedAt

	// SQLite datetime('now') resolves to second precision; sleep past one second
	// so a second upsert produces a strictly greater timestamp.
	time.Sleep(1100 * time.Millisecond)

	v2 := &EnvVariable{ProjectID: project.ID, Environment: "preview", Key: "API_KEY", Value: "v2"}
	if err := db.UpsertEnvVar(v2); err != nil {
		t.Fatalf("UpsertEnvVar(update): %v", err)
	}
	second, _ := db.ListEnvVars(project.ID, "preview")
	if len(second) != 1 {
		t.Fatalf("got %d vars after second upsert, want 1", len(second))
	}
	if !second[0].UpdatedAt.After(originalUpdatedAt) {
		t.Fatalf("updated_at did not advance: was %v, now %v", originalUpdatedAt, second[0].UpdatedAt)
	}
	if second[0].Value != "v2" {
		t.Fatalf("value = %q, want v2", second[0].Value)
	}

	// Bulk set should also stamp updated_at on inserted rows.
	bulkBefore := time.Now().Add(-time.Second)
	if err := db.BulkSetEnvVars(project.ID, "production", []EnvVariable{
		{Key: "BULK_KEY", Value: "bulk-val"},
	}); err != nil {
		t.Fatalf("BulkSetEnvVars: %v", err)
	}
	bulk, _ := db.ListEnvVars(project.ID, "production")
	if len(bulk) != 1 {
		t.Fatalf("bulk: got %d vars, want 1", len(bulk))
	}
	if !bulk[0].UpdatedAt.After(bulkBefore) {
		t.Fatalf("bulk updated_at = %v, want > %v", bulk[0].UpdatedAt, bulkBefore)
	}
}

func TestProjectLatestPerEnvDeployTimestamps(t *testing.T) {
	db := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 1, Username: "u", Role: "admin"}
	db.UpsertUser(user)

	project := &Project{Name: "deploy-ts-project", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, Framework: "nextjs", PackageManager: "auto", BuildCommand: "b", InstallCommand: "i",
		NodeVersion: "22", Status: "active"}
	if err := db.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// No deployments yet → both timestamps nil.
	got, err := db.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject(empty): %v", err)
	}
	if got.LatestPreviewDeployAt != nil || got.LatestProductionDeployAt != nil {
		t.Fatalf("expected nil per-env timestamps with no deploys, got preview=%v prod=%v",
			got.LatestPreviewDeployAt, got.LatestProductionDeployAt)
	}

	// Insert deployments with controlled created_at and statuses.
	// We pass created_at as a SQLite-format string ("YYYY-MM-DD HH:MM:SS")
	// instead of a time.Time so the stored TEXT matches production rows
	// (which are always written via datetime('now')).
	insert := func(env, status string, createdAt time.Time) {
		t.Helper()
		if _, err := db.Exec(
			`INSERT INTO deployments (id, project_id, environment, status, branch, triggered_by, created_at)
			 VALUES (?, ?, ?, ?, 'main', ?, ?)`,
			NewID(), project.ID, env, status, user.ID, createdAt.UTC().Format("2006-01-02 15:04:05"),
		); err != nil {
			t.Fatalf("insert deployment env=%s status=%s: %v", env, status, err)
		}
	}

	base := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	insert("preview", "live", base)                      // first preview live
	insert("preview", "replaced", base.Add(time.Minute)) // newer but replaced — should be ignored
	insert("preview", "live", base.Add(2*time.Minute))   // newest live preview
	insert("preview", "failed", base.Add(3*time.Minute)) // newer but failed — should be ignored

	insert("production", "failed", base.Add(time.Minute)) // failed only → no live timestamp
	// Intentionally no live production deploys yet.

	got, err = db.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject(after deploys): %v", err)
	}
	if got.LatestPreviewDeployAt == nil {
		t.Fatal("expected latest preview deploy timestamp, got nil")
	}
	expectedPreview := base.Add(2 * time.Minute)
	if !got.LatestPreviewDeployAt.Equal(expectedPreview) {
		t.Fatalf("latest preview deploy = %v, want %v", *got.LatestPreviewDeployAt, expectedPreview)
	}
	if got.LatestProductionDeployAt != nil {
		t.Fatalf("expected nil production timestamp (no live deploys), got %v", *got.LatestProductionDeployAt)
	}

	// Add a live production deploy → both timestamps populated.
	prodLive := base.Add(5 * time.Minute)
	insert("production", "live", prodLive)

	got, err = db.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject(after prod live): %v", err)
	}
	if got.LatestProductionDeployAt == nil || !got.LatestProductionDeployAt.Equal(prodLive) {
		t.Fatalf("latest production deploy = %v, want %v", got.LatestProductionDeployAt, prodLive)
	}

	// Same data must surface through ListProjectsWithLatestDeployment.
	listed, err := db.ListProjectsWithLatestDeployment(user.ID, "")
	if err != nil {
		t.Fatalf("ListProjectsWithLatestDeployment: %v", err)
	}
	if len(listed) == 0 {
		t.Fatal("expected at least one project")
	}
	var pw *ProjectWithLatestDeployment
	for i := range listed {
		if listed[i].ID == project.ID {
			pw = &listed[i]
			break
		}
	}
	if pw == nil {
		t.Fatal("project missing from list")
	}
	if pw.LatestPreviewDeployAt == nil || !pw.LatestPreviewDeployAt.Equal(expectedPreview) {
		t.Fatalf("list latest preview = %v, want %v", pw.LatestPreviewDeployAt, expectedPreview)
	}
	if pw.LatestProductionDeployAt == nil || !pw.LatestProductionDeployAt.Equal(prodLive) {
		t.Fatalf("list latest production = %v, want %v", pw.LatestProductionDeployAt, prodLive)
	}
}

func TestAPITokenCRUD(t *testing.T) {
	database := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 100, Username: "tokenowner", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	// Create.
	token := &APIToken{
		UserID:    user.ID,
		Name:      "skill-action-mode",
		TokenHash: "hash-abc",
	}
	if err := database.CreateAPIToken(token); err != nil {
		t.Fatalf("create: %v", err)
	}
	if token.ID == "" {
		t.Fatalf("CreateAPIToken did not assign an ID")
	}

	// GetByHash returns the token.
	got, err := database.GetAPITokenByHash("hash-abc")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || got.ID != token.ID {
		t.Fatalf("get returned %v, want token %s", got, token.ID)
	}

	// ListForUser returns it.
	list, err := database.ListAPITokensForUser(user.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].ID != token.ID {
		t.Fatalf("list = %v, want 1 entry with id %s", list, token.ID)
	}

	// TouchLastUsed sets last_used_at.
	if err := database.TouchAPITokenLastUsed(token.ID); err != nil {
		t.Fatalf("touch: %v", err)
	}
	got, err = database.GetAPITokenByHash("hash-abc")
	if err != nil {
		t.Fatalf("get after touch: %v", err)
	}
	if !got.LastUsedAt.Valid {
		t.Fatalf("last_used_at not set after touch")
	}

	// Revoke makes it invisible to GetByHash.
	if err := database.RevokeAPIToken(token.ID, user.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	got, err = database.GetAPITokenByHash("hash-abc")
	if err != nil {
		t.Fatalf("get after revoke: %v", err)
	}
	if got != nil {
		t.Fatalf("get returned token after revoke")
	}

	// But ListForUser still includes it (so the UI can show it as revoked).
	list, _ = database.ListAPITokensForUser(user.ID)
	if len(list) != 1 {
		t.Fatalf("list after revoke = %d, want 1 (still visible)", len(list))
	}
	if !list[0].RevokedAt.Valid {
		t.Fatalf("revoked_at not set on listed entry")
	}

	// Revoking someone else's token must error.
	other := &User{ID: NewID(), GithubID: 101, Username: "stranger", Role: "user"}
	if err := database.UpsertUser(other); err != nil {
		t.Fatalf("upsert other: %v", err)
	}
	otherToken := &APIToken{UserID: other.ID, Name: "x", TokenHash: "hash-other"}
	if err := database.CreateAPIToken(otherToken); err != nil {
		t.Fatalf("create other token: %v", err)
	}
	if err := database.RevokeAPIToken(otherToken.ID, user.ID); err == nil {
		t.Fatalf("expected error when revoking another user's token")
	}

	// Expired tokens disappear from GetByHash even before revoke.
	expired := &APIToken{
		UserID:    user.ID,
		Name:      "expired",
		TokenHash: "hash-expired",
		ExpiresAt: NewNullableTime(time.Now().Add(-1 * time.Hour)),
	}
	if err := database.CreateAPIToken(expired); err != nil {
		t.Fatalf("create expired: %v", err)
	}
	got, _ = database.GetAPITokenByHash("hash-expired")
	if got != nil {
		t.Fatalf("get returned expired token")
	}
}

func TestMigration018AddsProductionOptInAndWebhookEventOutcomes(t *testing.T) {
	database, err := OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if _, err := database.Exec(`
		CREATE TABLE IF NOT EXISTS _migrations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)
	`); err != nil {
		t.Fatalf("create migrations table: %v", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("ReadDir migrations: %v", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() >= "018_auto_production_auto_deploy.sql" {
			continue
		}
		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", entry.Name(), err)
		}
		if _, err := database.Exec(string(content)); err != nil {
			t.Fatalf("exec migration %s: %v", entry.Name(), err)
		}
		if _, err := database.Exec("INSERT INTO _migrations (name) VALUES (?)", entry.Name()); err != nil {
			t.Fatalf("record migration %s: %v", entry.Name(), err)
		}
	}

	user := &User{ID: NewID(), GithubID: 42, Username: "migration-user", Role: "admin"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	project := &Project{
		Name:           "migration-app",
		GithubRepo:     "repo",
		GithubOwner:    "owner",
		Branch:         "main",
		UserID:         user.ID,
		Framework:      "nextjs",
		BuildCommand:   "bun run build",
		InstallCommand: "bun install",
		NodeVersion:    "22",
		Status:         "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	deploymentID := NewID()
	if _, err := database.Exec(
		`INSERT INTO deployments (id, project_id, environment, branch, status, trigger_source, triggered_by)
		 VALUES (?, ?, 'preview', 'main', 'queued', 'webhook', ?)`,
		deploymentID, project.ID, user.ID,
	); err != nil {
		t.Fatalf("insert legacy deployment: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO auto_build_configs (id, project_id, enabled, production_branch, preview_branches, webhook_id, webhook_secret)
		 VALUES (?, ?, 1, 'main', '*', 123, 'encrypted-secret')`,
		NewID(), project.ID,
	); err != nil {
		t.Fatalf("insert legacy auto_build_config: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO webhook_events (project_id, github_delivery_id, event_type, branch, commit_sha, commit_message, pusher, deployment_id, status)
		 VALUES (?, 'delivery-1', 'push', 'main', 'sha1', 'message', 'pusher', ?, 'processed')`,
		project.ID, deploymentID,
	); err != nil {
		t.Fatalf("insert legacy webhook_event: %v", err)
	}

	if err := database.Migrate(); err != nil {
		t.Fatalf("Migrate with legacy webhook events: %v", err)
	}

	config, err := database.GetAutoBuildConfig(project.ID)
	if err != nil {
		t.Fatalf("GetAutoBuildConfig: %v", err)
	}
	if config == nil {
		t.Fatal("expected auto build config")
	}
	if config.AutoProductionEnabled {
		t.Fatal("auto_production_enabled = true, want false for migrated configs")
	}

	var environment string
	if err := database.QueryRow(
		`SELECT environment FROM webhook_events WHERE github_delivery_id = 'delivery-1' AND project_id = ?`,
		project.ID,
	).Scan(&environment); err != nil {
		t.Fatalf("select migrated webhook event: %v", err)
	}
	if environment != "preview" {
		t.Fatalf("environment = %q, want preview", environment)
	}

	if err := database.CreateWebhookEvent(&WebhookEvent{
		ProjectID:        project.ID,
		GithubDeliveryID: "delivery-2",
		EventType:        "push",
		Environment:      "preview",
		Branch:           "main",
		CommitSHA:        "sha2",
		Status:           "processed",
	}); err != nil {
		t.Fatalf("CreateWebhookEvent preview: %v", err)
	}
	if err := database.CreateWebhookEvent(&WebhookEvent{
		ProjectID:        project.ID,
		GithubDeliveryID: "delivery-2",
		EventType:        "push",
		Environment:      "production",
		Branch:           "main",
		CommitSHA:        "sha2",
		Status:           "processed",
	}); err != nil {
		t.Fatalf("CreateWebhookEvent production: %v", err)
	}
	if err := database.CreateWebhookEvent(&WebhookEvent{
		ProjectID:        project.ID,
		GithubDeliveryID: "delivery-2",
		EventType:        "push",
		Environment:      "preview",
		Branch:           "main",
		CommitSHA:        "sha2",
		Status:           "processed",
	}); err == nil {
		t.Fatal("expected duplicate delivery/project/environment insert to fail")
	}
}
