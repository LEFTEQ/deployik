package db

import "testing"

func TestSetAppMemberOrder(t *testing.T) {
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
	mk := func(name string) *Project {
		p := &Project{Name: name, GithubRepo: "r-" + name, GithubOwner: "o", Branch: "main", UserID: user.ID, OrganizationID: org.ID, Framework: "static", PackageManager: "auto", Status: "active"}
		if err := database.CreateProject(p); err != nil {
			t.Fatalf("CreateProject %s: %v", name, err)
		}
		if err := database.AddProjectsToApp(app.ID, []string{p.ID}); err != nil {
			t.Fatalf("AddProjectsToApp %s: %v", name, err)
		}
		return p
	}
	web, api, dbp := mk("web"), mk("api"), mk("db")

	if err := database.SetAppMemberOrder(app.ID, []string{dbp.ID, api.ID, web.ID}); err != nil {
		t.Fatalf("SetAppMemberOrder: %v", err)
	}
	members, err := database.ListProjectsByApp(app.ID)
	if err != nil {
		t.Fatalf("ListProjectsByApp: %v", err)
	}
	if len(members) != 3 || members[0].Name != "db" || members[1].Name != "api" || members[2].Name != "web" {
		t.Fatalf("unexpected order: %v", []string{members[0].Name, members[1].Name, members[2].Name})
	}

	if err := database.SetAppMemberOrder(app.ID, []string{web.ID, "not-a-member"}); err == nil {
		t.Fatalf("expected error for non-member project id")
	}
}
