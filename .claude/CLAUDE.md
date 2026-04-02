# Deployik

Self-hosted Vercel alternative for the Lovinka VPS. Deploys Next.js and static web apps from GitHub with automatic domains, SSL, environment variables, and blue-green zero-downtime deployments.

## Stack

- **Backend:** Go 1.25 (chi router, Docker SDK, SQLite via modernc.org/sqlite -- pure Go, no CGO)
- **Frontend:** React 19 + Vite 7 + TanStack Router/Query + Zustand + shadcn/ui (new-york style, zinc dark theme) + Tailwind CSS 4
- **Database:** SQLite (embedded, WAL mode, ULID primary keys)
- **Auth:** GitHub OAuth -> JWT (HS256, 1h access / 7d refresh tokens)
- **Encryption:** AES-256-GCM (SHA-256 key derivation) for env vars, secrets, and GitHub tokens at rest
- **Deployment:** Single Go binary embeds React SPA via `//go:embed`; Docker multi-stage build; CI/CD via GitHub Actions to GHCR + VPS

## Commands

| Action | Command |
|---|---|
| Dev API | `make dev-api` (sets DEV_MODE=true) |
| Dev Frontend | `make dev-web` (Vite on :5173, proxies to :8080) |
| Go tests | `go test ./...` |
| Frontend typecheck | `cd web && bunx tsc --noEmit` |
| Build production | `make build` |
| Docker build | `make docker-build` |
| Manual deploy | `./scripts/deploy.sh [tag]` |

## Project Structure

