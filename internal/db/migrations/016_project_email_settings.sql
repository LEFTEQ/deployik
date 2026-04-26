CREATE TABLE IF NOT EXISTS project_email_settings (
    project_id TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    provider TEXT NOT NULL DEFAULT 'webglobe' CHECK (provider IN ('webglobe', 'smtp')),
    smtp_host TEXT NOT NULL DEFAULT '',
    smtp_port INTEGER NOT NULL DEFAULT 587,
    smtp_security TEXT NOT NULL DEFAULT 'starttls' CHECK (smtp_security IN ('starttls', 'tls', 'none')),
    smtp_user TEXT NOT NULL DEFAULT '',
    email_from TEXT NOT NULL DEFAULT '',
    email_from_name TEXT NOT NULL DEFAULT '',
    contact_email_to TEXT NOT NULL DEFAULT '',
    recaptcha_site_key TEXT NOT NULL DEFAULT '',
    recaptcha_mode TEXT NOT NULL DEFAULT 'v3' CHECK (recaptcha_mode IN ('v3')),
    recaptcha_score_threshold REAL NOT NULL DEFAULT 0.5,
    status TEXT NOT NULL DEFAULT 'not_configured' CHECK (status IN ('not_configured', 'ready_to_install', 'smtp_tested', 'error')),
    last_tested_at DATETIME,
    last_test_error TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
