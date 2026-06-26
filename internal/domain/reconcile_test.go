package domain

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lefteq/lovinka-deployik/internal/db"
)

func TestReconcileActiveConfigsSkipsWWWForPreviewSubdomain(t *testing.T) {
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

	if err := ReconcileActiveConfigs(manager, targets, nil); err != nil {
		t.Fatalf("ReconcileActiveConfigs returned error: %v", err)
	}

	// A preview subdomain has no redirect variant, so certbot must NOT run —
	// only the nginx config test + reload should be invoked.
	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 runner invocations (nginx -t + reload), got %d", len(runner.calls))
	}
	for _, call := range runner.calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "certbot/certbot certonly") {
			t.Fatalf("did not expect certbot invocation for preview subdomain, got %s", joined)
		}
	}

	confPath := filepath.Join(confDir, "deployik-demo-web2-preview-example-com.conf")
	content, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("read nginx config: %v", err)
	}

	got := string(content)
	if strings.Contains(got, "server_name www.demo-web2.preview.example.com;") {
		t.Fatalf("did not expect preview www redirect server block, got:\n%s", got)
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
	// Use production apex domains so certbot runs for each target (preview
	// subdomains have no www variant and skip certbot entirely).
	runner := &selectiveFailRunner{failFor: "honza-web.com"}
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
			DomainName:  "honza-web.com",
			Environment: "production",
		},
		{
			ProjectID:   "01KNJENNY",
			ProjectName: "demo-web2",
			DomainName:  "demo-web2.com",
			Environment: "production",
		},
	}

	err := ReconcileActiveConfigs(manager, targets, nil)
	if err == nil {
		t.Fatal("expected reconcile to return aggregated error")
	}
	if !strings.Contains(err.Error(), "honza-web.com") {
		t.Fatalf("expected aggregated error to mention failed domain, got %v", err)
	}

	confPath := filepath.Join(confDir, "deployik-demo-web2-com.conf")
	content, readErr := os.ReadFile(confPath)
	if readErr != nil {
		t.Fatalf("expected later domain config to be written, got read error: %v", readErr)
	}

	got := string(content)
	if !strings.Contains(got, "server_name www.demo-web2.com;") {
		t.Fatalf("expected later target to still get production www redirect config, got:\n%s", got)
	}

	if len(runner.calls) < 4 {
		t.Fatalf("expected reconcile to continue through later target and reload nginx, got %d calls", len(runner.calls))
	}

	lastCall := strings.Join(runner.calls[len(runner.calls)-1], " ")
	if !strings.Contains(lastCall, "docker exec nginx-proxy nginx -s reload") {
		t.Fatalf("expected nginx reload after successful writes, got %s", lastCall)
	}
}
