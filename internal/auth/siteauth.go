package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SiteAuthBypassParam is the query parameter the Deployik screenshot capture
// flow appends to a URL so password-protected sites can be screenshotted
// without human login. Stable forever — changing it requires regenerating
// every nginx vhost.
const SiteAuthBypassParam = "_dpkauth"

// SiteAuthStaticBypassParam is the query parameter carrying the STABLE
// (non-expiring, rotate-to-revoke) bypass token. Unlike _dpkauth it is bound to
// the project's bypass_nonce, so revocation is a DB nonce rotation rather than
// an expiry.
const SiteAuthStaticBypassParam = "_dpkbypass"

// SiteAuthBypassTTL is the validity window of a freshly-minted bypass token.
// Short on purpose: the token rides in the URL (and therefore in nginx access
// logs), so we want it to be useless quickly.
const SiteAuthBypassTTL = 60 * time.Second

// MintSiteAuthBypassToken returns a short-lived signed token authorising one
// site-auth gate to let a request through for the given project + environment.
// Token format is "<expiry-unix>.<sha256-hex>" so it survives URL encoding
// without escapes. Domain separation ("bypass:" prefix in the signed message)
// keeps these signatures non-interchangeable with regular site-auth cookies
// even when the same JWT secret is reused.
func MintSiteAuthBypassToken(secret, projectID, environment string) string {
	expiry := time.Now().Add(SiteAuthBypassTTL).Unix()
	return signSiteAuthBypass(secret, projectID, environment, expiry)
}

// SignSiteAuthBypassWithExpiry is exposed for tests that need to mint tokens
// with explicit (often past) expiries to exercise expiry handling.
func SignSiteAuthBypassWithExpiry(secret, projectID, environment string, expiry int64) string {
	return signSiteAuthBypass(secret, projectID, environment, expiry)
}

func signSiteAuthBypass(secret, projectID, environment string, expiry int64) string {
	msg := fmt.Sprintf("bypass:%s:%s:%d", projectID, environment, expiry)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	return fmt.Sprintf("%d.%s", expiry, hex.EncodeToString(mac.Sum(nil)))
}

// VerifySiteAuthBypass returns true only when the token is well-formed,
// unexpired, and HMAC-signed for the given project + environment. False on
// any failure.
func VerifySiteAuthBypass(secret, token, expectedProject, expectedEnv string) bool {
	dot := strings.IndexByte(token, '.')
	if dot < 0 {
		return false
	}
	expiryStr := token[:dot]
	sig := token[dot+1:]
	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix() > expiry {
		return false
	}
	msg := fmt.Sprintf("bypass:%s:%s:%d", expectedProject, expectedEnv, expiry)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expected))
}

// ExtractBypassToken pulls the short-lived _dpkauth token out of a request URI.
func ExtractBypassToken(requestURI string) string {
	return extractQueryParam(requestURI, SiteAuthBypassParam)
}

func extractQueryParam(requestURI, param string) string {
	q := strings.IndexByte(requestURI, '?')
	if q < 0 {
		return ""
	}
	values, err := url.ParseQuery(requestURI[q+1:])
	if err != nil {
		return ""
	}
	return values.Get(param)
}

// MintStaticBypassToken returns a stable signed token authorising the site-auth
// gate for the given project + environment, bound to the project's bypass nonce.
// No expiry: rotating the nonce is what revokes it. Domain separation
// ("staticbypass:" prefix) keeps it non-interchangeable with _dpkauth + cookies.
func MintStaticBypassToken(secret, projectID, environment, nonce string) string {
	msg := fmt.Sprintf("staticbypass:%s:%s:%s", projectID, environment, nonce)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyStaticBypass returns true only when token is the valid HMAC for the
// given project + environment + nonce. An empty nonce or token always returns
// false (a project with no issued link cannot be bypassed).
func VerifyStaticBypass(secret, token, expectedProject, expectedEnv, nonce string) bool {
	if nonce == "" || token == "" {
		return false
	}
	expected := MintStaticBypassToken(secret, expectedProject, expectedEnv, nonce)
	return hmac.Equal([]byte(token), []byte(expected))
}

// ExtractStaticBypassToken pulls the _dpkbypass token out of a request URI.
func ExtractStaticBypassToken(requestURI string) string {
	return extractQueryParam(requestURI, SiteAuthStaticBypassParam)
}
