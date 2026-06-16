package domain

import (
	"os"
	"strings"
	"testing"
)

// Apache mirror of the nginx noindex rule: non-production VirtualHosts must
// emit X-Robots-Tag: noindex so staging domains aren't indexed; production
// stays indexable. Requires mod_headers (already assumed — the template uses
// RequestHeader for X-Forwarded-*).
func TestGenerateApacheConfig_NoindexOnNonProduction(t *testing.T) {
	gen := func(environment string) string {
		t.Helper()
		dir := t.TempDir()
		path, err := GenerateApacheConfig(dir, ApacheConfig{
			NginxConfig: NginxConfig{
				ProjectID:     "proj-1",
				ProjectName:   "demo",
				Domain:        "demo.preview.example.com",
				Environment:   environment,
				SSLDomain:     "demo.preview.example.com",
				ContainerName: "deployik-demo-" + environment,
			},
			CertFile: "/etc/letsencrypt/live/demo.preview.example.com/fullchain.pem",
			KeyFile:  "/etc/letsencrypt/live/demo.preview.example.com/privkey.pem",
		})
		if err != nil {
			t.Fatalf("GenerateApacheConfig: %v", err)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		return string(content)
	}

	const directive = `Header always set X-Robots-Tag "noindex, nofollow"`

	preview := gen("preview")
	if !strings.Contains(preview, directive) {
		t.Fatalf("preview Apache vhost must emit noindex X-Robots-Tag:\n%s", preview)
	}

	production := gen("production")
	if strings.Contains(production, "X-Robots-Tag") {
		t.Fatalf("production Apache vhost must NOT emit X-Robots-Tag:\n%s", production)
	}
}
