package domain

import (
	"fmt"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// ReconcileActiveConfigs rewrites nginx configs for already-active domains and reloads nginx once.
func ReconcileActiveConfigs(manager *Manager, targets []db.DomainProvisionTarget) error {
	if manager == nil || manager.NginxConfDir == "" || manager.ProxyContainer == "" || len(targets) == 0 {
		return nil
	}

	for _, target := range targets {
		if _, err := manager.WriteNginxConfig(ProvisionConfig{
			ProjectID:     target.ProjectID,
			ProjectName:   target.ProjectName,
			Domain:        target.DomainName,
			Environment:   target.Environment,
			ContainerName: fmt.Sprintf("deployik-%s-%s", target.ProjectName, target.Environment),
		}); err != nil {
			return fmt.Errorf("reconcile domain %s: %w", target.DomainName, err)
		}
	}

	if err := manager.ReloadNginx(); err != nil {
		return fmt.Errorf("reload nginx after reconcile: %w", err)
	}

	return nil
}
