package domain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct {
	calls [][]string
}

func (r *fakeRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	r.calls = append(r.calls, call)
	return []byte("ok"), nil
}

func TestRequestSSLCertUsesHostBindMountsAndKeepUntilExpiring(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	manager := &Manager{
		ProxyCertsDir: "/opt/nginx-proxy/certs",
		ProxyHTMLDir:  "/opt/nginx-proxy/html",
		SSLEmail:      "admin@example.com",
		runner:        runner,
	}

	if err := manager.RequestSSLCert("acme-web.preview.example.com"); err != nil {
		t.Fatalf("RequestSSLCert returned error: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 docker invocation, got %d", len(runner.calls))
	}

	got := strings.Join(runner.calls[0], " ")
	for _, want := range []string{
		"docker run --rm",
		"-v /opt/nginx-proxy/certs:/etc/letsencrypt",
		"-v /opt/nginx-proxy/html:/var/www/html",
		"certbot/certbot certonly",
		"--cert-name acme-web.preview.example.com",
		"-d acme-web.preview.example.com",
		"--expand",
		"--keep-until-expiring",
		"--non-interactive",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected command to contain %q, got %s", want, got)
		}
	}
}

func TestRequestSSLCertSupportsMultipleHostnames(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	manager := &Manager{
		ProxyCertsDir: "/opt/nginx-proxy/certs",
		ProxyHTMLDir:  "/opt/nginx-proxy/html",
		SSLEmail:      "admin@example.com",
		runner:        runner,
	}

	if err := manager.RequestSSLCert("example.com", "www.example.com"); err != nil {
		t.Fatalf("RequestSSLCert returned error: %v", err)
	}

	got := strings.Join(runner.calls[0], " ")
	for _, want := range []string{
		"-d example.com",
		"-d www.example.com",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected command to contain %q, got %s", want, got)
		}
	}
}

func TestReloadNginxTestsConfigBeforeReload(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	manager := &Manager{
		ProxyContainer: "nginx-proxy",
		runner:         runner,
	}

	if err := manager.ReloadNginx(); err != nil {
		t.Fatalf("ReloadNginx returned error: %v", err)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 docker invocations, got %d", len(runner.calls))
	}

	if got := strings.Join(runner.calls[0], " "); got != "docker exec nginx-proxy nginx -t" {
		t.Fatalf("unexpected config test command: %s", got)
	}

	if got := strings.Join(runner.calls[1], " "); got != "docker exec nginx-proxy nginx -s reload" {
		t.Fatalf("unexpected reload command: %s", got)
	}
}

func TestWriteNginxConfigUsesExplicitSSLDomain(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	manager := &Manager{NginxConfDir: confDir}

	confPath, err := manager.WriteNginxConfig(ProvisionConfig{
		ProjectID:     "01KNTESTPROJECT",
		ProjectName:   "acme-web",
		Domain:        "acme-web.preview.example.com",
		Environment:   "preview",
		SSLDomain:     "preview.example.com",
		ContainerName: "deployik-acme-web-preview",
	})
	if err != nil {
		t.Fatalf("WriteNginxConfig returned error: %v", err)
	}

	if filepath.Dir(confPath) != confDir {
		t.Fatalf("expected config to be written to %s, got %s", confDir, confPath)
	}

	content, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("read nginx config: %v", err)
	}

	got := string(content)
	if !strings.Contains(got, "/etc/nginx/certs/live/preview.example.com/fullchain.pem") {
		t.Fatalf("expected config to reference explicit SSL domain, got:\n%s", got)
	}
	if !strings.Contains(got, "access_log /var/log/nginx/deployik-01KNTESTPROJECT-acme-web-preview.json deployik_json;") {
		t.Fatalf("expected config to enable deployik usage logging, got:\n%s", got)
	}
}

