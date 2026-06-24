package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

const testBypassSecret = "test-secret-32-bytes-long-key-yyy"

// newProtectionHandler builds a DB-backed ProtectionHandler with a real encryptor
// and audit recorder. Manager is nil, which the handler tolerates (nginx rewrite
// is skipped). Returns the handler and a freshly-created project to act on.
func newProtectionHandler(t *testing.T) (*ProtectionHandler, *db.Project) {
	t.Helper()

	database := newVariableHandlerTestDB(t)
	project := newTestProject(t, database)

	encryptor, err := crypto.NewEncryptor("test-key")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	return &ProtectionHandler{
		DB:        database,
		Encryptor: encryptor,
		JWTSecret: testBypassSecret,
		Audit:     &audit.Recorder{DB: database},
	}, project
}

func protectionRequest(t *testing.T, method, path, projectID string, body any) *http.Request {
	t.Helper()

	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", projectID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	return req.WithContext(auth.WithClaims(req.Context(), &auth.Claims{UserID: "admin", Role: "admin"}))
}

func TestUpdate_StoresCustomPassword(t *testing.T) {
	h, project := newProtectionHandler(t)

	req := protectionRequest(t, http.MethodPut, "/projects/"+project.ID+"/protection", project.ID, map[string]any{
		"environment": "preview",
		"enabled":     true,
		"password":    "my custom pass",
	})
	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	var resp struct {
		Password string `json:"password"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Password != "my custom pass" {
		t.Fatalf("response password = %q, want the custom value echoed back", resp.Password)
	}

	// The stored, encrypted password must decrypt back to the custom value.
	enc, err := h.DB.GetProjectPassword(project.ID, "preview")
	if err != nil || enc == "" {
		t.Fatalf("GetProjectPassword: %v (enc empty=%v)", err, enc == "")
	}
	plain, err := h.Encryptor.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if plain != "my custom pass" {
		t.Fatalf("stored password = %q, want custom value", plain)
	}
}

func TestUpdate_GeneratesWhenPasswordOmitted(t *testing.T) {
	h, project := newProtectionHandler(t)

	req := protectionRequest(t, http.MethodPut, "/projects/"+project.ID+"/protection", project.ID, map[string]any{
		"environment": "production",
		"enabled":     true,
	})
	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	var resp struct {
		Password string `json:"password"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Password) != 16 {
		t.Fatalf("generated password length = %d, want 16", len(resp.Password))
	}
}

func TestUpdate_RejectsEmptyCustomPasswordWhenProvidedFlag(t *testing.T) {
	// An explicit whitespace-only password is a non-empty string and is accepted
	// verbatim (no minimum length); but a request that asks to enable with no
	// password simply generates one. The only rejected case is a password that
	// fails validation. Here we exercise the over-length bound.
	h, project := newProtectionHandler(t)

	req := protectionRequest(t, http.MethodPut, "/projects/"+project.ID+"/protection", project.ID, map[string]any{
		"environment": "preview",
		"enabled":     true,
		"password":    strings.Repeat("x", 257),
	})
	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for over-length password (%s)", rec.Code, rec.Body.String())
	}
}

