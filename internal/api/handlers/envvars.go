package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

type EnvVarHandler struct {
	DB        *db.DB
	Encryptor *crypto.Encryptor
}

type envVarEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type bulkSetRequest struct {
	Environment string        `json:"environment"`
	Variables   []envVarEntry `json:"variables"`
}

// List returns env vars for a project+environment with masked values.
func (h *EnvVarHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	env := r.URL.Query().Get("environment")
	if env == "" {
		env = "preview"
	}

	vars, err := h.DB.ListEnvVars(projectID, env)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list env vars"})
		return
	}

	// Mask values in the response
	type maskedVar struct {
		ID          string `json:"id"`
		Key         string `json:"key"`
		Value       string `json:"value"`
		Environment string `json:"environment"`
	}
	result := make([]maskedVar, 0, len(vars))
	for _, v := range vars {
		decrypted, err := h.Encryptor.Decrypt(v.Value)
		masked := "****"
		if err == nil {
			masked = crypto.MaskValue(decrypted)
		}
		result = append(result, maskedVar{
			ID:          v.ID,
			Key:         v.Key,
			Value:       masked,
			Environment: v.Environment,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// BulkSet replaces all env vars for a project+environment.
func (h *EnvVarHandler) BulkSet(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	var req bulkSetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	env := req.Environment
	if env == "" {
		env = "preview"
	}

	// Encrypt all values
	encrypted := make([]db.EnvVariable, 0, len(req.Variables))
	for _, v := range req.Variables {
		if v.Key == "" {
			continue
		}
		encValue, err := h.Encryptor.Encrypt(v.Value)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encryption failed"})
			return
		}
		encrypted = append(encrypted, db.EnvVariable{
			ProjectID:   projectID,
			Environment: env,
			Key:         v.Key,
			Value:       encValue,
		})
	}

	if err := h.DB.BulkSetEnvVars(projectID, env, encrypted); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save env vars"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":       len(encrypted),
		"environment": env,
	})
}

// Delete removes a single env var.
func (h *EnvVarHandler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	key := chi.URLParam(r, "key")
	env := r.URL.Query().Get("environment")
	if env == "" {
		env = "preview"
	}

	if err := h.DB.DeleteEnvVar(projectID, env, key); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete env var"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
