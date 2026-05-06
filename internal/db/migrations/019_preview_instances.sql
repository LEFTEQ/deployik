CREATE TABLE IF NOT EXISTS preview_instances (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    branch TEXT NOT NULL,
    branch_slug TEXT NOT NULL,
    is_default INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'deleted')),
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO preview_instances (id, project_id, branch, branch_slug, is_default, status, created_at, updated_at)
SELECT
    p.id || '-preview',
    p.id,
    p.branch,
    lower(replace(replace(replace(trim(p.branch), '/', '-'), '_', '-'), ' ', '-')),
    1,
    'active',
    p.created_at,
    p.updated_at
FROM projects p
WHERE p.status != 'deleted'
  AND NOT EXISTS (
      SELECT 1 FROM preview_instances pi WHERE pi.project_id = p.id AND pi.branch = p.branch
  );

CREATE UNIQUE INDEX IF NOT EXISTS idx_preview_instances_project_branch
ON preview_instances(project_id, branch);

CREATE UNIQUE INDEX IF NOT EXISTS idx_preview_instances_project_slug
ON preview_instances(project_id, branch_slug);

CREATE INDEX IF NOT EXISTS idx_preview_instances_project_status
ON preview_instances(project_id, status);

ALTER TABLE deployments
ADD COLUMN preview_instance_id TEXT REFERENCES preview_instances(id) ON DELETE SET NULL;

ALTER TABLE domains
ADD COLUMN preview_instance_id TEXT REFERENCES preview_instances(id) ON DELETE CASCADE;

ALTER TABLE webhook_events
ADD COLUMN preview_instance_id TEXT REFERENCES preview_instances(id) ON DELETE SET NULL;

UPDATE deployments
SET preview_instance_id = (
    SELECT pi.id
    FROM preview_instances pi
    WHERE pi.project_id = deployments.project_id
      AND pi.is_default = 1
    ORDER BY pi.created_at ASC
    LIMIT 1
)
WHERE environment = 'preview'
  AND preview_instance_id IS NULL;

UPDATE domains
SET preview_instance_id = (
    SELECT pi.id
    FROM preview_instances pi
    WHERE pi.project_id = domains.project_id
      AND pi.is_default = 1
    ORDER BY pi.created_at ASC
    LIMIT 1
)
WHERE environment = 'preview'
  AND preview_instance_id IS NULL;

UPDATE webhook_events
SET preview_instance_id = (
    SELECT pi.id
    FROM preview_instances pi
    WHERE pi.project_id = webhook_events.project_id
      AND pi.is_default = 1
    ORDER BY pi.created_at ASC
    LIMIT 1
)
WHERE environment = 'preview'
  AND preview_instance_id IS NULL;

DROP INDEX IF EXISTS idx_domains_one_primary_per_env;

CREATE UNIQUE INDEX IF NOT EXISTS idx_domains_one_primary_per_target
ON domains(project_id, environment, COALESCE(preview_instance_id, ''))
WHERE is_primary = 1;

CREATE INDEX IF NOT EXISTS idx_domains_preview_instance_id
ON domains(preview_instance_id);

CREATE INDEX IF NOT EXISTS idx_deployments_preview_instance_id
ON deployments(preview_instance_id);

