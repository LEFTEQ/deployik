-- Migration 021: per-project resource tier (Nano / Small / Medium / Large).
-- Drives runtime container limits (memory, CPU, swap, pids, OOM score) and the
-- build-phase --memory / --cpus caps passed to `docker buildx build`. The new
-- column defaults to 'small' (512 MB / 1.0 CPU) which exactly matches the
-- previous hardcoded limits, so existing rows pick up DEFAULT and behavior is
-- byte-identical on the current container until the next deploy.
ALTER TABLE projects
  ADD COLUMN resource_tier TEXT NOT NULL DEFAULT 'small'
  CHECK (resource_tier IN ('nano', 'small', 'medium', 'large'));
