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
