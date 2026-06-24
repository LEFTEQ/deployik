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

func TestPreviewContainerNameKeepsDNSLabelWithinLimit(t *testing.T) {
	// Reproduces the 502: a long project name + long branch slug whose *domain*
	// label fits 63 chars, but whose container name ("deployik-…-preview-<slug>")
	// would overflow and become unresolvable by Docker's embedded DNS.
	projectName := "hammer-and-screwdriver-b3afe2"
	slug, err := NormalizePreviewBranchSlug(projectName, "preview/o-nas-1a2865")
	if err != nil {
		t.Fatalf("NormalizePreviewBranchSlug: %v", err)
	}

	instance := &PreviewInstance{BranchSlug: slug}
	name := PreviewContainerName(projectName, instance)
	if len(name) > MaxDNSLabelLength {
		t.Fatalf("container name length = %d, want <= %d: %s", len(name), MaxDNSLabelLength, name)
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		t.Fatalf("container name should be trimmed of dashes, got %q", name)
	}
	if !strings.HasPrefix(name, "deployik-") {
		t.Fatalf("container name lost its deployik- prefix: %q", name)
	}

	// Deterministic: same inputs must always yield the same name (nginx upstream,
	// blue-green rename, reconcile and volume name all derive from this).
	if again := PreviewContainerName(projectName, instance); again != name {
		t.Fatalf("container name not deterministic: %q != %q", name, again)
	}

	// Distinct slugs must keep distinct names so two previews never collide.
	other := PreviewContainerName(projectName, &PreviewInstance{BranchSlug: slug + "-x"})
	if other == name {
		t.Fatalf("distinct slugs collided on container name: %q", name)
	}
}

func TestDeploymentContainerNameUnchangedWhenWithinLimit(t *testing.T) {
	// The common case must be byte-identical to the pre-fix output so existing
	// containers, volumes and nginx configs keep resolving unchanged.
	if got := DeploymentContainerName("demo-app", "production", nil); got != "deployik-demo-app-production" {
		t.Fatalf("production name = %q, want deployik-demo-app-production", got)
	}
	if got := PreviewContainerName("demo-app", &PreviewInstance{BranchSlug: "feature-foo"}); got != "deployik-demo-app-preview-feature-foo" {
		t.Fatalf("preview name = %q, want deployik-demo-app-preview-feature-foo", got)
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
