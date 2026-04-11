package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	ghclient "github.com/LEFTEQ/lovinka-deployik/internal/github"
)

// AutoBuildHandler manages auto-build configuration and webhook lifecycle.
type AutoBuildHandler struct {
	DB         *db.DB
	Encryptor  *crypto.Encryptor
	Audit      *audit.Recorder
	WebhookURL string
}

type autoBuildRequest struct {
	Enabled          bool   `json:"enabled"`
	ProductionBranch string `json:"production_branch"`
	PreviewBranches  string `json:"preview_branches"`
}

type autoBuildResponse struct {
	Enabled          bool   `json:"enabled"`
	ProductionBranch string `json:"production_branch"`
	PreviewBranches  string `json:"preview_branches"`
	WebhookActive    bool   `json:"webhook_active"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// Get returns the auto-build configuration for a project.
func (h *AutoBuildHandler) Get(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if _, _, ok := loadAuthorizedProject(w, r, h.DB, projectID); !ok {
		return
	}

	config, err := h.DB.GetAutoBuildConfig(projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get auto-build config"})
		return
	}

	if config == nil {
		writeJSON(w, http.StatusOK, autoBuildResponse{
			Enabled:          false,
			ProductionBranch: "main",
			PreviewBranches:  "*",
		})
		return
	}

	writeJSON(w, http.StatusOK, autoBuildResponse{
		Enabled:          config.Enabled,
		ProductionBranch: config.ProductionBranch,
		PreviewBranches:  config.PreviewBranches,
		WebhookActive:    config.Enabled && config.WebhookID != nil,
		CreatedAt:        config.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:        config.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// Put creates or updates the auto-build configuration.
func (h *AutoBuildHandler) Put(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	project, _, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}

	var req autoBuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.ProductionBranch == "" {
		req.ProductionBranch = "main"
	}
	if req.PreviewBranches == "" {
		req.PreviewBranches = "*"
	}

	// Get project owner's GitHub token
	user, err := h.DB.GetUserByID(project.UserID)
	if err != nil || user == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "project owner not found"})
		return
	}
	token, err := h.Encryptor.Decrypt(user.GithubToken)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to decrypt token"})
		return
	}

	existing, err := h.DB.GetAutoBuildConfig(project.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get existing config"})
		return
	}

	var webhookID *int64
	var webhookSecret string

	if existing == nil || existing.WebhookID == nil {
		// Need to create a webhook
		secretBytes := make([]byte, 32)
		if _, err := rand.Read(secretBytes); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate webhook secret"})
			return
		}
		rawSecret := hex.EncodeToString(secretBytes)

		encryptedSecret, err := h.Encryptor.Encrypt(rawSecret)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encrypt webhook secret"})
			return
		}

		log.Printf("Auto-build: creating webhook for %s/%s, url=%s", project.GithubOwner, project.GithubRepo, h.WebhookURL)
		id, err := ghclient.NewClient(token).CreateWebhook(project.GithubOwner, project.GithubRepo, h.WebhookURL, rawSecret)
		if err != nil {
			errMsg := err.Error()
			log.Printf("Auto-build: webhook creation failed for %s/%s: %s", project.GithubOwner, project.GithubRepo, errMsg)
			if strings.Contains(errMsg, "403") || strings.Contains(errMsg, "404") {
				writeJSON(w, http.StatusForbidden, map[string]interface{}{
					"error":   "insufficient_scope",
					"message": "Re-authorize with GitHub to enable auto-build. GitHub returned: " + errMsg,
				})
				return
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to create webhook: " + errMsg})
			return
		}
		webhookID = &id
		webhookSecret = encryptedSecret
	} else {
		webhookID = existing.WebhookID
		// If toggling enabled state, update the webhook on GitHub
		if existing.Enabled != req.Enabled {
			if err := ghclient.NewClient(token).UpdateWebhookActive(
				project.GithubOwner, project.GithubRepo, *existing.WebhookID, req.Enabled,
			); err != nil {
				log.Printf("Warning: failed to update webhook active state: %v", err)
			}
		}
	}

	config := &db.AutoBuildConfig{
		ProjectID:        project.ID,
		Enabled:          req.Enabled,
		ProductionBranch: req.ProductionBranch,
		PreviewBranches:  req.PreviewBranches,
		WebhookID:        webhookID,
		WebhookSecret:    webhookSecret,
	}
	if existing != nil {
		config.ID = existing.ID
	}

	if err := h.DB.UpsertAutoBuildConfig(config); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save auto-build config"})
		return
	}

	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "autobuild.update",
		ResourceType: "auto_build_config",
		ResourceID:   config.ID,
		ProjectID:    project.ID,
		Metadata: map[string]any{
			"enabled":           req.Enabled,
			"production_branch": req.ProductionBranch,
			"preview_branches":  req.PreviewBranches,
		},
	})

	writeJSON(w, http.StatusOK, autoBuildResponse{
		Enabled:          config.Enabled,
		ProductionBranch: config.ProductionBranch,
		PreviewBranches:  config.PreviewBranches,
		WebhookActive:    config.Enabled && config.WebhookID != nil,
		CreatedAt:        config.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:        config.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// Delete removes the auto-build configuration and its GitHub webhook.
func (h *AutoBuildHandler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	project, _, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}

	existing, err := h.DB.GetAutoBuildConfig(project.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get auto-build config"})
		return
	}

	if existing != nil && existing.WebhookID != nil {
		user, err := h.DB.GetUserByID(project.UserID)
		if err != nil || user == nil {
			log.Printf("Warning: could not load project owner for webhook cleanup: %v", err)
		} else {
			token, err := h.Encryptor.Decrypt(user.GithubToken)
			if err != nil {
				log.Printf("Warning: could not decrypt token for webhook cleanup: %v", err)
			} else {
				if err := ghclient.NewClient(token).DeleteWebhook(
					project.GithubOwner, project.GithubRepo, *existing.WebhookID,
				); err != nil {
					log.Printf("Warning: failed to delete webhook (may already be gone): %v", err)
				}
			}
		}
	}

	if err := h.DB.DeleteAutoBuildConfig(project.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete auto-build config"})
		return
	}

	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "autobuild.delete",
		ResourceType: "auto_build_config",
		ProjectID:    project.ID,
	})

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
