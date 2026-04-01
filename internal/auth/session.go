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
