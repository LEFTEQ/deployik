package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// WebhookHandler processes incoming GitHub webhook events.
type WebhookHandler struct {
	DB        *db.DB
	Encryptor *crypto.Encryptor
	Pipeline  *build.Pipeline
}

type githubPushPayload struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Deleted    bool   `json:"deleted"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	Pusher struct {
		Name string `json:"name"`
	} `json:"pusher"`
	HeadCommit *struct {
		Message string `json:"message"`
	} `json:"head_commit"`
}

// HandleGithub processes GitHub webhook push events and triggers deployments.
func (h *WebhookHandler) HandleGithub(w http.ResponseWriter, r *http.Request) {
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType != "push" {
		w.WriteHeader(http.StatusOK)
		return
	}

	deliveryID := r.Header.Get("X-GitHub-Delivery")
	if deliveryID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing delivery ID"})
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 10*1024*1024))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}

	var payload githubPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}

	if payload.Deleted {
		w.WriteHeader(http.StatusOK)
		return
	}

	parts := strings.SplitN(payload.Repository.FullName, "/", 2)
	if len(parts) != 2 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid repository name"})
		return
	}
	owner, repo := parts[0], parts[1]

	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")

	signatureHeader := r.Header.Get("X-Hub-Signature-256")

	configs, err := h.DB.ListActiveAutoBuildConfigsByRepo(owner, repo)
	if err != nil {
		log.Printf("Webhook: failed to list configs for %s/%s: %v", owner, repo, err)
		w.WriteHeader(http.StatusOK)
		return
	}

	for _, config := range configs {
		targets := webhookTargetEnvironments(branch, config)

		secret, err := h.Encryptor.Decrypt(config.WebhookSecret)
		if err != nil {
			log.Printf("Webhook: failed to decrypt secret for project %s: %v", config.ProjectID, err)
			continue
		}

		if !validateGithubSignature(secret, body, signatureHeader) {
			log.Printf("Webhook: invalid signature for project %s", config.ProjectID)
			continue
		}

		if len(targets) == 0 {
			claimed, err := claimWebhookEvent(h.DB, webhookEventPayload(config, deliveryID, "ignored", branch, payload, "ignored", "", nil))
			if err != nil {
				log.Printf("Webhook: failed to claim ignored event for project %s: %v", config.ProjectID, err)
			}
			if !claimed {
				continue
			}
			continue
		}

		project, err := h.DB.GetProject(config.ProjectID)
		if err != nil || project == nil {
			log.Printf("Webhook: project %s not found: %v", config.ProjectID, err)
			continue
		}

		user, err := h.DB.GetUserByID(project.UserID)
		if err != nil || user == nil {
			log.Printf("Webhook: project owner not found for %s: %v", config.ProjectID, err)
			continue
		}
		githubToken, err := h.Encryptor.Decrypt(user.GithubToken)
		if err != nil {
			log.Printf("Webhook: failed to decrypt token for project %s: %v", config.ProjectID, err)
			continue
		}

		for _, environment := range targets {
			claimed, err := claimWebhookEvent(h.DB, webhookEventPayload(config, deliveryID, environment, branch, payload, "received", "", nil))
			if err != nil {
				log.Printf("Webhook: failed to claim %s webhook event for project %s: %v", environment, config.ProjectID, err)
				continue
			}
			if !claimed {
				continue
			}

			deployment := &db.Deployment{
				ProjectID:           project.ID,
				Environment:         environment,
				Branch:              branch,
				CommitSHA:           payload.After,
				CommitMessage:       commitMessageFromPayload(payload),
				Status:              "queued",
				TriggerSource:       "webhook",
				TriggeredByUsername: payload.Pusher.Name,
				TriggeredBy:         project.UserID,
			}
			if err := h.DB.CreateDeployment(deployment); err != nil {
				log.Printf("Webhook: failed to create %s deployment for project %s: %v", environment, config.ProjectID, err)
				errMsg := "failed to create deployment: " + err.Error()
				if updateErr := h.DB.UpdateWebhookEventStatus(deliveryID, config.ProjectID, environment, "failed", "", &errMsg); updateErr != nil {
					log.Printf("Webhook: failed to mark %s webhook event failed for project %s: %v", environment, config.ProjectID, updateErr)
				}
				continue
			}

			if err := h.DB.UpdateWebhookEventStatus(deliveryID, config.ProjectID, environment, "processed", deployment.ID, nil); err != nil {
				log.Printf("Webhook: failed to mark %s webhook event processed for project %s: %v", environment, config.ProjectID, err)
			}

			h.Pipeline.Dispatch(project, deployment, githubToken, func(line string, stream string) {
				log.Printf("[webhook-deploy:%s] %s", deployment.ID[:8], line)
			})
		}
	}

	w.WriteHeader(http.StatusOK)
}

func webhookTargetEnvironments(branch string, config db.AutoBuildConfig) []string {
	environments := make([]string, 0, 2)
	if matchesPreviewBranches(branch, config.PreviewBranches) {
		environments = append(environments, "preview")
	}
	if config.AutoProductionEnabled && branch == config.ProductionBranch {
		environments = append(environments, "production")
	}
	return environments
}

func webhookEventPayload(config db.AutoBuildConfig, deliveryID, environment, branch string, payload githubPushPayload, status string, deploymentID string, errorMsg *string) *db.WebhookEvent {
	return &db.WebhookEvent{
		ProjectID:        config.ProjectID,
		GithubDeliveryID: deliveryID,
		EventType:        "push",
		Environment:      environment,
		Branch:           branch,
		CommitSHA:        payload.After,
		CommitMessage:    commitMessageFromPayload(payload),
		Pusher:           payload.Pusher.Name,
		DeploymentID:     deploymentID,
		Status:           status,
		ErrorMessage:     errorMsg,
	}
}

func commitMessageFromPayload(payload githubPushPayload) string {
	if payload.HeadCommit == nil {
		return ""
	}
	return payload.HeadCommit.Message
}

func claimWebhookEvent(database *db.DB, event *db.WebhookEvent) (bool, error) {
	if err := database.CreateWebhookEvent(event); err != nil {
		if isWebhookEventAlreadyClaimed(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func isWebhookEventAlreadyClaimed(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed") &&
		strings.Contains(err.Error(), "webhook_events")
}

func validateGithubSignature(secret string, body []byte, signatureHeader string) bool {
	if !strings.HasPrefix(signatureHeader, "sha256=") {
		return false
	}
	sig, err := hex.DecodeString(strings.TrimPrefix(signatureHeader, "sha256="))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(sig, mac.Sum(nil))
}

func matchesPreviewBranches(branch, pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "*" {
		return true
	}
	for _, p := range strings.Split(pattern, ",") {
		if strings.TrimSpace(p) == branch {
			return true
		}
	}
	return false
}
