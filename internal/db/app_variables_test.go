package db

import "testing"

func resolvedByKey(vars []ProjectVariable) map[string]string {
	m := make(map[string]string, len(vars))
	for _, v := range vars {
		m[v.Key] = v.Value
	}
	return m
}

func TestResolvedDeployVariablesPrecedence(t *testing.T) {
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
		Name: "web", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, OrganizationID: org.ID, Framework: "static",
		PackageManager: "auto", Status: "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if err := database.AddProjectsToApp(app.ID, []string{project.ID}); err != nil {
		t.Fatalf("AddProjectsToApp: %v", err)
	}
	project, err = database.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}

	// app shared: A=app-shared, B=app-shared, C=app-shared, D=app-shared
	for _, kv := range []struct{ k, v string }{{"A", "app-shared"}, {"B", "app-shared"}, {"C", "app-shared"}, {"D", "app-shared"}} {
		if err := database.UpsertAppVariable(&AppVariable{AppID: app.ID, Environment: "shared", Kind: VariableKindEnv, Key: kv.k, Value: kv.v}); err != nil {
			t.Fatalf("UpsertAppVariable %s: %v", kv.k, err)
		}
	}
	// app production overrides B, C, D
	for _, kv := range []struct{ k, v string }{{"B", "app-prod"}, {"C", "app-prod"}, {"D", "app-prod"}} {
		if err := database.UpsertAppVariable(&AppVariable{AppID: app.ID, Environment: "production", Kind: VariableKindEnv, Key: kv.k, Value: kv.v}); err != nil {
			t.Fatalf("UpsertAppVariable %s: %v", kv.k, err)
		}
	}
	// project shared overrides C, D
	for _, kv := range []struct{ k, v string }{{"C", "proj-shared"}, {"D", "proj-shared"}} {
		if err := database.UpsertProjectVariable(&ProjectVariable{ProjectID: project.ID, Environment: "shared", Kind: VariableKindEnv, Key: kv.k, Value: kv.v}); err != nil {
			t.Fatalf("UpsertProjectVariable %s: %v", kv.k, err)
		}
	}
	// project production overrides D (most specific)
	if err := database.UpsertProjectVariable(&ProjectVariable{ProjectID: project.ID, Environment: "production", Kind: VariableKindEnv, Key: "D", Value: "proj-prod"}); err != nil {
		t.Fatalf("UpsertProjectVariable D: %v", err)
	}

	resolved, err := database.ListResolvedDeployEnvVars(project, "production")
	if err != nil {
		t.Fatalf("ListResolvedDeployEnvVars: %v", err)
	}
	got := resolvedByKey(resolved)
	want := map[string]string{
		"A": "app-shared",  // only app shared sets it
		"B": "app-prod",    // app env beats app shared
		"C": "proj-shared", // project shared beats app layers
		"D": "proj-prod",   // project env wins over everything
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("var %s = %q, want %q (full: %#v)", k, got[k], v, got)
		}
	}
}

func TestResolvedDeployVariablesStandaloneUnchanged(t *testing.T) {
	database := newTestDB(t)
	user := createAppTestUser(t, database, "owner", 1)
	org, err := database.EnsurePersonalOrganization(user)
	if err != nil {
		t.Fatalf("EnsurePersonalOrganization: %v", err)
	}
	project := &Project{
		Name: "solo", GithubRepo: "r", GithubOwner: "o", Branch: "main",
		UserID: user.ID, OrganizationID: org.ID, Framework: "static",
		PackageManager: "auto", Status: "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if err := database.UpsertProjectVariable(&ProjectVariable{ProjectID: project.ID, Environment: "shared", Kind: VariableKindEnv, Key: "X", Value: "1"}); err != nil {
		t.Fatalf("UpsertProjectVariable: %v", err)
	}
	project, err = database.GetProject(project.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if project.AppID != "" {
		t.Fatalf("standalone project unexpectedly has app_id %q", project.AppID)
	}

	deploy, err := database.ListResolvedDeployEnvVars(project, "production")
	if err != nil {
		t.Fatalf("ListResolvedDeployEnvVars: %v", err)
	}
	base, err := database.ListResolvedEnvVars(project.ID, "production")
	if err != nil {
		t.Fatalf("ListResolvedEnvVars: %v", err)
	}
	if len(deploy) != len(base) || resolvedByKey(deploy)["X"] != "1" {
		t.Fatalf("standalone deploy resolution drifted: deploy=%#v base=%#v", deploy, base)
	}
}