```
cmd/server/main.go        Entry point: loads config, initializes all services, starts HTTP server
cmd/server/web_dist/      Embedded SPA (populated by `make build` or Docker build)

internal/
  api/
    router.go             chi route definitions (public + protected groups)
    spa.go                Serves embedded SPA with client-side routing fallback
    handlers/
      auth.go             GitHub OAuth callback, OAuth state verification, cookie session issuance, refresh, logout, /me
      projects.go         CRUD + GitHub repo/branch listing
    deployments.go      List, trigger, get, build logs; production releases can optionally create a git tag and deploy that tagged ref
      domains.go          Add, list, delete, verify (DNS + SSL)
      envvars.go          VariableHandler -- generic for both env and secret stores
      access.go           loadAuthorizedProject/Deployment helpers (calls authz)
      helpers.go          writeJSON utility
    middleware/
      auth.go             JWT extraction from Bearer header or access cookie
      cors.go             Allowlist-based CORS/origin checks
      ratelimit.go        In-memory per-IP rate limiter for auth, mutations, and ws routes

  auth/
    jwt.go                GenerateAccessToken, ValidateAccessToken
    session.go            Opaque token generation + hashing, shared cookie names
    context.go            WithClaims/GetClaims context helpers

  audit/
    recorder.go           Writes sensitive action events into audit_logs

  analytics/
    service.go            Project-level analytics orchestration (Umami audience + Loki runtime)
    umami.go              Umami API client: login, website provisioning, stats/pageviews/metrics/active queries
    loki.go               Loki HTTP client for summary + timeseries queries
    audience.go           Audience aggregation helpers for multi-host/domain rollups
    options.go            Analytics range/environment/timezone normalization
                          Install payloads support a separate tracker script URL so audience tracking can be served from Lovinka CDN while events still post to Umami

  authz/
    access.go             CanAccessProject, LoadProject, LoadDeployment (ownership + admin bypass)

  build/
    pipeline.go           Full deploy orchestration: clone -> patch -> build -> run -> health -> swap (uses deployment.Branch/ref, so tagged releases deploy the exact tagged commit)
    clone.go              Git shallow clone with OAuth token auth
    docker.go             Docker SDK: BuildImage, RunContainer, StopContainer, WaitForHealthy, ContainerExists
    dockerfile.go         Programmatic Dockerfile generation (Next.js standalone + static site)
    nextjs.go             Patches next.config.* to inject output: 'standalone'
    variables.go          Splits env vars into build-time (NEXT_PUBLIC_*) and runtime sets
    semaphore.go          Channel-based concurrency limiter (default: 1 concurrent build)

  config/
    config.go             Reads all env vars into Config struct with defaults

  crypto/
    encrypt.go            AES-256-GCM Encryptor (Encrypt, Decrypt, MaskValue)

  db/
    sqlite.go             Open/OpenMemory with WAL pragmas
    migrations.go         Embedded SQL runner with _migrations tracking table
    migrations/
      001_initial.sql          Users, projects, deployments, build_logs, domains, env_variables
      002_project_variable_kinds.sql  Adds kind (env/secret) + shared scope to env_variables
      003_project_build_settings.sql  Adds root_directory, output_directory to projects
      004_auth_sessions_and_audit_logs.sql  Adds refresh_tokens and audit_logs
      005_project_package_manager.sql  Adds package_manager to projects
      006_organizations.sql    Adds organizations, organization_memberships, and projects.organization_id
      007_project_analytics.sql Adds project_analytics for linked Umami website + audience analytics status
    models.go             User, Organization, OrganizationMembership, RefreshSession, AuditLog, Project, Deployment, BuildLog, Domain, ProjectVariable, VariableKind
    queries_project_analytics.go  Get/Upsert/Delete project_analytics rows
    queries_users.go      GetUserByGithubID, GetUserByID, UpsertUser
    queries_organizations.go  Personal workspace bootstrap, memberships, org listing
    queries_projects.go   ListProjects, GetProject, GetProjectForUser, Create, Update, Delete (soft)
    queries_deployments.go  List, Get, GetForUser, Create, UpdateStatus/Container/Duration, GetLiveDeployment
    queries_envvars.go    ListProjectVariables, ListResolvedEnvVars/Secrets, BulkSet*, Delete*, key conflict checks
    queries_domains.go    List, GetByName, Create, UpdateDNS/SSL, Delete, DeleteForProject
    queries_buildlogs.go  Insert, GetBuildLogs, PruneBuildLogs
    queries_refresh_tokens.go  Create/GetActive/Rotate/Revoke refresh sessions
    queries_audit_logs.go CreateAuditLog

  domain/
    ssl.go                Manager: ProvisionDomain (DNS verify -> certbot -> nginx -> reload)
    nginx.go              GenerateNginxConfig from Go template, RemoveNginxConfig
    reconcile.go          Rewrites nginx configs for already-active Deployik domains on startup
    dns.go                VerifyDNS (A-record lookup against VPS IP)
    variants.go           Canonicalizes production custom domains so apex stays primary and optional www alias redirects to it

  github/
    oauth.go              OAuthConfig: AuthorizeURL, ExchangeCode; GetUser
    client.go             Client: ListRepos, ListBranches, GetLatestCommit

  projectconfig/
    defaults.go           Framework presets (nextjs, vite, astro, static), Resolve(), path normalization

  ws/
    hub.go                Pub/sub hub: Subscribe, Unsubscribe, Publish per deployment
    logs.go               WebSocket handler with cookie/header auth + origin allowlist

web/src/
  app/app.tsx             TanStack Router tree, QueryClient, providers
  main.tsx                React root render
  pages/
    Login.tsx             GitHub OAuth redirect
    AuthCallback.tsx      Exchanges code/state for cookie session, stores only user state
    Projects.tsx          Dashboard: project list with spinner-based loading states
    NewProject.tsx        Two-step: select repo -> configure build settings
    ProjectDetail.tsx     Overview-first project workspace using shadcn dashboard patterns: compact command bar, quick endpoint links, deployment history, analytics, integration, settings
    DeploymentDetail.tsx  Build log viewer with real-time WebSocket streaming
  components/
    analytics/metric-chart.tsx  Reusable shadcn chart-card wrapper built on ui/chart
    analytics/stat-card.tsx  Reusable shadcn KPI summary card with CardAction/CardFooter layout
    layout/AppLayout.tsx  Protected top-nav shell with centered content container
    layout/CommandPalette.tsx  Global Cmd/Ctrl+K spotlight for actions, workspaces, and project search
    layout/SiteHeader.tsx Compact top navbar with shadcn NavigationMenu, workspace context, and global actions
    projects/build-settings.tsx  Reusable BuildSettingsFields component with framework + package manager presets
    projects/project-analytics.tsx  Analytics tab UI: filters, audience/runtime metrics, setup empty-state routing into Integration
    projects/project-integration.tsx  Analytics setup stepper: install, verify, track events
    BuildLog.tsx          Log viewer with auto-scroll, stderr highlighting
    ui/                   shadcn/ui components (button, card, dialog, input, etc.)
    ui/code-panel.tsx     Reusable fixed-height scrollable code/prompt card with copy action
    ui/spinner.tsx        Shared spinner + centered loading state used instead of skeleton loaders in app pages
    ui/navigation-menu.tsx  Official shadcn top-nav primitives for the app shell
    ui/sidebar.tsx        Official shadcn sidebar primitives kept available for future drawer/secondary navigation use
    ui/chart.tsx          Official shadcn chart primitives for Recharts
  hooks/
    useBuildLogs.ts       WebSocket hook for real-time build log streaming
    use-mobile.ts         Shared mobile breakpoint hook used by shadcn sidebar/drawer
    use-organizations.ts  React Query + Zustand bridge for accessible organizations and selected workspace
  lib/
    api.ts                ApiClient class wrapping fetch with cookie auth, refresh retry, auto-logout on unrecoverable 401
    project-tabs.ts       Shared project tab normalization for URL-driven deep links and deployment-detail return context
    utils.ts              cn() utility (clsx + tailwind-merge)
  store/
    auth.ts               Zustand store for current user/auth status only (tokens stay in HttpOnly cookies)
    organization.ts       Persisted selected workspace/org id
  types/
    api.ts                TypeScript interfaces matching Go models
```

