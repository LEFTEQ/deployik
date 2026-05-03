package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
)

const testBypassSecret = "test-secret-32-bytes-long-key-yyy"

func TestCheckHandler_AcceptsValidBypassViaOriginalURI(t *testing.T) {
	h := &ProtectionHandler{JWTSecret: testBypassSecret}
	token := auth.MintSiteAuthBypassToken(testBypassSecret, "proj-X", "production")

	req := httptest.NewRequest(http.MethodGet, "/api/site-auth/check", nil)
	req.Header.Set("X-Deployik-Project", "proj-X")
	req.Header.Set("X-Deployik-Environment", "production")
	req.Header.Set("X-Original-URI", "/?other=1&_dpkauth="+token)

	rr := httptest.NewRecorder()
	h.Check(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid bypass token, got %d", rr.Code)
	}
}

func TestCheckHandler_RejectsExpiredBypass(t *testing.T) {
	h := &ProtectionHandler{JWTSecret: testBypassSecret}
	expired := auth.SignSiteAuthBypassWithExpiry(testBypassSecret, "proj-X", "production", time.Now().Add(-1*time.Second).Unix())

	req := httptest.NewRequest(http.MethodGet, "/api/site-auth/check", nil)
	req.Header.Set("X-Deployik-Project", "proj-X")
	req.Header.Set("X-Deployik-Environment", "production")
	req.Header.Set("X-Original-URI", "/?_dpkauth="+expired)

	rr := httptest.NewRecorder()
	h.Check(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with expired bypass token, got %d", rr.Code)
	}
}

func TestCheckHandler_RejectsBypassForWrongProject(t *testing.T) {
	h := &ProtectionHandler{JWTSecret: testBypassSecret}
	token := auth.MintSiteAuthBypassToken(testBypassSecret, "proj-A", "production")

	req := httptest.NewRequest(http.MethodGet, "/api/site-auth/check", nil)
	req.Header.Set("X-Deployik-Project", "proj-B")
	req.Header.Set("X-Deployik-Environment", "production")
	req.Header.Set("X-Original-URI", "/?_dpkauth="+token)

	rr := httptest.NewRecorder()
	h.Check(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when bypass is for a different project, got %d", rr.Code)
	}
}

func TestCheckHandler_FallsBackToCookieWhenNoBypass(t *testing.T) {
	h := &ProtectionHandler{JWTSecret: testBypassSecret}
	expiry := time.Now().Add(siteAuthTTL).Unix()
	cookie := signSiteAuth(testBypassSecret, "proj-Y", "preview", expiry)

	req := httptest.NewRequest(http.MethodGet, "/api/site-auth/check", nil)
	req.Header.Set("X-Deployik-Project", "proj-Y")
	req.Header.Set("X-Deployik-Environment", "preview")
	req.AddCookie(&http.Cookie{Name: siteAuthCookieName, Value: cookie})

	rr := httptest.NewRecorder()
	h.Check(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 from cookie path, got %d", rr.Code)
	}
}
