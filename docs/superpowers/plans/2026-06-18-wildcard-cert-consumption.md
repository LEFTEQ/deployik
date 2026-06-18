# Wildcard-cert Consumption for Preview Subdomains — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let Deployik serve a pre-issued `*.preview.example.com` wildcard cert to single-label preview subdomains (skipping certbot), while custom domains keep their per-domain certbot flow byte-for-byte.

**Architecture:** A per-domain matcher (`wildcardCovers` / `certPathsFor`) on the domain `Manager` decides, for each hostname, whether to serve the configured wildcard cert (and skip certbot) or fall back to the per-domain Let's Encrypt path. Cert-path resolution is centralized inside `WriteNginxConfig` (the single non-test caller of `GenerateNginxConfig`), so all five caller paths — deploy, reconcile, domain-verify, domain-move, protection-toggle — are covered with no per-call-site edits. Wildcard issuance/renewal stays an ops runbook (GoDaddy DNS-01).

**Tech Stack:** Go (stdlib `text/template`, `testing`); nginx + certbot (Docker) on the Lovinka VPS.

**Spec:** `docs/superpowers/specs/2026-06-18-wildcard-cert-consumption-design.md`

---

## File Structure

- `internal/domain/ssl.go` — add `WildcardDomains` to `ManagerConfig` + `Manager`; thread through `NewManager`; add `hostUnderWildcard` / `wildcardCovers` / `certPathsFor`; swap the global certbot skip + apache cert defaulting for the matcher.
- `internal/domain/ssl_test.go` — matcher unit tests; `NewManager` plumbing test; `ProvisionDomain` certbot-skip behavioral test.
- `internal/domain/nginx.go` — add `CertFile`/`KeyFile` to `NginxConfig`; template them; default them in `GenerateNginxConfig`.
- `internal/domain/reconcile.go` — swap the certbot gate + apache cert defaulting for the matcher.
- `internal/config/config.go` — add `ProxySSLWildcardDomains` (`PROXY_SSL_WILDCARD_DOMAINS`).
- `cmd/server/main.go` — wire `WildcardDomains` from config into `ManagerConfig`.
- `docs/runbooks/wildcard-preview-cert.md` — new ops runbook (DNS-01 issuance + env + renewal).
- `.env.example` — document the new env var.

No DB/schema change.

---

## Task 1: Matcher helpers + `Manager.WildcardDomains`

**Files:**
- Modify: `internal/domain/ssl.go` (struct `Manager` lines 29-43; add helpers near `sslDomain`/`requestSSLDomains` ~line 317)
- Test: `internal/domain/ssl_test.go`

- [ ] **Step 1: Add the `WildcardDomains` field to the `Manager` struct**

In `internal/domain/ssl.go`, the `Manager` struct (lines 29-43) currently ends:
```go
	ProxySSLCert      string
	ProxySSLKey       string
	HTTP3             bool
	runner            commandRunner
}
```
Change to add `WildcardDomains`:
```go
	ProxySSLCert      string
	ProxySSLKey       string
	// WildcardDomains are the base domains a configured PROXY_SSL_CERT covers,
	// e.g. ["preview.example.com"]. A single-label subdomain of one of these is
	// served the wildcard cert and skips certbot. Empty → matcher never matches.
	WildcardDomains   []string
	HTTP3             bool
	runner            commandRunner
}
```

- [ ] **Step 2: Write the failing matcher test**

Append to `internal/domain/ssl_test.go`:
```go
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
		"preview.example.com",           // apex, not a subdomain
		"a.b.preview.example.com",        // two labels — *.preview.example.com covers one
		"acme-app.preview.example.org",    // custom domain, different base
		"acme-app.example.com",         // different base
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
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/domain/ -run 'TestWildcardCovers|TestCertPathsFor' -v`
Expected: FAIL — `m.wildcardCovers undefined` / `m.certPathsFor undefined`.

- [ ] **Step 4: Implement the helpers**