## Database Schema

SQLite with 6 migrations. Tables:

| Table | Key Fields | Notes |
|---|---|---|
| `users` | id (ULID), github_id (unique), username, github_token (encrypted), role (admin/user) | `ADMIN_GITHUB_USERS` provides explicit admin bootstrap; first user only auto-promotes when no admin list is configured |
| `organizations` | id (ULID), name, slug (unique), is_personal, personal_owner_user_id (nullable unique FK) | Every user gets a personal workspace; shared orgs use memberships |
| `organization_memberships` | organization_id (FK), user_id (FK), role (owner/member) | Grants workspace visibility |
| `projects` | id (ULID), name (unique slug), github_repo, github_owner, branch, user_id (creator FK), organization_id (nullable FK), framework, package_manager, root_directory, output_directory, build_command, install_command, node_version, status | Soft-delete via status='deleted' |
| `deployments` | id (ULID), project_id (FK), environment (preview/production), status (queued/building/deploying/live/failed/rolled_back/replaced), commit_sha, container_id, build_duration | |
| `build_logs` | id (auto), deployment_id (FK), line_number, content, stream (stdout/stderr) | |
| `domains` | id (ULID), project_id (FK), domain (unique), environment, is_auto, dns_verified, ssl_status (pending/active/error) | Auto-domains cannot be deleted |
| `env_variables` | id (ULID), project_id (FK), environment (shared/preview/production), kind (env/secret), key, value (encrypted) | UNIQUE(project_id, environment, key) |
| `project_analytics` | project_id (PK/FK), audience_enabled, tracking_mode, audience_status, umami_website_id, last_event_at, verified_at | One linked Umami website per project; stores audience analytics health/provisioning state |
| `refresh_tokens` | id (ULID), user_id (FK), token_hash, expires_at, last_used_at, revoked_at | Opaque refresh tokens are hashed at rest and rotated on use |
| `audit_logs` | id (auto), user_id (nullable FK), action, resource_type, resource_id, project_id, deployment_id, metadata | Records login/refresh/logout and sensitive mutating actions |

