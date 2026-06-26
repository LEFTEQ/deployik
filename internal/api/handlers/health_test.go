package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lefteq/lovinka-deployik/internal/version"
)

func TestHealthHandler_WithVersion(t *testing.T) {
	h := &HealthHandler{
		Version: version.New(
			"abc1234567890fedcba0123456789abcdef01234",
			"2026-04-19T10:23:11Z",
			"12345678",
			"lefteq/lovinka-deployik",
		),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type: got %q, want application/json", ct)
	}

	var body struct {
		Status  string        `json:"status"`
		Version *version.Info `json:"version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status field: got %q, want %q", body.Status, "ok")
	}
	if body.Version == nil {
		t.Fatal("version: missing from response")
	}
	if body.Version.GitSHA != "abc1234" {
		t.Errorf("version.git_sha: got %q, want %q", body.Version.GitSHA, "abc1234")
	}
	if body.Version.CommitURL != "https://github.com/lefteq/lovinka-deployik/commit/abc1234567890fedcba0123456789abcdef01234" {
		t.Errorf("version.commit_url: got %q", body.Version.CommitURL)
	}
	if body.Version.RunURL != "https://github.com/lefteq/lovinka-deployik/actions/runs/12345678" {
		t.Errorf("version.run_url: got %q", body.Version.RunURL)
	}
}

func TestHealthHandler_WithoutVersion(t *testing.T) {
	// Defensive: nil Version (older deploys, tests) must not panic and must
	// still report status:ok so docker healthchecks keep working.
	h := &HealthHandler{Version: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	var body struct {
		Status  string          `json:"status"`
		Version json.RawMessage `json:"version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status: got %q, want ok", body.Status)
	}
	if len(body.Version) != 0 && string(body.Version) != "null" {
		t.Errorf("version: expected null/omitted, got %s", string(body.Version))
	}
}
