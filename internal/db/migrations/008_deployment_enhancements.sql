ALTER TABLE deployments ADD COLUMN trigger_source TEXT NOT NULL DEFAULT 'manual'
    CHECK (trigger_source IN ('manual', 'webhook', 'api'));
ALTER TABLE deployments ADD COLUMN triggered_by_username TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN screenshot_path TEXT;
