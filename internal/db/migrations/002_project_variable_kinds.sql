CREATE TABLE env_variables_v2 (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    environment TEXT NOT NULL DEFAULT 'preview' CHECK (environment IN ('shared', 'preview', 'production')),
    kind TEXT NOT NULL DEFAULT 'env' CHECK (kind IN ('env', 'secret')),
    key TEXT NOT NULL,
    value TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(project_id, environment, key)
);

INSERT INTO env_variables_v2 (id, project_id, environment, kind, key, value, created_at)
SELECT id, project_id, environment, 'env', key, value, created_at
FROM env_variables;

DROP TABLE env_variables;

ALTER TABLE env_variables_v2 RENAME TO env_variables;

CREATE INDEX IF NOT EXISTS idx_env_variables_project_env
ON env_variables(project_id, environment);

CREATE INDEX IF NOT EXISTS idx_env_variables_project_env_kind
ON env_variables(project_id, environment, kind);
