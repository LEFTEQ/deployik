# Personal Access Tokens (PATs) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add user-scoped long-lived API tokens (Personal Access Tokens) to Deployik so external tooling — primarily the `deployik-howto` skill's action mode — can authenticate to the existing protected `/api/*` endpoints with a `Authorization: Bearer dpk_<...>` header instead of the browser-bound OAuth cookie/JWT flow.

**Architecture:** One additive migration, one queries file, one handler, and a single Bearer-prefix branch in the existing `Authenticate` middleware. PATs are SHA-256-hashed at rest (matching the `refresh_tokens` pattern), shown to the user once at creation, scoped to a single user, and revocable. The existing JWT path is unchanged — PATs are an alternative auth method that uses the same `Claims` shape so every downstream handler works without modification.

**Tech Stack:** Go 1.25 (chi, modernc.org/sqlite), React 19 + TanStack Router/Query + shadcn/ui (`Dialog`, `Table`, `Button`).

**Design doc:** `docs/plans/2026-04-26-deployik-howto-skill-design.md`

---

## File Structure

**New files:**
- `internal/db/migrations/017_api_tokens.sql` — `api_tokens` table + indexes.
- `internal/db/queries_api_tokens.go` — Create, GetByHash, ListForUser, Revoke, TouchLastUsed.
- `internal/api/handlers/tokens.go` — `TokenHandler` with Create/List/Revoke.
- `internal/api/handlers/tokens_test.go` — handler tests.
- `web/src/pages/UserTokens.tsx` — token management page.

**Modified files:**
- `internal/db/models.go` — add `APIToken` struct.
- `internal/db/db_test.go` — extend `TestMigrations` table list; add `TestAPITokenCRUD`.
- `internal/auth/session.go` — add `APITokenPrefix` constant + `GenerateAPIToken()`.
- `internal/auth/session_test.go` (new) — token generation + prefix invariants.
- `internal/api/middleware/auth.go` — extend `Authenticate(jwtSecret, *db.DB)` with PAT branch.
- `internal/api/middleware/auth_test.go` (new) — middleware test for both paths.
- `internal/api/router.go` — pass `cfg.DB` into `Authenticate`; register `/me/tokens` routes.
- `web/src/types/api.ts` — `APIToken`, `CreateAPITokenRequest`, `CreateAPITokenResponse`.
- `web/src/lib/api.ts` — `listMyTokens`, `createMyToken`, `revokeMyToken`.
- `web/src/lib/queryKeys.ts` — `myTokens` key.
- `web/src/app/app.tsx` — register `/account/tokens` route.
- `web/src/components/layout/AppSidebar.tsx` — add "Account → Tokens" item under workspace nav.
- `.claude/CLAUDE.md` — document new endpoint + table.

---

## Task 1: Migration + APIToken model

**Files:**
- Create: `internal/db/migrations/017_api_tokens.sql`
- Modify: `internal/db/models.go`
- Modify: `internal/db/db_test.go`

**Why:** Establish the table shape and the Go model first so subsequent queries and handler tests can rely on it. The schema mirrors `refresh_tokens` (hash-only at rest, optional expiry, soft revoke) but adds a user-supplied `name` so the user can tell tokens apart in the management UI.

- [ ] **Step 1: Extend the migrations table list test**

Edit `internal/db/db_test.go`. Find `TestMigrations` (~line 23) and append `"api_tokens"` to the `tables` slice:

```go
tables := []string{"users", "organizations", "organization_memberships", "projects", "project_analytics", "project_email_settings", "deployments", "build_logs", "domains", "env_variables", "refresh_tokens", "audit_logs", "api_tokens", "_migrations"}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/db -run TestMigrations -v`
Expected: FAIL — `table api_tokens does not exist`.

- [ ] **Step 3: Write the migration SQL**

Create `internal/db/migrations/017_api_tokens.sql`:

```sql
-- Personal Access Tokens (PATs) for non-browser API clients.
--
-- The existing auth flow assumes a browser: GitHub OAuth issues a 1h JWT
-- (access cookie) and a 7d opaque refresh token (refresh cookie). That works
-- for the SPA but not for tools and skills that need to call the API
-- without a browser session. PATs are user-scoped long-lived bearer tokens
-- that authenticate to the same protected endpoints with the same Claims
-- shape, so every downstream handler keeps working unchanged.
--
-- Storage mirrors refresh_tokens: only the SHA-256 hex hash is persisted, the
-- raw token is shown to the user once at creation and never again. Revocation
-- is a soft set on revoked_at so we can keep an audit trail without breaking
-- foreign keys. expires_at is nullable; null means the token never expires
-- on its own (still revocable manually).

CREATE TABLE IF NOT EXISTS api_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    last_used_at DATETIME,
    expires_at DATETIME,
    revoked_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_api_tokens_user_id ON api_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_api_tokens_token_hash ON api_tokens(token_hash);
```

