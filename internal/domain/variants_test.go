package domain

import "testing"

func TestResolveVariantPlanForProductionRootDomain(t *testing.T) {
	t.Parallel()

	plan := ResolveVariantPlan("Example.com.", "production")
	if plan.CanonicalDomain != "example.com" {
		t.Fatalf("canonical domain = %q, want example.com", plan.CanonicalDomain)
	}
	if plan.RedirectDomain != "www.example.com" {
		t.Fatalf("redirect domain = %q, want www.example.com", plan.RedirectDomain)
	}
}

func TestResolveVariantPlanStripsWWWForProductionDomain(t *testing.T) {
	t.Parallel()

	plan := ResolveVariantPlan("www.app.example.com", "production")
	if plan.CanonicalDomain != "app.example.com" {
		t.Fatalf("canonical domain = %q, want app.example.com", plan.CanonicalDomain)
	}
	if plan.RedirectDomain != "www.app.example.com" {
		t.Fatalf("redirect domain = %q, want www.app.example.com", plan.RedirectDomain)
	}
}

func TestResolveVariantPlanDoesNotAddWWWForSubdomain(t *testing.T) {
	t.Parallel()

	plan := ResolveVariantPlan("app.example.com", "production")
	if plan.CanonicalDomain != "app.example.com" {
		t.Fatalf("canonical domain = %q, want app.example.com", plan.CanonicalDomain)
	}
	if plan.RedirectDomain != "" {
		t.Fatalf("redirect domain = %q, want empty", plan.RedirectDomain)
	}
}

func TestResolveVariantPlanDoesNotAddWWWForPreviewSubdomain(t *testing.T) {
	t.Parallel()

	plan := ResolveVariantPlan("acme-web.preview.example.com", "preview")
	if plan.CanonicalDomain != "acme-web.preview.example.com" {
		t.Fatalf("canonical domain = %q, want preview host", plan.CanonicalDomain)
	}
	if plan.RedirectDomain != "" {
		t.Fatalf("redirect domain = %q, want empty for subdomain preview host", plan.RedirectDomain)
	}
}

func TestResolveVariantPlanStripsWWWForPreviewDomain(t *testing.T) {
	t.Parallel()

	plan := ResolveVariantPlan("www.acme-web.preview.example.com", "preview")
	if plan.CanonicalDomain != "acme-web.preview.example.com" {
		t.Fatalf("canonical domain = %q, want preview host", plan.CanonicalDomain)
	}
	if plan.RedirectDomain != "www.acme-web.preview.example.com" {
		t.Fatalf("redirect domain = %q, want preview www host", plan.RedirectDomain)
	}
}

func TestResolveVariantPlanAddsWWWForPreviewApex(t *testing.T) {
	t.Parallel()

	plan := ResolveVariantPlan("example.org", "preview")
	if plan.CanonicalDomain != "example.org" {
		t.Fatalf("canonical domain = %q, want example.org", plan.CanonicalDomain)
	}
	if plan.RedirectDomain != "www.example.org" {
		t.Fatalf("redirect domain = %q, want www.example.org", plan.RedirectDomain)
	}
}

func TestResolveVariantPlanForeignCustomPreviewSubdomain(t *testing.T) {
	t.Parallel()

	plan := ResolveVariantPlan("acme.example.org", "preview")
	if plan.CanonicalDomain != "acme.example.org" {
		t.Fatalf("canonical domain = %q, want acme.example.org", plan.CanonicalDomain)
	}
	if plan.RedirectDomain != "" {
		t.Fatalf("redirect domain = %q, want empty for custom preview subdomain", plan.RedirectDomain)
	}
}
