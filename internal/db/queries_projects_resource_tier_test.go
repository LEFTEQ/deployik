package db

import "testing"

func TestProjectResourceTierRoundTrip(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	user := &User{ID: NewID(), GithubID: 42, Username: "tier-user", Role: "user"}
	db.UpsertUser(user)

	project := &Project{
		Name:         "tier-project",
		GithubRepo:   "my-app",
		GithubOwner:  "tier-user",
		Branch:       "main",
		UserID:       user.ID,
		Framework:    "nextjs",
		Status:       "active",
		ResourceTier: "medium",
	}
	if err := db.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	got, err := db.GetProject(project.ID)
	if err != nil || got == nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.ResourceTier != "medium" {
		t.Errorf("resource_tier round-trip = %q, want %q", got.ResourceTier, "medium")
	}

	// Update to large
	got.ResourceTier = "large"
	if err := db.UpdateProject(got); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	again, _ := db.GetProject(project.ID)
	if again.ResourceTier != "large" {
		t.Errorf("after UpdateProject, resource_tier = %q, want %q", again.ResourceTier, "large")
	}
}

func TestProjectResourceTierDefaultsToSmall(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	user := &User{ID: NewID(), GithubID: 43, Username: "default-user", Role: "user"}
	db.UpsertUser(user)

	// Empty ResourceTier should land as "small" via the normalizer.
	project := &Project{
		Name:        "default-tier-project",
		GithubRepo:  "my-app",
		GithubOwner: "default-user",
		Branch:      "main",
		UserID:      user.ID,
		Framework:   "nextjs",
		Status:      "active",
		// ResourceTier intentionally left empty
	}
	if err := db.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if project.ResourceTier != "small" {
		t.Errorf("after CreateProject with empty tier, in-memory value = %q, want %q",
			project.ResourceTier, "small")
	}
	got, _ := db.GetProject(project.ID)
	if got.ResourceTier != "small" {
		t.Errorf("after GetProject, resource_tier = %q, want %q", got.ResourceTier, "small")
	}
}

func TestNormalizeStoredResourceTier(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"nano":    "nano",
		"small":   "small",
		"medium":  "medium",
		"large":   "large",
		"":        "small",
		"  Small": "small", // case + whitespace tolerated
		"LARGE":   "large",
		"xl":      "small", // unknown -> default
	}
	for in, want := range cases {
		if got := normalizeStoredResourceTier(in); got != want {
			t.Errorf("normalizeStoredResourceTier(%q) = %q, want %q", in, got, want)
		}
	}
}
