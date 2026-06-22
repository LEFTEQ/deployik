package db

import "testing"

func TestResolveLiveContainer(t *testing.T) {
	database := newTestDB(t)
	user := createAppTestUser(t, database, "owner", 2)
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

	mkLive := func(env, branch, instanceID, container string) {
		d := &Deployment{
			ProjectID: project.ID, Environment: env, Branch: branch,
			PreviewInstanceID: instanceID, Status: "live",
			TriggeredBy: user.ID, CommitSHA: "sha",
		}
		if err := database.CreateDeployment(d); err != nil {
			t.Fatalf("CreateDeployment %s/%s: %v", env, branch, err)
		}
		if err := database.UpdateDeploymentContainer(d.ID, "cid", container, "img"); err != nil {
			t.Fatalf("UpdateDeploymentContainer %s/%s: %v", env, branch, err)
		}
	}

	// Default preview instance (main) + a feature branch instance + a stale
	// instance with no live deployment.
	defInst, err := database.GetOrCreatePreviewInstance(project, "main")
	if err != nil {
		t.Fatalf("GetOrCreatePreviewInstance main: %v", err)
	}
	fxInst, err := database.GetOrCreatePreviewInstance(project, "feature-x")
	if err != nil {
		t.Fatalf("GetOrCreatePreviewInstance feature-x: %v", err)
	}
	staleInst, err := database.GetOrCreatePreviewInstance(project, "stale")
	if err != nil {
		t.Fatalf("GetOrCreatePreviewInstance stale: %v", err)
	}

	mkLive("production", "main", "", "deployik-api-production")
	mkLive("preview", "main", defInst.ID, "deployik-api-preview")
	mkLive("preview", "feature-x", fxInst.ID, "deployik-api-preview-feature-x")
	_ = staleInst // exists but has no live deployment

	cases := []struct {
		name        string
		environment string
		branch      string
		wantName    string
		wantFound   bool
	}{
		{"production", "production", "", "deployik-api-production", true},
		{"preview by branch", "preview", "feature-x", "deployik-api-preview-feature-x", true},
		{"preview default branch", "preview", "", "deployik-api-preview", true},
		{"preview unknown branch", "preview", "ghost", "", false},
		{"preview instance without live deploy", "preview", "stale", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, found, err := database.ResolveLiveContainer(project.ID, tc.environment, tc.branch)
			if err != nil {
				t.Fatalf("ResolveLiveContainer: %v", err)
			}
			if found != tc.wantFound || name != tc.wantName {
				t.Fatalf("got (%q, %v), want (%q, %v)", name, found, tc.wantName, tc.wantFound)
			}
		})
	}

	// A project with no deployments at all resolves to nothing (not an error).
	empty := &Project{
		Name: "empty", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, OrganizationID: org.ID, Framework: "static",
		PackageManager: "auto", Status: "active",
	}
	if err := database.CreateProject(empty); err != nil {
		t.Fatalf("CreateProject empty: %v", err)
	}
	if name, found, err := database.ResolveLiveContainer(empty.ID, "production", ""); err != nil || found || name != "" {
		t.Fatalf("expected empty production to resolve to (\"\", false, nil), got (%q, %v, %v)", name, found, err)
	}
}