- [ ] **Step 4: Add the `APIToken` struct**

Edit `internal/db/models.go`. Add the struct after `AuditLog` (around line 65):

```go
type APIToken struct {
	ID         string       `json:"id"`
	UserID     string       `json:"user_id"`
	Name       string       `json:"name"`
	TokenHash  string       `json:"-"`
	LastUsedAt sql.NullTime `json:"last_used_at"`
	ExpiresAt  sql.NullTime `json:"expires_at"`
	RevokedAt  sql.NullTime `json:"revoked_at"`
	CreatedAt  time.Time    `json:"created_at"`
}
```

- [ ] **Step 5: Run the migrations test**

Run: `go test ./internal/db -run TestMigrations -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/db/migrations/017_api_tokens.sql internal/db/models.go internal/db/db_test.go
git commit -m "feat(db): add api_tokens table + APIToken model"
```

---

## Task 2: API token queries

**Files:**
- Create: `internal/db/queries_api_tokens.go`
- Modify: `internal/db/db_test.go`

**Why:** All five operations are narrow and easy to verify in isolation. `GetAPITokenByHash` must return `nil` (not an error) for not-found AND for revoked/expired tokens, so the middleware can treat "not authenticated" the same way regardless of cause.

- [ ] **Step 1: Write the failing test**

Append to `internal/db/db_test.go`:

```go
func TestAPITokenCRUD(t *testing.T) {
	database := newTestDB(t)

	user := &User{ID: NewID(), GithubID: 100, Username: "tokenowner", Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	// Create.
	token := &APIToken{
		UserID:    user.ID,
		Name:      "skill-action-mode",
		TokenHash: "hash-abc",
	}
	if err := database.CreateAPIToken(token); err != nil {
		t.Fatalf("create: %v", err)
	}
	if token.ID == "" {
		t.Fatalf("CreateAPIToken did not assign an ID")
	}

	// GetByHash returns the token.
	got, err := database.GetAPITokenByHash("hash-abc")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || got.ID != token.ID {
		t.Fatalf("get returned %v, want token %s", got, token.ID)
	}

	// ListForUser returns it.
	list, err := database.ListAPITokensForUser(user.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].ID != token.ID {
		t.Fatalf("list = %v, want 1 entry with id %s", list, token.ID)
	}

	// TouchLastUsed sets last_used_at.
	if err := database.TouchAPITokenLastUsed(token.ID); err != nil {
		t.Fatalf("touch: %v", err)
	}
	got, err = database.GetAPITokenByHash("hash-abc")
	if err != nil {
		t.Fatalf("get after touch: %v", err)
	}
	if !got.LastUsedAt.Valid {
		t.Fatalf("last_used_at not set after touch")
	}

	// Revoke makes it invisible to GetByHash.
	if err := database.RevokeAPIToken(token.ID, user.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	got, err = database.GetAPITokenByHash("hash-abc")
	if err != nil {
		t.Fatalf("get after revoke: %v", err)
	}
	if got != nil {
		t.Fatalf("get returned token after revoke")
	}

	// But ListForUser still includes it (so the UI can show it as revoked).
	list, _ = database.ListAPITokensForUser(user.ID)
	if len(list) != 1 {
		t.Fatalf("list after revoke = %d, want 1 (still visible)", len(list))
	}
	if !list[0].RevokedAt.Valid {
		t.Fatalf("revoked_at not set on listed entry")
	}

	// Revoking someone else's token must error.
	other := &User{ID: NewID(), GithubID: 101, Username: "stranger", Role: "user"}
	if err := database.UpsertUser(other); err != nil {
		t.Fatalf("upsert other: %v", err)
	}
	otherToken := &APIToken{UserID: other.ID, Name: "x", TokenHash: "hash-other"}
	if err := database.CreateAPIToken(otherToken); err != nil {
		t.Fatalf("create other token: %v", err)
	}
	if err := database.RevokeAPIToken(otherToken.ID, user.ID); err == nil {
		t.Fatalf("expected error when revoking another user's token")
	}

	// Expired tokens disappear from GetByHash even before revoke.
	expired := &APIToken{
		UserID:    user.ID,
		Name:      "expired",
		TokenHash: "hash-expired",
		ExpiresAt: sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
	}
	if err := database.CreateAPIToken(expired); err != nil {
		t.Fatalf("create expired: %v", err)
	}
	got, _ = database.GetAPITokenByHash("hash-expired")
	if got != nil {
		t.Fatalf("get returned expired token")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/db -run TestAPITokenCRUD -v`
