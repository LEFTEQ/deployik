CREATE TABLE IF NOT EXISTS organizations (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    is_personal INTEGER NOT NULL DEFAULT 0 CHECK (is_personal IN (0, 1)),
    personal_owner_user_id TEXT UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS organization_memberships (
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'member')),
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (organization_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_organization_memberships_user_id
ON organization_memberships(user_id);

ALTER TABLE projects ADD COLUMN organization_id TEXT REFERENCES organizations(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_projects_organization_id
ON projects(organization_id);
