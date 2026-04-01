package db

import (
	"fmt"
	"sort"
)

func (db *DB) ListProjectVariables(projectID, environment string, kind VariableKind) ([]ProjectVariable, error) {
	rows, err := db.Query(
		`SELECT id, project_id, environment, kind, key, value, created_at
		 FROM env_variables WHERE project_id = ? AND environment = ? AND kind = ?
		 ORDER BY key ASC`, projectID, environment, kind,
	)
	if err != nil {
		return nil, fmt.Errorf("list project variables: %w", err)
	}
	defer rows.Close()

	var vars []ProjectVariable
	for rows.Next() {
		var v ProjectVariable
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.Environment, &v.Kind, &v.Key, &v.Value, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan project variable: %w", err)
		}
		vars = append(vars, v)
	}
	return vars, rows.Err()
}

func (db *DB) ListProjectVariableKeys(projectID string, kind VariableKind) ([]string, error) {
	rows, err := db.Query(
		`SELECT DISTINCT key
		 FROM env_variables WHERE project_id = ? AND kind = ?
		 ORDER BY key ASC`,
		projectID, kind,
	)
	if err != nil {
		return nil, fmt.Errorf("list project variable keys: %w", err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scan project variable key: %w", err)
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (db *DB) ListEnvVars(projectID, environment string) ([]ProjectVariable, error) {
	return db.ListProjectVariables(projectID, environment, VariableKindEnv)
}

func (db *DB) ListSecrets(projectID, environment string) ([]ProjectVariable, error) {
	return db.ListProjectVariables(projectID, environment, VariableKindSecret)
}

func mergeResolvedProjectVariables(sharedVars, scopedVars []ProjectVariable) []ProjectVariable {
	byKey := make(map[string]ProjectVariable, len(sharedVars)+len(scopedVars))

	for _, variable := range sharedVars {
		byKey[variable.Key] = variable
	}
	for _, variable := range scopedVars {
		byKey[variable.Key] = variable
	}

	merged := make([]ProjectVariable, 0, len(byKey))
	for _, variable := range byKey {
		merged = append(merged, variable)
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Key < merged[j].Key
	})

	return merged
}

func (db *DB) ListResolvedProjectVariables(projectID, environment string, kind VariableKind) ([]ProjectVariable, error) {
	if environment == "shared" {
		return db.ListProjectVariables(projectID, environment, kind)
	}

	sharedVars, err := db.ListProjectVariables(projectID, "shared", kind)
	if err != nil {
		return nil, err
	}

	scopedVars, err := db.ListProjectVariables(projectID, environment, kind)
	if err != nil {
		return nil, err
	}

	return mergeResolvedProjectVariables(sharedVars, scopedVars), nil
}

func (db *DB) ListResolvedEnvVars(projectID, environment string) ([]ProjectVariable, error) {
	return db.ListResolvedProjectVariables(projectID, environment, VariableKindEnv)
}

func (db *DB) ListResolvedSecrets(projectID, environment string) ([]ProjectVariable, error) {
	return db.ListResolvedProjectVariables(projectID, environment, VariableKindSecret)
}

func (db *DB) UpsertProjectVariable(v *ProjectVariable) error {
	if v.ID == "" {
		v.ID = NewID()
	}
	if v.Kind == "" {
		v.Kind = VariableKindEnv
	}
	_, err := db.Exec(
		`INSERT INTO env_variables (id, project_id, environment, kind, key, value)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(project_id, environment, key) DO UPDATE SET
		   value = excluded.value, id = excluded.id, kind = excluded.kind`,
		v.ID, v.ProjectID, v.Environment, v.Kind, v.Key, v.Value,
	)
	if err != nil {
		return fmt.Errorf("upsert project variable: %w", err)
	}
	return nil
}

func (db *DB) UpsertEnvVar(v *ProjectVariable) error {
	v.Kind = VariableKindEnv
	return db.UpsertProjectVariable(v)
}

// BulkSetProjectVariables replaces all variables of a kind for a project+environment.
func (db *DB) BulkSetProjectVariables(projectID, environment string, kind VariableKind, vars []ProjectVariable) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	// Delete existing vars for this project+environment+kind
	if _, err := tx.Exec(
		`DELETE FROM env_variables WHERE project_id = ? AND environment = ? AND kind = ?`,
		projectID, environment, kind,
	); err != nil {
		tx.Rollback()
		return fmt.Errorf("delete existing vars: %w", err)
	}

	// Insert new vars
	for _, v := range vars {
		id := NewID()
		if _, err := tx.Exec(
			`INSERT INTO env_variables (id, project_id, environment, kind, key, value)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			id, projectID, environment, kind, v.Key, v.Value,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("insert project variable %s: %w", v.Key, err)
		}
	}

	return tx.Commit()
}

func (db *DB) BulkSetEnvVars(projectID, environment string, vars []ProjectVariable) error {
	return db.BulkSetProjectVariables(projectID, environment, VariableKindEnv, vars)
}

func (db *DB) BulkSetSecrets(projectID, environment string, vars []ProjectVariable) error {
	return db.BulkSetProjectVariables(projectID, environment, VariableKindSecret, vars)
}

func (db *DB) DeleteProjectVariable(projectID, environment, key string, kind VariableKind) error {
	_, err := db.Exec(
		`DELETE FROM env_variables WHERE project_id = ? AND environment = ? AND key = ? AND kind = ?`,
		projectID, environment, key, kind,
	)
	if err != nil {
		return fmt.Errorf("delete project variable: %w", err)
	}
	return nil
}

func (db *DB) DeleteEnvVar(projectID, environment, key string) error {
	return db.DeleteProjectVariable(projectID, environment, key, VariableKindEnv)
}

func (db *DB) DeleteSecret(projectID, environment, key string) error {
	return db.DeleteProjectVariable(projectID, environment, key, VariableKindSecret)
}