Expected: FAIL — `CreateAPIToken` undefined.

- [ ] **Step 3: Implement the queries**

Create `internal/db/queries_api_tokens.go`:

```go
package db

import (
	"database/sql"
	"fmt"
	"time"
)

// CreateAPIToken inserts a new token. The caller must set TokenHash; the raw
// token is never persisted. ID is auto-assigned if empty.
func (db *DB) CreateAPIToken(token *APIToken) error {
	if token.ID == "" {
		token.ID = NewID()
	}
	var expiresAt any
	if token.ExpiresAt.Valid {
		expiresAt = token.ExpiresAt.Time
	} else {
		expiresAt = nil
	}
	_, err := db.Exec(
		`INSERT INTO api_tokens (id, user_id, name, token_hash, expires_at)
		 VALUES (?, ?, ?, ?, ?)`,
		token.ID, token.UserID, token.Name, token.TokenHash, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("create api token: %w", err)
	}
	return nil
}

// GetAPITokenByHash returns the token only when it is active: not revoked,
// not past its expires_at. Not-found, revoked, and expired all collapse to
// (nil, nil) so the middleware can treat them identically.
func (db *DB) GetAPITokenByHash(tokenHash string) (*APIToken, error) {
	t := &APIToken{}
	err := db.QueryRow(
		`SELECT id, user_id, name, token_hash, last_used_at, expires_at, revoked_at, created_at
		 FROM api_tokens
		 WHERE token_hash = ? AND revoked_at IS NULL`,
		tokenHash,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.LastUsedAt, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get api token: %w", err)
	}
	if t.ExpiresAt.Valid && time.Now().After(t.ExpiresAt.Time) {
		return nil, nil
	}
	return t, nil
}

// ListAPITokensForUser returns ALL tokens for a user, including revoked and
// expired ones — the UI shows them with status badges so the user can audit
// what was issued.
func (db *DB) ListAPITokensForUser(userID string) ([]APIToken, error) {
	rows, err := db.Query(
		`SELECT id, user_id, name, token_hash, last_used_at, expires_at, revoked_at, created_at
		 FROM api_tokens
		 WHERE user_id = ?
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list api tokens: %w", err)
	}
	defer rows.Close()

	var tokens []APIToken
	for rows.Next() {
		var t APIToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.LastUsedAt, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan api token: %w", err)
		}
		tokens = append(tokens, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows api tokens: %w", err)
	}
	return tokens, nil
}

