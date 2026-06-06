package db

import "testing"

func seedMonitorProject(t *testing.T, db *DB, userID, name, healthPath string) *Project {
	t.Helper()
	p := &Project{
		Name:           name,
		GithubRepo:     name,
		GithubOwner:    "owner",
		Branch:         "main",
		UserID:         userID,
		Framework:      "nextjs",
		PackageManager: "auto",
		BuildCommand:   "bun run build",
		InstallCommand: "bun install",
		NodeVersion:    "22",
		Status:         "active",
		HealthPath:     healthPath,
	}
	if err := db.CreateProject(p); err != nil {
		t.Fatalf("CreateProject(%s): %v", name, err)
	}
	return p
}

func mustCreateDomain(t *testing.T, db *DB, d *Domain) {
	t.Helper()
	if err := db.CreateDomain(d); err != nil {
		t.Fatalf("CreateDomain(%s): %v", d.DomainName, err)
	}
}

func TestListProductionMonitorTargets(t *testing.T) {
	db := newTestDB(t)
	user := &User{ID: NewID(), GithubID: 1, Username: "u", Role: "user"}
	if err := db.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	// alpha: protected, two active production domains. The apex is_primary should
	// win over the www alias, and only one target should be returned.
	alpha := seedMonitorProject(t, db, user.ID, "alpha", "/healthz")
	if err := db.SetProjectPassword(alpha.ID, "production", "enc-secret"); err != nil {
		t.Fatalf("SetProjectPassword: %v", err)
	}
	mustCreateDomain(t, db, &Domain{ProjectID: alpha.ID, DomainName: "alpha.com", Environment: "production", IsPrimary: true, SSLStatus: "active"})
	mustCreateDomain(t, db, &Domain{ProjectID: alpha.ID, DomainName: "www.alpha.com", Environment: "production", IsPrimary: false, SSLStatus: "active"})

	// bravo: unprotected, one active production domain + one active preview
	// domain (the preview must be ignored).
	bravo := seedMonitorProject(t, db, user.ID, "bravo", "")
	mustCreateDomain(t, db, &Domain{ProjectID: bravo.ID, DomainName: "bravo.com", Environment: "production", IsPrimary: true, SSLStatus: "active"})
	mustCreateDomain(t, db, &Domain{ProjectID: bravo.ID, DomainName: "bravo.preview.example.com", Environment: "preview", IsAuto: true, SSLStatus: "active"})

	// charlie: production domain with SSL still pending → omitted.
	charlie := seedMonitorProject(t, db, user.ID, "charlie", "")
	mustCreateDomain(t, db, &Domain{ProjectID: charlie.ID, DomainName: "charlie.com", Environment: "production", IsPrimary: true, SSLStatus: "pending"})

	// gone: active production domain but the project is soft-deleted → omitted.
	gone := seedMonitorProject(t, db, user.ID, "gone", "")
	mustCreateDomain(t, db, &Domain{ProjectID: gone.ID, DomainName: "gone.com", Environment: "production", IsPrimary: true, SSLStatus: "active"})
	if err := db.DeleteProject(gone.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	targets, err := db.ListProductionMonitorTargets()
	if err != nil {
		t.Fatalf("ListProductionMonitorTargets: %v", err)
	}

	byProject := map[string]ProductionMonitorTarget{}
	for _, tg := range targets {
		byProject[tg.ProjectName] = tg
	}

	if len(targets) != 2 {
		t.Fatalf("got %d targets, want 2 (alpha, bravo); got %+v", len(targets), targets)
	}

	a, ok := byProject["alpha"]
	if !ok {
		t.Fatal("alpha missing from targets")
	}
	if a.DomainName != "alpha.com" {
		t.Errorf("alpha domain: got %q, want apex alpha.com (is_primary)", a.DomainName)
	}
	if !a.Protected {
		t.Error("alpha should be protected (production_password set)")
	}
	if a.HealthPath != "/healthz" {
		t.Errorf("alpha health_path: got %q, want /healthz", a.HealthPath)
	}

	b, ok := byProject["bravo"]
	if !ok {
		t.Fatal("bravo missing from targets")
	}
	if b.DomainName != "bravo.com" {
		t.Errorf("bravo domain: got %q, want bravo.com", b.DomainName)
	}
	if b.Protected {
		t.Error("bravo should not be protected")
	}

	if _, ok := byProject["charlie"]; ok {
		t.Error("charlie (ssl pending) should be omitted")
	}
	if _, ok := byProject["gone"]; ok {
		t.Error("gone (deleted project) should be omitted")
	}
}
