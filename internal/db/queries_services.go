package db

import (
	"database/sql"
	"fmt"
)

// CreateService inserts a new project_services row. Assigns s.ID if empty.
func (db *DB) CreateService(s *ProjectService) error {
	if s.ID == "" {
		s.ID = NewID()
	}
	if s.ConfigJSON == "" {
		s.ConfigJSON = "{}"
	}
	if s.Status == "" {
		s.Status = ServiceStatusPending
	}
	_, err := db.Exec(
		`INSERT INTO project_services (id, project_id, environment, service_type, image,
			db_name, db_user, db_password_encrypted, host_port, config_json, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.ProjectID, s.Environment, string(s.ServiceType), s.Image,
		s.DBName, s.DBUser, s.DBPasswordEncrypted, s.HostPort, s.ConfigJSON, string(s.Status),
	)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	return nil
}

// GetService returns a single service by id, or (nil, nil) when absent.
func (db *DB) GetService(id string) (*ProjectService, error) {
	s := &ProjectService{}
	var lastStarted sql.NullString
	err := db.QueryRow(
		`SELECT id, project_id, environment, service_type, image,
		        db_name, db_user, db_password_encrypted, host_port, config_json, status,
		        last_started_at, created_at, updated_at
		 FROM project_services WHERE id = ?`, id,
	).Scan(
		&s.ID, &s.ProjectID, &s.Environment, &s.ServiceType, &s.Image,
		&s.DBName, &s.DBUser, &s.DBPasswordEncrypted, &s.HostPort, &s.ConfigJSON, &s.Status,
		&lastStarted, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get service: %w", err)
	}
	if lastStarted.Valid {
		if t := parseSQLiteDateTime(lastStarted.String); t != nil {
			s.LastStartedAt = NullableTime{NullTime: sql.NullTime{Time: *t, Valid: true}}
		}
	}
	return s, nil
}

// GetServiceByProjectEnv looks up a service by its natural key.
func (db *DB) GetServiceByProjectEnv(projectID, environment string, svcType ServiceType) (*ProjectService, error) {
	s := &ProjectService{}
	var lastStarted sql.NullString
	err := db.QueryRow(
		`SELECT id, project_id, environment, service_type, image,
		        db_name, db_user, db_password_encrypted, host_port, config_json, status,
		        last_started_at, created_at, updated_at
		 FROM project_services
		 WHERE project_id = ? AND environment = ? AND service_type = ?`,
		projectID, environment, string(svcType),
	).Scan(
		&s.ID, &s.ProjectID, &s.Environment, &s.ServiceType, &s.Image,
		&s.DBName, &s.DBUser, &s.DBPasswordEncrypted, &s.HostPort, &s.ConfigJSON, &s.Status,
		&lastStarted, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get service by project+env: %w", err)
	}
	if lastStarted.Valid {
		if t := parseSQLiteDateTime(lastStarted.String); t != nil {
			s.LastStartedAt = NullableTime{NullTime: sql.NullTime{Time: *t, Valid: true}}
		}
	}
	return s, nil
}

// ListServicesByProject returns all services across both environments.
func (db *DB) ListServicesByProject(projectID string) ([]ProjectService, error) {
	rows, err := db.Query(
		`SELECT id, project_id, environment, service_type, image,
		        db_name, db_user, db_password_encrypted, host_port, config_json, status,
		        last_started_at, created_at, updated_at
		 FROM project_services
		 WHERE project_id = ?
		 ORDER BY environment, service_type`, projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	defer rows.Close()

	var out []ProjectService
	for rows.Next() {
		var s ProjectService
		var lastStarted sql.NullString
		if err := rows.Scan(
			&s.ID, &s.ProjectID, &s.Environment, &s.ServiceType, &s.Image,
			&s.DBName, &s.DBUser, &s.DBPasswordEncrypted, &s.HostPort, &s.ConfigJSON, &s.Status,
			&lastStarted, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan service: %w", err)
		}
		if lastStarted.Valid {
			if t := parseSQLiteDateTime(lastStarted.String); t != nil {
				s.LastStartedAt = NullableTime{NullTime: sql.NullTime{Time: *t, Valid: true}}
			}
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// UpdateServiceHostPort persists the loopback host port assigned by Docker
// after EnsureRunning. last_started_at is bumped to NOW so the UI can show
// "last restarted X ago". Note: SQLite has no auto-update trigger for
// updated_at — every UPDATE path explicitly sets it.
func (db *DB) UpdateServiceHostPort(id string, hostPort int) error {
	_, err := db.Exec(
		`UPDATE project_services
		 SET host_port = ?, last_started_at = datetime('now'), updated_at = datetime('now')
		 WHERE id = ?`, hostPort, id,
	)
	if err != nil {
		return fmt.Errorf("update service host_port: %w", err)
	}
	return nil
}

// UpdateServiceStatus persists a status transition.
func (db *DB) UpdateServiceStatus(id string, status ServiceStatus) error {
	_, err := db.Exec(
		`UPDATE project_services
		 SET status = ?, updated_at = datetime('now')
		 WHERE id = ?`, string(status), id,
	)
	if err != nil {
		return fmt.Errorf("update service status: %w", err)
	}
	return nil
}

// UpdateServicePassword replaces the encrypted password (used by regenerate).
func (db *DB) UpdateServicePassword(id, encrypted string) error {
	_, err := db.Exec(
		`UPDATE project_services
		 SET db_password_encrypted = ?, updated_at = datetime('now')
		 WHERE id = ?`, encrypted, id,
	)
	if err != nil {
		return fmt.Errorf("update service password: %w", err)
	}
	return nil
}

// DeleteService removes the row. The container + volume are cleaned up
// separately by the services.Manager before this is called.
func (db *DB) DeleteService(id string) error {
	_, err := db.Exec(`DELETE FROM project_services WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	return nil
}

// ServicesExist reports whether the project has ANY service rows. Used by the
// rename guard — renaming a project would orphan its container + volume names.
func (db *DB) ServicesExist(projectID string) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM project_services WHERE project_id = ?`, projectID,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("services exist: %w", err)
	}
	return n > 0, nil
}
