package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// scanDeployment scans a deployment row. screenshot_path is nullable so we use sql.NullString.
func scanDeployment(row interface {
	Scan(...any) error
}, d *Deployment) error {
	var screenshotPath sql.NullString
	err := row.Scan(
		&d.ID, &d.ProjectID, &d.Environment, &d.PreviewInstanceID, &d.CommitSHA, &d.CommitMessage,
		&d.Branch, &d.Status, &d.ContainerID, &d.ContainerName, &d.ImageTag,
		&d.BuildDuration, &d.TriggeredBy, &d.ErrorMessage, &d.CreatedAt, &d.FinishedAt,
		&d.TriggerSource, &d.TriggeredByUsername, &screenshotPath,
	)
	if err != nil {
		return err
	}
	d.ScreenshotPath = screenshotPath.String
	return nil
}

func (db *DB) ListDeployments(projectID string, limit int) ([]Deployment, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.Query(
		`SELECT id, project_id, environment, COALESCE(preview_instance_id, ''), commit_sha, commit_message, branch, status,
		        container_id, container_name, image_tag, build_duration, triggered_by,
		        error_message, created_at, finished_at,
		        trigger_source, triggered_by_username, screenshot_path
		 FROM deployments WHERE project_id = ?
		 ORDER BY created_at DESC LIMIT ?`, projectID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	defer rows.Close()

	var deployments []Deployment
	for rows.Next() {
		var d Deployment
		if err := scanDeployment(rows, &d); err != nil {
			return nil, fmt.Errorf("scan deployment: %w", err)
		}
		deployments = append(deployments, d)
	}
	return deployments, rows.Err()
}

func (db *DB) GetDeployment(id string) (*Deployment, error) {
	d := &Deployment{}
	row := db.QueryRow(
		`SELECT id, project_id, environment, COALESCE(preview_instance_id, ''), commit_sha, commit_message, branch, status,
		        container_id, container_name, image_tag, build_duration, triggered_by,
		        error_message, created_at, finished_at,
		        trigger_source, triggered_by_username, screenshot_path
		 FROM deployments WHERE id = ?`, id,
	)
	if err := scanDeployment(row, d); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	return d, nil
}

func (db *DB) GetDeploymentForUser(id, userID string) (*Deployment, error) {
	d := &Deployment{}
	row := db.QueryRow(
		`SELECT d.id, d.project_id, d.environment, COALESCE(d.preview_instance_id, ''), d.commit_sha, d.commit_message, d.branch, d.status,
		        d.container_id, d.container_name, d.image_tag, d.build_duration, d.triggered_by,
		        d.error_message, d.created_at, d.finished_at,
		        d.trigger_source, d.triggered_by_username, d.screenshot_path
		 FROM deployments d
		 JOIN projects p ON p.id = d.project_id
		 WHERE d.id = ?
		   AND (
		     p.user_id = ?
		     OR EXISTS (
		       SELECT 1
		       FROM organization_memberships om
		       WHERE om.organization_id = p.organization_id AND om.user_id = ?
		     )
		   )`,
		id, userID, userID,
	)
	if err := scanDeployment(row, d); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("get deployment for user: %w", err)
	}
	return d, nil
}

