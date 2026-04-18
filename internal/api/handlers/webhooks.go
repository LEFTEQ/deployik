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

	exists, err := h.DB.WebhookEventExists(deliveryID)
	if err != nil {
		log.Printf("Webhook: failed to check idempotency for %s: %v", deliveryID, err)
	}
	if exists {
		w.WriteHeader(http.StatusOK)
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

	commitMessage := ""
	if payload.HeadCommit != nil {
		commitMessage = payload.HeadCommit.Message
	}

	signatureHeader := r.Header.Get("X-Hub-Signature-256")

	configs, err := h.DB.ListActiveAutoBuildConfigsByRepo(owner, repo)
	if err != nil {
		log.Printf("Webhook: failed to list configs for %s/%s: %v", owner, repo, err)
		w.WriteHeader(http.StatusOK)
		return
	}

	for _, config := range configs {
		secret, err := h.Encryptor.Decrypt(config.WebhookSecret)
		if err != nil {
			log.Printf("Webhook: failed to decrypt secret for project %s: %v", config.ProjectID, err)
			errMsg := "failed to decrypt webhook secret"
			h.DB.CreateWebhookEvent(&db.WebhookEvent{
				ProjectID:        config.ProjectID,
				GithubDeliveryID: deliveryID,
				EventType:        "push",
				Branch:           branch,
				CommitSHA:        payload.After,
				CommitMessage:    commitMessage,
				Pusher:           payload.Pusher.Name,
				Status:           "failed",
				ErrorMessage:     &errMsg,
			})
			continue
		}

		if !validateGithubSignature(secret, body, signatureHeader) {
			log.Printf("Webhook: invalid signature for project %s", config.ProjectID)
			errMsg := "invalid HMAC signature"
			h.DB.CreateWebhookEvent(&db.WebhookEvent{
				ProjectID:        config.ProjectID,
				GithubDeliveryID: deliveryID,
				EventType:        "push",
				Branch:           branch,
				CommitSHA:        payload.After,
				CommitMessage:    commitMessage,
				Pusher:           payload.Pusher.Name,
				Status:           "failed",
				ErrorMessage:     &errMsg,
			})
			continue
		}

		// Determine environment
		environment := ""
		if branch == config.ProductionBranch {
			environment = "production"
		} else if matchesPreviewBranches(branch, config.PreviewBranches) {
			environment = "preview"
		} else {
			h.DB.CreateWebhookEvent(&db.WebhookEvent{
				ProjectID:        config.ProjectID,
				GithubDeliveryID: deliveryID,
				EventType:        "push",
				Branch:           branch,
				CommitSHA:        payload.After,
				CommitMessage:    commitMessage,
				Pusher:           payload.Pusher.Name,
				Status:           "ignored",
			})
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

		deployment := &db.Deployment{
			ProjectID:           project.ID,
			Environment:         environment,
			Branch:              branch,
			CommitSHA:           payload.After,
			CommitMessage:       commitMessage,
			Status:              "queued",
			TriggerSource:       "webhook",
			TriggeredByUsername: payload.Pusher.Name,
			TriggeredBy:         project.UserID,
		}
		if err := h.DB.CreateDeployment(deployment); err != nil {
			log.Printf("Webhook: failed to create deployment for project %s: %v", config.ProjectID, err)
			continue
		}

		h.DB.CreateWebhookEvent(&db.WebhookEvent{
			ProjectID:        config.ProjectID,
			GithubDeliveryID: deliveryID,
			EventType:        "push",
			Branch:           branch,
			CommitSHA:        payload.After,
			CommitMessage:    commitMessage,
			Pusher:           payload.Pusher.Name,
			DeploymentID:     deployment.ID,
			Status:           "processed",
		})

		h.Pipeline.Dispatch(project, deployment, githubToken, func(line string, stream string) {
			log.Printf("[webhook-deploy:%s] %s", deployment.ID[:8], line)
		})
	}

	w.WriteHeader(http.StatusOK)
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
