package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const (
	AccessCookieName     = "deployik_access_token"
	RefreshCookieName    = "deployik_refresh_token"
	OAuthStateCookieName = "deployik_oauth_state"

	// APITokenPrefix prefixes Personal Access Tokens so middleware can route
	// Bearer values to the api_tokens lookup path without trying to parse them
	// as JWTs first. The prefix is stable forever — changing it would invalidate
	// every issued token.
	APITokenPrefix = "dpk_"
)

// GenerateOpaqueToken creates a random token suitable for refresh sessions and OAuth state.
func GenerateOpaqueToken() (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(tokenBytes), nil
}

// HashToken hashes an opaque token before persistence.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// GenerateAPIToken creates a Personal Access Token. The returned string is
// the raw token shown to the user once at creation; only its SHA-256 hash
// is persisted via api_tokens.token_hash.
func GenerateAPIToken() (string, error) {
	body, err := GenerateOpaqueToken()
	if err != nil {
		return "", fmt.Errorf("generate api token: %w", err)
	}
	return APITokenPrefix + body, nil
}
