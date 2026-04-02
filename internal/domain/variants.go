package domain

import (
	"strings"

	"golang.org/x/net/publicsuffix"
)

type VariantPlan struct {
	CanonicalDomain string
	RedirectDomain  string
}

func ResolveVariantPlan(domainName, environment string) VariantPlan {
	normalized := normalizeHostname(domainName)
	plan := VariantPlan{CanonicalDomain: normalized}
	if normalized == "" || environment != "production" {
		return plan
	}

	if strings.HasPrefix(normalized, "www.") {
		trimmed := strings.TrimPrefix(normalized, "www.")
		if trimmed != "" {
			plan.CanonicalDomain = trimmed
			plan.RedirectDomain = normalized
		}
		return plan
	}

	root, err := publicsuffix.EffectiveTLDPlusOne(normalized)
	if err == nil && root == normalized {
		plan.RedirectDomain = "www." + normalized
	}

	return plan
}

func (p VariantPlan) AllDomains() []string {
	values := []string{p.CanonicalDomain}
	if p.RedirectDomain != "" && p.RedirectDomain != p.CanonicalDomain {
		values = append(values, p.RedirectDomain)
	}
	return values
}

func normalizeHostname(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.TrimSuffix(normalized, ".")
	return normalized
}
