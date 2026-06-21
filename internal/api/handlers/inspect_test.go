// Package handlers — InspectHandler tests
//
// Coverage note: adapter translation (gh.ErrNotFound → monorepo.ErrFileNotFound)
// is exercised end-to-end by internal/monorepo/inspect_test.go (Inspect
// orchestrator's not-found handling) and the github client's 404 path is
// covered by internal/github/client_test.go. We intentionally do not duplicate
// that coverage here — the handler tests focus on the HTTP/auth wiring.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/monorepo"
)

func TestInspectHandler_MissingBranch(t *testing.T) {
	h := &InspectHandler{} // DB and Encryptor unused — check fires before DB access

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("owner", "LEFTEQ")
	rctx.URLParams.Add("repo", "acme")

	req := httptest.NewRequest(http.MethodGet, "/api/github/repos/LEFTEQ/acme/inspect", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "branch query parameter is required" {
		t.Fatalf("unexpected error message: %q", body["error"])
	}
}

func TestInspectHandler_HappyPath(t *testing.T) {
	// Set up in-memory DB and migrate.
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Create encryptor and encrypt a fake GitHub token.
	enc, err := crypto.NewEncryptor("test-encryption-key")
	if err != nil {
		t.Fatalf("new encryptor: %v", err)
	}
	encryptedToken, err := enc.Encrypt("fake-github-token")
	if err != nil {
		t.Fatalf("encrypt token: %v", err)
	}

	// Insert a user with the encrypted token.
	user := &db.User{
		ID:          db.NewID(),
		GithubID:    12345,
		Username:    "testuser",
		AvatarURL:   "",
		GithubToken: encryptedToken,
		Role:        "user",
	}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	// Known report the fake InspectFn will return.
	wantReport := &monorepo.Report{
		IsMonorepo:     false,
		PackageManager: "bun",
		Tooling:        []monorepo.Tooling{},
		Apps: []monorepo.App{
			{
				Name:                  "acme",
				Path:                  "",
				Framework:             "nextjs",
				OutputDirectory:       ".next",
				SuggestedBuildCommand: "bun run build",
				Buildable:             true,
			},
		},
	}

	h := &InspectHandler{
		DB:        database,
		Encryptor: enc,
		InspectFn: func(ctx context.Context, _ monorepo.RepoInspector, owner, repo, ref string) (*monorepo.Report, error) {
			if owner != "LEFTEQ" || repo != "acme" || ref != "main" {
				t.Errorf("unexpected inspect args: owner=%q repo=%q ref=%q", owner, repo, ref)
			}
			return wantReport, nil
		},
	}

	// Build request with chi route context.
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("owner", "LEFTEQ")
	rctx.URLParams.Add("repo", "acme")

	req := httptest.NewRequest(http.MethodGet, "/api/github/repos/LEFTEQ/acme/inspect?branch=main", nil)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = auth.WithClaims(ctx, &auth.Claims{UserID: user.ID})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var gotReport monorepo.Report
	if err := json.NewDecoder(w.Body).Decode(&gotReport); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if gotReport.IsMonorepo != wantReport.IsMonorepo {
		t.Errorf("IsMonorepo: got %v, want %v", gotReport.IsMonorepo, wantReport.IsMonorepo)
	}
	if gotReport.PackageManager != wantReport.PackageManager {
		t.Errorf("PackageManager: got %q, want %q", gotReport.PackageManager, wantReport.PackageManager)
	}
	if len(gotReport.Apps) != 1 || gotReport.Apps[0].Framework != wantReport.Apps[0].Framework {
		t.Errorf("Apps mismatch: got %+v", gotReport.Apps)
	}
}