func TestWriteNginxConfigSplitsStaticAndDynamicRateLimits(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	manager := &Manager{NginxConfDir: confDir}

	confPath, err := manager.WriteNginxConfig(ProvisionConfig{
		ProjectID:     "01KNTESTPROJECT",
		ProjectName:   "acme-web",
		Domain:        "acme.io",
		Environment:   "production",
		ContainerName: "deployik-acme-web-production",
	})
	if err != nil {
		t.Fatalf("WriteNginxConfig returned error: %v", err)
	}

	content, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("read nginx config: %v", err)
	}

	got := string(content)

	// Locate the canonical HTTPS server — anchor on the json access_log
	// directive, which only appears in that block.
	canonicalIdx := strings.Index(got, "access_log /var/log/nginx/deployik-")
	if canonicalIdx < 0 {
		t.Fatalf("expected canonical HTTPS server block, got:\n%s", got)
	}
	canonical := got[canonicalIdx:]

	staticIdx := strings.Index(canonical, "location ~* (^/_next/static/")
	rootIdx := strings.Index(canonical, "location / {")
	if staticIdx < 0 {
		t.Fatalf("expected static-asset location block in canonical server, got:\n%s", canonical)
	}
	if rootIdx < 0 {
		t.Fatalf("expected `location /` block in canonical server, got:\n%s", canonical)
	}
	if staticIdx >= rootIdx {
		t.Fatalf("static-asset location must precede `location /` for readability")
	}

	// Static block: rate-limited by deployik_static, not deployik_dynamic.
	staticBlock := canonical[staticIdx:rootIdx]
	for _, want := range []string{
		"limit_req zone=deployik_static burst=200 nodelay;",
		"limit_req_status 429;",
	} {
		if !strings.Contains(staticBlock, want) {
			t.Fatalf("expected %q in static block, got:\n%s", want, staticBlock)
		}
	}
	if strings.Contains(staticBlock, "deployik_dynamic") {
		t.Fatalf("static block must not use the dynamic zone, got:\n%s", staticBlock)
	}

	// Root block: rate-limited by deployik_dynamic with 429 status.
	rootBlock := canonical[rootIdx:]
	for _, want := range []string{
		"limit_req zone=deployik_dynamic burst=100 nodelay;",
		"limit_req_status 429;",
	} {
		if !strings.Contains(rootBlock, want) {
			t.Fatalf("expected %q in `location /`, got:\n%s", want, rootBlock)
		}
	}

	// Per-IP connection cap should be set on the canonical server.
	if !strings.Contains(canonical, "limit_conn deployik_perip 50;") {
		t.Fatalf("expected per-IP connection cap, got:\n%s", canonical)
	}
}

func TestWriteNginxConfigUsesSmallerBurstsForPreview(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	manager := &Manager{NginxConfDir: confDir}

	confPath, err := manager.WriteNginxConfig(ProvisionConfig{
		ProjectID:     "01KNTESTPROJECT",
		ProjectName:   "acme-web",
		Domain:        "acme-web.preview.example.com",
		Environment:   "preview",
		ContainerName: "deployik-acme-web-preview",
	})
	if err != nil {
		t.Fatalf("WriteNginxConfig returned error: %v", err)
	}

	content, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("read nginx config: %v", err)
	}

	got := string(content)
	for _, want := range []string{
		"limit_req zone=deployik_dynamic burst=20 nodelay;",
		"limit_req zone=deployik_static burst=50 nodelay;",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected preview to use smaller burst %q, got:\n%s", want, got)
		}
	}
}

func TestWriteNginxConfigAddsWWWRedirectWhenConfigured(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	manager := &Manager{NginxConfDir: confDir}

	confPath, err := manager.WriteNginxConfig(ProvisionConfig{
		ProjectID:      "01KNTESTPROJECT",
		ProjectName:    "acme-web",
		Domain:         "example.com",
		RedirectDomain: "www.example.com",
		Environment:    "production",
		ContainerName:  "deployik-acme-web-production",
	})
	if err != nil {
		t.Fatalf("WriteNginxConfig returned error: %v", err)
	}

	content, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("read nginx config: %v", err)
	}

	got := string(content)
	for _, want := range []string{
		"server_name example.com www.example.com;",
		"server_name www.example.com;",
		"return 301 https://example.com$request_uri;",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected config to contain %q, got:\n%s", want, got)
		}
	}
}

