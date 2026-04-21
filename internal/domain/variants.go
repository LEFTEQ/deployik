package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// hostnameLabel matches a single DNS label: alphanumeric start/end, internal
// hyphens allowed, 1-63 chars. No underscores (illegal per RFC 1123 for hostnames).
var hostnameLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ErrInvalidHostname is returned when a user-supplied domain name fails validation.
var ErrInvalidHostname = errors.New("invalid hostname")

// ValidateHostname ensures the input is a syntactically legal DNS hostname
// (RFC 1123) consisting of at least two labels, with no wildcards, IP literals,
// underscores, whitespace, or shell/nginx metacharacters. It returns a
// lower-cased, trimmed hostname on success.
//
// This is the hard boundary between user input and the nginx config writer —
// any injection (newlines, semicolons, braces) must be rejected here.
func ValidateHostname(value string) (string, error) {
	normalized := normalizeHostname(value)
	if normalized == "" {
		return "", fmt.Errorf("%w: empty", ErrInvalidHostname)
	}
	if len(normalized) > 253 {
		return "", fmt.Errorf("%w: exceeds 253 characters", ErrInvalidHostname)
	}
	labels := strings.Split(normalized, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf("%w: must contain at least two labels", ErrInvalidHostname)
	}
	for _, label := range labels {
		if !hostnameLabel.MatchString(label) {
			return "", fmt.Errorf("%w: label %q is not RFC 1123 compliant", ErrInvalidHostname, label)
		}
	}
	// Reject numeric-only TLDs (IPv4 literals slip through the label regex).
	tld := labels[len(labels)-1]
	allDigits := true
	for _, r := range tld {
		if r < '0' || r > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		return "", fmt.Errorf("%w: numeric TLDs are not allowed", ErrInvalidHostname)
	}
	return normalized, nil
}

type VariantPlan struct {
	CanonicalDomain string
	RedirectDomain  string
}

func ResolveVariantPlan(domainName, environment string) VariantPlan {
	normalized := normalizeHostname(domainName)
	plan := VariantPlan{CanonicalDomain: normalized}
	if normalized == "" {
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

	// Only add a www redirect variant when the input is the apex (eTLD+1).
	// For subdomains we would otherwise demand DNS the user never created
	// (e.g. www.forge.example.org), and wildcard DNS only covers one label so
	// www.* under auto-preview hosts never actually resolved either.
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
