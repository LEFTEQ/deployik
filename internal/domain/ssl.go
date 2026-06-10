package domain

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

var ErrDNSNotVerified = errors.New("dns record does not point to the configured VPS host")

type ManagerConfig struct {
	NginxConfDir      string
	ProxyContainer    string
	ProxyCertsDir     string
	ProxyHTMLDir      string
	VPSHost           string
	SSLEmail          string
	ProxyType         string // "docker" | "host-port"
	ProxyConfigFormat string // "nginx" | "apache"
	ProxyReloadCmd    string
	ProxySSLCert      string
	ProxySSLKey       string
	HTTP3             bool // nginx format only — see NginxConfig.HTTP3
}

type Manager struct {
	NginxConfDir      string
	ProxyContainer    string
	ProxyCertsDir     string
	ProxyHTMLDir      string
	VPSHost           string
	SSLEmail          string
	ProxyType         string
	ProxyConfigFormat string
	ProxyReloadCmd    string
	ProxySSLCert      string
	ProxySSLKey       string
	HTTP3             bool
	runner            commandRunner
}

type ProvisionConfig struct {
	ProjectID         string
	ProjectName       string
	Domain            string
	Environment       string
	SSLDomain         string
	SSLDomains        []string
	RedirectDomain    string
	ContainerName     string
	// ContainerUpstream is "host:port" — preferred. When set, it's written
	// verbatim. The deploy + reconcile paths build it explicitly because they
	// may point at 127.0.0.1:<random> in host-port mode.
	ContainerUpstream string
	// Port is the TCP port the container listens on. Used to build the
	// upstream when ContainerUpstream is empty (e.g. protection toggle, domain
	// verify — paths where the caller doesn't know about host-port mode).
	// Zero defaults to 3000.
	Port              int
	PasswordProtected bool
}

// ProvisionLogger emits structured log events during domain provisioning.
// step: "dns"|"ssl"|"nginx", status: "running"|"success"|"error".
type ProvisionLogger func(step, status, content string)

