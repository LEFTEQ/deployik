package db

import (
	"strings"
	"testing"
)

func TestNormalizePreviewBranchSlugSanitizesBranchForHostname(t *testing.T) {
	slug, err := NormalizePreviewBranchSlug("demo-app", "Feature/Checkout Flow")
	if err != nil {
		t.Fatalf("NormalizePreviewBranchSlug: %v", err)
	}
	if slug != "feature-checkout-flow" {
		t.Fatalf("slug = %q, want feature-checkout-flow", slug)
	}
}

func TestNormalizePreviewBranchSlugKeepsDomainLabelWithinDNSLimit(t *testing.T) {
	projectName := "really-long-project-name-that-still-fits"
	slug, err := NormalizePreviewBranchSlug(projectName, "feature/"+strings.Repeat("very-long-name-", 12))
	if err != nil {
		t.Fatalf("NormalizePreviewBranchSlug: %v", err)
	}

	label := projectName + "-" + slug
	if len(label) > MaxDNSLabelLength {
		t.Fatalf("label length = %d, want <= %d: %s", len(label), MaxDNSLabelLength, label)
	}
	if slug == "" || strings.HasSuffix(slug, "-") {
		t.Fatalf("slug should be non-empty and trimmed, got %q", slug)
	}
}

func TestGetOrCreatePreviewInstanceReusesBranchAndResolvesSlugCollision(t *testing.T) {
	database := newTestDB(t)
	user := &User{ID: NewID(), GithubID: 1, Username: "owner", Role: "admin"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	project := &Project{
		Name:           "demo-app",
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

	first, err := database.GetOrCreatePreviewInstance(project, "feature/foo")
	if err != nil {
		t.Fatalf("GetOrCreatePreviewInstance first: %v", err)
	}
	again, err := database.GetOrCreatePreviewInstance(project, "feature/foo")
	if err != nil {
		t.Fatalf("GetOrCreatePreviewInstance again: %v", err)
	}
	if again.ID != first.ID {
		t.Fatalf("same branch returned different instance: %s != %s", again.ID, first.ID)
	}
	if first.BranchSlug != "feature-foo" {
		t.Fatalf("first slug = %q, want feature-foo", first.BranchSlug)
	}
	if got := first.AutoDomain(project.Name); got != "demo-app-feature-foo.preview.example.com" {
		t.Fatalf("auto domain = %q", got)
	}

	colliding, err := database.GetOrCreatePreviewInstance(project, "feature_foo")
	if err != nil {
		t.Fatalf("GetOrCreatePreviewInstance colliding: %v", err)
	}
	if colliding.ID == first.ID {
		t.Fatal("different branch reused first preview instance")
	}
	if colliding.BranchSlug == first.BranchSlug {
		t.Fatalf("slug collision was not resolved: %q", colliding.BranchSlug)
	}
	if !strings.HasPrefix(colliding.BranchSlug, "feature-foo-") {
		t.Fatalf("colliding slug = %q, want feature-foo-<suffix>", colliding.BranchSlug)
	}
}
