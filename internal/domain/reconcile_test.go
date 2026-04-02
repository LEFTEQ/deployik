package domain

import (
	"fmt"
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
		"--cert-name demo-web2.preview.example.com",
		"--expand",
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

type selectiveFailRunner struct {
	fakeRunner
	failFor string
}

func (r *selectiveFailRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	r.calls = append(r.calls, call)
	if strings.Contains(strings.Join(call, " "), r.failFor) {
		return []byte("forced failure"), fmt.Errorf("forced failure")
	}
	return []byte("ok"), nil
}

func TestReconcileActiveConfigsContinuesAfterEarlierCertificateFailure(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	runner := &selectiveFailRunner{failFor: "honza-web.preview.example.com"}
	manager := &Manager{
		NginxConfDir:   confDir,
		ProxyContainer: "nginx-proxy",
		ProxyCertsDir:  "/opt/nginx-proxy/certs",
		ProxyHTMLDir:   "/opt/nginx-proxy/html",
		SSLEmail:       "admin@example.com",
		runner:         runner,
	}

	targets := []db.DomainProvisionTarget{
		{
			ProjectID:   "01KNHONZA",
			ProjectName: "honza-web",
			DomainName:  "honza-web.preview.example.com",
			Environment: "preview",
		},
		{
			ProjectID:   "01KNJENNY",
			ProjectName: "demo-web2",
			DomainName:  "demo-web2.preview.example.com",
			Environment: "preview",
		},
	}

	err := ReconcileActiveConfigs(manager, targets)
	if err == nil {
		t.Fatal("expected reconcile to return aggregated error")
	}
	if !strings.Contains(err.Error(), "honza-web.preview.example.com") {
		t.Fatalf("expected aggregated error to mention failed domain, got %v", err)
	}

	confPath := filepath.Join(confDir, "deployik-demo-web2-preview-lovinka-com.conf")
	content, readErr := os.ReadFile(confPath)
	if readErr != nil {
		t.Fatalf("expected later domain config to be written, got read error: %v", readErr)
	}

	got := string(content)
	if !strings.Contains(got, "server_name www.demo-web2.preview.example.com;") {
		t.Fatalf("expected later target to still get preview redirect config, got:\n%s", got)
	}

	if len(runner.calls) < 4 {
		t.Fatalf("expected reconcile to continue through later target and reload nginx, got %d calls", len(runner.calls))
	}

	lastCall := strings.Join(runner.calls[len(runner.calls)-1], " ")
	if !strings.Contains(lastCall, "docker exec nginx-proxy nginx -s reload") {
		t.Fatalf("expected nginx reload after successful writes, got %s", lastCall)
	}
}
