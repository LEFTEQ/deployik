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
	NginxConfDir   string
	ProxyContainer string
	ProxyCertsDir  string
	ProxyHTMLDir   string
	VPSHost        string
	SSLEmail       string
}

type Manager struct {
	NginxConfDir   string
	ProxyContainer string
	ProxyCertsDir  string
	ProxyHTMLDir   string
	VPSHost        string
	SSLEmail       string
	runner         commandRunner
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
		NginxConfDir:   cfg.NginxConfDir,
		ProxyContainer: cfg.ProxyContainer,
		ProxyCertsDir:  cfg.ProxyCertsDir,
		ProxyHTMLDir:   cfg.ProxyHTMLDir,
		VPSHost:        cfg.VPSHost,
		SSLEmail:       cfg.SSLEmail,
		runner:         execRunner{},
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

	sslDomains := cfg.requestSSLDomains()
	emit("ssl", "running", fmt.Sprintf("Requesting SSL certificate for %s...", strings.Join(sslDomains, ", ")))
	if err := m.RequestSSLCert(sslDomains...); err != nil {
		emit("ssl", "error", fmt.Sprintf("SSL certificate request failed: %v", err))
		return err
	}
	emit("ssl", "success", "SSL certificate issued successfully")

	emit("nginx", "running", fmt.Sprintf("Writing nginx config for %s...", cfg.Domain))
	if _, err := m.WriteNginxConfig(cfg); err != nil {
		emit("nginx", "error", fmt.Sprintf("Nginx config write failed: %v", err))
		return err
	}

	emit("nginx", "running", "Testing and reloading nginx...")
	if err := m.ReloadNginx(); err != nil {
		emit("nginx", "error", fmt.Sprintf("Nginx reload failed: %v", err))
		return err
	}
	emit("nginx", "success", "Nginx reloaded successfully")

	return nil
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

	return GenerateNginxConfig(m.NginxConfDir, NginxConfig{
		ProjectID:         cfg.ProjectID,
		ProjectName:       cfg.ProjectName,
		Domain:            cfg.Domain,
		RedirectDomain:    cfg.RedirectDomain,
		Environment:       cfg.Environment,
		SSLDomain:         cfg.sslDomain(),
		ContainerName:     cfg.ContainerName,
		PasswordProtected: cfg.PasswordProtected,
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
