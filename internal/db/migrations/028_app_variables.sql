-- Migration 028: app-level env vars / secrets.
--
-- Mirrors env_variables (002_project_variable_kinds.sql) but scoped to an app
-- instead of a project. Set once on the App, these are layered UNDERNEATH each
-- member project's own variables at deploy time (app shared → app env → project
-- shared → project env, most-specific wins). This is where app-owned shared
-- tokens and the app Postgres DATABASE_URL land (P4), so members inherit them
-- without per-project duplication.
--
-- Inert until an app actually has variables: standalone projects never read it.
CREATE TABLE app_variables (
  id          TEXT PRIMARY KEY,
  app_id      TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
  environment TEXT NOT NULL DEFAULT 'shared',
  kind        TEXT NOT NULL DEFAULT 'env',
  key         TEXT NOT NULL,
  value       TEXT NOT NULL,
  created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
  updated_at  DATETIME NOT NULL DEFAULT (datetime('now')),
  UNIQUE(app_id, environment, key)
);
CREATE INDEX idx_app_variables_app ON app_variables(app_id);
