package auth

import (
	"strings"
	"testing"
)

func TestGenerateAPIToken(t *testing.T) {
	a, err := GenerateAPIToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.HasPrefix(a, APITokenPrefix) {
		t.Fatalf("token %q missing prefix %q", a, APITokenPrefix)
	}
	body := strings.TrimPrefix(a, APITokenPrefix)
	if len(body) < 32 {
		t.Fatalf("token body too short: %d chars", len(body))
	}

	b, err := GenerateAPIToken()
	if err != nil {
		t.Fatalf("generate b: %v", err)
	}
	if a == b {
		t.Fatalf("two generated tokens must differ; got %q twice", a)
	}

	// Hashes must differ too — sanity check on the hashing helper.
	if HashToken(a) == HashToken(b) {
		t.Fatalf("hashes of distinct tokens collided")
	}
}

func TestAPITokenPrefixIsStable(t *testing.T) {
	// Guard against accidental prefix changes; existing tokens would stop
	// authenticating if this string ever changes.
	if APITokenPrefix != "dpk_" {
		t.Fatalf("APITokenPrefix changed to %q — existing tokens would break", APITokenPrefix)
	}
}
