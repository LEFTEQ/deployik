package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

type TokenHandler struct {
	DB    *db.DB
	Audit *audit.Recorder
}

type createTokenRequest struct {
	Name string `json:"name"`
}

type createTokenResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Token string `json:"token"`
}

func (h *TokenHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}

	var req createTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if len(name) > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name must be 100 characters or less"})
		return
	}

	raw, err := auth.GenerateAPIToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}

	token := &db.APIToken{
		UserID:    claims.UserID,
		Name:      name,
		TokenHash: auth.HashToken(raw),
	}
	if err := h.DB.CreateAPIToken(token); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create token"})
		return
	}

	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "api_token.create",
		ResourceType: "api_token",
		ResourceID:   token.ID,
		Metadata:     map[string]any{"name": name},
	})

	writeJSON(w, http.StatusCreated, createTokenResponse{
		ID:    token.ID,
		Name:  token.Name,
		Token: raw,
	})
}

func (h *TokenHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	tokens, err := h.DB.ListAPITokensForUser(claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list tokens"})
		return
	}
	if tokens == nil {
		tokens = []db.APIToken{}
	}
	writeJSON(w, http.StatusOK, tokens)
}

func (h *TokenHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing token id"})
		return
	}
	err := h.DB.RevokeAPIToken(id, claims.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		// Either the token doesn't exist, isn't owned by this user, or is
		// already revoked — collapse all three into 404 so the response
		// doesn't tell unauthenticated callers which IDs exist.
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke token"})
		return
	}

	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "api_token.revoke",
		ResourceType: "api_token",
		ResourceID:   id,
	})

	w.WriteHeader(http.StatusNoContent)
}
