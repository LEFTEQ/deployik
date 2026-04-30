ALTER TABLE auto_build_configs
ADD COLUMN auto_production_enabled INTEGER NOT NULL DEFAULT 0;

ALTER TABLE webhook_events RENAME TO webhook_events_old;

CREATE TABLE webhook_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    github_delivery_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    environment TEXT NOT NULL DEFAULT 'ignored'
        CHECK (environment IN ('preview', 'production', 'ignored')),
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

INSERT INTO webhook_events (
    id,
    project_id,
    github_delivery_id,
    event_type,
    environment,
    branch,
    commit_sha,
    commit_message,
    pusher,
    deployment_id,
    status,
    error_message,
    created_at
)
SELECT
    old.id,
    old.project_id,
    old.github_delivery_id,
    old.event_type,
    CASE
        WHEN old.status = 'ignored' THEN 'ignored'
        WHEN old.deployment_id IS NOT NULL THEN COALESCE(
            (SELECT d.environment FROM deployments d WHERE d.id = old.deployment_id),
            'preview'
        )
        ELSE 'ignored'
    END,
    old.branch,
    old.commit_sha,
    old.commit_message,
    old.pusher,
    old.deployment_id,
    old.status,
    old.error_message,
    old.created_at
FROM webhook_events_old old;

DROP TABLE webhook_events_old;

CREATE INDEX IF NOT EXISTS idx_webhook_events_project_id
ON webhook_events(project_id);

CREATE INDEX IF NOT EXISTS idx_webhook_events_delivery_id
ON webhook_events(github_delivery_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_webhook_events_delivery_project_env
ON webhook_events(github_delivery_id, project_id, environment);
