package db

import (
	"database/sql"
	"fmt"
)

// CreateAppRelease records a coordinated app deploy and its per-member
// deployment snapshot in one transaction. The release id is generated here.
func (db *DB) CreateAppRelease(release *AppRelease, members []AppReleaseMember) (*AppRelease, error) {
	if release == nil {
		return nil, fmt.Errorf("create app release: release is nil")
	}
	if release.AppID == "" {
		return nil, fmt.Errorf("create app release: app_id is required")
	}
	if release.Status == "" {
		release.Status = "pending"
	}
	release.ID = NewID()

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("create app release: begin: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO app_releases (id, app_id, environment, status) VALUES (?, ?, ?, ?)`,
		release.ID, release.AppID, release.Environment, release.Status,
	); err != nil {
		return nil, fmt.Errorf("create app release: insert: %w", err)
	}
	for _, m := range members {
		if _, err := tx.Exec(
			`INSERT INTO app_release_members (release_id, project_id, deployment_id)
			 VALUES (?, ?, ?)`,
			release.ID, m.ProjectID, nullableString(m.DeploymentID),
		); err != nil {
			return nil, fmt.Errorf("create app release: insert member %s: %w", m.ProjectID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create app release: commit: %w", err)
	}
	return db.GetAppRelease(release.ID)
}

// UpdateAppReleaseStatus transitions a release (e.g. pending → succeeded/failed).
func (db *DB) UpdateAppReleaseStatus(releaseID, status string) error {
	if _, err := db.Exec(
		`UPDATE app_releases SET status = ?, updated_at = datetime('now') WHERE id = ?`,
		status, releaseID,
	); err != nil {
		return fmt.Errorf("update app release status: %w", err)
	}
	return nil
}

// GetAppRelease fetches a release with its member snapshot.
func (db *DB) GetAppRelease(releaseID string) (*AppRelease, error) {
	r := &AppRelease{}
	err := db.QueryRow(
		`SELECT id, app_id, environment, status, created_at, updated_at
		 FROM app_releases WHERE id = ?`, releaseID,
	).Scan(&r.ID, &r.AppID, &r.Environment, &r.Status, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get app release: %w", err)
	}
	members, err := db.listAppReleaseMembers(releaseID)
	if err != nil {
		return nil, err
	}
	r.Members = members
	return r, nil
}

func (db *DB) listAppReleaseMembers(releaseID string) ([]AppReleaseMember, error) {
	rows, err := db.Query(
		`SELECT release_id, project_id, COALESCE(deployment_id, '')
		 FROM app_release_members WHERE release_id = ? ORDER BY project_id ASC`,
		releaseID,
	)
	if err != nil {
		return nil, fmt.Errorf("list app release members: %w", err)
	}
	defer rows.Close()

	var members []AppReleaseMember
	for rows.Next() {
		var m AppReleaseMember
		if err := rows.Scan(&m.ReleaseID, &m.ProjectID, &m.DeploymentID); err != nil {
			return nil, fmt.Errorf("scan app release member: %w", err)
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// ListAppReleases lists an app's releases for an environment, newest first
// (without member detail — call GetAppRelease for the snapshot).
func (db *DB) ListAppReleases(appID, environment string) ([]AppRelease, error) {
	rows, err := db.Query(
		`SELECT id, app_id, environment, status, created_at, updated_at
		 FROM app_releases WHERE app_id = ? AND environment = ?
		 ORDER BY created_at DESC, id DESC`,
		appID, environment,
	)
	if err != nil {
		return nil, fmt.Errorf("list app releases: %w", err)
	}
	defer rows.Close()

	var releases []AppRelease
	for rows.Next() {
		var r AppRelease
		if err := rows.Scan(&r.ID, &r.AppID, &r.Environment, &r.Status, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan app release: %w", err)
		}
		releases = append(releases, r)
	}
	return releases, rows.Err()
}
