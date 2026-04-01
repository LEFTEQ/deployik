package db

import "fmt"

func (db *DB) ListEnvVars(projectID, environment string) ([]EnvVariable, error) {
	rows, err := db.Query(
		`SELECT id, project_id, environment, key, value, created_at
		 FROM env_variables WHERE project_id = ? AND environment = ?
		 ORDER BY key ASC`, projectID, environment,
	)
	if err != nil {
		return nil, fmt.Errorf("list env vars: %w", err)
	}
	defer rows.Close()

	var vars []EnvVariable
	for rows.Next() {
		var v EnvVariable
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.Environment, &v.Key, &v.Value, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan env var: %w", err)
		}
		vars = append(vars, v)
	}
	return vars, rows.Err()
}

func (db *DB) UpsertEnvVar(v *EnvVariable) error {
	if v.ID == "" {
		v.ID = NewID()
	}
	_, err := db.Exec(
		`INSERT INTO env_variables (id, project_id, environment, key, value)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(project_id, environment, key) DO UPDATE SET
		   value = excluded.value, id = excluded.id`,
		v.ID, v.ProjectID, v.Environment, v.Key, v.Value,
	)
	if err != nil {
		return fmt.Errorf("upsert env var: %w", err)
	}
	return nil
}

// BulkSetEnvVars replaces all env vars for a project+environment.
func (db *DB) BulkSetEnvVars(projectID, environment string, vars []EnvVariable) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	// Delete existing vars for this project+environment
	if _, err := tx.Exec(
		`DELETE FROM env_variables WHERE project_id = ? AND environment = ?`,
		projectID, environment,
	); err != nil {
		tx.Rollback()
		return fmt.Errorf("delete existing vars: %w", err)
	}

	// Insert new vars
	for _, v := range vars {
		id := NewID()
		if _, err := tx.Exec(
			`INSERT INTO env_variables (id, project_id, environment, key, value)
			 VALUES (?, ?, ?, ?, ?)`,
			id, projectID, environment, v.Key, v.Value,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("insert env var %s: %w", v.Key, err)
		}
	}

	return tx.Commit()
}

func (db *DB) DeleteEnvVar(projectID, environment, key string) error {
	_, err := db.Exec(
		`DELETE FROM env_variables WHERE project_id = ? AND environment = ? AND key = ?`,
		projectID, environment, key,
	)
	if err != nil {
		return fmt.Errorf("delete env var: %w", err)
	}
	return nil
}
