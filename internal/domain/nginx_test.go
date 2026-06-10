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
