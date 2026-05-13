-- Migration 022: per-project start command and health check path.
--
-- These two columns make the new 'node-api' framework usable: the generated
-- Dockerfile needs a CMD (Express/Hono/Fastify all differ from NestJS's
-- canonical 'node dist/main.js') and a HEALTHCHECK target (most APIs expose
-- /health; some use /healthz or /api/health). The values are stored as plain
-- TEXT and validated in projectconfig.Resolve so the SQL stays permissive.
--
-- Defaults are empty strings; projectconfig.Resolve fills in framework-aware
-- defaults at runtime, so pre-existing projects continue to behave identically
-- (their framework is still nextjs/vite/astro/static and those runtimes ignore
-- start_command + health_path entirely).
ALTER TABLE projects ADD COLUMN start_command TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN health_path   TEXT NOT NULL DEFAULT '';
