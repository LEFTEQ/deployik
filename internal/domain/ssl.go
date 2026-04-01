package domain

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
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
	ProjectName   string
	Domain        string
	SSLDomain     string
	ContainerName string
}

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

func (m *Manager) ProvisionDomain(cfg ProvisionConfig, requireDNS bool) error {
	if requireDNS {
		verified, err := m.VerifyDomainDNS(cfg.Domain)
		if err != nil {
			return err
		}
		if !verified {
			return ErrDNSNotVerified
		}
	}

	if err := m.RequestSSLCert(cfg.sslDomain()); err != nil {
		return err
	}

	if _, err := m.WriteNginxConfig(cfg); err != nil {
		return err
	}

	if err := m.ReloadNginx(); err != nil {
		return err
	}

	return nil
}

func (m *Manager) RequestSSLCert(domainName string) error {
	if m.ProxyCertsDir == "" {
		return fmt.Errorf("proxy certs directory is not configured")
	}
	if m.ProxyHTMLDir == "" {
		return fmt.Errorf("proxy html directory is not configured")
	}
	if m.SSLEmail == "" {
		return fmt.Errorf("ssl email is not configured")
	}

	cmd := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/etc/letsencrypt", m.ProxyCertsDir),
		"-v", fmt.Sprintf("%s:/var/www/html", m.ProxyHTMLDir),
		"certbot/certbot", "certonly",
		"--webroot", "-w", "/var/www/html",
		"-d", domainName,
		"--email", m.SSLEmail,
		"--agree-tos",
		"--no-eff-email",
		"--non-interactive",
		"--keep-until-expiring",
	}

	output, err := m.runner.CombinedOutput("docker", cmd...)
	if err != nil {
		return fmt.Errorf("certbot failed for %s: %w\nOutput: %s", domainName, err, string(output))
	}

	log.Printf("SSL cert ensured for %s", domainName)
	return nil
}

func (m *Manager) WriteNginxConfig(cfg ProvisionConfig) (string, error) {
	if m.NginxConfDir == "" {
		return "", fmt.Errorf("nginx conf directory is not configured")
	}

	return GenerateNginxConfig(m.NginxConfDir, NginxConfig{
		ProjectName:   cfg.ProjectName,
		Domain:        cfg.Domain,
		SSLDomain:     cfg.sslDomain(),
		ContainerName: cfg.ContainerName,
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