// RevokeAPIToken sets revoked_at on a token, but only when the requesting
// user owns it — a strict ownership check guards against IDOR even though
// the caller is already authenticated.
func (db *DB) RevokeAPIToken(id, userID string) error {
	result, err := db.Exec(
		`UPDATE api_tokens
		 SET revoked_at = datetime('now')
		 WHERE id = ? AND user_id = ? AND revoked_at IS NULL`,
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("revoke api token: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("revoke rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// TouchAPITokenLastUsed is fire-and-forget from the middleware. Errors are
// non-fatal — a missed touch is acceptable; an auth failure is not.
func (db *DB) TouchAPITokenLastUsed(id string) error {
	_, err := db.Exec(
		`UPDATE api_tokens SET last_used_at = datetime('now') WHERE id = ?`,
		id,
	)
	if err != nil {
		return fmt.Errorf("touch api token: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run the test**

Run: `go test ./internal/db -run TestAPITokenCRUD -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/queries_api_tokens.go internal/db/db_test.go
git commit -m "feat(db): api_tokens CRUD + revoke + touch + expiry-aware lookup"
```

---

## Task 3: Token generation primitives in `auth` package

**Files:**
- Modify: `internal/auth/session.go`
- Create: `internal/auth/session_test.go`

**Why:** Centralize the `dpk_` prefix and the random-byte size next to the existing `GenerateOpaqueToken` so middleware and handler agree on the format. `dpk_` is a short fixed prefix that lets the middleware decide which auth path to take without parsing the token.

- [ ] **Step 1: Write the failing test**

Create `internal/auth/session_test.go`:

```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/auth -run TestGenerateAPIToken -v`
Expected: FAIL — `GenerateAPIToken` and `APITokenPrefix` undefined.

- [ ] **Step 3: Add the helpers**

Edit `internal/auth/session.go`. Inside the existing `const ( ... )` block (alongside `AccessCookieName`, `RefreshCookieName`, `OAuthStateCookieName`), add a blank-line-separated entry with this comment + constant:

```go
	// APITokenPrefix prefixes Personal Access Tokens so middleware can route
	// Bearer values to the api_tokens lookup path without trying to parse them
	// as JWTs first. The prefix is stable forever — changing it would invalidate
	// every issued token.
	APITokenPrefix = "dpk_"
```

After `HashToken`, add:

```go
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
```

- [ ] **Step 4: Run the test**

Run: `go test ./internal/auth -v`
Expected: both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/session.go internal/auth/session_test.go
git commit -m "feat(auth): GenerateAPIToken + APITokenPrefix"
```

---

## Task 4: Middleware extension — Bearer PAT branch

**Files:**
- Modify: `internal/api/middleware/auth.go`
- Create: `internal/api/middleware/auth_test.go`

**Why:** The existing `Authenticate` middleware only validates JWTs. It needs a second path that, when the Bearer token starts with `dpk_`, hashes it and looks it up in `api_tokens`, then synthesizes the same `*auth.Claims` shape so every downstream handler — `loadAuthorizedProject`, audit recorder, etc. — works without modification. The existing JWT path is unchanged.

- [ ] **Step 1: Write the failing tests**

Create `internal/api/middleware/auth_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/api/middleware -run TestAuthenticate -v`
Expected: tests fail to compile because `Authenticate` currently takes only one argument.

- [ ] **Step 3: Extend the middleware**

Replace the contents of `internal/api/middleware/auth.go` with:

```go
package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// Authenticate extracts and validates a credential from the request and
// stores the resulting Claims in the context. Two credential types are
// accepted:
//
//   - JWTs minted by GenerateAccessToken (the existing browser/cookie path).
//   - Personal Access Tokens prefixed with auth.APITokenPrefix (Bearer header
//     only — PATs are not allowed via cookie).
//
// PATs are routed by prefix before any JWT parse attempt so a malformed PAT
// cannot accidentally fall through to the JWT branch and produce a confusing
// error.
func Authenticate(jwtSecret string, database *db.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := ExtractAccessToken(r)
			if tokenStr == "" {
				http.Error(w, `{"error":"missing authorization token"}`, http.StatusUnauthorized)
				return
			}

			var claims *auth.Claims
			var err error

			if strings.HasPrefix(tokenStr, auth.APITokenPrefix) && isBearer(r) {
				claims, err = authenticateAPIToken(database, tokenStr)
			} else {
				claims, err = auth.ValidateAccessToken(jwtSecret, tokenStr)
			}
			if err != nil || claims == nil {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := auth.WithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// isBearer guards PAT acceptance to the Authorization header so PATs cannot
// be smuggled into a cookie (which would cross the CSRF boundary).
func isBearer(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ")
}

func authenticateAPIToken(database *db.DB, raw string) (*auth.Claims, error) {
	if database == nil {
		return nil, errors.New("db unavailable")
	}
	token, err := database.GetAPITokenByHash(auth.HashToken(raw))
	if err != nil {
		return nil, err
	}
	if token == nil {
		return nil, errors.New("token not found")
	}
	user, err := database.GetUserByID(token.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("token owner missing")
	}

	// Fire-and-forget last_used update — auth must not block on it.
	go func(id string) {
		if err := database.TouchAPITokenLastUsed(id); err != nil {
			// Logged-not-fatal at this layer; auth already succeeded.
			_ = err
		}
	}(token.ID)

	return &auth.Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
	}, nil
}

func ExtractAccessToken(r *http.Request) string {
	bearer := r.Header.Get("Authorization")
	if strings.HasPrefix(bearer, "Bearer ") {
		return strings.TrimPrefix(bearer, "Bearer ")
	}

	if cookie, err := r.Cookie(auth.AccessCookieName); err == nil && cookie.Value != "" {
		return cookie.Value
	}

	return ""
}
```

- [ ] **Step 4: Update the single caller in router.go**

Edit `internal/api/router.go`. Find the line in the protected group:

```go
r.Use(middleware.Authenticate(cfg.JWTSecret))
```

Replace with:

```go
r.Use(middleware.Authenticate(cfg.JWTSecret, cfg.DB))
```

- [ ] **Step 5: Run the middleware tests**

Run: `go test ./internal/api/middleware -run TestAuthenticate -v`
Expected: all five tests PASS.

- [ ] **Step 6: Run the full test suite to ensure no regression**

Run: `go test ./...`
Expected: PASS (unchanged JWT path still works for every other handler test).

- [ ] **Step 7: Commit**

```bash
git add internal/api/middleware/auth.go internal/api/middleware/auth_test.go internal/api/router.go
git commit -m "feat(middleware): accept dpk_-prefixed Bearer PATs alongside JWT"
```

---

## Task 5: `TokenHandler` — Create / List / Revoke

**Files:**
- Create: `internal/api/handlers/tokens.go`
- Create: `internal/api/handlers/tokens_test.go`

**Why:** Three endpoints under `/api/me/tokens`. Create returns the raw token exactly once. List masks `token_hash` (already hidden via JSON tag) and returns metadata only. Revoke is a soft delete that surfaces 404 when the token doesn't belong to the caller — never leaking whether the id exists.

- [ ] **Step 1: Write the failing tests**

Create `internal/api/handlers/tokens_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/api/handlers -run TestToken -v`
Expected: FAIL — `TokenHandler` undefined.

- [ ] **Step 3: Implement the handler**

Create `internal/api/handlers/tokens.go`:

```go
package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

type TokenHandler struct {
	DB    *db.DB
	Audit *audit.Recorder
}

type createTokenRequest struct {
	Name string `json:"name"`
}

type createTokenResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Token string `json:"token"`
}

func (h *TokenHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}

	var req createTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if len(name) > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name must be 100 characters or less"})
		return
	}

	raw, err := auth.GenerateAPIToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}

	token := &db.APIToken{
		UserID:    claims.UserID,
		Name:      name,
		TokenHash: auth.HashToken(raw),
	}
	if err := h.DB.CreateAPIToken(token); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create token"})
		return
	}

	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "api_token.create",
		ResourceType: "api_token",
		ResourceID:   token.ID,
		Metadata:     map[string]any{"name": name},
	})

	writeJSON(w, http.StatusCreated, createTokenResponse{
		ID:    token.ID,
		Name:  token.Name,
		Token: raw,
	})
}

