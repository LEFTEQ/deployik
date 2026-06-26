package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/lefteq/lovinka-deployik/internal/auth"
	"github.com/lefteq/lovinka-deployik/internal/db"
	"github.com/lefteq/lovinka-deployik/internal/github"
)

func TestGetGithubAuthSetsOAuthStateCookie(t *testing.T) {
	handler := &AuthHandler{
		OAuthConfig: &github.OAuthConfig{
			ClientID:    "client-id",
			RedirectURI: "http://localhost:5173/auth/callback",
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/auth/github", nil)
	rec := httptest.NewRecorder()

	handler.GetGithubAuth(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTemporaryRedirect)
	}

	redirectURL := rec.Header().Get("Location")
	if redirectURL == "" {
		t.Fatal("expected redirect location")
	}

	parsed, err := url.Parse(redirectURL)
	if err != nil {
		t.Fatalf("Parse redirect URL: %v", err)
	}
	state := parsed.Query().Get("state")
	if state == "" {
		t.Fatal("expected oauth state in redirect URL")
	}

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected oauth state cookie")
	}

	var stateCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == auth.OAuthStateCookieName {
			stateCookie = cookie
			break
		}
	}
	if stateCookie == nil {
		t.Fatal("missing oauth state cookie")
	}
	if stateCookie.Value != state {
		t.Fatalf("cookie state = %q, want %q", stateCookie.Value, state)
	}
}

func TestValidateOAuthStateRejectsMismatch(t *testing.T) {
	handler := &AuthHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/github/callback?state=query-state", nil)
	req.AddCookie(&http.Cookie{Name: auth.OAuthStateCookieName, Value: "cookie-state"})

	err := handler.validateOAuthState(req)
	if err == nil || !strings.Contains(err.Error(), "invalid oauth state") {
		t.Fatalf("validateOAuthState error = %v, want invalid oauth state", err)
	}
}

func TestRefreshTokenRotatesSessionAndSetsCookies(t *testing.T) {
	database := newVariableHandlerTestDB(t)
	user := &db.User{ID: db.NewID(), GithubID: 42, Username: "tester", Role: "admin"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	oldRefreshToken := "old-refresh-token"
	if err := database.CreateRefreshSession(&db.RefreshSession{
		UserID:    user.ID,
		TokenHash: auth.HashToken(oldRefreshToken),
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreateRefreshSession: %v", err)
	}

	handler := &AuthHandler{
		DB:        database,
		JWTSecret: "test-secret",
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: auth.RefreshCookieName, Value: oldRefreshToken})
	rec := httptest.NewRecorder()

	handler.RefreshToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload authResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload.User.ID != user.ID {
		t.Fatalf("user id = %q, want %q", payload.User.ID, user.ID)
	}

	result := rec.Result()
	var accessCookie *http.Cookie
	var refreshCookie *http.Cookie
	for _, cookie := range result.Cookies() {
		switch cookie.Name {
		case auth.AccessCookieName:
			accessCookie = cookie
		case auth.RefreshCookieName:
			refreshCookie = cookie
		}
	}

	if accessCookie == nil || accessCookie.Value == "" {
		t.Fatal("expected access cookie")
	}
	if refreshCookie == nil || refreshCookie.Value == "" {
		t.Fatal("expected rotated refresh cookie")
	}
	if refreshCookie.Value == oldRefreshToken {
		t.Fatal("expected refresh token rotation")
	}

	oldSession, err := database.GetActiveRefreshSessionByHash(auth.HashToken(oldRefreshToken))
	if err != nil {
		t.Fatalf("GetActiveRefreshSessionByHash(old): %v", err)
	}
	if oldSession != nil {
		t.Fatal("old refresh session should be revoked")
	}

	newSession, err := database.GetActiveRefreshSessionByHash(auth.HashToken(refreshCookie.Value))
	if err != nil {
		t.Fatalf("GetActiveRefreshSessionByHash(new): %v", err)
	}
	if newSession == nil {
		t.Fatal("expected rotated refresh session to be active")
	}
}

func TestDetermineRoleRequiresExplicitAdminWhenConfigured(t *testing.T) {
	database := newVariableHandlerTestDB(t)
	handler := &AuthHandler{
		DB:         database,
		AdminUsers: []string{"admin-user"},
	}

	role, err := handler.determineRole(nil, "plain-user")
	if err != nil {
		t.Fatalf("determineRole: %v", err)
	}
	if role != "user" {
		t.Fatalf("role = %q, want user", role)
	}

	role, err = handler.determineRole(nil, "admin-user")
	if err != nil {
		t.Fatalf("determineRole(admin): %v", err)
	}
	if role != "admin" {
		t.Fatalf("role = %q, want admin", role)
	}
}
