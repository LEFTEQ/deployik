package domain

import (
	"os"
	"strings"
	"testing"
)

func generateProtectedConfig(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	path, err := GenerateNginxConfig(dir, NginxConfig{
		ProjectID:         "proj-1",
		ProjectName:       "demo",
		Domain:            "demo.preview.example.com",
		Environment:       "preview",
		SSLDomain:         "demo.preview.example.com",
		ContainerName:     "deployik-demo-preview",
		PasswordProtected: true,
	})
	if err != nil {
		t.Fatalf("GenerateNginxConfig: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(content)
}

// The auth page must keep its 401 status. Serving it with 200 (the old
// `error_page 401 = @auth_page` form) let PWA service workers precache the
// password screen as the app shell, locking users out even after they
// entered the correct password.
func TestGenerateNginxConfig_AuthPageKeeps401Status(t *testing.T) {
	content := generateProtectedConfig(t)

	if strings.Contains(content, "error_page 401 =") {
		t.Fatalf("config rewrites 401 to the auth-page location status (200); auth page must be served with 401:\n%s", content)
	}
	if !strings.Contains(content, "error_page 401 /_deployik/auth.html;") {
		t.Fatalf("config missing 401 error_page pointing at internal auth page URI:\n%s", content)
	}
	if !strings.Contains(content, "location = /_deployik/auth.html") {
		t.Fatalf("config missing internal auth page location:\n%s", content)
	}
}

func TestGenerateNginxConfig_AuthPageIsNeverCacheable(t *testing.T) {
	content := generateProtectedConfig(t)

	authLoc := content[strings.Index(content, "location = /_deployik/auth.html"):]
	authLoc = authLoc[:strings.Index(authLoc, "}")+1]

	if !strings.Contains(authLoc, `add_header Cache-Control "no-store" always;`) {
		t.Fatalf("auth page location missing Cache-Control no-store:\n%s", authLoc)
	}
}

// The proxy must own response compression: upstreams (e.g. Next.js) gzip
// themselves when the client advertises gzip support, which bypasses nginx
// brotli and locks every browser to gzip. Stripping Accept-Encoding toward
// the upstream makes it respond identity-encoded so nginx can negotiate
// brotli or gzip per client.
func TestGenerateNginxConfig_StripsAcceptEncodingTowardUpstream(t *testing.T) {
	content := generateProtectedConfig(t)

	const directive = `proxy_set_header Accept-Encoding "";`
	if got := strings.Count(content, directive); got != 2 {
		t.Fatalf("expected Accept-Encoding stripped in both proxy locations (static + dynamic), found %d occurrences", got)
	}
}

// Non-production environments (preview, and any future non-"production" env)
// must carry X-Robots-Tag: noindex so search engines never index staging URLs
// and flag them as duplicate content against the real production domain. The
// header sits at the server level so every location (static + dynamic)
// inherits it; production stays fully indexable.
func TestGenerateNginxConfig_NoindexOnNonProduction(t *testing.T) {
	gen := func(environment string) string {
		t.Helper()
		dir := t.TempDir()
		path, err := GenerateNginxConfig(dir, NginxConfig{
			ProjectID:     "proj-1",
			ProjectName:   "demo",
			Domain:        "demo.preview.example.com",
			Environment:   environment,
			SSLDomain:     "demo.preview.example.com",
			ContainerName: "deployik-demo-" + environment,
		})
		if err != nil {
			t.Fatalf("GenerateNginxConfig: %v", err)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		return string(content)
	}

	const directive = `add_header X-Robots-Tag "noindex, nofollow" always;`

	preview := gen("preview")
	if !strings.Contains(preview, directive) {
		t.Fatalf("preview vhost must emit noindex X-Robots-Tag:\n%s", preview)
	}

	// Case-insensitive: a "Production" domain (any casing) must stay indexable.
	for _, prod := range []string{"production", "Production", "PRODUCTION"} {
		content := gen(prod)
		if strings.Contains(content, "X-Robots-Tag") {
			t.Fatalf("environment %q must NOT emit X-Robots-Tag (production stays indexable):\n%s", prod, content)
		}
	}
}

// nginx's add_header inheritance rule: a location that declares ANY add_header
// drops all server-level add_headers. The protected auth page re-declares the
// security headers for exactly this reason, so it must also re-declare noindex
// on non-production sites — otherwise the 401 login page on a preview domain
// would silently lose the header the rest of the site has.
func TestGenerateNginxConfig_NoindexReDeclaredOnProtectedAuthPage(t *testing.T) {
	full := generateProtectedConfig(t)

	authLoc := full[strings.Index(full, "location = /_deployik/auth.html"):]
	authLoc = authLoc[:strings.Index(authLoc, "}")+1]

	if !strings.Contains(authLoc, `add_header X-Robots-Tag "noindex, nofollow" always;`) {
		t.Fatalf("auth page location must re-declare noindex on non-production:\n%s", authLoc)
	}
}

// HTTP/3 is opt-in per instance (PROXY_HTTP3): when enabled the template adds
// QUIC listeners + the Alt-Svc discovery header to every 443 server block, but
// must never emit `reuseport` — nginx allows it once per address:port and it
// lives in the infra-owned default_server config, not in generated vhosts.
func TestGenerateNginxConfig_HTTP3(t *testing.T) {
	generate := func(http3 bool) string {
		t.Helper()
		dir := t.TempDir()
		path, err := GenerateNginxConfig(dir, NginxConfig{
			ProjectID:      "proj-1",
			ProjectName:    "demo",
			Domain:         "demo.example.com",
			RedirectDomain: "www.demo.example.com",
			Environment:    "production",
			SSLDomain:      "demo.example.com",
			ContainerName:  "deployik-demo-production",
			HTTP3:          http3,
		})
		if err != nil {
			t.Fatalf("GenerateNginxConfig: %v", err)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		return string(content)
	}

	enabled := generate(true)
	// Two 443 server blocks (redirect + main), each with v4 + v6 quic listeners.
	if got := strings.Count(enabled, "listen 443 quic;"); got != 2 {
		t.Fatalf("expected 2 IPv4 quic listeners, found %d:\n%s", got, enabled)
	}
	if got := strings.Count(enabled, "listen [::]:443 quic;"); got != 2 {
		t.Fatalf("expected 2 IPv6 quic listeners, found %d:\n%s", got, enabled)
	}
	// The value is the infra-owned $h3_alt_svc geo variable (infra-repo
	// 00-h3-policy.conf) — empty for VPN clients (header omitted, they stay
	// on h2/TCP), the h3 advertisement for everyone else. Never a literal:
	// QUIC over the VPN tunnel showed multi-minute response stalls browsers
	// can't fall back from mid-request.
	if got := strings.Count(enabled, `add_header Alt-Svc $h3_alt_svc always;`); got != 2 {
		t.Fatalf("expected Alt-Svc in both 443 server blocks, found %d:\n%s", got, enabled)
	}
	if strings.Contains(enabled, "reuseport") {
		t.Fatalf("generated vhost must never emit reuseport (allowed once per address:port, owned by infra default_server):\n%s", enabled)
	}

	disabled := generate(false)
	if strings.Contains(disabled, "quic") || strings.Contains(disabled, "Alt-Svc") {
		t.Fatalf("HTTP3=false must not emit quic listeners or Alt-Svc:\n%s", disabled)
	}
}
