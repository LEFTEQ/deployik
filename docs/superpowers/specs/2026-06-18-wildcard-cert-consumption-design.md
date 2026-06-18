# Wildcard-cert consumption for preview subdomains — Design

> **Status:** approved design, pre-implementation.
> **Scope:** Deployik (`internal/domain`, `internal/config`) + an ops runbook. Minimal "consume-only" wildcard support — Deployik serves a pre-issued `*.preview.example.com` wildcard cert to every matching subdomain instead of running certbot per subdomain. Wildcard **issuance/renewal stays outside Deployik** (ops).

## Problem

Deployik provisions a **separate Let's Encrypt certificate for every domain** via a per-domain certbot run (`internal/domain/ssl.go:220` `RequestSSLCert` → `docker run … certbot/certbot certonly --webroot … -d <domain>`). So **every** new preview subdomain — including API services like `acme-app-api.preview.example.com` — depends on a fresh certbot → Let's Encrypt handshake at deploy time.

That handshake is currently failing on the VPS:

```
certbot → acme-v02.api.letsencrypt.org:443
[SSL: UNEXPECTED_EOF_WHILE_READING] EOF occurred in violation of protocol
```

Let's Encrypt is reachable globally; the failure is VPS→LE connectivity inside the certbot container. Because each deploy needs its own certbot run, every subdomain deploy is hostage to that connectivity. The error surfaces as `Domain provisioning failed: … certbot failed …` (the `Provisioning domain …` prefix is `internal/build/pipeline.go:576`; the wrapped certbot output is `ssl.go:257`).

## Goal

A **single wildcard cert `*.preview.example.com`**, issued once and reused by every preview subdomain, so certbot runs ~once per renewal (~60 days) instead of once per deploy. A subdomain deploy then needs **zero** certbot calls — it just writes a vhost that points at the existing wildcard cert and reloads the proxy.

## Non-goals

- **No wildcard issuance/renewal in Deployik.** The wildcard is issued + renewed by an ops runbook (one-time certbot DNS-01 + cron). No ACME library, no DNS-provider integration added to Deployik. (A future phase could automate this; out of scope here.)
- **No change to custom-domain handling.** Domains not covered by a configured wildcard (e.g. `*.example.org`, customer apex domains) keep the existing per-domain certbot flow, byte-for-byte.
- **No new SSL lifecycle states.** A wildcard-served domain still ends in `ssl_status = 'active'`; the 3-value enum (`internal/db/migrations/001_initial.sql:69`) is unchanged.

## Key decision — suffix-scoped, not global

Deployik already has a `PROXY_SSL_CERT` / `PROXY_SSL_KEY` hook: when `ProxySSLCert != ""`, `ProvisionDomain` skips certbot entirely (`ssl.go:129-141`) and reuses that cert. **But it is global and all-or-nothing** — every domain gets that one cert. In a mixed environment (preview subdomains **and** custom domains in the same Deployik), a global wildcard would serve the `*.preview.example.com` cert for `*.example.org` too → invalid cert → broken custom domains.

So cert selection becomes **per-domain by suffix**:

- Domain is a **single-label** subdomain of a configured wildcard base (`<label>.preview.example.com`, no extra dots) **and** a wildcard cert is configured → **serve the wildcard cert, skip certbot.**
- Otherwise → **per-domain certbot**, exactly as today.

The single-label constraint matters: `*.preview.example.com` (and the matching wildcard DNS A record) covers exactly one label. `a.b.preview.example.com` is **not** covered and falls through to certbot. Preview subdomains are always single-label and have **no `www` redirect variant** (the `www` redirect in `internal/domain/variants.go:79-86` only applies to custom apex domains), so the wildcard path is clean for them; custom domains with their `www` variant stay on certbot.

## Architecture / change set

Three changes; only the first two are code.

### 1. Config — declare what the wildcard covers

`internal/config/config.go` (alongside `ProxySSLCert`/`ProxySSLKey` at `config.go:76-77`): add `PROXY_SSL_WILDCARD_DOMAINS` → `Config.ProxySSLWildcardDomains []string`, parsed with the **existing `splitCSV` helper** (`config.go:123-134`). Comma-separated bases the configured wildcard cert covers, e.g. `preview.example.com`. Empty (default) → the matcher never matches → fully current behavior.

