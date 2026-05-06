package auth

import (
	"testing"
	"time"
)

const testSecret = "test-secret-32-bytes-long-key-yyy"

func TestMintAndVerifyBypassToken_RoundTrip(t *testing.T) {
	token := MintSiteAuthBypassToken(testSecret, "proj-123", "production")

	if !VerifySiteAuthBypass(testSecret, token, "proj-123", "production") {
		t.Fatalf("freshly-minted token failed verification: %s", token)
	}
}

func TestVerifyBypassToken_RejectsExpired(t *testing.T) {
	expired := SignSiteAuthBypassWithExpiry(testSecret, "proj-123", "preview", time.Now().Add(-1*time.Second).Unix())

	if VerifySiteAuthBypass(testSecret, expired, "proj-123", "preview") {
		t.Fatal("expired token should not verify")
	}
}

func TestVerifyBypassToken_RejectsForeignProject(t *testing.T) {
	token := MintSiteAuthBypassToken(testSecret, "proj-A", "production")

	if VerifySiteAuthBypass(testSecret, token, "proj-B", "production") {
		t.Fatal("token minted for proj-A should not verify against proj-B")
	}
}

func TestVerifyBypassToken_RejectsForeignEnvironment(t *testing.T) {
	token := MintSiteAuthBypassToken(testSecret, "proj-123", "preview")

	if VerifySiteAuthBypass(testSecret, token, "proj-123", "production") {
		t.Fatal("token minted for preview should not verify against production")
	}
}

func TestVerifyBypassToken_RejectsTamperedSignature(t *testing.T) {
	token := MintSiteAuthBypassToken(testSecret, "proj-123", "production")
	replacement := "0"
	if token[len(token)-1] == '0' {
		replacement = "1"
	}
	tampered := token[:len(token)-1] + replacement

	if VerifySiteAuthBypass(testSecret, tampered, "proj-123", "production") {
		t.Fatal("tampered token should not verify")
	}
}

func TestVerifyBypassToken_RejectsBadFormat(t *testing.T) {
	cases := []string{
		"",
		"no-dot-separator",
		".justasignature",
		"notanumber.deadbeef",
	}
	for _, tok := range cases {
		if VerifySiteAuthBypass(testSecret, tok, "p", "preview") {
			t.Fatalf("malformed token should not verify: %q", tok)
		}
	}
}

func TestExtractBypassToken(t *testing.T) {
	cases := []struct {
		uri  string
		want string
	}{
		{"/", ""},
		{"/?other=1", ""},
		{"/?_dpkauth=12345.abc", "12345.abc"},
		{"/path?_dpkauth=t&x=1", "t"},
		{"/path?x=1&_dpkauth=t&y=2", "t"},
		{"", ""},
	}
	for _, c := range cases {
		got := ExtractBypassToken(c.uri)
		if got != c.want {
			t.Errorf("ExtractBypassToken(%q) = %q; want %q", c.uri, got, c.want)
		}
	}
}
