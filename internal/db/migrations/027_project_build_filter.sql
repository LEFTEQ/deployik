-- Migration 027: opt-in path-based build filtering (the monorepo fan-out fix).
--
-- A push to a repo webhook-fans a build to EVERY project bound to that repo,
-- regardless of which app's files changed. These two columns let a project opt
-- into changed-path filtering: it rebuilds only when a changed path is under its
-- root_directory or matches one of its watch_paths globs.
--
-- Inert by default: build_filter_enabled defaults 0 → today's "always build".
ALTER TABLE projects ADD COLUMN build_filter_enabled INTEGER NOT NULL DEFAULT 0;

-- watch_paths is a JSON array of globs (e.g. ["packages/shared/**","bun.lock"])
-- for shared dependencies outside root_directory. NULL/empty = none.
ALTER TABLE projects ADD COLUMN watch_paths TEXT;
