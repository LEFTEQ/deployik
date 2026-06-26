package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/lefteq/lovinka-deployik/internal/auth"
	"github.com/lefteq/lovinka-deployik/internal/crypto"
	"github.com/lefteq/lovinka-deployik/internal/db"
	gh "github.com/lefteq/lovinka-deployik/internal/github"
	"github.com/lefteq/lovinka-deployik/internal/monorepo"
)

// InspectHandler exposes GET /api/github/repos/{owner}/{repo}/inspect.
// It uses the authenticated user's GitHub OAuth token to inspect the repo
// at the specified branch and returns a monorepo.Report describing detected
// workspace structure and per-app build profiles.
type InspectHandler struct {
	DB        *db.DB
	Encryptor *crypto.Encryptor
	// InspectFn is the seam tests inject. In production it is set to
	// monorepo.Inspect; tests can supply a fake. Leave nil to use the
	// production implementation.
	InspectFn func(ctx context.Context, gh monorepo.RepoInspector, owner, repo, ref string) (*monorepo.Report, error)
}

func (h *InspectHandler) Get(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	branch := r.URL.Query().Get("branch")
	if owner == "" || repo == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "owner and repo are required"})
		return
	}
	if branch == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "branch query parameter is required"})
		return
	}

	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}

	user, err := h.DB.GetUserByID(claims.UserID)
	if err != nil || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user not found"})
		return
	}
	token, err := h.Encryptor.Decrypt(user.GithubToken)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to decrypt token"})
		return
	}

	inspector := newGitHubInspector(gh.NewClient(token))

	fn := h.InspectFn
	if fn == nil {
		fn = monorepo.Inspect
	}
	report, err := fn(r.Context(), inspector, owner, repo, branch)
	if err != nil {
		log.Printf("inspect %s/%s@%s: %v", owner, repo, branch, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to inspect repository"})
		return
	}

	writeJSON(w, http.StatusOK, report)
}

// gitHubInspectorAdapter bridges *gh.Client to monorepo.RepoInspector by
// translating gh.ErrNotFound into monorepo.ErrFileNotFound. Without this,
// monorepo.Inspect would treat absent files as hard errors.
type gitHubInspectorAdapter struct {
	c *gh.Client
}

func newGitHubInspector(c *gh.Client) monorepo.RepoInspector {
	return &gitHubInspectorAdapter{c: c}
}

func (a *gitHubInspectorAdapter) GetTree(ctx context.Context, owner, repo, ref string) ([]string, bool, error) {
	paths, truncated, err := a.c.GetTree(ctx, owner, repo, ref)
	if errors.Is(err, gh.ErrNotFound) {
		return nil, false, monorepo.ErrFileNotFound
	}
	return paths, truncated, err
}

func (a *gitHubInspectorAdapter) GetFileContent(ctx context.Context, owner, repo, ref, path string) ([]byte, error) {
	out, err := a.c.GetFileContent(ctx, owner, repo, ref, path)
	if errors.Is(err, gh.ErrNotFound) {
		return nil, monorepo.ErrFileNotFound
	}
	return out, err
}
