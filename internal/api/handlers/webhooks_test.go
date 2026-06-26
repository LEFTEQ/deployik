package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/lefteq/lovinka-deployik/internal/build"
	"github.com/lefteq/lovinka-deployik/internal/db"
)

const webhookSecretForTests = "webhook-secret"

func setupWebhookProject(t *testing.T, autoProductionEnabled bool, previewBranches string) (*db.DB, *WebhookHandler, *db.Project) {
	t.Helper()
	database, enc, user := setupProjectTestDB(t)
	project := &db.Project{
		Name:           "webhook-app",
		GithubRepo:     "repo",
		GithubOwner:    "owner",
		Branch:         "main",
		UserID:         user.ID,
		Framework:      "nextjs",
		BuildCommand:   "bun run build",
		InstallCommand: "bun install",
		NodeVersion:    "22",
		Status:         "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	encryptedSecret, err := enc.Encrypt(webhookSecretForTests)
	if err != nil {
		t.Fatalf("encrypt webhook secret: %v", err)
	}
	webhookID := int64(123)
	if err := database.UpsertAutoBuildConfig(&db.AutoBuildConfig{
		ProjectID:             project.ID,
		Enabled:               true,
		ProductionBranch:      "main",
		PreviewBranches:       previewBranches,
		AutoProductionEnabled: autoProductionEnabled,
		WebhookID:             &webhookID,
		WebhookSecret:         encryptedSecret,
	}); err != nil {
		t.Fatalf("UpsertAutoBuildConfig: %v", err)
	}
	handler := &WebhookHandler{
		DB:        database,
		Encryptor: enc,
		Pipeline:  &build.Pipeline{DB: database, Encryptor: enc, EnqueueOnly: true},
	}
	return database, handler, project
}

func sendGithubPush(t *testing.T, handler *WebhookHandler, deliveryID string, branch string) *httptest.ResponseRecorder {
	t.Helper()
	return sendGithubPushSignedWithSecret(t, handler, deliveryID, branch, webhookSecretForTests)
}

func sendGithubPushSignedWithSecret(t *testing.T, handler *WebhookHandler, deliveryID string, branch string, secret string) *httptest.ResponseRecorder {
	t.Helper()
	payload := map[string]any{
		"ref":     "refs/heads/" + branch,
		"after":   "abcdef1234567890",
		"deleted": false,
		"repository": map[string]any{
			"full_name": "owner/repo",
		},
		"pusher": map[string]any{
			"name": "alice",
		},
		"head_commit": map[string]any{
			"message": "Ship homepage",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", deliveryID)
	req.Header.Set("X-Hub-Signature-256", signature)
	rr := httptest.NewRecorder()
	handler.HandleGithub(rr, req)
	return rr
}

func deploymentEnvironmentCounts(t *testing.T, database *db.DB, projectID string) map[string]int {
	t.Helper()
	deployments, err := database.ListDeployments(projectID, 10)
	if err != nil {
		t.Fatalf("ListDeployments: %v", err)
	}
	counts := map[string]int{}
	for _, deployment := range deployments {
		counts[deployment.Environment]++
		if deployment.Branch != "main" {
			t.Fatalf("deployment branch = %q, want main", deployment.Branch)
		}
		if deployment.CommitSHA != "abcdef1234567890" {
			t.Fatalf("deployment commit = %q, want webhook SHA", deployment.CommitSHA)
		}
		if deployment.TriggerSource != "webhook" {
			t.Fatalf("trigger_source = %q, want webhook", deployment.TriggerSource)
		}
	}
	return counts
}

func deploymentsForProject(t *testing.T, database *db.DB, projectID string) []db.Deployment {
	t.Helper()
	deployments, err := database.ListDeployments(projectID, 10)
	if err != nil {
		t.Fatalf("ListDeployments: %v", err)
	}
	return deployments
}

func webhookEventCount(t *testing.T, database *db.DB, projectID, environment string) int {
	t.Helper()
	var count int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM webhook_events WHERE project_id = ? AND environment = ?`,
		projectID, environment,
	).Scan(&count); err != nil {
		t.Fatalf("count webhook events: %v", err)
	}
	return count
}

func assertProcessedWebhookEvent(t *testing.T, database *db.DB, projectID, environment string) {
	t.Helper()
	var status, deploymentID string
	if err := database.QueryRow(
		`SELECT status, COALESCE(deployment_id, '') FROM webhook_events WHERE project_id = ? AND environment = ?`,
		projectID, environment,
	).Scan(&status, &deploymentID); err != nil {
		t.Fatalf("get %s webhook event: %v", environment, err)
	}
	if status != "processed" {
		t.Fatalf("%s webhook status = %q, want processed", environment, status)
	}
	if deploymentID == "" {
		t.Fatalf("%s webhook deployment_id is empty", environment)
	}
}

func assertReceivedWebhookEvent(t *testing.T, database *db.DB, projectID, environment string) {
	t.Helper()
	var status, deploymentID string
	if err := database.QueryRow(
		`SELECT status, COALESCE(deployment_id, '') FROM webhook_events WHERE project_id = ? AND environment = ?`,
		projectID, environment,
	).Scan(&status, &deploymentID); err != nil {
		t.Fatalf("get %s webhook event: %v", environment, err)
	}
	if status != "received" {
		t.Fatalf("%s webhook status = %q, want received", environment, status)
	}
	if deploymentID != "" {
		t.Fatalf("%s webhook deployment_id = %q, want empty", environment, deploymentID)
	}
}

func TestWebhookMainBranchPreviewOnlyByDefault(t *testing.T) {
	database, handler, project := setupWebhookProject(t, false, "*")

	rr := sendGithubPush(t, handler, "delivery-preview-only", "main")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rr.Code, rr.Body.String())
	}

	counts := deploymentEnvironmentCounts(t, database, project.ID)
	if counts["preview"] != 1 {
		t.Fatalf("preview deployments = %d, want 1", counts["preview"])
	}
	if counts["production"] != 0 {
		t.Fatalf("production deployments = %d, want 0", counts["production"])
	}
	if webhookEventCount(t, database, project.ID, "preview") != 1 {
		t.Fatal("expected one preview webhook event")
	}
	assertProcessedWebhookEvent(t, database, project.ID, "preview")
}

func TestWebhookFeatureBranchCreatesBranchPreviewInstance(t *testing.T) {
	database, handler, project := setupWebhookProject(t, false, "*")

	rr := sendGithubPush(t, handler, "delivery-feature-preview", "feature/checkout-flow")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rr.Code, rr.Body.String())
	}

	deployments := deploymentsForProject(t, database, project.ID)
	if len(deployments) != 1 {
		t.Fatalf("deployments = %d, want 1", len(deployments))
	}
	deployment := deployments[0]
	if deployment.Environment != "preview" {
		t.Fatalf("environment = %q, want preview", deployment.Environment)
	}
	if deployment.Branch != "feature/checkout-flow" {
		t.Fatalf("branch = %q, want feature/checkout-flow", deployment.Branch)
	}
	if deployment.PreviewInstanceID == "" {
		t.Fatal("preview_instance_id should be set for branch preview deployment")
	}

	instance, err := database.GetPreviewInstanceByID(deployment.PreviewInstanceID)
	if err != nil {
		t.Fatalf("GetPreviewInstanceByID: %v", err)
	}
	if instance == nil {
		t.Fatal("preview instance not found")
	}
	if instance.Branch != "feature/checkout-flow" {
		t.Fatalf("instance branch = %q", instance.Branch)
	}
	if instance.BranchSlug != "feature-checkout-flow" {
		t.Fatalf("instance slug = %q", instance.BranchSlug)
	}

	domains, err := database.ListDomains(project.ID)
	if err != nil {
		t.Fatalf("ListDomains: %v", err)
	}
	if len(domains) != 1 {
		t.Fatalf("domains = %d, want 1", len(domains))
	}
	if domains[0].DomainName != "webhook-app-feature-checkout-flow.preview.example.com" {
		t.Fatalf("domain = %q", domains[0].DomainName)
	}
	if domains[0].PreviewInstanceID != instance.ID {
		t.Fatalf("domain preview_instance_id = %q, want %q", domains[0].PreviewInstanceID, instance.ID)
	}
}

func TestWebhookMainBranchPreviewAndProductionWhenOptedIn(t *testing.T) {
	database, handler, project := setupWebhookProject(t, true, "*")

	rr := sendGithubPush(t, handler, "delivery-preview-production", "main")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rr.Code, rr.Body.String())
	}

	counts := deploymentEnvironmentCounts(t, database, project.ID)
	if counts["preview"] != 1 {
		t.Fatalf("preview deployments = %d, want 1", counts["preview"])
	}
	if counts["production"] != 1 {
		t.Fatalf("production deployments = %d, want 1", counts["production"])
	}
	if webhookEventCount(t, database, project.ID, "preview") != 1 {
		t.Fatal("expected one preview webhook event")
	}
	if webhookEventCount(t, database, project.ID, "production") != 1 {
		t.Fatal("expected one production webhook event")
	}
	assertProcessedWebhookEvent(t, database, project.ID, "preview")
	assertProcessedWebhookEvent(t, database, project.ID, "production")
}

func TestWebhookNonMatchingBranchRecordsIgnored(t *testing.T) {
	database, handler, project := setupWebhookProject(t, true, "develop")

	rr := sendGithubPush(t, handler, "delivery-ignored", "feature")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rr.Code, rr.Body.String())
	}

	counts := deploymentEnvironmentCounts(t, database, project.ID)
	if counts["preview"] != 0 || counts["production"] != 0 {
		t.Fatalf("deployment counts = %#v, want no deployments", counts)
	}
	if webhookEventCount(t, database, project.ID, "ignored") != 1 {
		t.Fatal("expected one ignored webhook event")
	}
}

func TestWebhookIgnoredBranchInvalidSignatureDoesNotRecordEvent(t *testing.T) {
	database, handler, project := setupWebhookProject(t, true, "develop")

	rr := sendGithubPushSignedWithSecret(t, handler, "delivery-ignored-invalid-signature", "feature", "wrong-secret")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rr.Code, rr.Body.String())
	}

	counts := deploymentEnvironmentCounts(t, database, project.ID)
	if counts["preview"] != 0 || counts["production"] != 0 {
		t.Fatalf("deployment counts = %#v, want no deployments", counts)
	}
	if webhookEventCount(t, database, project.ID, "ignored") != 0 {
		t.Fatal("invalid signature should not create ignored webhook event")
	}
}

func TestWebhookRedeliveryDoesNotDuplicateDeployments(t *testing.T) {
	database, handler, project := setupWebhookProject(t, true, "*")

	sendGithubPush(t, handler, "delivery-redelivered", "main")
	sendGithubPush(t, handler, "delivery-redelivered", "main")

	counts := deploymentEnvironmentCounts(t, database, project.ID)
	if counts["preview"] != 1 {
		t.Fatalf("preview deployments = %d, want 1", counts["preview"])
	}
	if counts["production"] != 1 {
		t.Fatalf("production deployments = %d, want 1", counts["production"])
	}
}

// sendGithubPushFormEncoded mirrors sendGithubPush but transports the payload
// as application/x-www-form-urlencoded (GitHub's "Form" content type), which
// wraps the JSON inside a `payload=<url-encoded-JSON>` body. HMAC is computed
// over the raw form-encoded bytes, matching how GitHub signs.
func sendGithubPushFormEncoded(t *testing.T, handler *WebhookHandler, deliveryID string, branch string) *httptest.ResponseRecorder {
	t.Helper()
	payload := map[string]any{
		"ref":     "refs/heads/" + branch,
		"after":   "abcdef1234567890",
		"deleted": false,
		"repository": map[string]any{
			"full_name": "owner/repo",
		},
		"pusher": map[string]any{
			"name": "alice",
		},
		"head_commit": map[string]any{
			"message": "Ship homepage",
		},
	}
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	form := url.Values{}
	form.Set("payload", string(jsonBytes))
	body := []byte(form.Encode())
	mac := hmac.New(sha256.New, []byte(webhookSecretForTests))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/github", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", deliveryID)
	req.Header.Set("X-Hub-Signature-256", signature)
	rr := httptest.NewRecorder()
	handler.HandleGithub(rr, req)
	return rr
}

func TestWebhookAcceptsFormEncodedBody(t *testing.T) {
	database, handler, project := setupWebhookProject(t, false, "*")

	rr := sendGithubPushFormEncoded(t, handler, "delivery-form-encoded", "main")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rr.Code, rr.Body.String())
	}

	counts := deploymentEnvironmentCounts(t, database, project.ID)
	if counts["preview"] != 1 {
		t.Fatalf("preview deployments = %d, want 1", counts["preview"])
	}
	if webhookEventCount(t, database, project.ID, "preview") != 1 {
		t.Fatal("expected one preview webhook event")
	}
	assertProcessedWebhookEvent(t, database, project.ID, "preview")
}

func TestWebhookFormEncodedInvalidSignatureRejected(t *testing.T) {
	database, handler, project := setupWebhookProject(t, false, "*")

	// Build a form-encoded body, then sign it with the WRONG secret.
	payload := `{"ref":"refs/heads/main","after":"abcdef1234567890","deleted":false,` +
		`"repository":{"full_name":"owner/repo"},"pusher":{"name":"alice"},` +
		`"head_commit":{"message":"Ship homepage"}}`
	form := url.Values{}
	form.Set("payload", payload)
	body := []byte(form.Encode())
	mac := hmac.New(sha256.New, []byte("wrong-secret"))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/github", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", "delivery-form-bad-sig")
	req.Header.Set("X-Hub-Signature-256", signature)
	rr := httptest.NewRecorder()
	handler.HandleGithub(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rr.Code, rr.Body.String())
	}
	counts := deploymentEnvironmentCounts(t, database, project.ID)
	if counts["preview"] != 0 {
		t.Fatalf("preview deployments = %d, want 0 (invalid signature must not deploy)", counts["preview"])
	}
}

func TestWebhookFormEncodedRejectsMissingPayloadField(t *testing.T) {
	database, handler, project := setupWebhookProject(t, false, "*")

	// Form-encoded body with no `payload=` field. HMAC is over the literal bytes.
	body := []byte("not_payload=garbage")
	mac := hmac.New(sha256.New, []byte(webhookSecretForTests))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/github", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", "delivery-form-missing-field")
	req.Header.Set("X-Hub-Signature-256", signature)
	rr := httptest.NewRecorder()
	handler.HandleGithub(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid payload") {
		t.Fatalf("body = %q, want it to mention invalid payload", rr.Body.String())
	}
	if deploymentEnvironmentCounts(t, database, project.ID)["preview"] != 0 {
		t.Fatal("no deployment should be created for missing payload field")
	}
}

// sendGithubEvent posts an arbitrary push/delete event (any ref, deleted flag,
// and signing secret) so tests can exercise the branch-delete and tag-ref paths.
func sendGithubEvent(t *testing.T, handler *WebhookHandler, deliveryID, ref string, deleted bool, secret string) *httptest.ResponseRecorder {
	t.Helper()
	payload := map[string]any{
		"ref":     ref,
		"after":   "abcdef1234567890",
		"deleted": deleted,
		"repository": map[string]any{
			"full_name": "owner/repo",
		},
		"pusher": map[string]any{
			"name": "alice",
		},
		"head_commit": map[string]any{
			"message": "Ship homepage",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", deliveryID)
	req.Header.Set("X-Hub-Signature-256", signature)
	rr := httptest.NewRecorder()
	handler.HandleGithub(rr, req)
	return rr
}

func previewInstanceStatus(t *testing.T, database *db.DB, id string) string {
	t.Helper()
	var status string
	if err := database.QueryRow(`SELECT status FROM preview_instances WHERE id = ?`, id).Scan(&status); err != nil {
		t.Fatalf("get preview instance status: %v", err)
	}
	return status
}

func domainCountForInstance(t *testing.T, database *db.DB, projectID, instanceID string) int {
	t.Helper()
	domains, err := database.ListDomains(projectID)
	if err != nil {
		t.Fatalf("ListDomains: %v", err)
	}
	count := 0
	for _, d := range domains {
		if d.PreviewInstanceID == instanceID {
			count++
		}
	}
	return count
}

func TestWebhookBranchDeletionTearsDownPreviewInstance(t *testing.T) {
	database, handler, project := setupWebhookProject(t, false, "*")

	// First a push creates the branch preview instance + auto-domain.
	if rr := sendGithubPush(t, handler, "delivery-create-feature", "feature/cleanup-me"); rr.Code != http.StatusOK {
		t.Fatalf("create status = %d, body: %s", rr.Code, rr.Body.String())
	}
	instance, err := database.GetPreviewInstanceForBranch(project.ID, "feature/cleanup-me")
	if err != nil {
		t.Fatalf("GetPreviewInstanceForBranch: %v", err)
	}
	if instance == nil {
		t.Fatal("expected a preview instance for the feature branch")
	}
	if domainCountForInstance(t, database, project.ID, instance.ID) == 0 {
		t.Fatal("expected an auto-domain for the feature branch preview")
	}

	// Deleting the branch on GitHub fires a push with deleted=true.
	rr := sendGithubEvent(t, handler, "delivery-delete-feature", "refs/heads/feature/cleanup-me", true, webhookSecretForTests)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body: %s", rr.Code, rr.Body.String())
	}

	if status := previewInstanceStatus(t, database, instance.ID); status != "deleted" {
		t.Fatalf("preview instance status = %q, want deleted", status)
	}
	if count := domainCountForInstance(t, database, project.ID, instance.ID); count != 0 {
		t.Fatalf("domains for torn-down instance = %d, want 0", count)
	}
}

func TestWebhookBranchDeletionLeavesDefaultPreviewIntact(t *testing.T) {
	database, handler, project := setupWebhookProject(t, false, "*")

	if rr := sendGithubPush(t, handler, "delivery-create-main", "main"); rr.Code != http.StatusOK {
		t.Fatalf("create status = %d, body: %s", rr.Code, rr.Body.String())
	}
	instance, err := database.GetPreviewInstanceForBranch(project.ID, "main")
	if err != nil {
		t.Fatalf("GetPreviewInstanceForBranch: %v", err)
	}
	if instance == nil || !instance.IsDefault {
		t.Fatal("expected a default preview instance for main")
	}

	rr := sendGithubEvent(t, handler, "delivery-delete-main", "refs/heads/main", true, webhookSecretForTests)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body: %s", rr.Code, rr.Body.String())
	}

	if status := previewInstanceStatus(t, database, instance.ID); status != "active" {
		t.Fatalf("default preview status = %q, want active (must not be torn down)", status)
	}
}

func TestWebhookBranchDeletionInvalidSignatureKeepsPreview(t *testing.T) {
	database, handler, project := setupWebhookProject(t, false, "*")

	if rr := sendGithubPush(t, handler, "delivery-create-keep", "feature/keep-me"); rr.Code != http.StatusOK {
		t.Fatalf("create status = %d, body: %s", rr.Code, rr.Body.String())
	}
	instance, err := database.GetPreviewInstanceForBranch(project.ID, "feature/keep-me")
	if err != nil {
		t.Fatalf("GetPreviewInstanceForBranch: %v", err)
	}
	if instance == nil {
		t.Fatal("expected a preview instance for the feature branch")
	}

	rr := sendGithubEvent(t, handler, "delivery-delete-forged", "refs/heads/feature/keep-me", true, "wrong-secret")
	if rr.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body: %s", rr.Code, rr.Body.String())
	}

	if status := previewInstanceStatus(t, database, instance.ID); status != "active" {
		t.Fatalf("preview status = %q, want active (forged delete must not tear down)", status)
	}
}

func TestWebhookTagPushDoesNotCreatePreview(t *testing.T) {
	database, handler, project := setupWebhookProject(t, false, "*")

	rr := sendGithubEvent(t, handler, "delivery-tag-push", "refs/tags/v1.2.3", false, webhookSecretForTests)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rr.Code, rr.Body.String())
	}

	if deployments := deploymentsForProject(t, database, project.ID); len(deployments) != 0 {
		t.Fatalf("deployments = %d, want 0 (tag pushes must not deploy)", len(deployments))
	}
	instance, err := database.GetPreviewInstanceForBranch(project.ID, "refs/tags/v1.2.3")
	if err != nil {
		t.Fatalf("GetPreviewInstanceForBranch: %v", err)
	}
	if instance != nil {
		t.Fatal("tag push must not create a preview instance")
	}
}

func TestWebhookTagDeletionTearsDownLegacyPreview(t *testing.T) {
	database, handler, project := setupWebhookProject(t, false, "*")

	// Simulate a legacy junk preview created from a tag push before the guard
	// existed: its branch key is the raw, un-stripped ref.
	instance, err := database.GetOrCreatePreviewInstance(project, "refs/tags/old-release")
	if err != nil {
		t.Fatalf("GetOrCreatePreviewInstance: %v", err)
	}
	if _, err := database.EnsurePreviewAutoDomains(project, instance); err != nil {
		t.Fatalf("EnsurePreviewAutoDomains: %v", err)
	}

	rr := sendGithubEvent(t, handler, "delivery-tag-delete", "refs/tags/old-release", true, webhookSecretForTests)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rr.Code, rr.Body.String())
	}

	if status := previewInstanceStatus(t, database, instance.ID); status != "deleted" {
		t.Fatalf("legacy tag preview status = %q, want deleted", status)
	}
	if count := domainCountForInstance(t, database, project.ID, instance.ID); count != 0 {
		t.Fatalf("domains for torn-down legacy instance = %d, want 0", count)
	}
}

func TestWebhookPreclaimedEnvironmentDoesNotCreateDeployment(t *testing.T) {
	database, handler, project := setupWebhookProject(t, false, "*")
	if err := database.CreateWebhookEvent(&db.WebhookEvent{
		ProjectID:        project.ID,
		GithubDeliveryID: "delivery-preclaimed",
		EventType:        "push",
		Environment:      "preview",
		Branch:           "main",
		CommitSHA:        "preclaimed",
		Status:           "received",
	}); err != nil {
		t.Fatalf("CreateWebhookEvent preclaim: %v", err)
	}

	rr := sendGithubPush(t, handler, "delivery-preclaimed", "main")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rr.Code, rr.Body.String())
	}

	counts := deploymentEnvironmentCounts(t, database, project.ID)
	if counts["preview"] != 0 || counts["production"] != 0 {
		t.Fatalf("deployment counts = %#v, want no deployments", counts)
	}
	if webhookEventCount(t, database, project.ID, "preview") != 1 {
		t.Fatal("expected only the preclaimed preview webhook event")
	}
	assertReceivedWebhookEvent(t, database, project.ID, "preview")
}

// sendGithubPushWithCommits sends a push whose commits carry changed-file lists,
// exercising path-based build filtering.
func sendGithubPushWithCommits(t *testing.T, handler *WebhookHandler, deliveryID, branch string, changed []string) *httptest.ResponseRecorder {
	t.Helper()
	payload := map[string]any{
		"ref":         "refs/heads/" + branch,
		"after":       "abcdef1234567890",
		"deleted":     false,
		"repository":  map[string]any{"full_name": "owner/repo"},
		"pusher":      map[string]any{"name": "alice"},
		"head_commit": map[string]any{"message": "Ship homepage"},
		"commits": []map[string]any{
			{"added": changed, "modified": []string{}, "removed": []string{}},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	mac := hmac.New(sha256.New, []byte(webhookSecretForTests))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", deliveryID)
	req.Header.Set("X-Hub-Signature-256", signature)
	rr := httptest.NewRecorder()
	handler.HandleGithub(rr, req)
	return rr
}

func TestWebhookPathFilterSkipsUnchangedProject(t *testing.T) {
	database, handler, project := setupWebhookProject(t, false, "*")
	project.RootDirectory = "apps/web"
	project.BuildFilterEnabled = true
	if err := database.UpdateProject(project); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}

	// Change touches a sibling app only — the filtered project must be skipped.
	rr := sendGithubPushWithCommits(t, handler, "delivery-filter-skip", "main", []string{"apps/api/main.go"})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rr.Code, rr.Body.String())
	}

	counts := deploymentEnvironmentCounts(t, database, project.ID)
	if counts["preview"] != 0 || counts["production"] != 0 {
		t.Fatalf("deployment counts = %#v, want no deployments (skipped)", counts)
	}
	var status, msg string
	if err := database.QueryRow(
		`SELECT status, COALESCE(error_message, '') FROM webhook_events WHERE project_id = ? AND environment = 'preview'`,
		project.ID,
	).Scan(&status, &msg); err != nil {
		t.Fatalf("get preview webhook event: %v", err)
	}
	if status != "ignored" {
		t.Fatalf("preview webhook status = %q, want ignored", status)
	}
	if !strings.Contains(msg, "skipped") {
		t.Fatalf("preview webhook message = %q, want a skip reason", msg)
	}
}

func TestWebhookPathFilterBuildsChangedProject(t *testing.T) {
	database, handler, project := setupWebhookProject(t, false, "*")
	project.RootDirectory = "apps/web"
	project.BuildFilterEnabled = true
	if err := database.UpdateProject(project); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}

	// Change touches this project's root — it must build.
	rr := sendGithubPushWithCommits(t, handler, "delivery-filter-build", "main", []string{"apps/web/src/page.tsx"})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rr.Code, rr.Body.String())
	}

	counts := deploymentEnvironmentCounts(t, database, project.ID)
	if counts["preview"] != 1 {
		t.Fatalf("preview deployments = %d, want 1 (built)", counts["preview"])
	}
	assertProcessedWebhookEvent(t, database, project.ID, "preview")
}

func TestWebhookPathFilterDisabledAlwaysBuilds(t *testing.T) {
	// build_filter_enabled defaults off — a sibling-only change still builds,
	// guarding the inert-by-default behavior.
	database, handler, project := setupWebhookProject(t, false, "*")
	project.RootDirectory = "apps/web"
	if err := database.UpdateProject(project); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}

	rr := sendGithubPushWithCommits(t, handler, "delivery-filter-off", "main", []string{"apps/api/main.go"})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rr.Code, rr.Body.String())
	}

	counts := deploymentEnvironmentCounts(t, database, project.ID)
	if counts["preview"] != 1 {
		t.Fatalf("preview deployments = %d, want 1 (filter disabled)", counts["preview"])
	}
}