func TestWildcardCoversMatchesSingleLabelPreviewSubdomains(t *testing.T) {
	t.Parallel()

	m := &Manager{
		ProxySSLCert:    "/etc/nginx/certs/live/wildcard.preview.example.com/fullchain.pem",
		ProxySSLKey:     "/etc/nginx/certs/live/wildcard.preview.example.com/privkey.pem",
		WildcardDomains: []string{"preview.example.com"},
	}

	covered := []string{
		"acme-app-api.preview.example.com",
		"acme-app.preview.example.com",
	}
	for _, host := range covered {
		if !m.wildcardCovers(host) {
			t.Fatalf("expected %s to be wildcard-covered", host)
		}
	}

	notCovered := []string{
		"preview.example.com",         // apex, not a subdomain
		"a.b.preview.example.com",     // two labels — *.preview.example.com covers one
		"acme-app.preview.example.org", // custom domain, different base
		"acme-app.example.com",      // different base
	}
	for _, host := range notCovered {
		if m.wildcardCovers(host) {
			t.Fatalf("expected %s NOT to be wildcard-covered", host)
		}
	}
}

func TestWildcardCoversRequiresConfiguredCert(t *testing.T) {
	t.Parallel()

	// Wildcard base configured but no PROXY_SSL_CERT → never matches.
	m := &Manager{WildcardDomains: []string{"preview.example.com"}}
	if m.wildcardCovers("acme-app-api.preview.example.com") {
		t.Fatal("expected no wildcard match when ProxySSLCert is empty")
	}
}

func TestCertPathsForReturnsWildcardOrPerDomain(t *testing.T) {
	t.Parallel()

	m := &Manager{
		ProxySSLCert:    "/etc/nginx/certs/live/wildcard.preview.example.com/fullchain.pem",
		ProxySSLKey:     "/etc/nginx/certs/live/wildcard.preview.example.com/privkey.pem",
		WildcardDomains: []string{"preview.example.com"},
	}

	cert, key := m.certPathsFor("acme-app-api.preview.example.com")
	if cert != m.ProxySSLCert || key != m.ProxySSLKey {
		t.Fatalf("expected wildcard pair, got cert=%s key=%s", cert, key)
	}

	// Non-wildcard nginx domain → per-domain path under /etc/nginx/certs.
	cert, key = m.certPathsFor("acme.example.org")
	if cert != "/etc/nginx/certs/live/acme.example.org/fullchain.pem" {
		t.Fatalf("unexpected nginx per-domain cert: %s", cert)
	}
	if key != "/etc/nginx/certs/live/acme.example.org/privkey.pem" {
		t.Fatalf("unexpected nginx per-domain key: %s", key)
	}

	// Apache mode → per-domain path under /etc/letsencrypt.
	ma := &Manager{ProxyConfigFormat: "apache"}
	cert, key = ma.certPathsFor("acme.example.org")
	if cert != "/etc/letsencrypt/live/acme.example.org/fullchain.pem" {
		t.Fatalf("unexpected apache per-domain cert: %s", cert)
	}
	if key != "/etc/letsencrypt/live/acme.example.org/privkey.pem" {
		t.Fatalf("unexpected apache per-domain key: %s", key)
	}
}

func TestNewManagerThreadsWildcardDomains(t *testing.T) {
	t.Parallel()

	m := NewManager(ManagerConfig{
		ProxySSLCert:    "/etc/nginx/certs/live/wildcard.preview.example.com/fullchain.pem",
		ProxySSLKey:     "/etc/nginx/certs/live/wildcard.preview.example.com/privkey.pem",
		WildcardDomains: []string{"preview.example.com"},
	})
	if !m.wildcardCovers("acme-app-api.preview.example.com") {
		t.Fatal("expected NewManager to thread WildcardDomains through to the matcher")
	}
}
