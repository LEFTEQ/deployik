-- Per-project container port.
--
-- Before this, Deployik assumed every deployed container listened on 3000.
-- That was fine for the generated Dockerfile path (Next.js standalone +
-- `serve` both bind 3000) but blew up for user-provided Dockerfiles that
-- serve on a different port (e.g. nginx on 80, Flask on 5000, Express on
-- 8080) — nginx-proxy would upstream to <container>:3000, nothing listened
-- there, and the domain returned 502.
--
-- Now every project carries the port its deployed container actually listens
-- on. The pipeline uses it for `ExposedPorts`, `PortBindings` (host-port
-- mode), and the proxy upstream. Generated Dockerfiles also honor it (so
-- `ENV PORT`, `EXPOSE`, healthcheck, and `serve -l` all stay consistent).
--
-- 3000 is the existing default, so pre-existing projects keep working with
-- no action required.

ALTER TABLE projects ADD COLUMN port INTEGER NOT NULL DEFAULT 3000;
