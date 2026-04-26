package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

func newTokenTestHandler(t *testing.T) (*TokenHandler, *db.DB, *db.User) {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	user := &db.User{ID: db.NewID(), GithubID: 7, Username: "owner", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	h := &TokenHandler{DB: database, Audit: &audit.Recorder{DB: database}}
	return h, database, user
}

func withClaims(req *http.Request, userID, role string) *http.Request {
	ctx := auth.WithClaims(req.Context(), &auth.Claims{UserID: userID, Role: role})
	return req.WithContext(ctx)
}

func withChiID(ctx context.Context, key, value string) context.Context {
	rc := chi.NewRouteContext()
	rc.URLParams.Add(key, value)
	return context.WithValue(ctx, chi.RouteCtxKey, rc)
}

func TestTokenCreateReturnsRawTokenOnce(t *testing.T) {
	h, database, user := newTokenTestHandler(t)
	body, _ := json.Marshal(map[string]any{"name": "test-token"})
	req := withClaims(httptest.NewRequest(http.MethodPost, "/api/me/tokens", bytes.NewReader(body)), user.ID, user.Role)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	raw, _ := resp["token"].(string)
	if !strings.HasPrefix(raw, "dpk_") {
		t.Fatalf("token field missing dpk_ prefix: %q", raw)
	}
	if resp["id"] == "" {
		t.Fatalf("id missing")
	}

	// Verify the token actually authenticates: hash it, look it up.
	got, err := database.GetAPITokenByHash(auth.HashToken(raw))
	if err != nil || got == nil {
		t.Fatalf("token not stored: %v", err)
	}
	if got.UserID != user.ID {
		t.Fatalf("token owner = %q, want %q", got.UserID, user.ID)
	}
}

func TestTokenCreateRejectsBlankName(t *testing.T) {
	h, _, user := newTokenTestHandler(t)
	body, _ := json.Marshal(map[string]any{"name": "  "})
	req := withClaims(httptest.NewRequest(http.MethodPost, "/api/me/tokens", bytes.NewReader(body)), user.ID, user.Role)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestTokenListReturnsOwnedOnly(t *testing.T) {
	h, database, user := newTokenTestHandler(t)
	stranger := &db.User{ID: db.NewID(), GithubID: 8, Username: "stranger", Role: "user"}
	if err := database.UpsertUser(stranger); err != nil {
		t.Fatalf("upsert stranger: %v", err)
	}
	if err := database.CreateAPIToken(&db.APIToken{UserID: user.ID, Name: "mine", TokenHash: "h1"}); err != nil {
		t.Fatalf("create mine: %v", err)
	}
	if err := database.CreateAPIToken(&db.APIToken{UserID: stranger.ID, Name: "theirs", TokenHash: "h2"}); err != nil {
		t.Fatalf("create theirs: %v", err)
	}
	req := withClaims(httptest.NewRequest(http.MethodGet, "/api/me/tokens", nil), user.ID, user.Role)
	w := httptest.NewRecorder()
	h.List(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var tokens []db.APIToken
	if err := json.Unmarshal(w.Body.Bytes(), &tokens); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(tokens) != 1 || tokens[0].Name != "mine" {
		t.Fatalf("list = %+v, want only my token", tokens)
	}
	// token_hash must not leak.
	rawBody := w.Body.String()
	if strings.Contains(rawBody, "h1") {
		t.Fatalf("token_hash leaked in list response: %s", rawBody)
	}
}

func TestTokenRevokeOwnSucceeds(t *testing.T) {
	h, database, user := newTokenTestHandler(t)
	token := &db.APIToken{UserID: user.ID, Name: "to-revoke", TokenHash: "h-rev"}
	if err := database.CreateAPIToken(token); err != nil {
		t.Fatalf("create: %v", err)
	}
	req := withClaims(httptest.NewRequest(http.MethodDelete, "/api/me/tokens/"+token.ID, nil), user.ID, user.Role)
	req = req.WithContext(withChiID(req.Context(), "id", token.ID))
	w := httptest.NewRecorder()
	h.Revoke(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	got, _ := database.GetAPITokenByHash("h-rev")
	if got != nil {
		t.Fatalf("token still active after revoke")
	}
}

func TestTokenRevokeOthersReturns404(t *testing.T) {
	h, database, user := newTokenTestHandler(t)
	stranger := &db.User{ID: db.NewID(), GithubID: 9, Username: "stranger", Role: "user"}
	if err := database.UpsertUser(stranger); err != nil {
		t.Fatalf("upsert stranger: %v", err)
	}
	token := &db.APIToken{UserID: stranger.ID, Name: "their-token", TokenHash: "h-their"}
	if err := database.CreateAPIToken(token); err != nil {
		t.Fatalf("create: %v", err)
	}
	req := withClaims(httptest.NewRequest(http.MethodDelete, "/api/me/tokens/"+token.ID, nil), user.ID, user.Role)
	req = req.WithContext(withChiID(req.Context(), "id", token.ID))
	w := httptest.NewRecorder()
	h.Revoke(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (not 403 — don't leak existence)", w.Code)
	}
}
