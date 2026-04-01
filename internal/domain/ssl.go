package domain

import (
	"fmt"
	"log"
	"os/exec"
)

// RequestSSLCert triggers certbot to issue a certificate for a domain.
// This runs certbot via docker exec on the certbot container.
func RequestSSLCert(domain, email string) error {
	cmd := exec.Command("docker", "exec", "certbot",
		"certbot", "certonly",
		"--webroot",
		"--webroot-path=/var/www/html",
		"--email", email,
		"--agree-tos",
		"--no-eff-email",
		"-d", domain,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("certbot failed for %s: %w\nOutput: %s", domain, err, string(output))
	}

	log.Printf("SSL cert issued for %s", domain)
	return nil
}

// ReloadNginx sends a reload signal to the nginx proxy container.
func ReloadNginx() error {
	cmd := exec.Command("docker", "exec", "nginx-proxy", "nginx", "-s", "reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nginx reload failed: %w\nOutput: %s", err, string(output))
	}

	log.Printf("Nginx reloaded")
	return nil
}