In `internal/domain/ssl.go`, add immediately after the `sslDomain` method (after line 322, before `requestSSLDomains`):
```go
// hostUnderWildcard reports whether host is a single-label subdomain of base
// (e.g. "api.preview.example.com" under "preview.example.com"), the only shape a
// "*.base" wildcard cert covers. Multi-label hosts and the base apex are not.
func hostUnderWildcard(host, base string) bool {
	suffix := "." + base
	if !strings.HasSuffix(host, suffix) {
		return false
	}
	label := strings.TrimSuffix(host, suffix)
	return label != "" && !strings.Contains(label, ".")
}

// wildcardCovers reports whether host should be served by the configured
// wildcard cert (and therefore skip certbot): a wildcard cert must be configured
// AND host must be a single-label subdomain of one of the WildcardDomains bases.
func (m *Manager) wildcardCovers(host string) bool {
	if m.ProxySSLCert == "" {
		return false
	}
	for _, base := range m.WildcardDomains {
		if hostUnderWildcard(host, base) {
			return true
		}
	}
	return false
}

// certPathsFor returns the cert + key paths a vhost should reference for host:
// the configured wildcard pair when host is wildcard-covered, otherwise the
// per-domain Let's Encrypt live paths. The per-domain mount root differs by
// proxy format (nginx reads /etc/nginx/certs, apache reads /etc/letsencrypt).
func (m *Manager) certPathsFor(host string) (certFile, keyFile string) {
	if m.wildcardCovers(host) {
		return m.ProxySSLCert, m.ProxySSLKey
	}
	base := "/etc/nginx/certs"
	if m.ProxyConfigFormat == "apache" {
		base = "/etc/letsencrypt"
	}
	return fmt.Sprintf("%s/live/%s/fullchain.pem", base, host),
		fmt.Sprintf("%s/live/%s/privkey.pem", base, host)
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/domain/ -run 'TestWildcardCovers|TestCertPathsFor' -v`
Expected: PASS (3 tests).

- [ ] **Step 6: Commit**

```bash
git add internal/domain/ssl.go internal/domain/ssl_test.go
git commit -m "feat(ssl): wildcardCovers + certPathsFor matcher for preview wildcard cert"
```

---

## Task 2: Config + plumbing (`PROXY_SSL_WILDCARD_DOMAINS` → Manager)

**Files:**
- Modify: `internal/config/config.go` (struct lines 11-45; `Load` literal lines 48-79)
- Modify: `internal/domain/ssl.go` (`ManagerConfig` lines 14-27; `NewManager` lines 80-96)
- Modify: `cmd/server/main.go` (lines 110-121)
- Test: `internal/domain/ssl_test.go`

- [ ] **Step 1: Write the failing plumbing test**

Append to `internal/domain/ssl_test.go`:
```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/domain/ -run TestNewManagerThreadsWildcardDomains -v`
Expected: FAIL — `unknown field WildcardDomains in struct literal of type domain.ManagerConfig`.

- [ ] **Step 3: Add `WildcardDomains` to `ManagerConfig` and `NewManager`**

In `internal/domain/ssl.go`, the `ManagerConfig` struct (lines 14-27) currently ends:
```go
	ProxySSLCert      string
	ProxySSLKey       string
	HTTP3             bool // nginx format only — see NginxConfig.HTTP3
}
```
Change to:
```go
	ProxySSLCert      string
	ProxySSLKey       string
	WildcardDomains   []string // bases a configured PROXY_SSL_CERT covers
	HTTP3             bool     // nginx format only — see NginxConfig.HTTP3
}
```

In `NewManager` (lines 80-96), the returned struct currently has:
```go
		ProxySSLCert:      cfg.ProxySSLCert,
		ProxySSLKey:       cfg.ProxySSLKey,
		HTTP3:             cfg.HTTP3,
		runner:            execRunner{},
```
Change to:
```go
		ProxySSLCert:      cfg.ProxySSLCert,
		ProxySSLKey:       cfg.ProxySSLKey,
		WildcardDomains:   cfg.WildcardDomains,
		HTTP3:             cfg.HTTP3,
		runner:            execRunner{},
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/domain/ -run TestNewManagerThreadsWildcardDomains -v`
Expected: PASS.

- [ ] **Step 5: Add the config field + env loading**