func (h *TokenHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	tokens, err := h.DB.ListAPITokensForUser(claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list tokens"})
		return
	}
	if tokens == nil {
		tokens = []db.APIToken{}
	}
	writeJSON(w, http.StatusOK, tokens)
}

func (h *TokenHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing token id"})
		return
	}
	err := h.DB.RevokeAPIToken(id, claims.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		// Either the token doesn't exist, isn't owned by this user, or is
		// already revoked — collapse all three into 404 so the response
		// doesn't tell unauthenticated callers which IDs exist.
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke token"})
		return
	}

	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "api_token.revoke",
		ResourceType: "api_token",
		ResourceID:   id,
	})

	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/api/handlers -run TestToken -v`
Expected: all five PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/handlers/tokens.go internal/api/handlers/tokens_test.go
git commit -m "feat(api): /api/me/tokens — create, list, revoke"
```

---

## Task 6: Wire the routes

**Files:**
- Modify: `internal/api/router.go`

**Why:** Three new routes under the protected group, behind the standard mutation rate-limiter for the writing verbs.

- [ ] **Step 1: Add the routes**

Edit `internal/api/router.go`. Inside the protected group (after `r.Get("/auth/me", authHandler.GetMe)` near line 114), add:

```go
// Personal Access Tokens — used by the deployik-howto skill and any
// future external tooling that needs Bearer auth without a browser session.
tokenHandler := &handlers.TokenHandler{DB: cfg.DB, Audit: auditRecorder}
r.Get("/me/tokens", tokenHandler.List)
r.With(mutationLimiter.Middleware("token_create")).Post("/me/tokens", tokenHandler.Create)
r.With(mutationLimiter.Middleware("token_revoke")).Delete("/me/tokens/{id}", tokenHandler.Revoke)
```

- [ ] **Step 2: Build to verify**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 3: Run the full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/api/router.go
git commit -m "feat(api): wire /api/me/tokens routes"
```

---

## Task 7: Frontend types + API client + query keys

**Files:**
- Modify: `web/src/types/api.ts`
- Modify: `web/src/lib/api.ts`
- Modify: `web/src/lib/queryKeys.ts`

**Why:** TypeScript shapes and three client methods, ready for the page to use. `createMyToken` returns the response with the **raw token** included — distinct from `APIToken` which never includes the raw value.

- [ ] **Step 1: Add the types**

Edit `web/src/types/api.ts`. After the existing types (e.g. after `User`), add:

```ts
export interface APIToken {
  id: string;
  user_id: string;
  name: string;
  last_used_at: string | null;
  expires_at: string | null;
  revoked_at: string | null;
  created_at: string;
}