## API Endpoints

### Public
- `GET  /api/health` -- Health check
- `GET  /api/auth/github` -- Redirects to GitHub OAuth
- `GET  /api/auth/github/callback?code=&state=` -- Verifies OAuth state, sets session cookies, returns user
- `POST /api/auth/refresh` -- Rotates refresh cookie, returns user
- `POST /api/auth/logout` -- Revokes refresh session and clears cookies

### Protected (access cookie or Bearer token required)
- `GET  /api/auth/me` -- Current user
- `GET  /api/organizations` -- Organizations/workspaces current user can access

**GitHub:**
- `GET  /api/github/repos` -- User's GitHub repos
- `GET  /api/github/branches?owner=&repo=` -- Repo branches

**Projects:**
- `GET    /api/projects?organization_id=` -- List accessible projects, optionally filtered to one workspace
- `POST   /api/projects` -- Create project (auto-creates preview domain; defaults to personal workspace if no `organization_id`)
- `GET    /api/projects/{id}` -- Get project
- `PATCH  /api/projects/{id}` -- Update project
- `DELETE /api/projects/{id}` -- Soft-delete project
- `GET    /api/projects/{id}/analytics?environment=&range=&timezone=` -- Combined project analytics payload (Umami audience + Loki runtime)
- `POST   /api/projects/{id}/analytics/verify?environment=&range=&timezone=` -- Force an analytics refresh / verification cycle

**Deployments:**
- `GET  /api/projects/{id}/deployments` -- List deployments (limit 20)
- `POST /api/projects/{id}/deployments` -- Trigger deployment `{environment, branch?, create_tag?, tag_name?}`
- `GET  /api/projects/{id}/deployments/{did}` -- Get deployment
- `GET  /api/deployments/{did}/logs` -- Get build logs

**Domains:**
- `GET    /api/projects/{id}/domains` -- List domains
- `POST   /api/projects/{id}/domains` -- Add domain `{domain, environment}`
- `DELETE /api/projects/{id}/domains/{did}` -- Delete domain (not auto-domains)
- `POST   /api/projects/{id}/domains/{did}/verify` -- Verify DNS + provision SSL

**Environment Variables:**
- `GET    /api/projects/{id}/env?environment=` -- List env vars (values masked)
- `PUT    /api/projects/{id}/env` -- Bulk set `{environment, variables: [{key, value}]}`
- `DELETE /api/projects/{id}/env/{key}?environment=` -- Delete env var

**Secrets:**
- `GET    /api/projects/{id}/secrets?environment=` -- List secrets (values masked)
- `PUT    /api/projects/{id}/secrets` -- Bulk set
- `DELETE /api/projects/{id}/secrets/{key}?environment=` -- Delete secret

### WebSocket
- `GET /ws/deployments/{did}/logs` -- Real-time build log streaming (access cookie or Bearer token)

## Key Patterns and Conventions

### Go Backend

