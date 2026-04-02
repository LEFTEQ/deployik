package domain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

func TestReconcileActiveConfigsEnsuresPreviewWWWCertificate(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	runner := &fakeRunner{}
	manager := &Manager{
		NginxConfDir:   confDir,
		ProxyContainer: "nginx-proxy",
		ProxyCertsDir:  "/opt/nginx-proxy/certs",
		ProxyHTMLDir:   "/opt/nginx-proxy/html",
		SSLEmail:       "admin@example.com",
		runner:         runner,
	}

	targets := []db.DomainProvisionTarget{{
		ProjectID:   "01KNTESTPROJECT",
		ProjectName: "demo-web2",
		DomainName:  "demo-web2.preview.example.com",
		Environment: "preview",
	}}

	if err := ReconcileActiveConfigs(manager, targets); err != nil {
		t.Fatalf("ReconcileActiveConfigs returned error: %v", err)
	}

	if len(runner.calls) != 3 {
		t.Fatalf("expected 3 runner invocations, got %d", len(runner.calls))
	}

	certbotCall := strings.Join(runner.calls[0], " ")
	for _, want := range []string{
		"certbot/certbot certonly",
		"-d demo-web2.preview.example.com",
		"-d www.demo-web2.preview.example.com",
	} {
		if !strings.Contains(certbotCall, want) {
			t.Fatalf("expected certbot call to contain %q, got %s", want, certbotCall)
		}
	}

	confPath := filepath.Join(confDir, "deployik-demo-web2-preview-lovinka-com.conf")
	content, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("read nginx config: %v", err)
	}

	got := string(content)
	if !strings.Contains(got, "server_name www.demo-web2.preview.example.com;") {
		t.Fatalf("expected preview redirect server in nginx config, got:\n%s", got)
	}
	if !strings.Contains(got, "return 301 https://demo-web2.preview.example.com$request_uri;") {
		t.Fatalf("expected preview redirect target in nginx config, got:\n%s", got)
	}
}
