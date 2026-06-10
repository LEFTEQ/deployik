package domain

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const nginxProjectTemplate = `# Managed by Deployik - do not edit manually
# Project: {{.ProjectName}}
# Project ID: {{.ProjectID}}
# Domain: {{.Domain}}
{{- if .RedirectDomain }}
# Redirect: {{.RedirectDomain}} -> {{.Domain}}
{{- end }}

server {
    listen 80;
    listen [::]:80;
    server_name {{.Domain}}{{if .RedirectDomain}} {{.RedirectDomain}}{{end}};
    access_log off;

    location /.well-known/acme-challenge/ {
        root /var/www/html;
    }

    location /health {
        access_log off;
        return 200 "ok\n";
    }

    location / {
        return 301 https://{{.Domain}}$request_uri;
    }
}

{{- if .RedirectDomain }}
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
{{- if .HTTP3 }}
    listen 443 quic;
    listen [::]:443 quic;
{{- end }}
    server_name {{.RedirectDomain}};

    ssl_certificate /etc/nginx/certs/live/{{.SSLDomain}}/fullchain.pem;
    ssl_certificate_key /etc/nginx/certs/live/{{.SSLDomain}}/privkey.pem;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 1d;

    add_header Strict-Transport-Security "max-age=31536000" always;
    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;
{{- if .HTTP3 }}
    add_header Alt-Svc 'h3=":443"; ma=86400' always;
{{- end }}

    location / {
        return 301 https://{{.Domain}}$request_uri;
    }
}
{{- end }}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
{{- if .HTTP3 }}
    listen 443 quic;
    listen [::]:443 quic;
{{- end }}
    server_name {{.Domain}};

    ssl_certificate /etc/nginx/certs/live/{{.SSLDomain}}/fullchain.pem;
    ssl_certificate_key /etc/nginx/certs/live/{{.SSLDomain}}/privkey.pem;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 1d;

    add_header Strict-Transport-Security "max-age=31536000" always;
    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;
{{- if .HTTP3 }}
    add_header Alt-Svc 'h3=":443"; ma=86400' always;
{{- end }}
    access_log /var/log/nginx/deployik-{{.ProjectID}}-{{.ProjectName}}-{{.Environment}}.json deployik_json;
{{- if .PasswordProtected }}

    set $deployik_project_id "{{.ProjectID}}";
    set $deployik_environment "{{.Environment}}";

    location = /_deployik/auth-check {
        internal;
        proxy_pass http://deployik:8080/api/site-auth/check;
        proxy_set_header X-Deployik-Project $deployik_project_id;
        proxy_set_header X-Deployik-Environment $deployik_environment;
        proxy_set_header X-Original-URI $request_uri;
        proxy_set_header Cookie $http_cookie;
        proxy_pass_request_body off;
        proxy_set_header Content-Length "";
    }

    # The URI form (no "=") preserves the 401 status while serving the auth
    # page body. Returning it with 200 lets PWA service workers precache the
    # password screen as the app shell, locking visitors out even after they
    # log in. no-store keeps it out of HTTP caches for the same reason.
    error_page 401 /_deployik/auth.html;

    location = /_deployik/auth.html {
        internal;
        root /var/www/html;
        try_files /auth.html =503;

        # add_header in a location suppresses inherited server-level headers,
        # so the security headers are re-declared alongside no-store.
        add_header Cache-Control "no-store" always;
        add_header Strict-Transport-Security "max-age=31536000" always;
        add_header X-Frame-Options "DENY" always;
        add_header X-Content-Type-Options "nosniff" always;
    }

    location = /_deployik/verify {
        proxy_pass http://deployik:8080/api/site-auth/verify;
        proxy_set_header X-Deployik-Project $deployik_project_id;
        proxy_set_header X-Deployik-Environment $deployik_environment;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
{{- end }}

    # Per-IP concurrent connection cap (defense against slowloris and
    # connection-exhaustion regardless of request rate).
    limit_conn deployik_perip {{.MaxConnPerIP}};

    # Static assets (Next.js chunks, images, fonts, etc.) — generous rate limit.
    # Framework prefetches (Next.js RSC, Nuxt islands, etc.) fan out dozens of
    # these per page load, so we use a much higher rate than dynamic requests,
    # but still cap to prevent bandwidth-DoS by a single IP. Password protection
    # doesn't apply here — static chunks aren't sensitive, and auth would cause
    # partial paints.
    location ~* (^/_next/static/|^/favicon|\.(?:js|css|map|png|jpe?g|gif|webp|avif|svg|ico|woff2?|ttf|otf|eot)$) {
        set $upstream {{.ContainerUpstream}};
        limit_req zone=deployik_static burst={{.StaticBurst}} nodelay;
        limit_req_status 429;
        proxy_pass http://$upstream;

        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        # Ask the upstream for identity encoding so the proxy owns compression.
        # App servers (e.g. Next.js) gzip themselves otherwise, which would
        # bypass nginx brotli and lock all clients to gzip.
        proxy_set_header Accept-Encoding "";

        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }

    location / {
        set $upstream {{.ContainerUpstream}};
        limit_req zone=deployik_dynamic burst={{.RateLimitBurst}} nodelay;
        limit_req_status 429;
{{- if .PasswordProtected }}
        auth_request /_deployik/auth-check;
{{- end }}
        proxy_pass http://$upstream;

        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        # See the static location above — proxy owns compression (brotli/gzip).
        proxy_set_header Accept-Encoding "";
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";

        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
}
`

