-- Migration 029: coordinated app deploys (deploy-together + single rollback).
--
-- projects.deploy_order  — member ordering within an app deploy, honored only
--   when the app's deploy_ordered = 1 (low deploys first; equal = parallel).
-- project_services.app_id — forward-compatible hook so a service can be
--   associated with an app. Per design non-goal D6, the existing live DB stays
--   project-attached; project_id remains NOT NULL here (full app-ownership /
--   re-home is a later backup-gated step). This column lets app-owned-service
--   work land without another projects-table migration.
-- app_releases / app_release_members — the snapshot of one coordinated deploy:
--   exactly which deployment each member ran, so a single rollback can redeploy
--   the whole set to a known-good point.
ALTER TABLE projects ADD COLUMN deploy_order INTEGER NOT NULL DEFAULT 0;
ALTER TABLE project_services ADD COLUMN app_id TEXT REFERENCES apps(id) ON DELETE CASCADE;
CREATE INDEX idx_project_services_app ON project_services(app_id);

CREATE TABLE app_releases (
  id          TEXT PRIMARY KEY,
  app_id      TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
  environment TEXT NOT NULL CHECK (environment IN ('preview','production')),
  status      TEXT NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending','succeeded','failed','rolled_back')),
  created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
  updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_app_releases_app ON app_releases(app_id, environment);

CREATE TABLE app_release_members (
  release_id    TEXT NOT NULL REFERENCES app_releases(id) ON DELETE CASCADE,
  project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  deployment_id TEXT REFERENCES deployments(id),
  PRIMARY KEY (release_id, project_id)
);
