package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSPAHandlerCacheHeaders(t *testing.T) {
	SetStaticFS(fstest.MapFS{
		"index.html":           {Data: []byte("<!doctype html>")},
		"sw.js":                {Data: []byte("// sw")},
		"manifest.webmanifest": {Data: []byte("{}")},
		"assets/app-abc123.js": {Data: []byte("// app")},
	})
	t.Cleanup(func() { SetStaticFS(nil) })

	handler := SPAHandler()

	tests := []struct {
		path            string
		wantCache       string
		wantContentType string
	}{
		// sw.js must be revalidated every launch or clients pin to a stale shell.
		{path: "/sw.js", wantCache: "no-cache", wantContentType: "application/javascript"},
		// "/" maps to index.html internally (literal /index.html 301s to /).
		{path: "/", wantCache: "no-cache", wantContentType: "text/html"},
		{path: "/manifest.webmanifest", wantCache: "no-cache", wantContentType: "application/manifest+json"},
		{path: "/assets/app-abc123.js", wantCache: "public, max-age=31536000, immutable", wantContentType: "application/javascript"},
		// SPA fallback for client-side routes serves index.html uncached.
		{path: "/projects/some-id", wantCache: "no-cache", wantContentType: "text/html"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			handler(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if got := rec.Header().Get("Cache-Control"); got != tc.wantCache {
				t.Errorf("Cache-Control = %q, want %q", got, tc.wantCache)
			}
			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, tc.wantContentType) {
				t.Errorf("Content-Type = %q, want prefix %q", got, tc.wantContentType)
			}
		})
	}
}