In `internal/config/config.go`, the `Config` struct (lines 11-45) has:
```go
	ProxySSLCert            string
	ProxySSLKey             string
	WebhookURL              string
```
Change to insert the new field:
```go
	ProxySSLCert            string
	ProxySSLKey             string
	ProxySSLWildcardDomains []string
	WebhookURL              string
```

In `Load`, the struct literal (lines 48-79) has:
```go
		ProxySSLCert:            os.Getenv("PROXY_SSL_CERT"),
		ProxySSLKey:             os.Getenv("PROXY_SSL_KEY"),
		MonitoringToken:         os.Getenv("MONITORING_TOKEN"),
```
Change to:
```go
		ProxySSLCert:            os.Getenv("PROXY_SSL_CERT"),
		ProxySSLKey:             os.Getenv("PROXY_SSL_KEY"),
		ProxySSLWildcardDomains: splitCSV(os.Getenv("PROXY_SSL_WILDCARD_DOMAINS")),
		MonitoringToken:         os.Getenv("MONITORING_TOKEN"),
```
(`splitCSV` is defined at `config.go:123` and returns `nil` for an empty value.)

- [ ] **Step 6: Wire config → ManagerConfig in main.go**

In `cmd/server/main.go`, the `domain.NewManager(domain.ManagerConfig{...})` block (lines 110-121) has:
```go
		ProxySSLCert:      cfg.ProxySSLCert,
		ProxySSLKey:       cfg.ProxySSLKey,
```
Change to:
```go
		ProxySSLCert:      cfg.ProxySSLCert,
		ProxySSLKey:       cfg.ProxySSLKey,
		WildcardDomains:   cfg.ProxySSLWildcardDomains,
```

- [ ] **Step 7: Build + run the package tests**

Run: `go build ./... && go test ./internal/domain/ ./internal/config/ -v`
Expected: build succeeds; all tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/config/config.go internal/domain/ssl.go internal/domain/ssl_test.go cmd/server/main.go
git commit -m "feat(config): PROXY_SSL_WILDCARD_DOMAINS threaded into the domain Manager"
```

---

## Task 3: nginx cert wiring (`CertFile`/`KeyFile`)

**Files:**
- Modify: `internal/domain/nginx.go` (template lines 50-51, 80-81; struct `NginxConfig` lines 204-238; `GenerateNginxConfig` lines 270-304)
- Modify: `internal/domain/ssl.go` (`WriteNginxConfig` lines 264-289)
- Test: `internal/domain/ssl_test.go`

- [ ] **Step 1: Write the failing nginx cert-path test**

Append to `internal/domain/ssl_test.go`:
```go
func TestWriteNginxConfigUsesWildcardCertWhenCovered(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	manager := &Manager{
		NginxConfDir:    confDir,
		ProxySSLCert:    "/etc/nginx/certs/live/wildcard.preview.example.com/fullchain.pem",
		ProxySSLKey:     "/etc/nginx/certs/live/wildcard.preview.example.com/privkey.pem",
		WildcardDomains: []string{"preview.example.com"},
	}

	confPath, err := manager.WriteNginxConfig(ProvisionConfig{
		ProjectID:     "01KNTESTPROJECT",
		ProjectName:   "acme-app-api",
		Domain:        "acme-app-api.preview.example.com",
		Environment:   "preview",
		ContainerName: "deployik-acme-app-api-preview",
	})
	if err != nil {
		t.Fatalf("WriteNginxConfig returned error: %v", err)
	}

	content, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("read nginx config: %v", err)
	}
	got := string(content)

	if !strings.Contains(got, "ssl_certificate /etc/nginx/certs/live/wildcard.preview.example.com/fullchain.pem;") {
		t.Fatalf("expected wildcard cert path, got:\n%s", got)
	}
	if !strings.Contains(got, "ssl_certificate_key /etc/nginx/certs/live/wildcard.preview.example.com/privkey.pem;") {
		t.Fatalf("expected wildcard key path, got:\n%s", got)
	}
	// Must NOT fall back to a per-domain path for a covered host.
	if strings.Contains(got, "/etc/nginx/certs/live/acme-app-api.preview.example.com/") {
		t.Fatalf("did not expect per-domain cert path for a wildcard-covered host, got:\n%s", got)
	}
}

