-- Migration 023: per-project service sidecars (Postgres in v1; reserved for
-- redis/mysql later via service_type discriminator).
--
-- One row per (project, environment, service_type). The Postgres container
-- and its named data volume are deterministic from those keys
-- ("deployik-<project>-<env>-pg" / "deployik-<project>-<env>-pg-data") so the
-- row doesn't need to store them — only the credentials and assigned host
-- loopback port. Passwords are AES-256-GCM encrypted via internal/crypto,
-- never logged, never returned by list endpoints — only by an explicit
-- credentials reveal.
--
-- host_port is the random :0 binding Docker assigned on first start. It's
-- restored on container restart by re-running services.EnsureRunning, which
-- reads the live port via DockerClient.GetHostPort and updates this row.
-- Stored as int (0 = "not started yet") rather than nullable to keep the
-- queries simple.
--
-- config_json is reserved for future Postgres knobs (shared_buffers,
-- max_connections) and Redis settings — empty {} on insert so the column
-- never has to be nullable.
CREATE TABLE project_services (
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
  UNIQUE(project_id, environment, service_type)
);
CREATE INDEX idx_project_services_project ON project_services(project_id);
