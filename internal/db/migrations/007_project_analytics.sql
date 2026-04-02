CREATE TABLE IF NOT EXISTS project_analytics (
    project_id TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    audience_enabled INTEGER NOT NULL DEFAULT 1 CHECK (audience_enabled IN (0, 1)),
    tracking_mode TEXT NOT NULL DEFAULT 'ai_install' CHECK (tracking_mode IN ('ai_install', 'manual', 'disabled')),
    audience_status TEXT NOT NULL DEFAULT 'ready_to_install' CHECK (
        audience_status IN ('provisioning', 'ready_to_install', 'waiting_for_data', 'receiving_data', 'stale', 'unavailable', 'error')
    ),
    umami_website_id TEXT NOT NULL DEFAULT '',
    umami_website_name TEXT NOT NULL DEFAULT '',
    last_event_at DATETIME,
    verified_at DATETIME,
    last_error TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_project_analytics_website_id
ON project_analytics(umami_website_id);
