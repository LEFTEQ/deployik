package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/buildfilter"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/domain"
	"github.com/LEFTEQ/lovinka-deployik/internal/push"
)

// WebhookHandler processes incoming GitHub webhook events.
type WebhookHandler struct {
	DB        *db.DB
	Encryptor *crypto.Encryptor
	Pipeline  *build.Pipeline
	Docker    *build.DockerClient
	Manager   *domain.Manager
	Notifier  *push.Notifier
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
	// Commits carries per-commit changed-file lists, used by path-based build
	// filtering. GitHub caps this at 2048 commits / 3000 files per push; beyond
	// that the lists are truncated and we fail safe to "build" (see
	// changedPathsFromPush).
	Commits []struct {
		Added    []string `json:"added"`
		Modified []string `json:"modified"`
		Removed  []string `json:"removed"`
	} `json:"commits"`
}

// changedPathsFromPush returns the union of changed paths across a push's
// commits, plus whether that file list is trustworthy. Returns available=false
// when no commits carry a usable file list (truncated huge push, synthetic/
// branch-create event) so the caller fails safe to "build".
func changedPathsFromPush(payload githubPushPayload) ([]string, bool) {
	if len(payload.Commits) == 0 {
		return nil, false
	}
	seen := make(map[string]struct{})
	var paths []string
	add := func(list []string) {
		for _, p := range list {
			if p == "" {
				continue
			}
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			paths = append(paths, p)
		}
	}
	for _, c := range payload.Commits {
		add(c.Added)
		add(c.Modified)
		add(c.Removed)
	}
	if len(paths) == 0 {
		// Commits present but no file lists (e.g. merge-only payload) — ambiguous,
		// so fail safe to build rather than risk a wrong skip.
		return nil, false
	}
	return paths, true
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

	// GitHub supports two content types for webhook deliveries:
	//   application/json              — body is the JSON object directly
	//   application/x-www-form-urlencoded — body is `payload=<url-encoded JSON>`
	// HMAC is always computed over the raw transport bytes, so signature
	// validation downstream keeps using `body` regardless of shape.
	jsonBytes, err := extractGithubPayloadJSON(body, r.Header.Get("Content-Type"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}

	var payload githubPushPayload
	if err := json.Unmarshal(jsonBytes, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}

	parts := strings.SplitN(payload.Repository.FullName, "/", 2)
	if len(parts) != 2 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid repository name"})
		return
	}
	owner, repo := parts[0], parts[1]

	// branch is the "branch key" stored on preview instances: for branch refs
	// it's the name with refs/heads/ stripped; for legacy tag previews it's the
	// raw refs/tags/... ref they were created with before the tag-ref guard.
	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")

	signatureHeader := r.Header.Get("X-Hub-Signature-256")

	configs, err := h.DB.ListActiveAutoBuildConfigsByRepo(owner, repo)
	if err != nil {
		log.Printf("Webhook: failed to list configs for %s/%s: %v", owner, repo, err)
		w.WriteHeader(http.StatusOK)
		return
	}

	// A deleted ref (branch or tag) tears down its preview instance instead of
	// deploying. Handled before the tag-ref guard so deleting a tag also reaps
	// any legacy preview a tag push created.
	if payload.Deleted {
		h.handleRefDeleted(r.Context(), configs, branch, body, signatureHeader)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Tag-ref guard: only branch pushes create or refresh preview/production
	// targets. Tag pushes (refs/tags/*) and other non-branch refs are ignored
	// so they never spin up a deployment.
	if !strings.HasPrefix(payload.Ref, "refs/heads/") {
		w.WriteHeader(http.StatusOK)
		return
	}

	changedPaths, fileListAvailable := changedPathsFromPush(payload)

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
			claimed, err := claimWebhookEvent(h.DB, webhookEventPayload(config, deliveryID, "ignored", "", branch, payload, "ignored", "", nil))
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

		// Path-based build filtering (opt-in). A project with build_filter_enabled
		// only rebuilds when a changed path is under its root_directory or matches
		// a watch_paths glob. Fail-safe = build. The skip is recorded per target
		// environment as an "ignored" webhook event for observability.
		if shouldBuild, reason := buildfilter.ShouldBuild(project.BuildFilterEnabled, project.RootDirectory, project.WatchPaths, changedPaths, fileListAvailable); !shouldBuild {
			log.Printf("Webhook: skipping build for project %s (%s): %s", project.Name, config.ProjectID, reason)
			skipMsg := "build skipped: " + reason
			for _, environment := range targets {
				if _, err := claimWebhookEvent(h.DB, webhookEventPayload(config, deliveryID, environment, "", branch, payload, "ignored", "", &skipMsg)); err != nil {
					log.Printf("Webhook: failed to record skipped %s event for project %s: %v", environment, config.ProjectID, err)
				}
			}
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
			var previewInstance *db.PreviewInstance
			if environment == "preview" {
				instance, _, err := ensurePreviewTarget(h.DB, project, branch)
				if err != nil {
					log.Printf("Webhook: failed to prepare preview target for project %s branch %s: %v", config.ProjectID, branch, err)
					continue
				}
				previewInstance = instance
			}

			previewInstanceID := ""
			if previewInstance != nil {
				previewInstanceID = previewInstance.ID
			}

			claimed, err := claimWebhookEvent(h.DB, webhookEventPayload(config, deliveryID, environment, previewInstanceID, branch, payload, "received", "", nil))
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
				PreviewInstanceID:   previewInstanceID,
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

			h.Notifier.Notify(project.ID, push.EventBuildStart, push.Message{
				Title: fmt.Sprintf("%s: %s build started", project.Name, environment),
				Body:  fmt.Sprintf("%s pushed to %s", payload.Pusher.Name, branch),
				URL:   fmt.Sprintf("/projects/%s/deployments/%s", project.ID, deployment.ID),
				Tag:   "deploy-" + deployment.ID,
			})
		}
	}

	w.WriteHeader(http.StatusOK)
}

