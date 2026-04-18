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
    server_name {{.RedirectDomain}};

    ssl_certificate /etc/nginx/certs/live/{{.SSLDomain}}/fullchain.pem;
    ssl_certificate_key /etc/nginx/certs/live/{{.SSLDomain}}/privkey.pem;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 1d;

    add_header Strict-Transport-Security "max-age=31536000" always;
    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;

    location / {
        return 301 https://{{.Domain}}$request_uri;
    }
}
{{- end }}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name {{.Domain}};

    ssl_certificate /etc/nginx/certs/live/{{.SSLDomain}}/fullchain.pem;
    ssl_certificate_key /etc/nginx/certs/live/{{.SSLDomain}}/privkey.pem;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 1d;

    add_header Strict-Transport-Security "max-age=31536000" always;
    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;
    access_log /var/log/nginx/deployik-{{.ProjectID}}-{{.ProjectName}}-{{.Environment}}.json deployik_json;
{{- if .PasswordProtected }}

    set $deployik_project_id "{{.ProjectID}}";
    set $deployik_environment "{{.Environment}}";

    location = /_deployik/auth-check {
        internal;
        proxy_pass http://deployik:8080/api/site-auth/check;
        proxy_set_header X-Deployik-Project $deployik_project_id;
        proxy_set_header X-Deployik-Environment $deployik_environment;
        proxy_set_header Cookie $http_cookie;
        proxy_pass_request_body off;
        proxy_set_header Content-Length "";
    }

    error_page 401 = @auth_page;

    location @auth_page {
        root /var/www/html;
        try_files /auth.html =503;
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

    location / {
        set $upstream {{.ContainerUpstream}};
        limit_req zone=deployik_preview burst=20 nodelay;
{{- if .PasswordProtected }}
        auth_request /_deployik/auth-check;
{{- end }}
        proxy_pass http://$upstream;

        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
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
}

// GenerateNginxConfig creates an nginx config file for a domain.
func GenerateNginxConfig(confDir string, cfg NginxConfig) (string, error) {
	if cfg.ContainerUpstream == "" {
		cfg.ContainerUpstream = cfg.ContainerName + ":3000"
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
