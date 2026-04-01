package db

import "testing"

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
	tables := []string{"users", "projects", "deployments", "build_logs", "domains", "env_variables", "_migrations"}
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
	projects, err := db.ListProjects(user.ID)
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
	projects, _ = db.ListProjects(user.ID)
	if len(projects) != 0 {
		t.Error("deleted project still shows in list")
	}
}

func TestDeploymentCRUD(t *testing.T) {
	db := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 1, Username: "u", Role: "admin"}
	db.UpsertUser(user)

	project := &Project{Name: "p1", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, Framework: "nextjs", RootDirectory: "", OutputDirectory: ".next", BuildCommand: "build", InstallCommand: "install",
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

func TestEnvVarCRUD(t *testing.T) {
	db := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 1, Username: "u", Role: "admin"}
	db.UpsertUser(user)

	project := &Project{Name: "p1", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, Framework: "nextjs", RootDirectory: "", OutputDirectory: ".next", BuildCommand: "b", InstallCommand: "i",
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
		UserID: user.ID, Framework: "nextjs", BuildCommand: "b", InstallCommand: "i",
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
