package domain

import (
	"errors"
	"fmt"
	"strings"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// ReconcileActiveConfigs rewrites nginx configs for already-active domains and reloads nginx once.
func ReconcileActiveConfigs(manager *Manager, targets []db.DomainProvisionTarget) error {
	if manager == nil || manager.NginxConfDir == "" || manager.ProxyContainer == "" || len(targets) == 0 {
		return nil
	}

	var errs []string
	wroteConfig := false
	for _, target := range targets {
		plan := ResolveVariantPlan(target.DomainName, target.Environment)
		if plan.RedirectDomain != "" {
			if err := manager.RequestSSLCert(plan.AllDomains()...); err != nil {
				errs = append(errs, fmt.Sprintf("ensure ssl for %s: %v", target.DomainName, err))
				continue
			}
		}
		if _, err := manager.WriteNginxConfig(ProvisionConfig{
			ProjectID:      target.ProjectID,
			ProjectName:    target.ProjectName,
			Domain:         plan.CanonicalDomain,
			RedirectDomain: plan.RedirectDomain,
			Environment:    target.Environment,
			ContainerName:  fmt.Sprintf("deployik-%s-%s", target.ProjectName, target.Environment),
		}); err != nil {
			errs = append(errs, fmt.Sprintf("reconcile domain %s: %v", target.DomainName, err))
			continue
		}
		wroteConfig = true
	}

	if wroteConfig {
		if err := manager.ReloadNginx(); err != nil {
			errs = append(errs, fmt.Sprintf("reload nginx after reconcile: %v", err))
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}

	return nil
}
