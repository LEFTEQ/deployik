package middleware

import (
	"net/http"
	"strings"

	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
)

// Authenticate extracts and validates the JWT from the Authorization header.
// If valid, stores the claims in the request context.
func Authenticate(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := ExtractAccessToken(r)
			if tokenStr == "" {
				http.Error(w, `{"error":"missing authorization token"}`, http.StatusUnauthorized)
				return
			}

			claims, err := auth.ValidateAccessToken(jwtSecret, tokenStr)
			if err != nil {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := auth.WithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func ExtractAccessToken(r *http.Request) string {
	// Check Authorization header
	bearer := r.Header.Get("Authorization")
	if strings.HasPrefix(bearer, "Bearer ") {
		return strings.TrimPrefix(bearer, "Bearer ")
	}

	if cookie, err := r.Cookie(auth.AccessCookieName); err == nil && cookie.Value != "" {
		return cookie.Value
	}

	return ""
}