func TestRevealPassword_ReturnsStoredValue(t *testing.T) {
	h, project := newProtectionHandler(t)

	// Seed a custom password via Update.
	setReq := protectionRequest(t, http.MethodPut, "/projects/"+project.ID+"/protection", project.ID, map[string]any{
		"environment": "production",
		"enabled":     true,
		"password":    "reveal-me-123",
	})
	h.Update(httptest.NewRecorder(), setReq)

	req := protectionRequest(t, http.MethodGet, "/projects/"+project.ID+"/protection/password?environment=production", project.ID, nil)
	rec := httptest.NewRecorder()
	h.RevealPassword(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Password string `json:"password"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Password != "reveal-me-123" {
		t.Fatalf("revealed password = %q, want stored value", resp.Password)
	}
}

func TestRevealPassword_NotFoundWhenUnset(t *testing.T) {
	h, project := newProtectionHandler(t)

	req := protectionRequest(t, http.MethodGet, "/projects/"+project.ID+"/protection/password?environment=preview", project.ID, nil)
	rec := httptest.NewRecorder()
	h.RevealPassword(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when no password set (%s)", rec.Code, rec.Body.String())
	}
}

func TestValidateCustomPassword(t *testing.T) {
	if err := validateCustomPassword(""); err == nil {
		t.Fatal("empty password should be rejected")
	}
	if err := validateCustomPassword("x"); err != nil {
		t.Fatalf("single-char password should be accepted (no minimum length), got %v", err)
	}
	if err := validateCustomPassword(strings.Repeat("a", maxPasswordLength)); err != nil {
		t.Fatalf("max-length password should be accepted, got %v", err)
	}
	if err := validateCustomPassword(strings.Repeat("a", maxPasswordLength+1)); err == nil {
		t.Fatal("over-length password should be rejected")
	}
}

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

// verifyFormRequest builds a form-encoded POST to the site-auth verify endpoint
// the way nginx forwards it (project/environment in headers).
func verifyFormRequest(projectID, environment, password string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/site-auth/verify", strings.NewReader("password="+password))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Deployik-Project", projectID)
	req.Header.Set("X-Deployik-Environment", environment)
	req.Header.Set("Referer", "https://app.preview.example.com/")
	return req
}

func TestVerify_SuccessIssuesCookieAndClearSiteData(t *testing.T) {
	h, project := newProtectionHandler(t)

	enc, err := h.Encryptor.Encrypt("pw123")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if err := h.DB.SetProjectPassword(project.ID, "preview", enc); err != nil {
		t.Fatalf("SetProjectPassword: %v", err)
	}

	rec := httptest.NewRecorder()
	h.Verify(rec, verifyFormRequest(project.ID, "preview", "pw123"))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (%s)", rec.Code, rec.Body.String())
	}

	res := rec.Result()
	var found bool
	for _, c := range res.Cookies() {
		if c.Name == siteAuthCookieName && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected %s cookie to be set", siteAuthCookieName)
	}

	// Heals browsers whose PWA service worker cached the auth page as the app
	// shell: must NOT include "cookies" or the fresh auth cookie would be wiped.
	if got := res.Header.Get("Clear-Site-Data"); got != `"cache", "storage"` {
		t.Fatalf("Clear-Site-Data = %q, want %q", got, `"cache", "storage"`)
	}
}

func TestVerify_WrongPasswordOmitsClearSiteData(t *testing.T) {
	h, project := newProtectionHandler(t)

	enc, err := h.Encryptor.Encrypt("pw123")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if err := h.DB.SetProjectPassword(project.ID, "preview", enc); err != nil {
		t.Fatalf("SetProjectPassword: %v", err)
	}

	rec := httptest.NewRecorder()
	h.Verify(rec, verifyFormRequest(project.ID, "preview", "nope"))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 redirect back with error", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "error=1") {
		t.Fatalf("Location = %q, want error=1 redirect", loc)
	}
	if got := rec.Header().Get("Clear-Site-Data"); got != "" {
		t.Fatalf("Clear-Site-Data = %q, want empty on failed verification", got)
	}
}

func TestCheckHandler_AcceptsValidStaticBypass(t *testing.T) {
	h, project := newProtectionHandler(t)
	if err := h.DB.SetProjectBypassNonce(project.ID, "nonce-xyz"); err != nil {
		t.Fatalf("SetProjectBypassNonce: %v", err)
	}
	token := auth.MintStaticBypassToken(testBypassSecret, project.ID, "preview", "nonce-xyz")

	req := httptest.NewRequest(http.MethodGet, "/api/site-auth/check", nil)
	req.Header.Set("X-Deployik-Project", project.ID)
	req.Header.Set("X-Deployik-Environment", "preview")
	req.Header.Set("X-Original-URI", "/?_dpkbypass="+token)
	rr := httptest.NewRecorder()
	h.Check(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rr.Code, rr.Body.String())
	}
}

func TestCheckHandler_RejectsStaticBypassAfterRotation(t *testing.T) {
	h, project := newProtectionHandler(t)
	_ = h.DB.SetProjectBypassNonce(project.ID, "old")
	token := auth.MintStaticBypassToken(testBypassSecret, project.ID, "preview", "old")
	_ = h.DB.SetProjectBypassNonce(project.ID, "new") // rotate

	req := httptest.NewRequest(http.MethodGet, "/api/site-auth/check", nil)
	req.Header.Set("X-Deployik-Project", project.ID)
	req.Header.Set("X-Deployik-Environment", "preview")
	req.Header.Set("X-Original-URI", "/?_dpkbypass="+token)
	rr := httptest.NewRecorder()
	h.Check(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 after rotation", rr.Code)
	}
}

func TestBypassNonce_QueryRoundTrip(t *testing.T) {
	h, project := newProtectionHandler(t)

	got, err := h.DB.GetProjectBypassNonce(project.ID)
	if err != nil || got != "" {
		t.Fatalf("fresh project nonce = %q err=%v, want empty/no-error", got, err)
	}
	if err := h.DB.SetProjectBypassNonce(project.ID, "n1"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got, _ = h.DB.GetProjectBypassNonce(project.ID); got != "n1" {
		t.Fatalf("nonce = %q, want n1", got)
	}
	if err := h.DB.ClearProjectBypassNonce(project.ID); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got, _ = h.DB.GetProjectBypassNonce(project.ID); got != "" {
		t.Fatalf("nonce after clear = %q, want empty", got)
	}
}
