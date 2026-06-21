package db

import (
	"database/sql"
	"fmt"
)

// AppDeployment is a deployment row enriched with the triggering user and the
// owning member project's name, for the App dashboard's unified feed.
type AppDeployment struct {
	DeploymentWithUser
	ProjectName string `json:"project_name"`
}

// ListAppDeployments returns recent deployments across every (non-deleted)
// member project of an app for one environment, newest first. limit<=0 -> 20.
func (db *DB) ListAppDeployments(appID, environment string, limit int) ([]AppDeployment, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.Query(
		`SELECT d.id, d.project_id, d.environment, COALESCE(d.preview_instance_id, ''),
		        d.commit_sha, d.commit_message, d.branch, d.status,
		        d.container_id, d.container_name, d.image_tag, d.build_duration, d.triggered_by,
		        d.error_message, d.created_at, d.finished_at,
		        d.trigger_source, d.triggered_by_username, d.screenshot_path,
		        COALESCE(u.username, '') AS username, COALESCE(u.avatar_url, '') AS avatar_url,
		        p.name AS project_name
		 FROM deployments d
		 JOIN projects p ON p.id = d.project_id
		 LEFT JOIN users u ON u.id = d.triggered_by
		 WHERE p.app_id = ? AND p.status != 'deleted' AND d.environment = ?
		 ORDER BY d.created_at DESC, d.rowid DESC
		 LIMIT ?`,
		appID, environment, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list app deployments: %w", err)
	}
	defer rows.Close()

	out := make([]AppDeployment, 0)
	for rows.Next() {
		var row AppDeployment
		var screenshotPath sql.NullString
		d := &row.Deployment
		if err := rows.Scan(
			&d.ID, &d.ProjectID, &d.Environment, &d.PreviewInstanceID, &d.CommitSHA, &d.CommitMessage,
			&d.Branch, &d.Status, &d.ContainerID, &d.ContainerName, &d.ImageTag,
			&d.BuildDuration, &d.TriggeredBy, &d.ErrorMessage, &d.CreatedAt, &d.FinishedAt,
			&d.TriggerSource, &d.TriggeredByUsername, &screenshotPath,
			&row.Username, &row.AvatarURL,
			&row.ProjectName,
		); err != nil {
			return nil, fmt.Errorf("scan app deployment: %w", err)
		}
		d.ScreenshotPath = screenshotPath.String
		out = append(out, row)
	}
	return out, rows.Err()
}
