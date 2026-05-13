package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

type VariableHandler struct {
	DB        *db.DB
	Encryptor *crypto.Encryptor
	Kind      db.VariableKind
	Audit     *audit.Recorder
}

type variableEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type bulkSetRequest struct {
	Environment string          `json:"environment"`
	Variables   []variableEntry `json:"variables"`
}

type variableResponse struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	Environment string          `json:"environment"`
	Kind        db.VariableKind `json:"kind"`
	Key         string          `json:"key"`
	Value       string          `json:"value"`
	CreatedAt   time.Time       `json:"created_at"`
}

var variableKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func normalizeVariableEnvironment(value string) (string, error) {
	environment := strings.TrimSpace(value)
	if environment == "" {
		return "preview", nil
	}

	switch environment {
	case "shared", "preview", "production":
		return environment, nil
	default:
		return "", fmt.Errorf("environment must be shared, preview, or production")
	}
}

func (h *VariableHandler) storeLabel() string {
	if h.Kind == db.VariableKindSecret {
		return "secret"
	}
	return "env var"
}

func (h *VariableHandler) storeLabelPlural() string {
	if h.Kind == db.VariableKindSecret {
		return "secrets"
	}
	return "env vars"
}

func (h *VariableHandler) oppositeKind() db.VariableKind {
	if h.Kind == db.VariableKindSecret {
		return db.VariableKindEnv
	}
	return db.VariableKindSecret
}

func (h *VariableHandler) oppositeStoreLabelPlural() string {
	if h.Kind == db.VariableKindSecret {
		return "env vars"
	}
	return "secrets"
}

func (h *VariableHandler) listProjectVariables(projectID, environment string) ([]db.ProjectVariable, error) {
	if h.Kind == db.VariableKindSecret {
		return h.DB.ListSecrets(projectID, environment)
	}
	return h.DB.ListEnvVars(projectID, environment)
}

func (h *VariableHandler) bulkSetProjectVariables(projectID, environment string, vars []db.ProjectVariable) error {
	if h.Kind == db.VariableKindSecret {
		return h.DB.BulkSetSecrets(projectID, environment, vars)
	}
	return h.DB.BulkSetEnvVars(projectID, environment, vars)
}

func (h *VariableHandler) deleteProjectVariable(projectID, environment, key string) error {
	if h.Kind == db.VariableKindSecret {
		return h.DB.DeleteSecret(projectID, environment, key)
	}
	return h.DB.DeleteEnvVar(projectID, environment, key)
}

func (h *VariableHandler) validateVariableKey(key string) error {
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

func (h *VariableHandler) ensureNoStoreConflicts(projectID string, keys []string) error {
	existingKeys, err := h.DB.ListProjectVariableKeys(projectID, h.oppositeKind())
	if err != nil {
		return err
	}

	existing := make(map[string]struct{}, len(existingKeys))
	for _, key := range existingKeys {
		existing[key] = struct{}{}
	}

	for _, key := range keys {
		if _, conflict := existing[key]; conflict {
			return fmt.Errorf("%s already exists in the %s store; a key can belong to only one store per project", key, h.oppositeStoreLabelPlural())
		}
	}

	return nil
}

// List returns project variables for one scope with masked values.
func (h *VariableHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if _, _, ok := loadAuthorizedProject(w, r, h.DB, projectID); !ok {
		return
	}
	environment, err := normalizeVariableEnvironment(r.URL.Query().Get("environment"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	vars, err := h.listProjectVariables(projectID, environment)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to list %s", h.storeLabelPlural())})
		return
	}

	result := make([]variableResponse, 0, len(vars))
	for _, variable := range vars {
		decrypted, err := h.Encryptor.Decrypt(variable.Value)
		masked := "****"
		if err == nil {
			masked = crypto.MaskValue(decrypted)
		}
		result = append(result, variableResponse{
			ID:          variable.ID,
			ProjectID:   variable.ProjectID,
			Environment: variable.Environment,
			Kind:        variable.Kind,
			Key:         variable.Key,
			Value:       masked,
			CreatedAt:   variable.CreatedAt,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// BulkSet replaces all project variables for one scope and store.
func (h *VariableHandler) BulkSet(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if _, _, ok := loadAuthorizedProject(w, r, h.DB, projectID); !ok {
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

	normalizedKeys := make([]string, 0, len(req.Variables))
	seenKeys := make(map[string]struct{}, len(req.Variables))
	encrypted := make([]db.ProjectVariable, 0, len(req.Variables))

	for _, variable := range req.Variables {
		key := strings.TrimSpace(variable.Key)
		if key == "" {
			continue
		}
		if err := h.validateVariableKey(key); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if _, exists := seenKeys[key]; exists {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("duplicate key %s", key)})
			return
		}
		seenKeys[key] = struct{}{}
		normalizedKeys = append(normalizedKeys, key)

		encryptedValue, err := h.Encryptor.Encrypt(variable.Value)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encryption failed"})
			return
		}
		encrypted = append(encrypted, db.ProjectVariable{
			ProjectID:   projectID,
			Environment: environment,
			Kind:        h.Kind,
			Key:         key,
			Value:       encryptedValue,
		})
	}

	if err := h.ensureNoStoreConflicts(projectID, normalizedKeys); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	if err := h.bulkSetProjectVariables(projectID, environment, encrypted); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to save %s", h.storeLabelPlural())})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":       len(encrypted),
		"environment": environment,
		"kind":        h.Kind,
	})
	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       string(h.Kind) + ".bulk_set",
		ResourceType: string(h.Kind),
		ResourceID:   projectID + ":" + environment,
		ProjectID:    projectID,
		Metadata: map[string]any{
			"environment": environment,
			"count":       len(encrypted),
		},
	})
}

// Upsert adds or updates a single variable in one scope and store.
func (h *VariableHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if _, _, ok := loadAuthorizedProject(w, r, h.DB, projectID); !ok {
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
	if err := h.validateVariableKey(key); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	environment, err := normalizeVariableEnvironment(req.Environment)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := h.ensureNoStoreConflicts(projectID, []string{key}); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	encryptedValue, err := h.Encryptor.Encrypt(req.Value)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encryption failed"})
		return
	}

	v := &db.ProjectVariable{
		ProjectID:   projectID,
		Environment: environment,
		Kind:        h.Kind,
		Key:         key,
		Value:       encryptedValue,
	}
	if err := h.DB.UpsertProjectVariable(v); err != nil {
		log.Printf("envvars upsert: project=%s scope=%s key=%s kind=%s: %v", projectID, environment, key, h.Kind, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to save %s: %s", h.storeLabel(), err.Error())})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved", "key": key, "environment": environment})
	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       string(h.Kind) + ".upsert",
		ResourceType: string(h.Kind),
		ResourceID:   key,
		ProjectID:    projectID,
		Metadata: map[string]any{
			"environment": environment,
			"key":         key,
		},
	})
}

// Delete removes a single project variable from one scope and store.
func (h *VariableHandler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if _, _, ok := loadAuthorizedProject(w, r, h.DB, projectID); !ok {
		return
	}
	key := strings.TrimSpace(chi.URLParam(r, "key"))
	if err := h.validateVariableKey(key); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	environment, err := normalizeVariableEnvironment(r.URL.Query().Get("environment"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := h.deleteProjectVariable(projectID, environment, key); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to delete %s", h.storeLabel())})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       string(h.Kind) + ".delete",
		ResourceType: string(h.Kind),
		ResourceID:   key,
		ProjectID:    projectID,
		Metadata: map[string]any{
			"environment": environment,
			"key":         key,
		},
	})
}