Thread it onto the domain layer: add `WildcardDomains []string` to both `ManagerConfig` and `Manager` (`ssl.go:14-43`), populated in `NewManager` (`ssl.go:80-96`) and wired from config at `cmd/server/main.go:110-123`.

`PROXY_SSL_CERT` / `PROXY_SSL_KEY` keep their meaning (paths to the wildcard fullchain/privkey) but are now consulted **per-domain via the matcher**, not as a global switch.

### 2. Cert selection + nginx wiring

**(a) A single cert-resolution helper** on `Manager` (new, in `internal/domain/ssl.go`):

```go
// wildcardCovers reports whether `host` is served by the configured wildcard
// cert (and therefore skips certbot). certPathsFor returns the cert/key paths to
// write into the vhost: the wildcard pair when covered, else the per-domain
// certbot paths (current behavior).
func (m *Manager) wildcardCovers(host string) bool
func (m *Manager) certPathsFor(host string) (certFile, keyFile string)
```

- `wildcardCovers(host)` = `ProxySSLCert != ""` AND `host` is a single-label subdomain of some base in `WildcardDomains`.
- `certPathsFor(host)` returns `(ProxySSLCert, ProxySSLKey)` when `wildcardCovers(host)`, else the per-domain live paths (`/etc/nginx/certs/live/<sslDomain>/fullchain.pem` + `privkey.pem`) — the exact strings currently hardcoded in the nginx template and defaulted in the Apache branch.

- `wildcardCovers(host)` is called with the **canonical/SSL domain** (`cfg.sslDomain()`, `ssl.go:317-322`), not the raw target — matching whatever name the vhost+cert key off.

**(b) Replace the global certbot skip with the per-domain matcher.** In `ProvisionDomain` (`ssl.go:129-141`) and the startup reconcile path (`reconcile.go:36-42`), gate certbot on `!m.wildcardCovers(cfg.sslDomain())` instead of `m.ProxySSLCert == ""`. A wildcard-covered domain **always** skips `RequestSSLCert`, independent of any redirect condition — note `reconcile.go:37` currently also gates on `plan.RedirectDomain != ""`; the matcher supersedes that for wildcard domains (and the deploy path `ssl.go:131` has no such gate, so the two paths converge on the matcher). Emit `"Using wildcard certificate for <host>"`.

**(c) Close the nginx gap — resolve cert paths *inside* `WriteNginxConfig`.** Today the nginx template hardcodes per-domain paths and `NginxConfig` has no cert fields (`nginx.go:50-51,80-81`; struct `nginx.go:204-238`), so the wildcard hook is honored only by Apache (`apache.go:24-25,35-36`; struct `apache.go:62-67`). Fix:
   - Add `CertFile` / `KeyFile` to `NginxConfig`; render the `ssl_certificate` / `ssl_certificate_key` lines in **both** the canonical and the redirect server blocks off `{{.CertFile}}` / `{{.KeyFile}}`.
   - **Centralize resolution in `WriteNginxConfig` (`ssl.go:264-289`):** before calling `GenerateNginxConfig`, compute `certFile, keyFile := m.certPathsFor(cfg.sslDomain())` and set them on `NginxConfig`, defaulting to the per-domain path so **non-wildcard domains produce byte-identical config to today**. Because resolution lives in `WriteNginxConfig` (not at call sites), **every** caller is covered automatically — see (d).
   - Mirror the same `certPathsFor` into the Apache call site (`ssl.go:149-156`) and the reconcile Apache defaulting (`reconcile.go:91-98`) so both proxy formats agree.

**(d) Caller coverage (from the verification pass).** Five paths generate proxy config / provision domains: the deploy pipeline (`ProvisionDomain`, `pipeline.go:579`), startup reconcile (`reconcile.go`), the domain-verify handler (`internal/api/handlers/domains.go:432`), the domain-move handler (`domains.go:274`), and the **password-protection toggle** (`internal/api/handlers/protection.go:474` → `WriteNginxConfig`). Putting the certbot decision in `ProvisionDomain`/`reconcile` (the matcher) and the cert-path resolution **inside `WriteNginxConfig`** covers all five with no bespoke per-handler edits — critically the protection toggle, which today calls `WriteNginxConfig` with no cert info and would otherwise regenerate a wildcard domain's vhost pointing at a non-existent per-domain cert. *Adjacent, pre-existing:* the move handler omits `SSLDomains` (unlike verify); harmless for wildcard domains (certbot is skipped) but worth aligning while in the file.

