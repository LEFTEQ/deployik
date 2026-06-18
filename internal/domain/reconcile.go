package domain

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// DockerInspector is used by reconcile to inspect running containers for host-port mode.
type DockerInspector interface {
	ContainerExists(ctx context.Context, name string) (string, bool)
	// GetHostPort returns the host port bound to the container's target port
	// (pass the same port the container was started with; 0 defaults to 3000).
	GetHostPort(ctx context.Context, containerID string, port int) (string, error)
}

// ReconcileActiveConfigs rewrites proxy configs for already-active domains and reloads once.
func ReconcileActiveConfigs(manager *Manager, targets []db.DomainProvisionTarget, docker DockerInspector) error {
	if manager == nil || manager.NginxConfDir == "" || len(targets) == 0 {
		return nil
	}
	// In docker mode, also require a proxy container to reload
	if manager.ProxyType != "host-port" && manager.ProxyContainer == "" {
		return nil
	}

	var errs []string
	wroteConfig := false
	for _, target := range targets {
		plan := ResolveVariantPlan(target.DomainName, target.Environment)

		// Skip certbot for wildcard-covered domains; otherwise keep the existing
		// behavior (request a multi-domain cert only when there's a www redirect).
		if !manager.wildcardCovers(plan.CanonicalDomain) && plan.RedirectDomain != "" {
			if err := manager.RequestSSLCert(plan.AllDomains()...); err != nil {
				errs = append(errs, fmt.Sprintf("ensure ssl for %s: %v", target.DomainName, err))
				continue
			}
		}

		containerName := db.DeploymentContainerName(target.ProjectName, target.Environment, &db.PreviewInstance{
			ID:         target.PreviewInstanceID,
			Branch:     target.PreviewBranch,
			BranchSlug: target.PreviewBranchSlug,
			IsDefault:  target.PreviewInstanceDefault,
		})
		targetPort := target.Port
		if targetPort <= 0 {
			targetPort = 3000
		}
		upstream := fmt.Sprintf("%s:%d", containerName, targetPort)

		// In host-port mode, the proxy lives on the host and talks to
		// 127.0.0.1:<random>. We must look up the live port from Docker — if
		// the container isn't running (e.g. first boot before any deploy), the
		// container-name upstream would be unresolvable and produce a broken
		// vhost. Skip the target in that case so we don't write a 502 machine.
		if manager.ProxyType == "host-port" {
			if docker == nil {
				log.Printf("reconcile: skipping %s — host-port mode requires a Docker inspector", target.DomainName)
				continue
			}
			containerID, exists := docker.ContainerExists(context.Background(), containerName)
			if !exists {
				log.Printf("reconcile: skipping %s — container %s not running yet", target.DomainName, containerName)
				continue
			}
			hostPort, err := docker.GetHostPort(context.Background(), containerID, targetPort)
			if err != nil {
				errs = append(errs, fmt.Sprintf("host port lookup for %s: %v", target.DomainName, err))
				continue
			}
			upstream = "127.0.0.1:" + hostPort
		}

		cfg := ProvisionConfig{
			ProjectID:         target.ProjectID,
			ProjectName:       target.ProjectName,
			Domain:            plan.CanonicalDomain,
			RedirectDomain:    plan.RedirectDomain,
			Environment:       target.Environment,
			ContainerName:     containerName,
			ContainerUpstream: upstream,
			PasswordProtected: target.PasswordProtected,
		}

		if manager.ProxyConfigFormat == "apache" {
			certFile, keyFile := manager.certPathsFor(cfg.sslDomain())
			if _, err := GenerateApacheConfig(manager.NginxConfDir, ApacheConfig{
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
				errs = append(errs, fmt.Sprintf("reconcile domain %s: %v", target.DomainName, err))
				continue
			}
		} else {
			if _, err := manager.WriteNginxConfig(cfg); err != nil {
				errs = append(errs, fmt.Sprintf("reconcile domain %s: %v", target.DomainName, err))
				continue
			}
		}
		wroteConfig = true
	}

	if wroteConfig {
		if err := manager.ReloadProxy(); err != nil {
			errs = append(errs, fmt.Sprintf("reload proxy after reconcile: %v", err))
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}

	return nil
}
