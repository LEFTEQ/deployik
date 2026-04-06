package db

import (
	"database/sql"
	"fmt"
)

func (db *DB) GetAutoBuildConfig(projectID string) (*AutoBuildConfig, error) {
	c := &AutoBuildConfig{}
	err := db.QueryRow(
		`SELECT id, project_id, enabled, production_branch, preview_branches,
		        webhook_id, webhook_secret, created_at, updated_at
		 FROM auto_build_configs
		 WHERE project_id = ?`,
		projectID,
	).Scan(
		&c.ID,
		&c.ProjectID,
		&c.Enabled,
		&c.ProductionBranch,
		&c.PreviewBranches,
		&c.WebhookID,
		&c.WebhookSecret,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get auto build config: %w", err)
	}
	return c, nil
}

func (db *DB) UpsertAutoBuildConfig(c *AutoBuildConfig) error {
	if c.ID == "" {
		c.ID = NewID()
	}
	_, err := db.Exec(
		`INSERT INTO auto_build_configs (id, project_id, enabled, production_branch, preview_branches, webhook_id, webhook_secret)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(project_id) DO UPDATE SET
		     enabled = excluded.enabled,
		     production_branch = excluded.production_branch,
		     preview_branches = excluded.preview_branches,
		     webhook_id = excluded.webhook_id,
		     webhook_secret = CASE WHEN excluded.webhook_secret = '' THEN webhook_secret ELSE excluded.webhook_secret END,
		     updated_at = datetime('now')`,
		c.ID, c.ProjectID, c.Enabled, c.ProductionBranch, c.PreviewBranches, c.WebhookID, c.WebhookSecret,
	)
	if err != nil {
		return fmt.Errorf("upsert auto build config: %w", err)
	}

	stored, err := db.GetAutoBuildConfig(c.ProjectID)
	if err != nil {
		return err
	}
	if stored != nil {
		*c = *stored
	}
	return nil
}

func (db *DB) DeleteAutoBuildConfig(projectID string) error {
	if _, err := db.Exec(`DELETE FROM auto_build_configs WHERE project_id = ?`, projectID); err != nil {
		return fmt.Errorf("delete auto build config: %w", err)
	}
	return nil
}

func (db *DB) ListActiveAutoBuildConfigsByRepo(owner, repo string) ([]AutoBuildConfig, error) {
	rows, err := db.Query(
		`SELECT c.id, c.project_id, c.enabled, c.production_branch, c.preview_branches,
		        c.webhook_id, c.webhook_secret, c.created_at, c.updated_at
		 FROM auto_build_configs c
		 JOIN projects p ON p.id = c.project_id
		 WHERE p.github_owner = ? AND p.github_repo = ?
		   AND p.status = 'active' AND c.enabled = 1`,
		owner, repo,
	)
	if err != nil {
		return nil, fmt.Errorf("list active auto build configs by repo: %w", err)
	}
	defer rows.Close()

	var configs []AutoBuildConfig
	for rows.Next() {
		var c AutoBuildConfig
		if err := rows.Scan(
			&c.ID, &c.ProjectID, &c.Enabled, &c.ProductionBranch, &c.PreviewBranches,
			&c.WebhookID, &c.WebhookSecret, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan auto build config: %w", err)
		}
		configs = append(configs, c)
	}
	return configs, rows.Err()
}