export interface CreateAPITokenRequest {
  name: string;
}

export interface CreateAPITokenResponse {
  id: string;
  name: string;
  /** Raw token value — shown to the user once at creation, never stored. */
  token: string;
}
```

- [ ] **Step 2: Add the API client methods**

Edit `web/src/lib/api.ts`. Extend the import block to include the new types:

```ts
import type {
  // …existing imports…
  APIToken,
  CreateAPITokenRequest,
  CreateAPITokenResponse,
} from "@/types/api";
```

Add these methods on `ApiClient` (after `getMe`, around line 120):

```ts
async listMyTokens(): Promise<APIToken[]> {
  return this.request<APIToken[]>("/me/tokens");
}

async createMyToken(req: CreateAPITokenRequest): Promise<CreateAPITokenResponse> {
  return this.request<CreateAPITokenResponse>("/me/tokens", {
    method: "POST",
    body: JSON.stringify(req),
  });
}

async revokeMyToken(id: string): Promise<void> {
  await this.request<void>(`/me/tokens/${id}`, { method: "DELETE" });
}
```

- [ ] **Step 3: Add the query key**

Edit `web/src/lib/queryKeys.ts`. Add a `myTokens` key to the existing exports (the file uses an object pattern; add this entry next to similar singular keys):

```ts
myTokens: ["me", "tokens"] as const,
```

If the file uses `as const` arrays directly: add `export const myTokensKey = ["me", "tokens"] as const;`. Use whichever pattern matches the file's existing style — open the file, mirror it, don't introduce a new convention.

- [ ] **Step 4: Typecheck**

Run: `cd web && bunx tsc --noEmit`
Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/types/api.ts web/src/lib/api.ts web/src/lib/queryKeys.ts
git commit -m "feat(web): APIToken types + client methods + query key"
```

---

## Task 8: `UserTokens` page + sidebar entry + route

**Files:**
- Create: `web/src/pages/UserTokens.tsx`
- Modify: `web/src/app/app.tsx`
- Modify: `web/src/components/layout/AppSidebar.tsx`

**Why:** The user needs a place to mint and revoke tokens. The page lists existing tokens with revoke buttons and a "Create token" button that opens a dialog. On creation, the raw token is shown once with a Copy button; closing the dialog clears it.

- [ ] **Step 1: Build the page**

Create `web/src/pages/UserTokens.tsx`:

