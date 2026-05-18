ALTER TABLE organizations
ADD COLUMN display_order INTEGER NOT NULL DEFAULT 0;

UPDATE organizations
SET display_order = rowid
WHERE display_order = 0;

CREATE TABLE group_invitations (
  id                 TEXT PRIMARY KEY,
  organization_id    TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  github_username    TEXT NOT NULL,
  role               TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'member')),
  invited_by_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  status             TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'declined', 'canceled')),
  responded_at       DATETIME,
  created_at         DATETIME NOT NULL DEFAULT (datetime('now')),
  updated_at         DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX idx_group_invitations_pending_recipient
ON group_invitations(organization_id, github_username)
WHERE status = 'pending';

CREATE INDEX idx_group_invitations_username_status
ON group_invitations(github_username, status);

CREATE INDEX idx_group_invitations_organization
ON group_invitations(organization_id, status);
