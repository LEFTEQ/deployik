package middleware

import (
	"net/http"
	"strings"
)

// DefaultBodyLimit is applied to all API requests that carry a body. Individual
// routes can raise the limit by chaining BodyLimit(n) explicitly.
const DefaultBodyLimit = 1 << 20 // 1 MiB

// bodyLimitExemptPrefixes lists URL prefixes where BodyLimit should be bypassed
// because the handler itself wraps the body with a route-specific MaxBytesReader
// (e.g., GitHub webhooks accept payloads up to 10 MiB).
var bodyLimitExemptPrefixes = []string{
	"/api/webhooks/",
}

// BodyLimit wraps r.Body with http.MaxBytesReader(n), so an oversized request
// body returns 413 instead of consuming unbounded memory in json.Decode.
// GET/HEAD/DELETE/OPTIONS requests (which rarely carry a body) are passed
// through untouched.
func BodyLimit(limit int64) func(http.Handler) http.Handler {
	if limit <= 0 {
		limit = DefaultBodyLimit
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, prefix := range bodyLimitExemptPrefixes {
				if strings.HasPrefix(r.URL.Path, prefix) {
					next.ServeHTTP(w, r)
					return
				}
			}
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch:
				if r.Body != nil {
					r.Body = http.MaxBytesReader(w, r.Body, limit)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
