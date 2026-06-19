package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// AppVariableHandler serves app-scoped env vars / secrets (/apps/{id}/env and
// /apps/{id}/secrets). It mirrors VariableHandler but is owned by an app and
// gated on app-organization membership. Two instances are registered, one per
// Kind, exactly like the project variable stores.
type AppVariableHandler struct {
	DB        *db.DB
	Encryptor *crypto.Encryptor
	Kind      db.VariableKind
}

func (h *AppVariableHandler) storeLabelPlural() string {
	if h.Kind == db.VariableKindSecret {
		return "secrets"
	}
	return "env vars"
}

func (h *AppVariableHandler) oppositeKind() db.VariableKind {
	if h.Kind == db.VariableKindSecret {
		return db.VariableKindEnv
	}
	return db.VariableKindSecret
}

func (h *AppVariableHandler) oppositeStoreLabelPlural() string {
	if h.Kind == db.VariableKindSecret {
		return "env vars"
	}
	return "secrets"
}

// loadMemberApp resolves the URL app id and verifies org membership.
func (h *AppVariableHandler) loadMemberApp(w http.ResponseWriter, r *http.Request) (*db.App, bool) {
	claims := auth.GetClaims(r.Context())
	app, err := h.DB.GetAppForUser(chi.URLParam(r, "id"), claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load app"})
		return nil, false
	}
	if app == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "app not found"})
		return nil, false
	}
	return app, true
}

func (h *AppVariableHandler) validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("key is required")
	}
	if !variableKeyPattern.MatchString(key) {
		return fmt.Errorf("key must contain only letters, numbers, and underscores, and cannot start with a number")
	}
	if h.Kind == db.VariableKindSecret && strings.HasPrefix(key, "NEXT_PUBLIC_") {
		return fmt.Errorf("secret keys cannot use NEXT_PUBLIC_ because secrets are runtime-only")
	}
	return nil
}

func (h *AppVariableHandler) ensureNoStoreConflicts(appID string, keys []string) error {
	existingKeys, err := h.DB.ListAppVariableKeys(appID, h.oppositeKind())
	if err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(existingKeys))
	for _, key := range existingKeys {
		existing[key] = struct{}{}
	}
	for _, key := range keys {
		if _, conflict := existing[key]; conflict {
			return fmt.Errorf("%s already exists in the %s store; a key can belong to only one store per app", key, h.oppositeStoreLabelPlural())
		}
	}
	return nil
}

// List returns an app's variables for one scope with masked values.
func (h *AppVariableHandler) List(w http.ResponseWriter, r *http.Request) {
	app, ok := h.loadMemberApp(w, r)
	if !ok {
		return
	}
	environment, err := normalizeVariableEnvironment(r.URL.Query().Get("environment"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	vars, err := h.DB.ListAppVariables(app.ID, environment, h.Kind)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to list %s", h.storeLabelPlural())})
		return
	}
	result := make([]map[string]any, 0, len(vars))
	for _, variable := range vars {
		masked := "****"
		if decrypted, err := h.Encryptor.Decrypt(variable.Value); err == nil {
			masked = crypto.MaskValue(decrypted)
		}
		result = append(result, map[string]any{
			"id":          variable.ID,
			"app_id":      variable.AppID,
			"environment": variable.Environment,
			"kind":        variable.Kind,
			"key":         variable.Key,
			"value":       masked,
			"created_at":  variable.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

// BulkSet replaces all of an app's variables for one scope and store.
func (h *AppVariableHandler) BulkSet(w http.ResponseWriter, r *http.Request) {
	app, ok := h.loadMemberApp(w, r)
	if !ok {
		return
	}
	var req bulkSetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	environment, err := normalizeVariableEnvironment(req.Environment)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	keys := make([]string, 0, len(req.Variables))
	seen := make(map[string]struct{}, len(req.Variables))
	encrypted := make([]db.AppVariable, 0, len(req.Variables))
	for _, variable := range req.Variables {
		key := strings.TrimSpace(variable.Key)
		if key == "" {
			continue
		}
		if err := h.validateKey(key); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if _, dup := seen[key]; dup {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("duplicate key %s", key)})
			return
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
		encryptedValue, err := h.Encryptor.Encrypt(variable.Value)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encryption failed"})
			return
		}
		encrypted = append(encrypted, db.AppVariable{Key: key, Value: encryptedValue})
	}
	if err := h.ensureNoStoreConflicts(app.ID, keys); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if err := h.DB.BulkSetAppVariables(app.ID, environment, h.Kind, encrypted); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to save %s", h.storeLabelPlural())})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(encrypted), "environment": environment, "kind": h.Kind})
}

// Upsert adds or updates a single app variable.
func (h *AppVariableHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	app, ok := h.loadMemberApp(w, r)
	if !ok {
		return
	}
	var req struct {
		Key         string `json:"key"`
		Value       string `json:"value"`
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	key := strings.TrimSpace(req.Key)
	if err := h.validateKey(key); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	environment, err := normalizeVariableEnvironment(req.Environment)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := h.ensureNoStoreConflicts(app.ID, []string{key}); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	encryptedValue, err := h.Encryptor.Encrypt(req.Value)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encryption failed"})
		return
	}
	if err := h.DB.UpsertAppVariable(&db.AppVariable{AppID: app.ID, Environment: environment, Kind: h.Kind, Key: key, Value: encryptedValue}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to save %s", h.storeLabelPlural())})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved", "key": key, "environment": environment})
}

// Delete removes a single app variable from one scope and store.
func (h *AppVariableHandler) Delete(w http.ResponseWriter, r *http.Request) {
	app, ok := h.loadMemberApp(w, r)
	if !ok {
		return
	}
	key := strings.TrimSpace(chi.URLParam(r, "key"))
	if err := h.validateKey(key); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	environment, err := normalizeVariableEnvironment(r.URL.Query().Get("environment"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := h.DB.DeleteAppVariable(app.ID, environment, key, h.Kind); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to delete %s", h.storeLabelPlural())})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
