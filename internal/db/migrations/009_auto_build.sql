CREATE TABLE IF NOT EXISTS auto_build_configs (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL UNIQUE REFERENCES projects(id) ON DELETE CASCADE,
    enabled INTEGER NOT NULL DEFAULT 0,
    production_branch TEXT NOT NULL DEFAULT 'main',
    preview_branches TEXT NOT NULL DEFAULT '*',
    webhook_id INTEGER,
    webhook_secret TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_auto_build_configs_project_id
ON auto_build_configs(project_id);

CREATE TABLE IF NOT EXISTS webhook_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    github_delivery_id TEXT NOT NULL UNIQUE,
    event_type TEXT NOT NULL,
    branch TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    commit_message TEXT NOT NULL DEFAULT '',
    pusher TEXT NOT NULL DEFAULT '',
    deployment_id TEXT REFERENCES deployments(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'received'
        CHECK (status IN ('received', 'processed', 'ignored', 'failed')),
    error_message TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_webhook_events_project_id
ON webhook_events(project_id);

CREATE INDEX IF NOT EXISTS idx_webhook_events_delivery_id
ON webhook_events(github_delivery_id);
