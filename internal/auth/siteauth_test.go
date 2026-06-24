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

func TestStaticBypass_RoundTrip(t *testing.T) {
	tok := MintStaticBypassToken(testSecret, "proj-1", "preview", "nonce-abc")
	if !VerifyStaticBypass(testSecret, tok, "proj-1", "preview", "nonce-abc") {
		t.Fatal("freshly-minted static bypass token failed verification")
	}
}

func TestStaticBypass_RotatedNonceRevokes(t *testing.T) {
	tok := MintStaticBypassToken(testSecret, "proj-1", "preview", "old-nonce")
	if VerifyStaticBypass(testSecret, tok, "proj-1", "preview", "new-nonce") {
		t.Fatal("token minted against old nonce must not verify against a rotated nonce")
	}
}

func TestStaticBypass_EmptyNonceRejected(t *testing.T) {
	if VerifyStaticBypass(testSecret, "anything", "proj-1", "preview", "") {
		t.Fatal("empty nonce must never verify (no link issued)")
	}
}

func TestStaticBypass_WrongProjectOrEnv(t *testing.T) {
	tok := MintStaticBypassToken(testSecret, "proj-A", "preview", "n")
	if VerifyStaticBypass(testSecret, tok, "proj-B", "preview", "n") {
		t.Fatal("wrong project must not verify")
	}
	if VerifyStaticBypass(testSecret, tok, "proj-A", "production", "n") {
		t.Fatal("wrong environment must not verify")
	}
}

func TestExtractStaticBypassToken(t *testing.T) {
	if got := ExtractStaticBypassToken("/path?a=1&_dpkbypass=deadbeef&b=2"); got != "deadbeef" {
		t.Fatalf("got %q, want deadbeef", got)
	}
	if got := ExtractStaticBypassToken("/path?_dpkauth=x.y"); got != "" {
		t.Fatalf("got %q, want empty (different param)", got)
	}
}
