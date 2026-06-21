package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetFileContent_HappyPath(t *testing.T) {
	wantContent := []byte(`{"name": "@acme/web"}`)
	encoded := base64.StdEncoding.EncodeToString(wantContent)

	// GitHub wraps at 60 chars; simulate that.
	var wrapped strings.Builder
	for i, ch := range encoded {
		if i > 0 && i%60 == 0 {
			wrapped.WriteByte('\n')
		}
		wrapped.WriteRune(ch)
	}
	wrapped.WriteByte('\n')

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Authorization header.
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization header = %q, want %q", got, "Bearer test-token")
		}
		// Verify URL path and query.
		if !strings.Contains(r.URL.Path, "apps/web/package.json") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("ref") != "main" {
			t.Errorf("ref query = %q, want %q", r.URL.Query().Get("ref"), "main")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(contentsResponse{
			Type:     "file",
			Encoding: "base64",
			Content:  wrapped.String(),
		})
	}))
	defer ts.Close()

	oldBase := apiBase
	apiBase = ts.URL
	defer func() { apiBase = oldBase }()

	c := NewClient("test-token")
	got, err := c.GetFileContent(context.Background(), "owner", "repo", "main", "apps/web/package.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(wantContent) {
		t.Errorf("content = %q, want %q", got, wantContent)
	}

	// Empty content field should return []byte{} without error.
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(contentsResponse{
			Type:     "file",
			Encoding: "base64",
			Content:  "",
		})
	}))
	defer ts2.Close()

	apiBase = ts2.URL
	got2, err2 := c.GetFileContent(context.Background(), "owner", "repo", "main", "empty.txt")
	if err2 != nil {
		t.Fatalf("unexpected error for empty file: %v", err2)
	}
	if len(got2) != 0 {
		t.Errorf("expected empty slice, got %v", got2)
	}
}

func TestGetFileContent_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	oldBase := apiBase
	apiBase = ts.URL
	defer func() { apiBase = oldBase }()

	c := NewClient("test-token")
	got, err := c.GetFileContent(context.Background(), "owner", "repo", "main", "missing.json")
	if got != nil {
		t.Errorf("expected nil bytes, got %v", got)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetTree_HappyPath(t *testing.T) {
	// Use a 40-char hex SHA so SHA resolution is skipped.
	const sha = "aabbccddeeff00112233445566778899aabbccdd"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should hit the trees endpoint.
		if !strings.Contains(r.URL.Path, "git/trees/"+sha) {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("recursive") != "1" {
			t.Errorf("missing recursive=1 query param")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(treeResponse{
			SHA: sha,
			Tree: []treeEntry{
				{Path: "README.md", Type: "blob"},
				{Path: "apps/web/package.json", Type: "blob"},
			},
			Truncated: false,
		})
	}))
	defer ts.Close()

	oldBase := apiBase
	apiBase = ts.URL
	defer func() { apiBase = oldBase }()

	c := NewClient("test-token")
	paths, truncated, err := c.GetTree(context.Background(), "owner", "repo", sha)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if truncated {
		t.Errorf("expected truncated=false, got true")
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d: %v", len(paths), paths)
	}
	want := map[string]bool{"README.md": true, "apps/web/package.json": true}
	for _, p := range paths {
		if !want[p] {
			t.Errorf("unexpected path %q", p)
		}
	}
}