- **Router:** chi v5 with middleware chain: Logger -> Recoverer -> RequestID -> RealIP -> CORS
- **Auth middleware:** Extracts JWT from `Authorization: Bearer` header or the `deployik_access_token` cookie
- **Authorization:** All project-scoped endpoints call `loadAuthorizedProject()` or `loadAuthorizedDeployment()` from `handlers/access.go`, which delegates to `authz.LoadProject()` / `authz.LoadDeployment()`. Regular users can access projects if they created them or belong to the owning organization; admins still retain bypass access.
- **Session model:** The SPA never stores tokens. `AuthCallback.tsx` exchanges `code` + `state`, the server sets `HttpOnly` cookies, `api.ts` uses `credentials: 'include'`, and route guards rehydrate auth via `/api/auth/me`.
- **Refresh rotation:** Refresh tokens are opaque random strings, stored only as SHA-256 hashes in `refresh_tokens`, revoked on logout, and rotated every time `/api/auth/refresh` succeeds.
- **Perimeter controls:** `cors.go` blocks origins outside `Config.AllowedOrigins`, `ratelimit.go` applies per-IP limits to auth/mutation/ws routes, and `audit/recorder.go` records security-relevant mutations to `audit_logs`.
- **Usage telemetry:** Deployik-managed nginx configs now emit per-project JSON access logs at `/var/log/nginx/deployik-<project-id>-<project-name>-<environment>.json`; the monitoring stack can ship these to Loki for request, bandwidth, latency, and API-path analytics without writing raw events into SQLite.
- **Audience analytics:** Each project maps to one Umami website via `project_analytics`. Provisioning is best-effort on project create/update and lazy on analytics reads, so existing projects gain analytics without a manual migration. Deployik segments preview vs production in the UI by hostname/domain filters instead of using separate Umami websites.
- **Install with AI:** `projects/project-analytics.tsx` surfaces a generated AI-install prompt and exact manual Umami snippet. The prompt is built from project settings (`framework`, `package_manager`, `root_directory`) plus the linked website ID so users can hand it to any coding AI instead of relying on brittle proxy injection.
- **Reusable analytics UI:** `components/analytics/stat-card.tsx` and `components/analytics/metric-chart.tsx` are the shared primitives for KPI cards and chart cards. Reuse them for future dashboard work instead of building one-off chart wrappers inside each page.
- **IDs:** ULIDs generated via `db.NewID()` (time-sortable, no collision risk)
- **Encryption:** All sensitive values (GitHub tokens, env vars, secrets) encrypted with AES-256-GCM before storage. Key derived from `ENCRYPTION_KEY` env var via SHA-256.
- **Migrations:** Embedded SQL files in `internal/db/migrations/`, applied in order, tracked in `_migrations` table. Each migration runs in a transaction.
- **Error handling:** Handlers return JSON `{"error": "message"}` with appropriate HTTP status codes. No panics in handlers -- pipeline panics are recovered in the goroutine wrapper.
- **Project names:** Must match `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$` (used as DNS subdomain).
- **Soft deletes:** Projects use `status='deleted'` rather than actual row deletion. Domains use hard delete (but auto-domains are protected by `is_auto=0` check).
- **Workspace model:** Users always get a personal organization via `EnsurePersonalOrganization()`. Shared organizations are represented by `organizations + organization_memberships`, and the frontend persists the currently selected workspace in `store/organization.ts`.

### Variable System (Env Vars vs Secrets)

Two stores in one table (`env_variables`), distinguished by `kind` column:

| | Env Vars | Secrets |
|---|---|---|
| Scopes | shared, preview, production | shared, preview, production |
| Build-time | `NEXT_PUBLIC_*` baked into Dockerfile as `ENV` | Never -- runtime only |
| Runtime | All passed as Docker env vars | All passed as Docker env vars |
| API response | Values masked (`****last4`) | Values masked |
| Key constraint | `NEXT_PUBLIC_*` allowed | `NEXT_PUBLIC_*` forbidden |
| Cross-store | A key belongs to only one store per project | Same |

Resolution at deploy time: shared vars loaded first, then environment-scoped vars override by key. Both env and secret stores are resolved independently and merged for the container.

### Build Pipeline

The pipeline runs in a background goroutine per deployment:

1. **Semaphore** -- Blocks until build slot available (max 1 concurrent)
2. **Clone** -- Shallow git clone with OAuth token
3. **Settings resolution** -- `projectconfig.Resolve()` merges project fields with framework defaults
4. **Root directory** -- If set, the build WORKDIR shifts to that subdirectory
5. **Next.js patching** -- Injects `output: 'standalone'` into next.config if not present
6. **Variable resolution** -- Separate env var and secret resolution, each with shared+scoped merge
7. **Dockerfile generation** -- Respects the project package-manager setting (`auto`, `bun`, `pnpm`, `npm`, `yarn`). `auto` still detects from lock files first (checks repo root first for monorepos, then app dir). If user provides a Dockerfile, it is used as-is.
8. **Docker build** -- Streams output line-by-line via WebSocket hub
9. **Container start** -- Temporary name (`{canonical}-{deploy_id_prefix}`)
10. **Health check** -- Polls container state; healthy after 5s of running
11. **Domain provisioning** -- For each domain in the environment: DNS verify -> certbot SSL -> nginx config -> nginx reload
12. **Blue-green swap** -- Stop old container, rename new to canonical name
13. **Finalize** -- Mark previous live as "replaced", new as "live"