No DB/schema change. The wildcard-covered domain still transitions `pending → active` via `UpdateDomainSSL` (`internal/db/queries_domains.go:84`).

### 3. Ops runbook (documented in the spec/plan, not Deployik code)

One-time on the VPS (`203.0.113.10`), since `example.com` DNS is **GoDaddy** (`pdns0{7,8}.domaincontrol.com`):

**Mount mapping (one host dir, two container views).** `/opt/nginx-proxy/certs` on the host is mounted into the **nginx** proxy container as `/etc/nginx/certs` and into the **certbot** container as `/etc/letsencrypt`. So a cert certbot issues to `/etc/letsencrypt/live/<name>/` (= host `/opt/nginx-proxy/certs/live/<name>/`) is read by nginx at `/etc/nginx/certs/live/<name>/` — exactly how the existing per-domain certs already work. `PROXY_SSL_CERT`/`KEY` are the **nginx-container** paths.

1. Issue `*.preview.example.com` via certbot **DNS-01** (GoDaddy DNS plugin, or `--manual` DNS-01 placing a `_acme-challenge.preview.example.com` TXT record), writing under `/opt/nginx-proxy/certs/live/<wildcard-name>/`.
2. Set on the Deployik service: `PROXY_SSL_CERT=/etc/nginx/certs/live/<wildcard-name>/fullchain.pem`, `PROXY_SSL_KEY=/etc/nginx/certs/live/<wildcard-name>/privkey.pem`, `PROXY_SSL_WILDCARD_DOMAINS=preview.example.com`.
3. Add a `certbot renew` cron (DNS-01) + nginx reload on renewal.
4. Restart Deployik to pick up the env; trigger one preview deploy to confirm the wildcard path (no certbot run; the vhost points at the wildcard cert).

## Testing

All offline (no live LE), following the existing `internal/domain` test patterns (`t.TempDir`, `strings.Contains` on generated config, the `fakeRunner` at `ssl_test.go:10-18`):

- **Unit — matcher.** `wildcardCovers` / `certPathsFor`: `acme-app-api.preview.example.com` → wildcard; `preview.example.com` (apex), `a.b.preview.example.com` (two labels), `acme-app.preview.example.org` (custom) → per-domain. With `ProxySSLCert == ""` or empty `WildcardDomains` → always per-domain (regression guard).
- **nginx config — cert path.** Like `nginx_test.go` / `apache_test.go` (`strings.Contains` on the generated vhost): a wildcard domain's vhost contains the `PROXY_SSL_CERT` path; a non-wildcard domain's contains `/etc/nginx/certs/live/<domain>/fullchain.pem` — **unchanged from today**.
- **certbot skip (behavioral).** Reuse `fakeRunner` + the `reconcile_test.go:13-60` call-count assertion: `ProvisionDomain`/reconcile invoke `docker … certbot` **zero** times for a wildcard-covered domain and once for a custom domain.
- **Reconcile parity.** Same cert path + same certbot decision on the startup `reconcile.go` path as on the deploy path for the same domain.
- Optional `newTestManager(cert, key string, wildcards []string)` helper to cut the inline `Manager` construction boilerplate (e.g. `ssl_test.go:24-29`).

## Rollout / rollback

- **Inert by default:** with `PROXY_SSL_WILDCARD_DOMAINS` empty (and/or `PROXY_SSL_CERT` empty) the matcher never matches → every domain takes the existing certbot path → zero behavior change. Safe to ship the code before the wildcard cert exists.
- **Activate** by issuing the wildcard + setting the three env vars. **Rollback** = unset `PROXY_SSL_WILDCARD_DOMAINS` (or `PROXY_SSL_CERT`) and redeploy/reconcile → back to per-domain certbot.

## Risks

- **Single-label matching bug** would either miss preview subdomains (fall back to certbot — safe) or wrongly match a custom domain (serve the wrong cert — visible TLS error). Covered by the matcher unit tests, including the two-label and custom-domain negatives.
- **Mount/path mismatch** between the certbot output dir, the nginx container cert mount, and the configured `PROXY_SSL_CERT` path. The runbook pins all three to `/opt/nginx-proxy/certs` ↔ `/etc/nginx/certs`.
- **Wildcard renewal is now a single point of failure** for all preview subdomains (vs. independent per-domain certs). Mitigated by the renewal cron + monitoring; and a failed renewal degrades gracefully (the old cert serves until expiry).