func (db *DB) CreateDeployment(d *Deployment) error {
	d.ID = NewID()
	triggerSource := d.TriggerSource
	if triggerSource == "" {
		triggerSource = "manual"
	}
	_, err := db.Exec(
		`INSERT INTO deployments (id, project_id, environment, preview_instance_id, commit_sha, commit_message,
		                          branch, status, triggered_by, trigger_source, triggered_by_username)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.ProjectID, d.Environment, nullableString(d.PreviewInstanceID), d.CommitSHA, d.CommitMessage,
		d.Branch, d.Status, d.TriggeredBy, triggerSource, d.TriggeredByUsername,
	)
	if err != nil {
		return fmt.Errorf("create deployment: %w", err)
	}
	d.TriggerSource = triggerSource
	return nil
}

func (db *DB) UpdateDeploymentStatus(id, status, errorMsg string) error {
	_, err := db.Exec(
		`UPDATE deployments SET status = ?, error_message = ?,
		        finished_at = CASE WHEN ? IN ('live', 'failed', 'rolled_back', 'replaced') THEN datetime('now') ELSE finished_at END
		 WHERE id = ?`,
		status, errorMsg, status, id,
	)
	if err != nil {
		return fmt.Errorf("update deployment status: %w", err)
	}
	return nil
}

func (db *DB) UpdateDeploymentContainer(id, containerID, containerName, imageTag string) error {
	_, err := db.Exec(
		`UPDATE deployments SET container_id = ?, container_name = ?, image_tag = ? WHERE id = ?`,
		containerID, containerName, imageTag, id,
	)
	if err != nil {
		return fmt.Errorf("update deployment container: %w", err)
	}
	return nil
}

func (db *DB) UpdateDeploymentPreviewInstance(id, previewInstanceID string) error {
	_, err := db.Exec(
		`UPDATE deployments SET preview_instance_id = ? WHERE id = ?`,
		nullableString(previewInstanceID), id,
	)
	if err != nil {
		return fmt.Errorf("update deployment preview instance: %w", err)
	}
	return nil
}

func (db *DB) UpdateDeploymentDuration(id string, duration int) error {
	_, err := db.Exec(
		`UPDATE deployments SET build_duration = ? WHERE id = ?`, duration, id,
	)
	if err != nil {
		return fmt.Errorf("update deployment duration: %w", err)
	}
	return nil
}

func (db *DB) UpdateDeploymentScreenshot(id, screenshotPath string) error {
	_, err := db.Exec(`UPDATE deployments SET screenshot_path = ? WHERE id = ?`, screenshotPath, id)
	if err != nil {
		return fmt.Errorf("update deployment screenshot: %w", err)
	}
	return nil
}

// GetLiveDeployment returns the current live deployment for a project+environment.
func (db *DB) GetLiveDeployment(projectID, environment string) (*Deployment, error) {
	return db.GetLiveDeploymentForTarget(projectID, environment, "")
}

func (db *DB) GetLiveDeploymentForTarget(projectID, environment, previewInstanceID string) (*Deployment, error) {
	d := &Deployment{}
	row := db.QueryRow(
		`SELECT id, project_id, environment, COALESCE(preview_instance_id, ''), commit_sha, commit_message, branch, status,
		        container_id, container_name, image_tag, build_duration, triggered_by,
		        error_message, created_at, finished_at,
		        trigger_source, triggered_by_username, screenshot_path
		 FROM deployments
		 WHERE project_id = ? AND environment = ? AND COALESCE(preview_instance_id, '') = ? AND status = 'live'
		 ORDER BY created_at DESC LIMIT 1`, projectID, environment, previewInstanceID,
	)
	if err := scanDeployment(row, d); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("get live deployment: %w", err)
	}
	return d, nil
}

// ResolveLiveContainer returns the running container name for a project's live
// deployment in the given environment. For preview, branch selects the preview
// instance (empty branch → the project's default instance); production ignores
// branch. Returns ("", false, nil) when nothing is live for that target —
// distinct from a real error — so the logs WS can answer 404 cleanly.
func (db *DB) ResolveLiveContainer(projectID, environment, branch string) (string, bool, error) {
	previewInstanceID := ""
	if environment == "preview" {
		var (
			inst *PreviewInstance
			err  error
		)
		if branch != "" {
			inst, err = db.GetPreviewInstanceForBranch(projectID, branch)
		} else {
			inst, err = db.GetDefaultPreviewInstance(projectID)
		}
		if err != nil {
			return "", false, err
		}
		if inst == nil {
			return "", false, nil
		}
		previewInstanceID = inst.ID
	}

	dep, err := db.GetLiveDeploymentForTarget(projectID, environment, previewInstanceID)
	if err != nil {
		return "", false, err
	}
	if dep == nil || dep.ContainerName == "" {
		return "", false, nil
	}
	return dep.ContainerName, true, nil
}

// GetLatestDeployment returns the most recent deployment for a project in an
// environment regardless of status (live/failed/building/...), or (nil, nil)
// when none exist. Used by the App dashboard to derive per-member status.
func (db *DB) GetLatestDeployment(projectID, environment string) (*Deployment, error) {
	row := db.QueryRow(
		`SELECT d.id, d.project_id, d.environment, COALESCE(d.preview_instance_id, ''),
		        d.commit_sha, d.commit_message, d.branch, d.status,
		        d.container_id, d.container_name, d.image_tag, d.build_duration, d.triggered_by,
		        d.error_message, d.created_at, d.finished_at,
		        d.trigger_source, d.triggered_by_username, d.screenshot_path
		 FROM deployments d
		 WHERE d.project_id = ? AND d.environment = ?
		 ORDER BY d.created_at DESC, d.rowid DESC
		 LIMIT 1`,
		projectID, environment,
	)
	var d Deployment
	if err := scanDeployment(row, &d); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get latest deployment: %w", err)
	}
	return &d, nil
}

func (db *DB) ListDeploymentsFiltered(f DeploymentFilter) (*DeploymentListResponse, error) {
	if f.Limit <= 0 {
		f.Limit = 20
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	whereClauses := []string{"d.project_id = ?"}
	args := []any{f.ProjectID}

	if f.Branch != "" {
		whereClauses = append(whereClauses, "d.branch = ?")
		args = append(args, f.Branch)
	}
	if f.Environment != "" {
		whereClauses = append(whereClauses, "d.environment = ?")
		args = append(args, f.Environment)
	}
	if f.PreviewInstanceID != "" {
		whereClauses = append(whereClauses, "d.preview_instance_id = ?")
		args = append(args, f.PreviewInstanceID)
	}
	if f.Status != "" {
		statuses := strings.Split(f.Status, ",")
		placeholders := make([]string, len(statuses))
		for i, s := range statuses {
			placeholders[i] = "?"
			args = append(args, strings.TrimSpace(s))
		}
		whereClauses = append(whereClauses, "d.status IN ("+strings.Join(placeholders, ", ")+")")
	}
	if f.TriggeredBy != "" {
		whereClauses = append(whereClauses, "d.triggered_by = ?")
		args = append(args, f.TriggeredBy)
	}
	if f.From != "" {
		whereClauses = append(whereClauses, "d.created_at >= ?")
		args = append(args, f.From)
	}
	if f.To != "" {
		whereClauses = append(whereClauses, "d.created_at <= ?")
		args = append(args, f.To)
	}

	where := strings.Join(whereClauses, " AND ")

	var total int
	if err := db.QueryRow("SELECT COUNT(*) FROM deployments d WHERE "+where, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count deployments: %w", err)
	}

	dataArgs := append(args, f.Limit, f.Offset)
	rows, err := db.Query(
		`SELECT d.id, d.project_id, d.environment, COALESCE(d.preview_instance_id, ''), d.commit_sha, d.commit_message, d.branch, d.status,
		        d.container_id, d.container_name, d.image_tag, d.build_duration, d.triggered_by,
		        d.error_message, d.created_at, d.finished_at,
		        d.trigger_source, d.triggered_by_username, d.screenshot_path,
		        COALESCE(u.username, '') AS username, COALESCE(u.avatar_url, '') AS avatar_url
		 FROM deployments d
		 LEFT JOIN users u ON u.id = d.triggered_by
		 WHERE `+where+`
		 ORDER BY d.created_at DESC
		 LIMIT ? OFFSET ?`,
		dataArgs...,
	)
	if err != nil {
		return nil, fmt.Errorf("list deployments filtered: %w", err)
	}
	defer rows.Close()

	var deployments []DeploymentWithUser
	for rows.Next() {
		var dw DeploymentWithUser
		var screenshotPath sql.NullString
		d := &dw.Deployment
		if err := rows.Scan(
			&d.ID, &d.ProjectID, &d.Environment, &d.PreviewInstanceID, &d.CommitSHA, &d.CommitMessage,
			&d.Branch, &d.Status, &d.ContainerID, &d.ContainerName, &d.ImageTag,
			&d.BuildDuration, &d.TriggeredBy, &d.ErrorMessage, &d.CreatedAt, &d.FinishedAt,
			&d.TriggerSource, &d.TriggeredByUsername, &screenshotPath,
			&dw.Username, &dw.AvatarURL,
		); err != nil {
			return nil, fmt.Errorf("scan deployment with user: %w", err)
		}
		d.ScreenshotPath = screenshotPath.String
		deployments = append(deployments, dw)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list deployments filtered rows: %w", err)
	}

	return &DeploymentListResponse{
		Deployments: deployments,
		Total:       total,
	}, nil
}