func TestWriteNginxConfigUsesPerDomainCertForCustomDomain(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	// Wildcard configured, but acme.example.org is NOT under it → per-domain path.
	manager := &Manager{
		NginxConfDir:    confDir,
		ProxySSLCert:    "/etc/nginx/certs/live/wildcard.preview.example.com/fullchain.pem",
		ProxySSLKey:     "/etc/nginx/certs/live/wildcard.preview.example.com/privkey.pem",
		WildcardDomains: []string{"preview.example.com"},
	}

	confPath, err := manager.WriteNginxConfig(ProvisionConfig{
		ProjectID:     "01KNTESTPROJECT",
		ProjectName:   "acme",
		Domain:        "acme.example.org",
		Environment:   "production",
		ContainerName: "deployik-acme-production",
	})
	if err != nil {
		t.Fatalf("WriteNginxConfig returned error: %v", err)
	}

	content, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("read nginx config: %v", err)
	}
	got := string(content)

	if !strings.Contains(got, "ssl_certificate /etc/nginx/certs/live/acme.example.org/fullchain.pem;") {
		t.Fatalf("expected per-domain cert path, got:\n%s", got)
	}
	if strings.Contains(got, "wildcard.preview.example.com") {
		t.Fatalf("did not expect the wildcard cert for a custom domain, got:\n%s", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/domain/ -run 'TestWriteNginxConfigUsesWildcardCert|TestWriteNginxConfigUsesPerDomainCert' -v`
Expected: FAIL — the generated config still contains the hardcoded `/etc/nginx/certs/live/acme-app-api.preview.example.com/...` (template uses `{{.SSLDomain}}`), so the wildcard assertion fails.

- [ ] **Step 3: Add `CertFile`/`KeyFile` to `NginxConfig`**

In `internal/domain/nginx.go`, the `NginxConfig` struct (lines 204-238) has:
```go
	SSLDomain         string // may differ for wildcard certs
	ContainerName     string
```
Change to:
```go
	SSLDomain         string // legacy default source for CertFile/KeyFile
	// CertFile/KeyFile are the cert+key paths the vhost references. Set by the
	// caller (WriteNginxConfig resolves wildcard-vs-per-domain via certPathsFor).
	// When empty, GenerateNginxConfig defaults them to the per-domain path built
	// from SSLDomain, preserving the pre-wildcard behavior for direct callers.
	CertFile          string
	KeyFile           string
	ContainerName     string
```

- [ ] **Step 4: Template the cert lines (both server blocks)**

In `internal/domain/nginx.go`, the redirect server block (lines 50-51) and the canonical server block (lines 80-81) both currently read:
```
    ssl_certificate /etc/nginx/certs/live/{{.SSLDomain}}/fullchain.pem;
    ssl_certificate_key /etc/nginx/certs/live/{{.SSLDomain}}/privkey.pem;
```
Replace **both** occurrences with:
```
    ssl_certificate {{.CertFile}};
    ssl_certificate_key {{.KeyFile}};
```

- [ ] **Step 5: Default `CertFile`/`KeyFile` in `GenerateNginxConfig`**

In `internal/domain/nginx.go`, `GenerateNginxConfig` (lines 270-304) starts:
```go
func GenerateNginxConfig(confDir string, cfg NginxConfig) (string, error) {
	if cfg.ContainerUpstream == "" {
		cfg.ContainerUpstream = cfg.ContainerName + ":3000"
	}
```
Insert the cert defaults immediately after the opening brace:
```go
func GenerateNginxConfig(confDir string, cfg NginxConfig) (string, error) {
	if cfg.CertFile == "" {
		cfg.CertFile = fmt.Sprintf("/etc/nginx/certs/live/%s/fullchain.pem", cfg.SSLDomain)
	}
	if cfg.KeyFile == "" {
		cfg.KeyFile = fmt.Sprintf("/etc/nginx/certs/live/%s/privkey.pem", cfg.SSLDomain)
	}
	if cfg.ContainerUpstream == "" {
		cfg.ContainerUpstream = cfg.ContainerName + ":3000"
	}
```
(This keeps every direct `GenerateNginxConfig` caller — e.g. `nginx_test.go` and `TestWriteNginxConfigUsesExplicitSSLDomain` — byte-identical to today: `{{.CertFile}}` renders the same `/etc/nginx/certs/live/<SSLDomain>/fullchain.pem` string the old template hardcoded.)

- [ ] **Step 6: Resolve cert paths in `WriteNginxConfig`**

In `internal/domain/ssl.go`, `WriteNginxConfig` (lines 264-289) currently ends:
```go
	return GenerateNginxConfig(m.NginxConfDir, NginxConfig{
		ProjectID:         cfg.ProjectID,
		ProjectName:       cfg.ProjectName,
		Domain:            cfg.Domain,
		RedirectDomain:    cfg.RedirectDomain,
		Environment:       cfg.Environment,
		SSLDomain:         cfg.sslDomain(),
		ContainerName:     cfg.ContainerName,
		ContainerUpstream: upstream,
		PasswordProtected: cfg.PasswordProtected,
		HTTP3:             m.HTTP3,
	})
}
```
Change to resolve and pass the cert paths:
```go
	certFile, keyFile := m.certPathsFor(cfg.sslDomain())
	return GenerateNginxConfig(m.NginxConfDir, NginxConfig{
		ProjectID:         cfg.ProjectID,
		ProjectName:       cfg.ProjectName,
		Domain:            cfg.Domain,
		RedirectDomain:    cfg.RedirectDomain,
		Environment:       cfg.Environment,
		SSLDomain:         cfg.sslDomain(),
		CertFile:          certFile,
		KeyFile:           keyFile,
		ContainerName:     cfg.ContainerName,
		ContainerUpstream: upstream,
		PasswordProtected: cfg.PasswordProtected,
		HTTP3:             m.HTTP3,
	})
}
```

- [ ] **Step 7: Run the new tests + the full domain suite**

Run: `go test ./internal/domain/ -v`
Expected: the two new tests PASS; **all pre-existing tests still PASS** — in particular `TestWriteNginxConfigUsesExplicitSSLDomain` (still finds `/etc/nginx/certs/live/preview.example.com/fullchain.pem`) and every `TestGenerateNginxConfig_*`.

- [ ] **Step 8: Commit**

```bash
git add internal/domain/nginx.go internal/domain/ssl.go internal/domain/ssl_test.go
git commit -m "feat(nginx): template cert paths via CertFile/KeyFile; serve wildcard when covered"
```

---

## Task 4: Per-domain certbot gate (deploy + reconcile + apache parity)

**Files:**
- Modify: `internal/domain/ssl.go` (`ProvisionDomain` lines 129-141 and 149-156)
- Modify: `internal/domain/reconcile.go` (lines 36-42 and 91-98)
- Test: `internal/domain/ssl_test.go`

- [ ] **Step 1: Write the failing certbot-skip behavioral test**

Append to `internal/domain/ssl_test.go`:
```go
func TestProvisionDomainSkipsCertbotForWildcardCoveredDomain(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	runner := &fakeRunner{}
	manager := &Manager{
		NginxConfDir:    confDir,
		ProxyContainer:  "nginx-proxy",
		ProxyCertsDir:   "/opt/nginx-proxy/certs",
		ProxyHTMLDir:    "/opt/nginx-proxy/html",
		SSLEmail:        "admin@example.com",
		ProxySSLCert:    "/etc/nginx/certs/live/wildcard.preview.example.com/fullchain.pem",
		ProxySSLKey:     "/etc/nginx/certs/live/wildcard.preview.example.com/privkey.pem",
		WildcardDomains: []string{"preview.example.com"},
		runner:          runner,
	}

	err := manager.ProvisionDomain(ProvisionConfig{
		ProjectID:     "01KNTESTPROJECT",
		ProjectName:   "acme-app-api",
		Domain:        "acme-app-api.preview.example.com",
		Environment:   "preview",
		ContainerName: "deployik-acme-app-api-preview",
	}, false, nil)
	if err != nil {
		t.Fatalf("ProvisionDomain returned error: %v", err)
	}

	for _, call := range runner.calls {
		if strings.Contains(strings.Join(call, " "), "certbot/certbot certonly") {
			t.Fatalf("did not expect certbot for a wildcard-covered domain, got %s", strings.Join(call, " "))
		}
	}
}

func TestProvisionDomainRunsCertbotForCustomDomain(t *testing.T) {
	t.Parallel()

	confDir := t.TempDir()
	runner := &fakeRunner{}
	manager := &Manager{
		NginxConfDir:    confDir,
		ProxyContainer:  "nginx-proxy",
		ProxyCertsDir:   "/opt/nginx-proxy/certs",
		ProxyHTMLDir:    "/opt/nginx-proxy/html",
		SSLEmail:        "admin@example.com",
		ProxySSLCert:    "/etc/nginx/certs/live/wildcard.preview.example.com/fullchain.pem",
		ProxySSLKey:     "/etc/nginx/certs/live/wildcard.preview.example.com/privkey.pem",
		WildcardDomains: []string{"preview.example.com"},
		runner:          runner,
	}

	err := manager.ProvisionDomain(ProvisionConfig{
		ProjectID:     "01KNTESTPROJECT",
		ProjectName:   "acme",
		Domain:        "acme.example.org",
		Environment:   "production",
		ContainerName: "deployik-acme-production",
	}, false, nil)
	if err != nil {
		t.Fatalf("ProvisionDomain returned error: %v", err)
	}

	ranCertbot := false
	for _, call := range runner.calls {
		if strings.Contains(strings.Join(call, " "), "certbot/certbot certonly") {
			ranCertbot = true
		}
	}
	if !ranCertbot {
		t.Fatal("expected certbot to run for a custom (non-wildcard) domain")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/domain/ -run 'TestProvisionDomainSkipsCertbot|TestProvisionDomainRunsCertbot' -v`
Expected: FAIL — `TestProvisionDomainSkipsCertbot...` currently passes only because `ProxySSLCert != ""` globally skips certbot, but the wildcard test would also pass today and the custom-domain test FAILS (the global skip means certbot never runs for `acme.example.org` either). Confirm `TestProvisionDomainRunsCertbotForCustomDomain` fails ("expected certbot to run").

- [ ] **Step 3: Replace the global certbot skip + apache cert defaulting in `ProvisionDomain`**

In `internal/domain/ssl.go`, the skip block (lines 129-141) currently reads:
```go
	// Skip certbot when a wildcard cert is configured — we'll reuse it for
	// every vhost and don't need a per-domain Let's Encrypt challenge.
	if m.ProxySSLCert == "" {
		sslDomains := cfg.requestSSLDomains()
		emit("ssl", "running", fmt.Sprintf("Requesting SSL certificate for %s...", strings.Join(sslDomains, ", ")))
		if err := m.RequestSSLCert(sslDomains...); err != nil {
			emit("ssl", "error", fmt.Sprintf("SSL certificate request failed: %v", err))
			return err
		}
		emit("ssl", "success", "SSL certificate issued successfully")
	} else {
		emit("ssl", "success", "Using configured wildcard certificate")
	}
```
Change to the per-domain matcher:
```go
	// Skip certbot only for domains the configured wildcard cert covers;
	// everything else still gets its own per-domain Let's Encrypt challenge.
	if !m.wildcardCovers(cfg.sslDomain()) {
		sslDomains := cfg.requestSSLDomains()
		emit("ssl", "running", fmt.Sprintf("Requesting SSL certificate for %s...", strings.Join(sslDomains, ", ")))
		if err := m.RequestSSLCert(sslDomains...); err != nil {
			emit("ssl", "error", fmt.Sprintf("SSL certificate request failed: %v", err))
			return err
		}
		emit("ssl", "success", "SSL certificate issued successfully")
	} else {
		emit("ssl", "success", fmt.Sprintf("Using wildcard certificate for %s", cfg.sslDomain()))
	}
```

In the same function, the apache branch (lines 149-156) currently reads:
```go
		certFile := m.ProxySSLCert
		if certFile == "" {
			certFile = fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", cfg.sslDomain())
		}
		keyFile := m.ProxySSLKey
		if keyFile == "" {
			keyFile = fmt.Sprintf("/etc/letsencrypt/live/%s/privkey.pem", cfg.sslDomain())
		}
```
Change to:
```go
		certFile, keyFile := m.certPathsFor(cfg.sslDomain())
```

- [ ] **Step 4: Replace the certbot gate + apache cert defaulting in `reconcile.go`**

In `internal/domain/reconcile.go`, the certbot gate (lines 36-42) currently reads:
```go
		// Skip certbot if we have a wildcard cert configured
		if manager.ProxySSLCert == "" && plan.RedirectDomain != "" {
			if err := manager.RequestSSLCert(plan.AllDomains()...); err != nil {
				errs = append(errs, fmt.Sprintf("ensure ssl for %s: %v", target.DomainName, err))
				continue
			}
		}
```
Change to:
```go
		// Skip certbot for wildcard-covered domains; otherwise keep the existing
		// behavior (request a multi-domain cert only when there's a www redirect).
		if !manager.wildcardCovers(plan.CanonicalDomain) && plan.RedirectDomain != "" {
			if err := manager.RequestSSLCert(plan.AllDomains()...); err != nil {
				errs = append(errs, fmt.Sprintf("ensure ssl for %s: %v", target.DomainName, err))
				continue
			}
		}
```

In the same file, the apache cert defaulting (lines 91-98) currently reads:
```go
			certFile := manager.ProxySSLCert
			if certFile == "" {
				certFile = fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", cfg.sslDomain())
			}
			keyFile := manager.ProxySSLKey
			if keyFile == "" {
				keyFile = fmt.Sprintf("/etc/letsencrypt/live/%s/privkey.pem", cfg.sslDomain())
			}
```
Change to:
```go
			certFile, keyFile := manager.certPathsFor(cfg.sslDomain())
```

- [ ] **Step 5: Run the new tests + the full domain suite**

Run: `go test ./internal/domain/ -v`
Expected: the two new `TestProvisionDomain*` tests PASS; the existing reconcile tests still PASS (`TestReconcileActiveConfigsSkipsWWWForPreviewSubdomain`, `TestReconcileActiveConfigsContinuesAfterEarlierCertificateFailure` — both run with no wildcard configured, so `wildcardCovers` is false and the gate behaves exactly as before).

- [ ] **Step 6: Build the whole repo + run all tests**

Run: `go build ./... && go test ./...`
Expected: build succeeds; all tests PASS. (`gofmt -l internal/domain internal/config cmd/server` should print nothing — run `gofmt -w` on any flagged file.)

- [ ] **Step 7: Commit**

```bash
git add internal/domain/ssl.go internal/domain/reconcile.go internal/domain/ssl_test.go
git commit -m "feat(ssl): per-domain certbot gate — skip only wildcard-covered hosts"
```

---

## Task 5: Ops runbook + `.env.example`

**Files:**
- Create: `docs/runbooks/wildcard-preview-cert.md`
- Modify: `.env.example`

- [ ] **Step 1: Write the runbook**

Create `docs/runbooks/wildcard-preview-cert.md`:
```markdown
# Runbook: wildcard cert for `*.preview.example.com`

Deployik serves this one cert to every single-label preview subdomain (skipping
certbot per deploy). Issuance/renewal is operated here, not by Deployik.

## Mount mapping (one host dir, two container views)
`/opt/nginx-proxy/certs` (host) is mounted into the **nginx** proxy container as
`/etc/nginx/certs` and into the **certbot** container as `/etc/letsencrypt`. A
cert certbot writes to `/etc/letsencrypt/live/<name>/` is read by nginx at
`/etc/nginx/certs/live/<name>/`. `PROXY_SSL_CERT`/`KEY` are the **nginx** paths.

## 1. Issue the wildcard (DNS-01, GoDaddy)
`example.com` DNS is GoDaddy (`pdns0{7,8}.domaincontrol.com`). DNS-01 is required
(HTTP-01 cannot validate a wildcard). Either use a GoDaddy certbot DNS plugin, or
`--manual`:

    docker run --rm -it \
      -v /opt/nginx-proxy/certs:/etc/letsencrypt \
      certbot/certbot certonly --manual --preferred-challenges dns \
      --agree-tos -m admin@example.com \
      --cert-name wildcard.preview.example.com \
      -d '*.preview.example.com'

Place the `_acme-challenge.preview.example.com` TXT record in GoDaddy when prompted.
Result: `/opt/nginx-proxy/certs/live/wildcard.preview.example.com/{fullchain,privkey}.pem`.

## 2. Configure Deployik (env)
    PROXY_SSL_CERT=/etc/nginx/certs/live/wildcard.preview.example.com/fullchain.pem
    PROXY_SSL_KEY=/etc/nginx/certs/live/wildcard.preview.example.com/privkey.pem
    PROXY_SSL_WILDCARD_DOMAINS=preview.example.com
Restart Deployik so the domain Manager picks them up.

## 3. Verify
Deploy any preview project (e.g. `acme-app-api`). The deploy log should show
"Using wildcard certificate for …" (no certbot run), and the generated vhost
(`/opt/nginx-proxy/conf.d/deployik-<domain>.conf`) should reference the wildcard
cert path. `curl -I https://<sub>.preview.example.com/` should return a valid TLS
response.

## 4. Renewal
`--manual` DNS-01 does not auto-renew. Either re-run step 1 before expiry (~60
days) or script a GoDaddy DNS-01 renewal hook, then `docker exec nginx-proxy
nginx -s reload`. A failed renewal degrades gracefully — the existing cert serves
until expiry.

## Rollback
Unset `PROXY_SSL_WILDCARD_DOMAINS` (or `PROXY_SSL_CERT`) and restart Deployik;
the next deploy/reconcile reverts to per-domain certbot.
```

- [ ] **Step 2: Document the env var in `.env.example`**

In `.env.example`, find the existing `PROXY_SSL_CERT` / `PROXY_SSL_KEY` block (around line 58-63) and add directly after it:
```
# PROXY_SSL_WILDCARD_DOMAINS — comma-separated base domains the PROXY_SSL_CERT
# wildcard covers (e.g. preview.example.com). A single-label subdomain of one of
# these is served the wildcard cert and skips per-domain certbot; everything else
# keeps the per-domain Let's Encrypt flow. Empty = wildcard matching off.
# PROXY_SSL_WILDCARD_DOMAINS=preview.example.com
```

- [ ] **Step 3: Commit**

```bash
git add docs/runbooks/wildcard-preview-cert.md .env.example
git commit -m "docs(ssl): wildcard preview-cert runbook + PROXY_SSL_WILDCARD_DOMAINS env"
```

---

## Self-Review

**Spec coverage:** config field + matcher (Task 1–2) · nginx `CertFile`/`KeyFile` wiring resolved inside `WriteNginxConfig` so all five callers are covered (Task 3) · per-domain certbot gate on deploy + reconcile + apache parity (Task 4) · GoDaddy DNS-01 runbook + mount mapping + env doc (Task 5). The protection-toggle and domain-verify/move callers are covered transitively (they call `WriteNginxConfig` / `ProvisionDomain`) — no bespoke edits, matching the spec's §2(d).

**Placeholder scan:** every code step shows the exact old→new code; every run step shows the command + expected result. The runbook's GoDaddy automation is intentionally left as "plugin or `--manual`" (an ops choice), not a code TODO.

**Type consistency:** `WildcardDomains []string` (Manager + ManagerConfig), `ProxySSLWildcardDomains []string` (Config), `wildcardCovers(host string) bool`, `certPathsFor(host string) (certFile, keyFile string)`, `hostUnderWildcard(host, base string) bool`, and the `NginxConfig.CertFile/KeyFile` fields are used consistently across Tasks 1–4. `certPathsFor` is always called with `cfg.sslDomain()` (deploy/reconcile) or `plan.CanonicalDomain` (reconcile gate), matching the vhost's cert key.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-18-wildcard-cert-consumption.md`. Two execution options:

1. **Subagent-Driven (recommended)** — a fresh subagent per task, reviewed between tasks.
2. **Inline Execution** — execute tasks in this session with checkpoints.

Which approach?
