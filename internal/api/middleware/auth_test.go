package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

const testJWTSecret = "test-secret"

func newAuthTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func sinkHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.GetClaims(r.Context())
		if claims == nil {
			http.Error(w, "no claims", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(claims.UserID))
	})
}

func TestAuthenticateAcceptsValidJWT(t *testing.T) {
	database := newAuthTestDB(t)
	user := &db.User{ID: db.NewID(), GithubID: 1, Username: "u", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	jwtStr, err := auth.GenerateAccessToken(testJWTSecret, user.ID, user.Username, user.Role)
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	req.Header.Set("Authorization", "Bearer "+jwtStr)
	w := httptest.NewRecorder()
	Authenticate(testJWTSecret, database)(sinkHandler()).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if w.Body.String() != user.ID {
		t.Fatalf("body = %q, want %q", w.Body.String(), user.ID)
	}
}

func TestAuthenticateAcceptsValidPAT(t *testing.T) {
	database := newAuthTestDB(t)
	user := &db.User{ID: db.NewID(), GithubID: 2, Username: "patowner", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	raw, err := auth.GenerateAPIToken()
	if err != nil {
		t.Fatalf("gen pat: %v", err)
	}
	if err := database.CreateAPIToken(&db.APIToken{
		UserID: user.ID, Name: "test", TokenHash: auth.HashToken(raw),
	}); err != nil {
		t.Fatalf("create pat: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	w := httptest.NewRecorder()
	Authenticate(testJWTSecret, database)(sinkHandler()).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if w.Body.String() != user.ID {
		t.Fatalf("claims user_id = %q, want %q", w.Body.String(), user.ID)
	}

	// Wait briefly so the fire-and-forget last_used touch has a chance to land.
	time.Sleep(50 * time.Millisecond)
	got, _ := database.GetAPITokenByHash(auth.HashToken(raw))
	if got == nil || !got.LastUsedAt.Valid {
		t.Fatalf("last_used_at not updated")
	}
}

func TestAuthenticateRejectsRevokedPAT(t *testing.T) {
	database := newAuthTestDB(t)
	user := &db.User{ID: db.NewID(), GithubID: 3, Username: "rev", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	raw, _ := auth.GenerateAPIToken()
	token := &db.APIToken{UserID: user.ID, Name: "x", TokenHash: auth.HashToken(raw)}
	if err := database.CreateAPIToken(token); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := database.RevokeAPIToken(token.ID, user.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	w := httptest.NewRecorder()
	Authenticate(testJWTSecret, database)(sinkHandler()).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestAuthenticateRejectsUnknownPAT(t *testing.T) {
	database := newAuthTestDB(t)
	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	req.Header.Set("Authorization", "Bearer "+auth.APITokenPrefix+"completely-bogus")
	w := httptest.NewRecorder()
	Authenticate(testJWTSecret, database)(sinkHandler()).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestAuthenticateRejectsMissing(t *testing.T) {
	database := newAuthTestDB(t)
	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	w := httptest.NewRecorder()
	Authenticate(testJWTSecret, database)(sinkHandler()).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}
