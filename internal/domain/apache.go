package domain

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const apacheProjectTemplate = `# Managed by Deployik - do not edit manually
# Project: {{.ProjectName}} | Domain: {{.Domain}}

<VirtualHost *:80>
    ServerName {{.Domain}}{{if .RedirectDomain}} {{.RedirectDomain}}{{end}}
    RewriteEngine On
    RewriteRule ^ https://{{.Domain}}%{REQUEST_URI} [R=301,L]
</VirtualHost>
{{- if .RedirectDomain}}

<VirtualHost *:443>
    ServerName {{.RedirectDomain}}
    SSLEngine on
    SSLCertificateFile {{.CertFile}}
    SSLCertificateKeyFile {{.KeyFile}}
    Include /etc/letsencrypt/options-ssl-apache.conf
    Redirect permanent / https://{{.Domain}}/
</VirtualHost>
{{- end}}

<VirtualHost *:443>
    ServerName {{.Domain}}

    SSLEngine on
    SSLCertificateFile {{.CertFile}}
    SSLCertificateKeyFile {{.KeyFile}}
    Include /etc/letsencrypt/options-ssl-apache.conf

    Protocols h2 http/1.1

    ProxyPreserveHost On
    ProxyPass / http://{{.ContainerUpstream}}/
    ProxyPassReverse / http://{{.ContainerUpstream}}/

    RewriteEngine On
    RewriteCond %{HTTP:Upgrade} websocket [NC]
    RewriteCond %{HTTP:Connection} upgrade [NC]
    RewriteRule /(.*) ws://{{.ContainerUpstream}}/$1 [P,L]

    RequestHeader set X-Forwarded-Proto "https"
    RequestHeader set X-Forwarded-Host "{{.Domain}}"

    CustomLog "/var/log/apache2/deployik-{{.ProjectID}}-{{.ProjectName}}-{{.Environment}}.log" combined
    ErrorLog "/var/log/apache2/deployik-{{.ProjectID}}-{{.ProjectName}}-{{.Environment}}_error.log"
</VirtualHost>
`

// ApacheConfig holds data for generating an Apache VirtualHost config.
type ApacheConfig struct {
	NginxConfig
	CertFile string
	KeyFile  string
}

// GenerateApacheConfig writes an Apache VirtualHost config file for a domain.
func GenerateApacheConfig(confDir string, cfg ApacheConfig) (string, error) {
	if cfg.ContainerUpstream == "" {
		cfg.ContainerUpstream = cfg.ContainerName + ":3000"
	}

	tmpl, err := template.New("apache").Parse(apacheProjectTemplate)
	if err != nil {
		return "", fmt.Errorf("parse apache template: %w", err)
	}

	filename := fmt.Sprintf("deployik-%s.conf", sanitizeFilename(cfg.Domain))
	confPath := filepath.Join(confDir, filename)

	var buf strings.Builder
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return "", fmt.Errorf("execute apache template: %w", err)
	}

	if err := os.WriteFile(confPath, []byte(buf.String()), 0644); err != nil {
		return "", fmt.Errorf("write apache config: %w", err)
	}

	return confPath, nil
}
