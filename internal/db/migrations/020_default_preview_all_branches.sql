-- Migration 020: default preview branches to "*" (all branches).
-- Earlier project-create code wrote preview_branches = projects.branch (the bad
-- default) so pushes to any other branch were silently ignored. Rewrite any row
-- that still holds that value so users don't have to fix the field by hand.
-- Rows with explicit lists ("develop,staging") or already "*" are preserved.
UPDATE auto_build_configs
SET preview_branches = '*',
    updated_at = datetime('now')
WHERE preview_branches = (
    SELECT projects.branch
    FROM projects
    WHERE projects.id = auto_build_configs.project_id
);
