package db

import "fmt"

// ListAppVariables returns an app's variables of a kind for one environment
// scope (no shared/scoped merge — the raw rows).
func (db *DB) ListAppVariables(appID, environment string, kind VariableKind) ([]AppVariable, error) {
	rows, err := db.Query(
		`SELECT id, app_id, environment, kind, key, value, created_at, updated_at
		 FROM app_variables WHERE app_id = ? AND environment = ? AND kind = ?
		 ORDER BY key ASC`, appID, environment, kind,
	)
	if err != nil {
		return nil, fmt.Errorf("list app variables: %w", err)
	}
	defer rows.Close()

	var vars []AppVariable
	for rows.Next() {
		var v AppVariable
		if err := rows.Scan(&v.ID, &v.AppID, &v.Environment, &v.Kind, &v.Key, &v.Value, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan app variable: %w", err)
		}
		vars = append(vars, v)
	}
	return vars, rows.Err()
}

// ListAppVariableKeys returns the distinct keys an app uses in a given store,
// across all environments — used to enforce the env/secret cross-store rule.
func (db *DB) ListAppVariableKeys(appID string, kind VariableKind) ([]string, error) {
	rows, err := db.Query(
		`SELECT DISTINCT key FROM app_variables WHERE app_id = ? AND kind = ? ORDER BY key ASC`,
		appID, kind,
	)
	if err != nil {
		return nil, fmt.Errorf("list app variable keys: %w", err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scan app variable key: %w", err)
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// ListResolvedAppVariables merges an app's shared + environment-scoped variables
// (scoped wins) and returns them as ProjectVariable so they can be layered under
// project variables by the deploy resolver. Only Key/Value/Kind/Environment are
// populated — the merge keys on Key alone.
func (db *DB) ListResolvedAppVariables(appID, environment string, kind VariableKind) ([]ProjectVariable, error) {
	toProjectVars := func(in []AppVariable) []ProjectVariable {
		out := make([]ProjectVariable, 0, len(in))
		for _, v := range in {
			out = append(out, ProjectVariable{
				Environment: v.Environment, Kind: v.Kind, Key: v.Key, Value: v.Value,
			})
		}
		return out
	}

	shared, err := db.ListAppVariables(appID, "shared", kind)
	if err != nil {
		return nil, err
	}
	if environment == "shared" {
		return toProjectVars(shared), nil
	}
	scoped, err := db.ListAppVariables(appID, environment, kind)
	if err != nil {
		return nil, err
	}
	return mergeResolvedProjectVariables(toProjectVars(shared), toProjectVars(scoped)), nil
}

// ListResolvedDeployVariables produces the final variable set a member project
// receives at deploy time, layering app variables UNDERNEATH the project's own:
//
//	app shared → app env → project shared → project env   (most-specific wins)
//
// Inert for standalone projects: when AppID is empty there is no app layer and
// the result is byte-identical to ListResolvedProjectVariables. Values stay
// encrypted; the merge keys on Key only, so the caller decrypts the winner.
func (db *DB) ListResolvedDeployVariables(project *Project, environment string, kind VariableKind) ([]ProjectVariable, error) {
	var appVars []ProjectVariable
	if project.AppID != "" {
		v, err := db.ListResolvedAppVariables(project.AppID, environment, kind)
		if err != nil {
			return nil, err
		}
		appVars = v
	}
	projVars, err := db.ListResolvedProjectVariables(project.ID, environment, kind)
	if err != nil {
		return nil, err
	}
	// projVars last → project overrides app on key collision.
	return mergeResolvedProjectVariables(appVars, projVars), nil
}

func (db *DB) ListResolvedDeployEnvVars(project *Project, environment string) ([]ProjectVariable, error) {
	return db.ListResolvedDeployVariables(project, environment, VariableKindEnv)
}

func (db *DB) ListResolvedDeploySecrets(project *Project, environment string) ([]ProjectVariable, error) {
	return db.ListResolvedDeployVariables(project, environment, VariableKindSecret)
}

// UpsertAppVariable inserts or updates a single app variable by (app, env, key).
func (db *DB) UpsertAppVariable(v *AppVariable) error {
	if v.ID == "" {
		v.ID = NewID()
	}
	if v.Kind == "" {
		v.Kind = VariableKindEnv
	}
	if v.Environment == "" {
		v.Environment = "shared"
	}
	_, err := db.Exec(
		`INSERT INTO app_variables (id, app_id, environment, kind, key, value, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, datetime('now'))
		 ON CONFLICT(app_id, environment, key) DO UPDATE SET
		   value = excluded.value, id = excluded.id, kind = excluded.kind, updated_at = datetime('now')`,
		v.ID, v.AppID, v.Environment, v.Kind, v.Key, v.Value,
	)
	if err != nil {
		return fmt.Errorf("upsert app variable: %w", err)
	}
	return nil
}

// BulkSetAppVariables replaces all variables of a kind for an app+environment.
func (db *DB) BulkSetAppVariables(appID, environment string, kind VariableKind, vars []AppVariable) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`DELETE FROM app_variables WHERE app_id = ? AND environment = ? AND kind = ?`,
		appID, environment, kind,
	); err != nil {
		return fmt.Errorf("delete existing app vars: %w", err)
	}
	for _, v := range vars {
		if _, err := tx.Exec(
			`INSERT INTO app_variables (id, app_id, environment, kind, key, value, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, datetime('now'))`,
			NewID(), appID, environment, kind, v.Key, v.Value,
		); err != nil {
			return fmt.Errorf("insert app variable %s: %w", v.Key, err)
		}
	}
	return tx.Commit()
}

// DeleteAppVariable removes one app variable by (app, env, key, kind).
func (db *DB) DeleteAppVariable(appID, environment, key string, kind VariableKind) error {
	if _, err := db.Exec(
		`DELETE FROM app_variables WHERE app_id = ? AND environment = ? AND key = ? AND kind = ?`,
		appID, environment, key, kind,
	); err != nil {
		return fmt.Errorf("delete app variable: %w", err)
	}
	return nil
}