type commandRunner interface {
	CombinedOutput(name string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func NewManager(cfg ManagerConfig) *Manager {
	return &Manager{
		NginxConfDir:      cfg.NginxConfDir,
		ProxyContainer:    cfg.ProxyContainer,
		ProxyCertsDir:     cfg.ProxyCertsDir,
		ProxyHTMLDir:      cfg.ProxyHTMLDir,
		VPSHost:           cfg.VPSHost,
		SSLEmail:          cfg.SSLEmail,
		ProxyType:         cfg.ProxyType,
		ProxyConfigFormat: cfg.ProxyConfigFormat,
		ProxyReloadCmd:    cfg.ProxyReloadCmd,
		ProxySSLCert:      cfg.ProxySSLCert,
		ProxySSLKey:       cfg.ProxySSLKey,
		HTTP3:             cfg.HTTP3,
		runner:            execRunner{},
	}
}

func (m *Manager) VerifyDomainDNS(domainName string) (bool, error) {
	if m.VPSHost == "" {
		return true, nil
	}

	return VerifyDNS(domainName, m.VPSHost)
}

func (m *Manager) ProvisionDomain(cfg ProvisionConfig, requireDNS bool, logger ProvisionLogger) error {
	emit := func(step, status, content string) {
		if logger != nil {
			logger(step, status, content)
		}
	}

	if requireDNS {
		for _, domainName := range cfg.domainsToVerify() {
			emit("dns", "running", fmt.Sprintf("Checking DNS for %s...", domainName))
			verified, err := m.VerifyDomainDNS(domainName)
			if err != nil {
				emit("dns", "error", fmt.Sprintf("DNS lookup failed for %s: %v", domainName, err))
				return err
			}
			if !verified {
				emit("dns", "error", fmt.Sprintf("%s does not point to %s", domainName, m.VPSHost))
				return ErrDNSNotVerified
			}
			emit("dns", "success", fmt.Sprintf("%s → %s", domainName, m.VPSHost))
		}
	}

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

	// Write the proxy config in whichever format the operator chose. The
	// per-format branch keeps the hot path simple — nginx mode writes a
	// server block, apache mode writes a VirtualHost with the same data.
	proxyStep := "nginx"
	if m.ProxyConfigFormat == "apache" {
		proxyStep = "apache"
		certFile := m.ProxySSLCert
		if certFile == "" {
			certFile = fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", cfg.sslDomain())
		}
		keyFile := m.ProxySSLKey
		if keyFile == "" {
			keyFile = fmt.Sprintf("/etc/letsencrypt/live/%s/privkey.pem", cfg.sslDomain())
		}
		emit(proxyStep, "running", fmt.Sprintf("Writing Apache vhost for %s...", cfg.Domain))
		if _, err := GenerateApacheConfig(m.NginxConfDir, ApacheConfig{
			NginxConfig: NginxConfig{
				ProjectID:         cfg.ProjectID,
				ProjectName:       cfg.ProjectName,
				Domain:            cfg.Domain,
				RedirectDomain:    cfg.RedirectDomain,
				Environment:       cfg.Environment,
				SSLDomain:         cfg.sslDomain(),
				ContainerName:     cfg.ContainerName,
				ContainerUpstream: cfg.ContainerUpstream,
				PasswordProtected: cfg.PasswordProtected,
			},
			CertFile: certFile,
			KeyFile:  keyFile,
		}); err != nil {
			emit(proxyStep, "error", fmt.Sprintf("Apache vhost write failed: %v", err))
			return err
		}
	} else {
		emit(proxyStep, "running", fmt.Sprintf("Writing nginx config for %s...", cfg.Domain))
		if _, err := m.WriteNginxConfig(cfg); err != nil {
			emit(proxyStep, "error", fmt.Sprintf("Nginx config write failed: %v", err))
			return err
		}
	}

	emit(proxyStep, "running", "Testing and reloading proxy...")
	if err := m.ReloadProxy(); err != nil {
		emit(proxyStep, "error", fmt.Sprintf("Proxy reload failed: %v", err))
		return err
	}
	emit(proxyStep, "success", "Proxy reloaded successfully")

	return nil
}

// ReloadProxy reloads the proxy using the configured method.
//
// In host-port mode, PROXY_RELOAD_CMD is passed to `sh -c`, so the operator
// can use the full shell vocabulary (quoted args, pipes, &&, env vars, etc.).
// Examples that all work:
//
//	apachectl graceful
//	sudo -n systemctl reload nginx
//	nsenter -t 1 -m -- apachectl graceful
//	bash -c 'apachectl configtest && apachectl graceful'
func (m *Manager) ReloadProxy() error {
	if m.ProxyType == "host-port" {
		cmd := strings.TrimSpace(m.ProxyReloadCmd)
		if cmd == "" {
			return fmt.Errorf("PROXY_RELOAD_CMD not configured for host-port proxy mode")
		}
		output, err := m.runner.CombinedOutput("sh", "-c", cmd)
		if err != nil {
			return fmt.Errorf("proxy reload failed: %w\nOutput: %s", err, string(output))
		}
		log.Printf("Proxy reloaded via: %s", cmd)
		return nil
	}
	return m.ReloadNginx()
}

func (m *Manager) RequestSSLCert(domainNames ...string) error {
	if m.ProxyCertsDir == "" {
		return fmt.Errorf("proxy certs directory is not configured")
	}
	if m.ProxyHTMLDir == "" {
		return fmt.Errorf("proxy html directory is not configured")
	}
	if m.SSLEmail == "" {
		return fmt.Errorf("ssl email is not configured")
	}
	if len(domainNames) == 0 {
		return fmt.Errorf("at least one domain is required for SSL provisioning")
	}

	cmd := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/etc/letsencrypt", m.ProxyCertsDir),
		"-v", fmt.Sprintf("%s:/var/www/html", m.ProxyHTMLDir),
		"certbot/certbot", "certonly",
		"--webroot", "-w", "/var/www/html",
		"--email", m.SSLEmail,
		"--agree-tos",
		"--no-eff-email",
		"--non-interactive",
		"--expand",
		"--keep-until-expiring",
	}
	cmd = append(cmd, "--cert-name", domainNames[0])
	for _, domainName := range domainNames {
		if domainName == "" {
			continue
		}
		cmd = append(cmd, "-d", domainName)
	}

	output, err := m.runner.CombinedOutput("docker", cmd...)
	if err != nil {
		return fmt.Errorf("certbot failed for %s: %w\nOutput: %s", strings.Join(domainNames, ", "), err, string(output))
	}

	log.Printf("SSL cert ensured for %s", strings.Join(domainNames, ", "))
	return nil
}

func (m *Manager) WriteNginxConfig(cfg ProvisionConfig) (string, error) {
	if m.NginxConfDir == "" {
		return "", fmt.Errorf("nginx conf directory is not configured")
	}

	upstream := cfg.ContainerUpstream
	if upstream == "" {
		port := cfg.Port
		if port <= 0 {
			port = 3000
		}
		upstream = fmt.Sprintf("%s:%d", cfg.ContainerName, port)
	}
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

func (m *Manager) ReloadNginx() error {
	if m.ProxyContainer == "" {
		return fmt.Errorf("proxy container is not configured")
	}

	output, err := m.runner.CombinedOutput("docker", "exec", m.ProxyContainer, "nginx", "-t")
	if err != nil {
		return fmt.Errorf("nginx config test failed: %w\nOutput: %s", err, string(output))
	}

	output, err = m.runner.CombinedOutput("docker", "exec", m.ProxyContainer, "nginx", "-s", "reload")
	if err != nil {
		return fmt.Errorf("nginx reload failed: %w\nOutput: %s", err, string(output))
	}

	log.Printf("Nginx reloaded")
	return nil
}

func (m *Manager) RemoveDomain(domainName string) error {
	if err := RemoveNginxConfig(m.NginxConfDir, domainName); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (cfg ProvisionConfig) sslDomain() string {
	if cfg.SSLDomain != "" {
		return cfg.SSLDomain
	}
	return cfg.Domain
}

func (cfg ProvisionConfig) requestSSLDomains() []string {
	if len(cfg.SSLDomains) > 0 {
		return uniqueNonEmpty(cfg.SSLDomains...)
	}
	if cfg.SSLDomain != "" {
		return []string{cfg.SSLDomain}
	}
	return uniqueNonEmpty(cfg.Domain, cfg.RedirectDomain)
}

func (cfg ProvisionConfig) domainsToVerify() []string {
	return uniqueNonEmpty(cfg.Domain, cfg.RedirectDomain)
}

func uniqueNonEmpty(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}
