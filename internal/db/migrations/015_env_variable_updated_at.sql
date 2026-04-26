ALTER TABLE env_variables ADD COLUMN updated_at DATETIME;

UPDATE env_variables SET updated_at = created_at WHERE updated_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_env_variables_updated_at
ON env_variables(project_id, environment, updated_at);