Container naming: `deployik-{project_name}-{environment}` (e.g., `deployik-my-app-preview`)

### Dockerfile Generation

Two runtimes:
- **`nextjs-standalone`** -- Multi-stage: node base -> deps install -> build -> copy standalone + static to minimal runner with `node server.js`
- **`static`** -- Multi-stage: node base -> deps install -> build -> copy output to runner with `serve -s site -l 3000`

Package manager detection priority in `auto`: `bun.lockb`/`bun.lock` -> `pnpm-lock.yaml` -> `yarn.lock` -> `package-lock.json` -> command inference -> fallback to bun. Lock file searched in install directory (repo root for monorepos, then app root).

Build-time env vars (`NEXT_PUBLIC_*`) are injected as `ENV` lines in the builder stage with properly quoted values.

### Domain Management

- **Auto domains:** Created on project creation (`{name}.preview.example.com`), cannot be deleted
- **Custom domains:** User adds domain, must verify DNS (A record pointing to VPS IP), then SSL is provisioned
- **SSL provisioning:** Runs certbot in a Docker container with bind-mounted cert/html dirs (`--keep-until-expiring` for idempotency)
- **Nginx config:** Generated from Go template, written to shared conf.d directory, nginx tested (`-t`) then reloaded (`-s reload`)
- **DNS verification:** Looks up domain's A records, checks if VPS IP is among them

### Framework Presets

| Framework | Runtime | Default Output | Build Command |
|---|---|---|---|
| `nextjs` | `nextjs-standalone` | `.next` | `bun run build` |
| `vite` | `static` | `dist` | `bun run build` |
| `astro` | `static` | `dist` | `bun run build` |
| `static` | `static` | `dist` | `bun run build` |

Default install command: `bun install`. Default Node.js version: `22`.

The `projectconfig.Resolve()` function is the backend source of truth. The frontend `BuildSettingsFields` component mirrors the same defaults. When framework changes, install/build/output commands reset to framework defaults only if they were at their previous default values (preserving user customizations).

### Frontend Patterns

- **Routing:** TanStack Router with file-based route creation in `app/app.tsx`. Protected routes use `beforeLoad` guard checking Zustand auth state.
- **Data fetching:** TanStack Query with 30s stale time, 1 retry. Auto-refetch on active deployments (3s interval).
- **Auth store:** Zustand with `persist` middleware (localStorage key: `deployik-auth`). On 401 API response, auto-logout.
- **API client:** Class-based `ApiClient` in `lib/api.ts` with typed methods. Reads token from Zustand store.
- **WebSocket:** `useBuildLogs` hook connects for active deployments, merges streaming + historical logs, deduplicates by line number, caps at 5000 lines.
- **Build settings:** Reusable `BuildSettingsFields` component used in both NewProject and ProjectDetail settings tab. Framework change auto-syncs dependent fields.
- **UI theme:** Dark theme (zinc palette), shadcn/ui new-york variant. Glassy card style with subtle borders.
- **Path alias:** `@/` maps to `web/src/` in both tsconfig and Vite config.

### Testing

- Go tests use in-memory SQLite (`db.OpenMemory()`), no external dependencies needed
- Tests cover: DB CRUD operations, migration idempotency, authorization boundaries (foreign user rejection, admin cross-tenant access), env var validation (key format, cross-store conflicts, NEXT_PUBLIC_ secret rejection, value masking), Dockerfile generation (monorepo paths, static runtime, env var quoting), Next.js config patching (typed configs, wrapped configs, idempotency), SSL cert commands (bind mounts, flags), nginx reload (config test before reload), WebSocket auth
- Frontend: No test framework currently set up (relies on TypeScript strict mode + build verification)