```tsx
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, KeyRound, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import type { APIToken } from "@/types/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";

function statusOf(token: APIToken): { label: string; tone: "active" | "revoked" | "expired" } {
  if (token.revoked_at) return { label: "Revoked", tone: "revoked" };
  if (token.expires_at && new Date(token.expires_at) < new Date()) {
    return { label: "Expired", tone: "expired" };
  }
  return { label: "Active", tone: "active" };
}

export function UserTokens() {
  const queryClient = useQueryClient();
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [createdToken, setCreatedToken] = useState<string | null>(null);

  const tokensQuery = useQuery({
    queryKey: queryKeys.myTokens,
    queryFn: () => api.listMyTokens(),
  });

  const createMutation = useMutation({
    mutationFn: (n: string) => api.createMyToken({ name: n }),
    onSuccess: (resp) => {
      setCreatedToken(resp.token);
      setName("");
      queryClient.invalidateQueries({ queryKey: queryKeys.myTokens });
    },
    onError: (err) => toast.error(err.message),
  });

  const revokeMutation = useMutation({
    mutationFn: (id: string) => api.revokeMyToken(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.myTokens });
      toast.success("Token revoked");
    },
    onError: (err) => toast.error(err.message),
  });

  const tokens = tokensQuery.data ?? [];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Personal access tokens</h1>
          <p className="text-sm text-muted-foreground">
            Long-lived bearer tokens for tools and skills that call the Deployik API.
            Each token has the same permissions as your account. Treat them like passwords.
          </p>
        </div>
        <Button onClick={() => setCreating(true)}>
          <KeyRound className="mr-2 h-4 w-4" />
          Create token
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Your tokens</CardTitle>
        </CardHeader>
        <CardContent>
          {tokensQuery.isLoading ? (
            <p className="text-sm text-muted-foreground">Loading...</p>
          ) : tokens.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No tokens yet. Create one to use the Deployik API from outside the dashboard.
            </p>
          ) : (
            <ul className="divide-y divide-border">
              {tokens.map((token) => {
                const status = statusOf(token);
                const lastUsed = token.last_used_at
                  ? new Date(token.last_used_at).toLocaleString()
                  : "Never used";
                return (
                  <li key={token.id} className="flex items-center justify-between py-3">
                    <div className="space-y-0.5">
                      <div className="flex items-center gap-2">
                        <span className="font-medium">{token.name}</span>
                        <Badge
                          variant={status.tone === "active" ? "outline" : "secondary"}
                          className={
                            status.tone === "revoked"
                              ? "border-red-500/40 text-red-300"
                              : status.tone === "expired"
                                ? "border-amber-500/40 text-amber-200"
                                : "border-emerald-500/40 text-emerald-200"
                          }
                        >
                          {status.label}
                        </Badge>
                      </div>
                      <p className="text-xs text-muted-foreground">
                        Created {new Date(token.created_at).toLocaleDateString()} ·{" "}
                        {lastUsed}
                      </p>
                    </div>
                    {status.tone === "active" ? (
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => revokeMutation.mutate(token.id)}
                        disabled={revokeMutation.isPending}
                      >
                        <Trash2 className="mr-1.5 h-3.5 w-3.5" />
                        Revoke
                      </Button>
                    ) : null}
                  </li>
                );
              })}
            </ul>
          )}
        </CardContent>
      </Card>

      {/* Create dialog (name input → create) */}
      <Dialog
        open={creating && createdToken === null}
        onOpenChange={(open) => {
          if (!open) {
            setCreating(false);
            setName("");
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create token</DialogTitle>
            <DialogDescription>
              Give the token a name so you can recognize it later. The token value
              is shown once and never again.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="token-name">Name</Label>
            <Input
              id="token-name"
              placeholder="e.g. deployik-howto skill"
              value={name}
              onChange={(e) => setName(e.target.value)}
              autoFocus
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreating(false)}>
              Cancel
            </Button>
            <Button
              onClick={() => createMutation.mutate(name.trim())}
              disabled={!name.trim() || createMutation.isPending}
            >
              {createMutation.isPending ? "Creating..." : "Create"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Reveal dialog (shown once, contains the raw token) */}
      <Dialog
        open={createdToken !== null}
        onOpenChange={(open) => {
          if (!open) {
            setCreatedToken(null);
            setCreating(false);
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Token created</DialogTitle>
            <DialogDescription>
              Copy this token now — it will never be shown again. Store it somewhere
              safe (e.g. <code>~/.config/deployik/config</code> for the deployik-howto skill).
            </DialogDescription>
          </DialogHeader>
          <div className="rounded-md border bg-muted/40 p-3 font-mono text-sm break-all">
            {createdToken}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                if (createdToken) {
                  void navigator.clipboard.writeText(createdToken);
                  toast.success("Copied to clipboard");
                }
              }}
            >
              <Copy className="mr-2 h-4 w-4" />
              Copy
            </Button>
            <Button
              onClick={() => {
                setCreatedToken(null);
                setCreating(false);
              }}
            >
              Done
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
```

- [ ] **Step 2: Register the route**

Edit `web/src/app/app.tsx`. Add a lazy import after the other lazy imports:

```ts
const UserTokens = lazy(() =>
  import("@/pages/UserTokens").then((m) => ({ default: m.UserTokens })),
);
```

Add a new route under the workspace layout (next to `indexRoute`):

```ts
const userTokensRoute = createRoute({
  getParentRoute: () => workspaceLayoutRoute,
  path: "/account/tokens",
  component: UserTokens,
});
```

Update the `routeTree` definition to include it. Find:

```ts
workspaceLayoutRoute.addChildren([indexRoute]),
```

Replace with:

```ts
workspaceLayoutRoute.addChildren([indexRoute, userTokensRoute]),
```

- [ ] **Step 3: Add the sidebar item**

Edit `web/src/components/layout/AppSidebar.tsx`. Find `getWorkspaceItems`:

```ts
function getWorkspaceItems(): NavItem[] {
  return [
    {
      label: "Projects",
      icon: FolderKanban,
      to: "/",
      matchPath: (p) => p === "/",
    },
  ];
}
```

Replace with:

```ts
function getWorkspaceItems(): NavItem[] {
  return [
    {
      label: "Projects",
      icon: FolderKanban,
      to: "/",
      matchPath: (p) => p === "/",
    },
    {
      label: "Access tokens",
      icon: KeyRound,
      to: "/account/tokens",
      matchPath: (p) => p === "/account/tokens",
    },
  ];
}
```

`KeyRound` is already imported at the top of the file (used elsewhere); confirm it appears in the lucide-react import list — if not, add it.

- [ ] **Step 4: Typecheck**

