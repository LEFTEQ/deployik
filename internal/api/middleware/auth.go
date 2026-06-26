package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/lefteq/lovinka-deployik/internal/auth"
	"github.com/lefteq/lovinka-deployik/internal/db"
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

// AuthenticateToken applies the same JWT-or-PAT decision used by the HTTP
// Authenticate middleware. WebSocket handlers call it directly because they
// can't sit behind chi middleware (the upgrade response is written by the
// handler itself).
//
// PATs are still gated to Bearer headers (no cookies / no query) for the same
// CSRF reason — the caller must pass the `bearer` flag derived from isBearer.
func AuthenticateToken(database *db.DB, jwtSecret, tokenStr string, bearer bool) (*auth.Claims, error) {
	if tokenStr == "" {
		return nil, errors.New("empty token")
	}
	if strings.HasPrefix(tokenStr, auth.APITokenPrefix) && bearer {
		return authenticateAPIToken(database, tokenStr)
	}
	return auth.ValidateAccessToken(jwtSecret, tokenStr)
}

// IsBearer is the exported variant of isBearer used by callers outside this
// package (WS handlers).
func IsBearer(r *http.Request) bool {
	return isBearer(r)
}
