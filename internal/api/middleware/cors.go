package middleware

import (
	"net/http"
	"strings"
)

func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimRight(r.Header.Get("Origin"), "/")
			if origin != "" {
				w.Header().Add("Vary", "Origin")
				if !OriginAllowed(origin, allowedOrigins) {
					http.Error(w, `{"error":"origin not allowed"}`, http.StatusForbidden)
					return
				}

				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func OriginAllowed(origin string, allowedOrigins []string) bool {
	origin = strings.TrimRight(origin, "/")
	if origin == "" {
		return true
	}
	for _, candidate := range allowedOrigins {
		if origin == strings.TrimRight(candidate, "/") {
			return true
		}
	}
	return false
}
