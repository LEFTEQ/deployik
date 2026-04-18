package domain

import (
	"strings"
	"testing"
)

func TestValidateHostname_Accepts(t *testing.T) {
	valid := []string{
		"example.com",
		"foo.example.com",
		"my-app.preview.example.com",
		"a.b",
		"xn--bcher-kva.de",
		strings.Repeat("a", 63) + ".com",
	}
	for _, host := range valid {
		t.Run(host, func(t *testing.T) {
			got, err := ValidateHostname(host)
			if err != nil {
				t.Fatalf("ValidateHostname(%q) unexpected error: %v", host, err)
			}
			if got == "" {
				t.Fatalf("ValidateHostname(%q) returned empty", host)
			}
		})
	}
}

func TestValidateHostname_RejectsInjection(t *testing.T) {
	injections := []string{
		"",
		"no-tld",
		"example.com;add_header X-Pwn 1",
		"example.com\nadd_header X-Pwn 1;",
		"example.com\r\nssl_certificate /etc/passwd",
		"*.example.com",
		"example..com",
		"_dmarc.example.com",                                       // underscore illegal for RFC 1123 hostnames
		"192.168.1.1",                                              // numeric TLD
		"example.com/" + strings.Repeat("a", 300),                  // contains slash
		"http://example.com",                                       // scheme
		"example .com",                                             // space
		"example.com:8080",                                         // port
		strings.Repeat("a", 250) + "." + strings.Repeat("a", 50),   // exceeds 253
		strings.Repeat("a", 64) + ".com",                           // label too long
	}
	for _, host := range injections {
		t.Run(host, func(t *testing.T) {
			if _, err := ValidateHostname(host); err == nil {
				t.Fatalf("ValidateHostname(%q) accepted an illegal hostname", host)
			}
		})
	}
}

func TestValidateHostname_TrailingDotNormalized(t *testing.T) {
	// Trailing dot is FQDN-legal; normalizeHostname strips it, so this should pass.
	got, err := ValidateHostname("example.com.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "example.com" {
		t.Fatalf("got %q, want example.com", got)
	}
}
