-- Migration 030: fix project_services.app_id foreign key.
--
-- 029 added app_id with ON DELETE CASCADE, which would DELETE a project's
-- Postgres service row (orphaning its container + data volume) if the app it is
-- associated with is removed. That is a data-safety landmine. The correct
-- behavior is ON DELETE SET NULL: removing an app detaches its services, never
-- destroys them.
--
-- SQLite cannot ALTER a foreign-key constraint, so the table is rebuilt
-- preserving every row + index. Nothing references project_services, so the
-- drop/rename is safe with foreign_keys ON. Runs once, inside a transaction.
CREATE TABLE project_services_new (
  id                    TEXT PRIMARY KEY,
  project_id            TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  environment           TEXT NOT NULL CHECK (environment IN ('preview','production')),
  service_type          TEXT NOT NULL CHECK (service_type IN ('postgres')),
  image                 TEXT NOT NULL DEFAULT 'postgres:16',
  db_name               TEXT NOT NULL,
  db_user               TEXT NOT NULL,
  db_password_encrypted TEXT NOT NULL,
  host_port             INTEGER NOT NULL DEFAULT 0,
  config_json           TEXT NOT NULL DEFAULT '{}',
  status                TEXT NOT NULL DEFAULT 'pending'
                          CHECK (status IN ('pending','running','stopped','failed')),
  last_started_at       DATETIME,
  created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  app_id                TEXT REFERENCES apps(id) ON DELETE SET NULL,
  UNIQUE(project_id, environment, service_type)
);

INSERT INTO project_services_new
  (id, project_id, environment, service_type, image, db_name, db_user,
   db_password_encrypted, host_port, config_json, status, last_started_at,
   created_at, updated_at, app_id)
SELECT
   id, project_id, environment, service_type, image, db_name, db_user,
   db_password_encrypted, host_port, config_json, status, last_started_at,
   created_at, updated_at, app_id
FROM project_services;

DROP TABLE project_services;
ALTER TABLE project_services_new RENAME TO project_services;

CREATE INDEX idx_project_services_project ON project_services(project_id);
CREATE INDEX idx_project_services_app ON project_services(app_id);
