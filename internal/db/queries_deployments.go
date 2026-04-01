package db

import (
	"database/sql"
	"fmt"
)

func (db *DB) ListDeployments(projectID string, limit int) ([]Deployment, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.Query(
		`SELECT id, project_id, environment, commit_sha, commit_message, branch, status,
		        container_id, container_name, image_tag, build_duration, triggered_by,
		        error_message, created_at, finished_at
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
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.Environment, &d.CommitSHA, &d.CommitMessage,
			&d.Branch, &d.Status, &d.ContainerID, &d.ContainerName, &d.ImageTag,
			&d.BuildDuration, &d.TriggeredBy, &d.ErrorMessage, &d.CreatedAt, &d.FinishedAt); err != nil {
			return nil, fmt.Errorf("scan deployment: %w", err)
		}
		deployments = append(deployments, d)
	}
	return deployments, rows.Err()
}

func (db *DB) GetDeployment(id string) (*Deployment, error) {
	d := &Deployment{}
	err := db.QueryRow(
		`SELECT id, project_id, environment, commit_sha, commit_message, branch, status,
		        container_id, container_name, image_tag, build_duration, triggered_by,
		        error_message, created_at, finished_at
		 FROM deployments WHERE id = ?`, id,
	).Scan(&d.ID, &d.ProjectID, &d.Environment, &d.CommitSHA, &d.CommitMessage,
		&d.Branch, &d.Status, &d.ContainerID, &d.ContainerName, &d.ImageTag,
		&d.BuildDuration, &d.TriggeredBy, &d.ErrorMessage, &d.CreatedAt, &d.FinishedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	return d, nil
}

func (db *DB) CreateDeployment(d *Deployment) error {
	d.ID = NewID()
	_, err := db.Exec(
		`INSERT INTO deployments (id, project_id, environment, commit_sha, commit_message,
		                          branch, status, triggered_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.ProjectID, d.Environment, d.CommitSHA, d.CommitMessage,
		d.Branch, d.Status, d.TriggeredBy,
	)
	if err != nil {
		return fmt.Errorf("create deployment: %w", err)
	}
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

func (db *DB) UpdateDeploymentDuration(id string, duration int) error {
	_, err := db.Exec(
		`UPDATE deployments SET build_duration = ? WHERE id = ?`, duration, id,
	)
	if err != nil {
		return fmt.Errorf("update deployment duration: %w", err)
	}
	return nil
}

// GetLiveDeployment returns the current live deployment for a project+environment.
func (db *DB) GetLiveDeployment(projectID, environment string) (*Deployment, error) {
	d := &Deployment{}
	err := db.QueryRow(
		`SELECT id, project_id, environment, commit_sha, commit_message, branch, status,
		        container_id, container_name, image_tag, build_duration, triggered_by,
		        error_message, created_at, finished_at
		 FROM deployments WHERE project_id = ? AND environment = ? AND status = 'live'
		 ORDER BY created_at DESC LIMIT 1`, projectID, environment,
	).Scan(&d.ID, &d.ProjectID, &d.Environment, &d.CommitSHA, &d.CommitMessage,
		&d.Branch, &d.Status, &d.ContainerID, &d.ContainerName, &d.ImageTag,
		&d.BuildDuration, &d.TriggeredBy, &d.ErrorMessage, &d.CreatedAt, &d.FinishedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get live deployment: %w", err)
	}
	return d, nil
}
