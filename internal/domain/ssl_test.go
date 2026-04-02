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
		"-d acme-web.preview.example.com",
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
