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
	if got := strings.Count(enabled, `add_header Alt-Svc 'h3=":443"; ma=86400' always;`); got != 2 {
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
