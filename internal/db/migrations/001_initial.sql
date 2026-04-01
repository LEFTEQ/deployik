-- Deployik initial schema

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    github_id INTEGER UNIQUE NOT NULL,
    username TEXT NOT NULL,
    avatar_url TEXT NOT NULL DEFAULT '',
    github_token TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user')),
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    github_repo TEXT NOT NULL,
    github_owner TEXT NOT NULL,
    branch TEXT NOT NULL DEFAULT 'main',
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    framework TEXT NOT NULL DEFAULT 'nextjs',
    build_command TEXT NOT NULL DEFAULT 'bun run build',
    install_command TEXT NOT NULL DEFAULT 'bun install',
    node_version TEXT NOT NULL DEFAULT '22',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'deleted')),
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS deployments (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    environment TEXT NOT NULL DEFAULT 'preview' CHECK (environment IN ('preview', 'production')),
    commit_sha TEXT NOT NULL DEFAULT '',
    commit_message TEXT NOT NULL DEFAULT '',
    branch TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'building', 'deploying', 'live', 'failed', 'rolled_back', 'replaced')),
    container_id TEXT NOT NULL DEFAULT '',
    container_name TEXT NOT NULL DEFAULT '',
    image_tag TEXT NOT NULL DEFAULT '',
    build_duration INTEGER NOT NULL DEFAULT 0,
    triggered_by TEXT NOT NULL DEFAULT '' REFERENCES users(id) ON DELETE SET DEFAULT,
    error_message TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    finished_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_deployments_project_id ON deployments(project_id);
CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments(status);
CREATE INDEX IF NOT EXISTS idx_deployments_project_env ON deployments(project_id, environment);

CREATE TABLE IF NOT EXISTS build_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    deployment_id TEXT NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    line_number INTEGER NOT NULL,
    content TEXT NOT NULL,
    stream TEXT NOT NULL DEFAULT 'stdout' CHECK (stream IN ('stdout', 'stderr')),
    timestamp DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_build_logs_deployment_id ON build_logs(deployment_id);

CREATE TABLE IF NOT EXISTS domains (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    domain TEXT UNIQUE NOT NULL,
    environment TEXT NOT NULL DEFAULT 'preview' CHECK (environment IN ('preview', 'production')),
    is_auto INTEGER NOT NULL DEFAULT 0,
    dns_verified INTEGER NOT NULL DEFAULT 0,
    ssl_status TEXT NOT NULL DEFAULT 'pending' CHECK (ssl_status IN ('pending', 'active', 'error')),
    ssl_expires_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_domains_project_id ON domains(project_id);
CREATE INDEX IF NOT EXISTS idx_domains_domain ON domains(domain);

CREATE TABLE IF NOT EXISTS env_variables (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    environment TEXT NOT NULL DEFAULT 'preview' CHECK (environment IN ('preview', 'production')),
    key TEXT NOT NULL,
    value TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(project_id, environment, key)
);

CREATE INDEX IF NOT EXISTS idx_env_variables_project_env ON env_variables(project_id, environment);
