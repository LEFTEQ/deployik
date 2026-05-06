package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
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