// NginxConfig holds data for generating an nginx config.
type NginxConfig struct {
	ProjectID         string
	ProjectName       string
	Domain            string
	RedirectDomain    string
	Environment       string
	SSLDomain         string // may differ for wildcard certs
	ContainerName     string
	ContainerUpstream string // "host:port" — preferred; falls back to ContainerName:3000
	PasswordProtected bool
	// HTTP3 adds QUIC listeners + the Alt-Svc discovery header. Requires the
	// proxy's nginx to be built with http_v3_module, UDP 443 reachable, and a
	// `reuseport` quic listener configured once elsewhere (on the Lovinka VPS:
	// 00-default-https.conf in infra-repo) — this template never emits
	// reuseport itself, nginx allows it only once per address:port.
	HTTP3             bool
	RateLimitBurst    int // burst for dynamic requests; defaults via defaultRateLimitBurst
	StaticBurst       int // burst for static-asset requests; defaults via defaultStaticBurst
	MaxConnPerIP      int // concurrent connections per IP cap; defaults to 50
}

// defaultRateLimitBurst returns the per-vhost limit_req burst for dynamic
// requests, suited to the environment. Production sites get a larger burst
// because real users (esp. with framework prefetches like Next.js App Router
// RSC) easily fan out dozens of dynamic requests from a single page load.
func defaultRateLimitBurst(environment string) int {
	if strings.EqualFold(environment, "production") {
		return 100
	}
	return 20
}

// defaultStaticBurst returns the per-vhost limit_req burst for static assets.
// Higher than dynamic because a single page load on a Next.js / Nuxt-style
// app fetches many JS chunks, fonts, and images in parallel.
func defaultStaticBurst(environment string) int {
	if strings.EqualFold(environment, "production") {
		return 200
	}
	return 50
}

// GenerateNginxConfig creates an nginx config file for a domain.
func GenerateNginxConfig(confDir string, cfg NginxConfig) (string, error) {
	if cfg.ContainerUpstream == "" {
		cfg.ContainerUpstream = cfg.ContainerName + ":3000"
	}
	if cfg.RateLimitBurst <= 0 {
		cfg.RateLimitBurst = defaultRateLimitBurst(cfg.Environment)
	}
	if cfg.StaticBurst <= 0 {
		cfg.StaticBurst = defaultStaticBurst(cfg.Environment)
	}
	if cfg.MaxConnPerIP <= 0 {
		cfg.MaxConnPerIP = 50
	}

	tmpl, err := template.New("nginx").Parse(nginxProjectTemplate)
	if err != nil {
		return "", fmt.Errorf("parse nginx template: %w", err)
	}

	// Sanitize filename
	filename := fmt.Sprintf("deployik-%s.conf", sanitizeFilename(cfg.Domain))
	confPath := filepath.Join(confDir, filename)

	var buf strings.Builder
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return "", fmt.Errorf("execute nginx template: %w", err)
	}

	if err := os.WriteFile(confPath, []byte(buf.String()), 0644); err != nil {
		return "", fmt.Errorf("write nginx config: %w", err)
	}

	return confPath, nil
}

// RemoveNginxConfig removes an nginx config file for a domain.
func RemoveNginxConfig(confDir, domain string) error {
	filename := fmt.Sprintf("deployik-%s.conf", sanitizeFilename(domain))
	confPath := filepath.Join(confDir, filename)
	return os.Remove(confPath)
}

func sanitizeFilename(s string) string {
	return strings.NewReplacer(".", "-", "/", "-", ":", "-").Replace(s)
}
