package db

import (
	"fmt"
)

func (db *DB) CreateWebhookEvent(e *WebhookEvent) error {
	_, err := db.Exec(
		`INSERT INTO webhook_events (project_id, github_delivery_id, event_type, branch,
		                             commit_sha, commit_message, pusher, deployment_id, status, error_message)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ProjectID, e.GithubDeliveryID, e.EventType, e.Branch,
		e.CommitSHA, e.CommitMessage, e.Pusher, nullableString(e.DeploymentID), e.Status, e.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("create webhook event: %w", err)
	}
	return nil
}

func (db *DB) WebhookEventExists(deliveryID string) (bool, error) {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM webhook_events WHERE github_delivery_id = ?`, deliveryID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check webhook event exists: %w", err)
	}
	return count > 0, nil
}

func (db *DB) UpdateWebhookEventStatus(deliveryID, status, deploymentID string, errorMsg *string) error {
	_, err := db.Exec(
		`UPDATE webhook_events SET status = ?, deployment_id = ?, error_message = ? WHERE github_delivery_id = ?`,
		status, nullableString(deploymentID), errorMsg, deliveryID,
	)
	if err != nil {
		return fmt.Errorf("update webhook event status: %w", err)
	}
	return nil
}
