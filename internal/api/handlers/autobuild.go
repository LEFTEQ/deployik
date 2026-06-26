package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/lefteq/lovinka-deployik/internal/audit"
	"github.com/lefteq/lovinka-deployik/internal/auth"
	"github.com/lefteq/lovinka-deployik/internal/crypto"
	"github.com/lefteq/lovinka-deployik/internal/db"
	ghclient "github.com/lefteq/lovinka-deployik/internal/github"
)

// AutoBuildHandler manages auto-build configuration and webhook lifecycle.
type AutoBuildHandler struct {
	DB         *db.DB
	Encryptor  *crypto.Encryptor
	Audit      *audit.Recorder
	WebhookURL string
}

type autoBuildRequest struct {
	Enabled               bool   `json:"enabled"`
	ProductionBranch      string `json:"production_branch"`
	PreviewBranches       string `json:"preview_branches"`
	AutoProductionEnabled *bool  `json:"auto_production_enabled"`
}

type autoBuildResponse struct {
	Enabled               bool   `json:"enabled"`
	ProductionBranch      string `json:"production_branch"`
	PreviewBranches       string `json:"preview_branches"`
	AutoProductionEnabled bool   `json:"auto_production_enabled"`
	WebhookActive         bool   `json:"webhook_active"`
	CreatedAt             string `json:"created_at"`
	UpdatedAt             string `json:"updated_at"`
}

func defaultAutoBuildBranch(project *db.Project) string {
	if project == nil {
		return "main"
	}
	branch := strings.TrimSpace(project.Branch)
	if branch == "" {
		return "main"
	}
	return branch
}

