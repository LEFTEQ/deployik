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
# Domain: {{.Domain}}

server {
    listen 80;
    listen [::]:80;
    server_name {{.Domain}};

    location /.well-known/acme-challenge/ {
        root /var/www/html;
    }

    location /health {
        access_log off;
        return 200 "ok\n";
    }

    location / {
        return 301 https://$host$request_uri;
    }
}

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

    location / {
        set $upstream {{.ContainerName}}:3000;
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
	ProjectName   string
	Domain        string
	SSLDomain     string // may differ for wildcard certs
	ContainerName string
}

// GenerateNginxConfig creates an nginx config file for a domain.
func GenerateNginxConfig(confDir string, cfg NginxConfig) (string, error) {
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
