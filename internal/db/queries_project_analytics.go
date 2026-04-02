package db

import (
	"database/sql"
	"fmt"
)

func (db *DB) GetProjectAnalytics(projectID string) (*ProjectAnalytics, error) {
	record := &ProjectAnalytics{}
	err := db.QueryRow(
		`SELECT project_id, audience_enabled, tracking_mode, audience_status, umami_website_id, umami_website_name,
		        last_event_at, verified_at, last_error, created_at, updated_at
		 FROM project_analytics
		 WHERE project_id = ?`,
		projectID,
	).Scan(
		&record.ProjectID,
		&record.AudienceEnabled,
		&record.TrackingMode,
		&record.AudienceStatus,
		&record.UmamiWebsiteID,
		&record.UmamiWebsiteName,
		&record.LastEventAt,
		&record.VerifiedAt,
		&record.LastError,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get project analytics: %w", err)
	}
	return record, nil
}

func (db *DB) UpsertProjectAnalytics(record *ProjectAnalytics) error {
	if record == nil {
		return fmt.Errorf("project analytics record is required")
	}

	_, err := db.Exec(
		`INSERT INTO project_analytics (
		    project_id, audience_enabled, tracking_mode, audience_status, umami_website_id, umami_website_name,
		    last_event_at, verified_at, last_error
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id) DO UPDATE SET
		    audience_enabled = excluded.audience_enabled,
		    tracking_mode = excluded.tracking_mode,
		    audience_status = excluded.audience_status,
		    umami_website_id = excluded.umami_website_id,
		    umami_website_name = excluded.umami_website_name,
		    last_event_at = excluded.last_event_at,
		    verified_at = excluded.verified_at,
		    last_error = excluded.last_error,
		    updated_at = datetime('now')`,
		record.ProjectID,
		record.AudienceEnabled,
		record.TrackingMode,
		record.AudienceStatus,
		record.UmamiWebsiteID,
		record.UmamiWebsiteName,
		record.LastEventAt,
		record.VerifiedAt,
		record.LastError,
	)
	if err != nil {
		return fmt.Errorf("upsert project analytics: %w", err)
	}

	stored, err := db.GetProjectAnalytics(record.ProjectID)
	if err != nil {
		return err
	}
	if stored != nil {
		*record = *stored
	}
	return nil
}

func (db *DB) DeleteProjectAnalytics(projectID string) error {
	if _, err := db.Exec(`DELETE FROM project_analytics WHERE project_id = ?`, projectID); err != nil {
		return fmt.Errorf("delete project analytics: %w", err)
	}
	return nil
}