## CI/CD Pipeline

`.github/workflows/ci.yml` triggers on push/PR to main:

1. **test job:** Go tests + frontend install + typecheck + build
2. **build-and-push job** (main only): Docker multi-stage build, push to `ghcr.io/lefteq/lovinka-deployik` with SHA tag + `latest`
3. **deploy-vps job** (main only): SSH to VPS, `docker compose pull app`, `docker compose up -d`, health check

The Docker image is built from `docker/Dockerfile`:
- Stage 1: Bun builds frontend (`web/dist`)
- Stage 2: Go builds binary with embedded frontend (`cmd/server/web_dist`)
- Stage 3: Alpine runtime with `ca-certificates`, `git`, `docker-cli`

Production runs via `docker/docker-compose.yml` with:
- Docker socket mount (to manage deployed app containers)
- Nginx conf.d volume (to write proxy configs)
- Named volume for SQLite data
- External `proxy` network (shared with nginx-proxy)

## Environment Variables Reference

### Required

| Variable | Description |
|---|---|
| `JWT_SECRET` | Signs JWT tokens |
| `ENCRYPTION_KEY` | Derives AES-256-GCM key for encrypting env vars, secrets, GitHub tokens |
| `GITHUB_CLIENT_ID` | GitHub OAuth App client ID |
| `GITHUB_CLIENT_SECRET` | GitHub OAuth App client secret |

### Optional

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Server port |
| `DATABASE_PATH` | `data/deployik.db` | SQLite database path |
| `ALLOWED_GITHUB_USERS` | _(all)_ | Comma-separated allowed GitHub usernames |
| `FRONTEND_URL` | `http://localhost:5173` | Frontend URL for OAuth callback |
| `NGINX_CONF_DIR` | `/opt/nginx-proxy/conf.d` | Where to write nginx configs |
| `PROXY_CONTAINER_NAME` | `nginx-proxy` | Nginx container name for reload commands |
| `PROXY_CERTS_DIR` | `/opt/nginx-proxy/certs` | Host path to Let's Encrypt certs |
| `PROXY_HTML_DIR` | `/opt/nginx-proxy/html` | Host path for ACME challenges |
| `SSL_EMAIL` | `admin@example.com` | Let's Encrypt registration email |
| `BUILD_DIR` | `/tmp/deployik-builds` | Temp dir for builds (cleaned after each deploy) |
| `VPS_HOST` | `203.0.113.10` | Expected IP for DNS verification |
| `DEV_MODE` | _(unset)_ | Set to `true` to allow startup without required env vars |

## Design Decisions

- **SQLite over Postgres:** Single binary deployment, no separate database container. WAL mode provides good concurrent read performance. Adequate for the expected scale (single VPS, handful of projects).
- **Programmatic Dockerfile generation over templates:** The original `templates/nextjs.Dockerfile.tmpl` was replaced by Go string builders in `dockerfile.go` for better control over monorepo paths, package manager detection, and conditional stages.
- **AES-256-GCM encryption:** All env vars and secrets encrypted at rest. Separate nonces per encryption ensure identical values produce different ciphertexts.
- **Env vars vs secrets as separate stores:** Keys cannot exist in both stores for the same project. Secrets are runtime-only (never in Dockerfile `ENV` lines). `NEXT_PUBLIC_*` keys are forbidden in secrets because they need build-time embedding.
- **Shared scope for variables:** Variables set in the "shared" scope apply to both preview and production unless overridden by an environment-specific value. This reduces duplication for common config.
- **Blue-green deploy:** New container starts with a temp name, gets health-checked, then the old container is stopped and the new one renamed. No traffic disruption since nginx resolves by container name on the Docker network.