Run: `cd web && bunx tsc --noEmit`
Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/UserTokens.tsx web/src/app/app.tsx web/src/components/layout/AppSidebar.tsx
git commit -m "feat(web): UserTokens page + sidebar entry + /account/tokens route"
```

---

## Task 9: End-to-end verification + living docs

**Why:** Prove the whole flow works against a running dev server. Update CLAUDE.md so future Claude sessions know about the new endpoint, table, and migration.

- [ ] **Step 1: Start dev servers**

Run in one terminal: `make dev-api`
Run in another: `make dev-web`
Run once if DB is empty: `make dev-seed`

- [ ] **Step 2: Exercise the flow in the browser**

1. Sign in via dev-login.
2. In the sidebar, click **Access tokens** → page loads with empty state.
3. Click **Create token**. Enter name `e2e-test`. Submit.
4. The reveal dialog shows a `dpk_...` token. Click **Copy**. Click **Done**.
5. Token appears in the list with status **Active** and "Never used".
6. In a terminal, exercise the token:

   ```bash
   curl -sS -H "Authorization: Bearer <paste-token>" http://localhost:8080/api/projects | head
   ```

   Expected: a JSON list of projects (200), not an auth error.
7. Refresh the tokens page. The "Last used" line should now show a recent timestamp.
8. Click **Revoke** on the token. Confirm it switches to **Revoked** badge.
9. Run the curl again — now it returns 401.

- [ ] **Step 3: Run the full Go test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 4: Update living-docs**

Edit `.claude/CLAUDE.md`. In `## API Endpoints → Protected`, add a new subsection (after `**Domains:**` or wherever fits the alphabetical-ish order):

```markdown
**Personal Access Tokens (PATs):**
- `GET    /api/me/tokens` -- List the caller's tokens (raw values never returned)
- `POST   /api/me/tokens` -- Create a token `{name}`; returns `{id, name, token}` once
- `DELETE /api/me/tokens/{id}` -- Revoke a token (soft delete)
```

In the Database Schema table, add a new row after `audit_logs`:

```markdown
| `api_tokens` | id (ULID), user_id (FK), name, token_hash, last_used_at, expires_at, revoked_at | SHA-256 hashed at rest; raw token shown once at creation; routed by `dpk_` prefix in middleware |
```

In the migration list:

```markdown
      017_api_tokens.sql        Adds api_tokens for Personal Access Tokens (Bearer auth alongside JWT)
```

In the Auth bullet (under Stack), update:

```markdown
- **Auth:** GitHub OAuth (scope: `repo,read:user,admin:repo_hook`) -> JWT (HS256, 1h access / 7d refresh tokens). Personal Access Tokens (`dpk_<...>`) accepted via Authorization Bearer for non-browser clients.
```

- [ ] **Step 5: Final commit**

```bash
git add .claude/CLAUDE.md
git commit -m "docs: document /api/me/tokens, api_tokens table, dpk_ Bearer support"
```

---

## Self-review checklist results

**Spec coverage** — verified:
- Migration `015_api_tokens.sql` from design doc → implemented as `017_api_tokens.sql` (numbers 015 and 016 already taken by `env_variable_updated_at` and `project_email_settings`); table shape unchanged ✓
- `internal/db/queries_api_tokens.go` ✓ (Task 2)
- `/api/me/tokens` CRUD ✓ (Task 5 + Task 6 wires)
- Bearer middleware extension ✓ (Task 4)
- Audit on create/revoke ✓ (Task 5)
- Rate limit on POST/DELETE ✓ (Task 6 uses `mutationLimiter`)
- SPA management UI with create-once reveal ✓ (Task 8)

**Type consistency** — `APIToken` (Go) ↔ `APIToken` (TS) field names match (`token_hash` is JSON-hidden in Go via `json:"-"` and absent from the TS type). `CreateAPITokenResponse.token` is the only place the raw value appears.

**Placeholder scan** — no "TBD", no "similar to above", no "implement later". Every code block is complete.

**Open questions parked from design doc** (resolved here):
- *Token expiry default.* v1 ships with **no expiry** (`ExpiresAt` nullable, UI doesn't expose a picker yet). Adding an expiry picker is a 5-line follow-up.
- *Token scopes.* v1 = full user-scoped. Per-project scoping deferred — issue can be filed after this lands.
- *Sidebar location.* Workspace nav under "Access tokens" — follows the existing `getWorkspaceItems` pattern. No new top-level nav category.
- *Helper script distribution.* Out of scope for Plan A; covered by Plan B (`deployik-howto` skill).