// Get returns the auto-build configuration for a project.
func (h *AutoBuildHandler) Get(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	project, _, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}

	config, err := h.DB.GetAutoBuildConfig(projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get auto-build config"})
		return
	}

	if config == nil {
		writeJSON(w, http.StatusOK, autoBuildResponse{
			Enabled:               false,
			ProductionBranch:      defaultAutoBuildBranch(project),
			PreviewBranches:       "*",
			AutoProductionEnabled: false,
		})
		return
	}

	writeJSON(w, http.StatusOK, autoBuildResponse{
		Enabled:               config.Enabled,
		ProductionBranch:      config.ProductionBranch,
		PreviewBranches:       config.PreviewBranches,
		AutoProductionEnabled: config.AutoProductionEnabled,
		WebhookActive:         config.Enabled && config.WebhookID != nil,
		CreatedAt:             config.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:             config.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// provisionWebhook generates a webhook secret, calls GitHub to create the webhook,
// encrypts the secret, and inserts a new auto_build_configs row.
// It returns the inserted config or an error if anything fails.
// The caller is responsible for deciding how to handle the error
// (fail the request vs. log-and-continue best-effort).
func provisionWebhook(
	_ context.Context,
	database *db.DB,
	encryptor *crypto.Encryptor,
	project *db.Project,
	githubToken, webhookURL, productionBranch, previewBranches string,
	autoProductionEnabled bool,
) (*db.AutoBuildConfig, error) {
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return nil, fmt.Errorf("generate webhook secret: %w", err)
	}
	rawSecret := hex.EncodeToString(secretBytes)

	encryptedSecret, err := encryptor.Encrypt(rawSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypt webhook secret: %w", err)
	}

	log.Printf("Auto-build: creating webhook for %s/%s, url=%s", project.GithubOwner, project.GithubRepo, webhookURL)
	id, err := ghclient.NewClient(githubToken).CreateWebhook(project.GithubOwner, project.GithubRepo, webhookURL, rawSecret)
	if err != nil {
		return nil, fmt.Errorf("create webhook: %w", err)
	}

	config := &db.AutoBuildConfig{
		ProjectID:             project.ID,
		Enabled:               true,
		ProductionBranch:      productionBranch,
		PreviewBranches:       previewBranches,
		AutoProductionEnabled: autoProductionEnabled,
		WebhookID:             &id,
		WebhookSecret:         encryptedSecret,
	}
	if err := database.UpsertAutoBuildConfig(config); err != nil {
		return nil, fmt.Errorf("upsert auto-build config: %w", err)
	}
	return config, nil
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
		req.ProductionBranch = defaultAutoBuildBranch(project)
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

	autoProductionEnabled := false
	if existing != nil {
		autoProductionEnabled = existing.AutoProductionEnabled
	}
	if req.AutoProductionEnabled != nil {
		autoProductionEnabled = *req.AutoProductionEnabled
	}

	var config *db.AutoBuildConfig

	if existing == nil || existing.WebhookID == nil {
		// Create path: use the shared helper.
		config, err = provisionWebhook(r.Context(), h.DB, h.Encryptor, project, token, h.WebhookURL, req.ProductionBranch, req.PreviewBranches, autoProductionEnabled)
		if err != nil {
			errMsg := err.Error()
			log.Printf("Auto-build: webhook creation failed for %s/%s: %s", project.GithubOwner, project.GithubRepo, errMsg)
			if strings.Contains(errMsg, "404") {
				writeJSON(w, http.StatusForbidden, map[string]interface{}{
					"error":   "no_admin_access",
					"message": "You need admin access to " + project.GithubOwner + "/" + project.GithubRepo + " on GitHub to create webhooks. Ask the repo owner to add you as an admin collaborator.",
				})
				return
			}
			if strings.Contains(errMsg, "403") {
				writeJSON(w, http.StatusForbidden, map[string]interface{}{
					"error":   "insufficient_scope",
					"message": "Re-authorize with GitHub to enable auto-build.",
				})
				return
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to create webhook: " + errMsg})
			return
		}
		// Reflect the requested enabled/branch values on top of provisioned config.
		config.Enabled = req.Enabled
		config.ProductionBranch = req.ProductionBranch
		config.PreviewBranches = req.PreviewBranches
		config.AutoProductionEnabled = autoProductionEnabled
		if err := h.DB.UpsertAutoBuildConfig(config); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save auto-build config"})
			return
		}
	} else {
		// Update path: reuse existing webhook.
		// If toggling enabled state, update the webhook on GitHub.
		if existing.Enabled != req.Enabled {
			if err := ghclient.NewClient(token).UpdateWebhookActive(
				project.GithubOwner, project.GithubRepo, *existing.WebhookID, req.Enabled,
			); err != nil {
				log.Printf("Warning: failed to update webhook active state: %v", err)
			}
		}
		config = &db.AutoBuildConfig{
			ID:                    existing.ID,
			ProjectID:             project.ID,
			Enabled:               req.Enabled,
			ProductionBranch:      req.ProductionBranch,
			PreviewBranches:       req.PreviewBranches,
			AutoProductionEnabled: autoProductionEnabled,
			WebhookID:             existing.WebhookID,
			WebhookSecret:         existing.WebhookSecret,
		}
		if err := h.DB.UpsertAutoBuildConfig(config); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save auto-build config"})
			return
		}
	}

	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "autobuild.update",
		ResourceType: "auto_build_config",
		ResourceID:   config.ID,
		ProjectID:    project.ID,
		Metadata: map[string]any{
			"enabled":                 req.Enabled,
			"production_branch":       req.ProductionBranch,
			"preview_branches":        req.PreviewBranches,
			"auto_production_enabled": autoProductionEnabled,
		},
	})

	writeJSON(w, http.StatusOK, autoBuildResponse{
		Enabled:               config.Enabled,
		ProductionBranch:      config.ProductionBranch,
		PreviewBranches:       config.PreviewBranches,
		AutoProductionEnabled: config.AutoProductionEnabled,
		WebhookActive:         config.Enabled && config.WebhookID != nil,
		CreatedAt:             config.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:             config.UpdatedAt.Format("2006-01-02T15:04:05Z"),
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
