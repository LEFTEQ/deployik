-- Migration 026: first-class "apps" — a bundle of projects inside a workspace
-- (organizations row). An app groups several independently-deployed projects so
-- they can later share a network, env, and a coordinated deploy (P3/P4). This
-- phase is inert: only the entity + the nullable projects.app_id link exist.
--
-- deploy_ordered is created now (an attribute of the entity) but is not acted
-- upon until the coordinated-deploy phase.
CREATE TABLE apps (
  id              TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name            TEXT NOT NULL,
  slug            TEXT NOT NULL,
  deploy_ordered  INTEGER NOT NULL DEFAULT 0,
  display_order   INTEGER NOT NULL DEFAULT 0,
  created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
  updated_at      DATETIME NOT NULL DEFAULT (datetime('now')),
  UNIQUE(organization_id, slug)
);
CREATE INDEX idx_apps_organization ON apps(organization_id);

-- Nullable FK; SET NULL on app delete so a project survives its app's removal.
-- (Same shape as 006_organizations.sql's organization_id ALTER, which SQLite
-- allows because the added column defaults to NULL.)
ALTER TABLE projects ADD COLUMN app_id TEXT REFERENCES apps(id) ON DELETE SET NULL;
CREATE INDEX idx_projects_app ON projects(app_id);
