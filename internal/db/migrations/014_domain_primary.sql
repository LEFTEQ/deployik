-- Explicit "primary domain per environment" selection.
--
-- Existing UI helpers inferred a primary domain from is_auto and insertion
-- order, which made "canonical" host selection implicit and impossible to
-- control. This stores that intent directly and enforces at most one primary
-- per (project_id, environment) scope.

ALTER TABLE domains ADD COLUMN is_primary INTEGER NOT NULL DEFAULT 0;

-- Backfill one primary per (project_id, environment).
-- Preview prefers auto domains; production prefers custom domains.
-- Ties break by created_at ASC, then id ASC.
UPDATE domains
SET is_primary = 1
WHERE id IN (
    SELECT d1.id
    FROM domains d1
    WHERE NOT EXISTS (
        SELECT 1
        FROM domains d2
        WHERE d2.project_id = d1.project_id
          AND d2.environment = d1.environment
          AND d2.id != d1.id
          AND (
              (d1.environment = 'preview' AND d2.is_auto > d1.is_auto) OR
              (d1.environment = 'production' AND d2.is_auto < d1.is_auto) OR
              (d2.is_auto = d1.is_auto AND (
                  d2.created_at < d1.created_at OR
                  (d2.created_at = d1.created_at AND d2.id < d1.id)
              ))
          )
    )
);

CREATE UNIQUE INDEX idx_domains_one_primary_per_env
    ON domains(project_id, environment)
    WHERE is_primary = 1;
