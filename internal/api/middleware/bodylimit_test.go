package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBodyLimitRejectsOversizedPOST(t *testing.T) {
	mw := BodyLimit(100)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			http.Error(w, "too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	oversized := bytes.Repeat([]byte("x"), 500)
	req := httptest.NewRequest("POST", "/api/projects", bytes.NewReader(oversized))
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rr.Code)
	}
}

func TestBodyLimitPassesWithinSizePOST(t *testing.T) {
	mw := BodyLimit(100)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			http.Error(w, "too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/api/projects", strings.NewReader("small"))
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestBodyLimitSkipsExemptWebhookPath(t *testing.T) {
	mw := BodyLimit(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handler will re-wrap with its own larger limit; middleware must not
		// cap the body ahead of it.
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "too large", http.StatusRequestEntityTooLarge)
			return
		}
		if len(body) != 500 {
			http.Error(w, "truncated", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	oversized := bytes.Repeat([]byte("x"), 500)
	req := httptest.NewRequest("POST", "/api/webhooks/github", bytes.NewReader(oversized))
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 (webhook path exempt), got %d", rr.Code)
	}
}

func TestBodyLimitIgnoresGET(t *testing.T) {
	mw := BodyLimit(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/api/projects", nil)
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}