// handleRefDeleted tears down the preview instance for a deleted ref across all
// auto-build configs on the repo. The webhook signature is validated per config
// (each project has its own secret) so a forged delete cannot destroy infra.
// The default preview (e.g. main) is never torn down. Teardown is idempotent,
// so GitHub redelivery is harmless.
func (h *WebhookHandler) handleRefDeleted(ctx context.Context, configs []db.AutoBuildConfig, branch string, body []byte, signatureHeader string) {
	for _, config := range configs {
		secret, err := h.Encryptor.Decrypt(config.WebhookSecret)
		if err != nil {
			log.Printf("Webhook: failed to decrypt secret for project %s: %v", config.ProjectID, err)
			continue
		}
		if !validateGithubSignature(secret, body, signatureHeader) {
			log.Printf("Webhook: invalid signature for project %s", config.ProjectID)
			continue
		}

		instance, err := h.DB.GetPreviewInstanceForBranch(config.ProjectID, branch)
		if err != nil {
			log.Printf("Webhook: failed to look up preview instance for project %s branch %s: %v", config.ProjectID, branch, err)
			continue
		}
		if instance == nil || instance.Status == "deleted" || instance.IsDefault {
			continue
		}

		project, err := h.DB.GetProject(config.ProjectID)
		if err != nil || project == nil {
			log.Printf("Webhook: project %s not found for ref-delete cleanup: %v", config.ProjectID, err)
			continue
		}

		// Delete the volume too: a preview's data volume is branch-isolated
		// (deployik-{project}-preview-{slug}-data), so dropping it on branch
		// deletion can't touch another branch's or production's data, and it
		// stops one orphaned volume accumulating per deleted branch.
		if err := teardownPreviewInstance(ctx, h.DB, h.Docker, h.Manager, project, instance, true); err != nil {
			log.Printf("Webhook: failed to tear down preview instance %s for project %s: %v", instance.ID, config.ProjectID, err)
			continue
		}
		log.Printf("Webhook: tore down preview instance %s (branch %s) for project %s after ref deletion", instance.ID, branch, config.ProjectID)
	}
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

func webhookEventPayload(config db.AutoBuildConfig, deliveryID, environment, previewInstanceID, branch string, payload githubPushPayload, status string, deploymentID string, errorMsg *string) *db.WebhookEvent {
	return &db.WebhookEvent{
		ProjectID:         config.ProjectID,
		GithubDeliveryID:  deliveryID,
		EventType:         "push",
		Environment:       environment,
		PreviewInstanceID: previewInstanceID,
		Branch:            branch,
		CommitSHA:         payload.After,
		CommitMessage:     commitMessageFromPayload(payload),
		Pusher:            payload.Pusher.Name,
		DeploymentID:      deploymentID,
		Status:            status,
		ErrorMessage:      errorMsg,
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

// extractGithubPayloadJSON returns the JSON bytes inside a webhook body,
// handling both application/json (body is JSON) and application/x-www-form-urlencoded
// (body is `payload=<url-encoded JSON>`). Unknown / missing content types fall
// back to treating the body as JSON, which matches the historical behaviour.
func extractGithubPayloadJSON(body []byte, contentType string) ([]byte, error) {
	mediaType := strings.TrimSpace(strings.ToLower(strings.SplitN(contentType, ";", 2)[0]))
	if mediaType == "application/x-www-form-urlencoded" {
		values, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, err
		}
		raw := values.Get("payload")
		if raw == "" {
			return nil, errMissingFormPayload
		}
		return []byte(raw), nil
	}
	return body, nil
}

var errMissingFormPayload = errors.New("form body missing payload field")

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
