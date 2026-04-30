# Deployik

Self-hosted Vercel alternative for the Lovinka VPS. Deploys Next.js and static web apps from GitHub with automatic domains, SSL, environment variables, blue-green zero-downtime deployments, auto-build via GitHub webhooks, per-environment password protection, post-deploy screenshot capture, optional per-project host-network access and persistent named Docker volumes, and a configurable reverse proxy (Docker nginx-proxy, host nginx, or host Apache — picked via `PROXY_TYPE` / `PROXY_CONFIG_FORMAT`).

## Stack

- **Backend:** Go 1.25 (chi router, Docker SDK, SQLite via modernc.org/sqlite -- pure Go, no CGO)
- **Frontend:** React 19 + Vite 7 + TanStack Router/Query + Zustand + shadcn/ui (new-york style, zinc dark theme) + Tailwind CSS 4
- **Database:** SQLite (embedded, WAL mode, ULID primary keys)
- **Auth:** GitHub OAuth (scope: `repo,read:user,admin:repo_hook`) -> JWT (HS256, 1h access / 7d refresh tokens). Personal Access Tokens (`dpk_<...>`) accepted via `Authorization: Bearer` for non-browser clients.
- **Encryption:** AES-256-GCM (SHA-256 key derivation) for env vars, secrets, and GitHub tokens at rest
- **Deployment:** Single Go binary embeds React SPA via `//go:embed`; Docker multi-stage build; CI/CD via GitHub Actions to GHCR + VPS

## Commands

| Action | Command |
|---|---|
| Dev API | `make dev-api` (sets DEV_MODE=true) |
| Dev Frontend | `make dev-web` (Vite on :5173, proxies to :8080) |
| Seed dev data | `make dev-seed` (runs `scripts/seed-dev.sh`) |
| Go tests | `go test ./...` |
| Frontend typecheck | `cd web && bunx tsc --noEmit` |
| Build production | `make build` |
| Docker build | `make docker-build` |
| Manual deploy | `./scripts/deploy.sh [tag]` |

## Project Structure

```
cmd/server/main.go        Entry point: loads config, initializes all services, starts HTTP server
cmd/backup/main.go        Backup helper binary: create/verify consistent SQLite snapshots for infra timers and off-site sync
cmd/server/web_dist/      Embedded SPA (populated by `make build` or Docker build)
scripts/
  seed-dev.sh             Seeds local dev database with test data
  deploy.sh               Manual deployment script
  deploy-vps.sh           VPS deployment helper
  examples/               systemd unit templates with @DEPLOYIK_HOME@ / @DEPLOYIK_USER@ / @BUN_BIN@ placeholders for operators running Deployik directly on the host (needed for PROXY_TYPE=host-port)

internal/
  backup/
    sqlite.go             CreateSQLiteSnapshot() uses VACUUM INTO + integrity_check for live SQLite backups
  api/
    router.go             chi route definitions (public + protected groups)
    spa.go                Serves embedded SPA with client-side routing fallback
    handlers/
      auth.go             GitHub OAuth callback, OAuth state verification, cookie session issuance, refresh, logout, /me, DevLogin (DEV_MODE only)
      projects.go         CRUD + GitHub repo/branch listing; dev-mode mock repos/branches for Playwright testing
      deployments.go      List (filtered/paginated), trigger, get, build logs; production releases can optionally create a git tag
      domains.go          Add, list, move/set-primary via PATCH, delete, verify (DNS + SSL) with real-time WebSocket log streaming
      envvars.go          VariableHandler -- generic for both env and secret stores; BulkSet + single Upsert
      health.go           HealthHandler: GET /api/health -- {status, version} JSON; nil-safe for tests/older builds
      autobuild.go        Auto-build config CRUD: creates/deletes GitHub webhooks, manages webhook secrets
      webhooks.go         Incoming GitHub webhook handler: validates HMAC signature, matches branch to config, triggers deployments
      protection.go       Password protection: get/update/regenerate per-environment; site-auth verify + check endpoints for nginx auth_request
      screenshots.go      Serves deployment screenshot PNGs
      platform.go         GET /api/platform: returns VPS IP for DNS setup guides
      organizations.go    List organizations for current user
      analytics.go        Project analytics: combined Umami audience + Loki runtime data
      email.go            Project email setup: get/update Webglobe SMTP + reCAPTCHA settings and trigger audited SMTP test sends
      volumes.go          VolumeHandler: list (name + on-disk size + in_use flag), delete, recreate per project-environment; errdefs-classified errors surface 409 on in-use volumes instead of masking as 500
      inspect.go          InspectHandler: GET /api/github/repos/{owner}/{repo}/inspect; uses user's OAuth token to detect monorepo structure via GitHub API
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

  version/
    version.go            Build metadata (git SHA, build time, GH Actions run); New() derives commit/run URLs from raw inputs

  analytics/
    service.go            Project-level analytics orchestration (Umami audience + Loki runtime)
    umami.go              Umami API client: login, website provisioning, stats/pageviews/metrics/active queries
    loki.go               Loki HTTP client for summary + timeseries queries
    audience.go           Audience aggregation helpers for multi-host/domain rollups
    options.go            Analytics range/environment/timezone normalization
                          Install payloads support a separate tracker script URL so audience tracking can be served from Lovinka CDN while events still post to Umami

  email/
    service.go            Project email onboarding orchestration: derives Webglobe SMTP defaults from domains, writes shared env/secrets, generates contact-form AI install prompts, and performs SMTP test sends through an injectable Sender

  authz/
    access.go             CanAccessProject, LoadProject, LoadDeployment (ownership + admin bypass)

  build/
    pipeline.go           Full deploy orchestration: clone -> patch -> build -> run (with optional ExtraHosts, VolumeBinds, host-port binding) -> health -> swap -> screenshot
    clone.go              Git shallow clone with OAuth token auth
    docker.go             Docker SDK: BuildImage, RunContainer (accepts RunContainerOptions with ExtraHosts/BindHostPort/VolumeBinds), StopContainer, WaitForHealthy, ContainerExists, GetHostPort, EnsureVolume, RemoveVolume, VolumesDiskUsage (hits /system/df so sizes are populated)
    dockerfile.go         Programmatic Dockerfile generation (Next.js standalone + static site)
    nextjs.go             Patches next.config.* to inject output: 'standalone'
    variables.go          Splits env vars into build-time (NEXT_PUBLIC_*) and runtime sets
    semaphore.go          Channel-based concurrency limiter (default: 1 concurrent build)
    screenshot.go         CaptureScreenshot(): runs headless Chrome (zenika/alpine-chrome) in Docker to screenshot deployed site

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
      008_deployment_enhancements.sql  Adds trigger_source, triggered_by_username, screenshot_path to deployments
      009_auto_build.sql       Adds auto_build_configs and webhook_events tables
      010_password_protection.sql  Adds preview_password, production_password to projects
      011_host_network_access.sql  Adds host_network_access to projects (opt-in `host.docker.internal` via ExtraHosts)
      012_proxy_and_volumes.sql  Adds data_volume_enabled and data_mount_path (default `/app/data`) to projects
      013_project_port.sql     Adds port (default 3000) to projects; drives container ExposedPorts + nginx upstream for user-provided Dockerfiles that don't listen on 3000
      014_domain_primary.sql  Adds domains.is_primary with a partial unique index per `(project_id, environment)` and backfills existing rows from the legacy is_auto heuristic
      015_env_variable_updated_at.sql  Adds env_variables.updated_at for changed-since-deploy signals
      016_project_email_settings.sql  Adds project_email_settings for Webglobe SMTP/reCAPTCHA onboarding metadata; encrypted credentials remain in env_variables as secrets
      017_api_tokens.sql        Adds api_tokens for Personal Access Tokens (Bearer auth alongside JWT)
    models.go             User, Organization, OrganizationMembership, RefreshSession, AuditLog, Project (with password fields, host_network_access, data_volume_enabled, data_mount_path), Deployment (with trigger/screenshot fields), BuildLog, Domain, ProjectVariable, VariableKind, AutoBuildConfig, WebhookEvent, ProjectWithLatestDeployment, DeploymentWithUser, DeploymentListResponse, DeploymentFilter
    queries_users.go      GetUserByGithubID, GetUserByID, UpsertUser
    queries_organizations.go  Personal workspace bootstrap, memberships, org listing
    queries_projects.go   ListProjectsWithLatestDeployment, GetProject, Create, Update, Delete (soft), GetProjectPassword, SetProjectPassword, ClearProjectPassword
    queries_deployments.go  ListDeploymentsFiltered (with pagination/filters), Get, Create, UpdateStatus/Container/Duration/Screenshot, GetLiveDeployment
    queries_envvars.go    ListProjectVariables, ListResolvedEnvVars/Secrets, BulkSet*, UpsertProjectVariable, Delete*, key conflict checks
    queries_domains.go    List, GetByName, Create, UpdateDNS/SSL/Environment, SetDomainPrimary, Delete, DeleteForProject
    queries_buildlogs.go  Insert, GetBuildLogs, PruneBuildLogs
    queries_refresh_tokens.go  Create/GetActive/Rotate/Revoke refresh sessions
    queries_audit_logs.go CreateAuditLog
    queries_project_analytics.go  Get/Upsert/Delete project_analytics rows
    queries_project_email.go  Get/Upsert/Delete project_email_settings rows
    queries_autobuild.go  GetAutoBuildConfig, UpsertAutoBuildConfig, DeleteAutoBuildConfig, ListActiveAutoBuildConfigsByRepo
    queries_webhook_events.go  CreateWebhookEvent, WebhookEventExists (idempotency)

  domain/
    ssl.go                Manager: ProvisionDomain (DNS verify -> cert (certbot or configured wildcard) -> proxy config -> reload); ProvisionLogger for structured step events; ReloadProxy() dispatches to ReloadNginx() in docker mode or runs PROXY_RELOAD_CMD via `sh -c` in host-port mode
    nginx.go              GenerateNginxConfig from Go template with password protection support (auth_request blocks), ContainerUpstream so host-port mode can point nginx at 127.0.0.1:<port>, RemoveNginxConfig
    apache.go             GenerateApacheConfig: Apache VirtualHost template (HTTP→HTTPS redirect, www canonicalization, TLS with optional wildcard cert, HTTP/2 + WebSocket upgrade). Password protection is not yet supported in this path
    reconcile.go          Rewrites proxy configs (nginx or Apache) for already-active Deployik domains on startup. In host-port mode, looks up each container's live host port via DockerInspector and skips targets whose container isn't running so no broken vhost lands on disk
    dns.go                VerifyDNS (A-record lookup against VPS IP)
    variants.go           Canonicalizes production custom domains so apex stays primary and optional www alias redirects to it
    auth_page.go          WriteAuthPage: generates static Czech-language auth HTML page for password-protected sites

  github/
    oauth.go              OAuthConfig: AuthorizeURL (scope: repo,read:user,admin:repo_hook), ExchangeCode; GetUser
    client.go             Client: ListRepos, ListBranches, GetLatestCommit, CreateTagReference, CreateWebhook, DeleteWebhook, UpdateWebhookActive, GetFileContent, GetTree (used by monorepo inspector)

  monorepo/
    inspect.go            Inspect() orchestrator + RepoInspector interface + ErrFileNotFound sentinel
    detect.go             Pure detection helpers: package manager, tooling (turborepo/nx), workspaces, glob expansion, per-app profile, vite outDir
    types.go              Report, App, Tooling structs (JSON-serialisable, mirror frontend types)

  projectconfig/
    defaults.go           Framework presets (nextjs, vite, astro, static), Resolve(), ApplyProjectDefaults(), path normalization

  ws/
    hub.go                Pub/sub hub: Subscribe, Unsubscribe, Publish per deployment/domain
    logs.go               WebSocket handler for build log streaming with cookie/header auth + origin allowlist
    domain_logs.go        WebSocket handler for domain verification log streaming

web/src/
  app/app.tsx             TanStack Router tree with nested layouts, QueryClient, providers
  main.tsx                React root render
  pages/
    Login.tsx             GitHub OAuth redirect
    AuthCallback.tsx      Exchanges code/state for cookie session, stores only user state
    Projects.tsx          Dashboard: project list with latest deployment info
    NewProject.tsx        Three-step state machine: (A) pick repo -> (B) pick app from monorepo (skipped for single-app repos) -> (C) configure build settings
    ProjectOverview.tsx   Dual environment rows (preview + production), domain strips, recent deployments, release panel
    ProjectDeployments.tsx  Deployment history with filters (branch, environment, status, date range), pagination
    ProjectAnalytics.tsx  Integrations/Analytics route: dashboard plus embedded setup/how-to section
    ProjectEmail.tsx      Integrations/Email route: Webglobe SMTP/reCAPTCHA setup
    ProjectSettings.tsx   Build settings page (framework, package manager, commands, directories)
    ProjectSettingsDomains.tsx  Domain management: inline add form, environment grouping, primary badge, verify + move/set-primary/delete actions, DNS setup guide, verification with real-time log streaming
    ProjectSettingsEnv.tsx  Environment variables + secrets with Vercel-style individual add/edit/delete, .env import
    ProjectSettingsProtection.tsx  Per-environment password protection toggle with password reveal
    DeploymentDetail.tsx  Build log viewer with real-time WebSocket streaming
  components/
    analytics/metric-chart.tsx  Reusable shadcn chart-card wrapper built on ui/chart
    analytics/stat-card.tsx  Reusable shadcn KPI summary card with CardAction/CardFooter layout
    layout/AppLayout.tsx  SidebarProvider wrapper -- renders <Outlet> for nested layouts
    layout/AppSidebar.tsx Sidebar navigation: context-aware (workspace vs project), collapsible Integrations and Settings sections with sub-items, workspace switcher in footer, project picker in header
    layout/WorkspaceLayout.tsx  Sidebar + header + content for workspace-level pages (projects list)
    layout/ProjectLayout.tsx  Sidebar + breadcrumb header + content for project-level pages
    layout/ProjectPicker.tsx  Command-based project switcher in sidebar header (search + navigate)
    layout/CommandPalette.tsx  Global Cmd/Ctrl+K spotlight for actions, workspaces, and project search
    layout/TopBar.tsx     Legacy top bar component (kept but not used in main layout; functionality moved to sidebar)
    layout/VersionRow.tsx Sidebar footer row showing commit SHA + GH Actions run; collapses to icon + tooltip in icon-only sidebar mode
    projects/build-settings.tsx  Reusable BuildSettingsFields component with framework + package manager presets
    projects/variable-store.tsx  Vercel-style variable store: individual add/edit/delete rows, .env import, scope badges
    projects/dns-setup-guide.tsx  Collapsible DNS setup instructions with platform IP lookup
    projects/overview-stat-card.tsx  Overview page stat card for environment status
    projects/release-panel.tsx  Production release panel with tag creation
    projects/project-analytics.tsx  Analytics tab UI: 4 key stats + 2 collapsible sections (Audience + Runtime)
    projects/project-analytics-meta.ts  AUDIENCE_STATUS_META: badge styles and descriptions for analytics statuses
    projects/project-integration.tsx  Analytics setup/how-to section embedded into Integrations -> Analytics: Install -> Verify -> Events
    projects/project-email.tsx  Email setup page: Webglobe SMTP/reCAPTCHA form, status cards, SMTP test action, help/how-to, and AI install prompt for app-owned Next.js contact routes
    projects/project-multi-locale.tsx  Multi Locale integration page: Next.js-only next-intl prompt workflow with searchable locale picker and generic guide fallback for other frameworks
    projects/project-multi-locale-utils.ts  Shared locale option defaults, locale-code normalization, picker state helpers, and next-intl prompt generation used by Multi Locale tests/UI
    projects/pick-app.tsx Monorepo app picker shown as Step 2 in NewProject when inspection detects workspaces; renders detected framework/output/build per app
    BuildLog.tsx          Log viewer with auto-scroll, stderr highlighting
    ui/                   shadcn/ui components (button, card, dialog, input, select, etc.)
    ui/breadcrumb.tsx     Breadcrumb primitives for project layout header
    ui/code-panel.tsx     Reusable fixed-height scrollable code/prompt card with copy action
    ui/collapsible.tsx    Collapsible primitives used in sidebar Settings section and analytics sections
    ui/spinner.tsx        Shared spinner + centered loading state
    ui/sidebar.tsx        Official shadcn sidebar primitives (SidebarProvider, Sidebar, SidebarInset, SidebarTrigger, etc.)
    ui/chart.tsx          Official shadcn chart primitives for Recharts
  hooks/
    useBuildLogs.ts       WebSocket hook for real-time build log streaming
    useDomainVerification.ts  WebSocket hook for real-time domain verification log streaming
    use-mobile.ts         Shared mobile breakpoint hook used by shadcn sidebar/drawer
    use-organizations.ts  React Query + Zustand bridge for accessible organizations and selected workspace
  lib/
    api.ts                ApiClient class wrapping fetch with cookie auth, refresh retry, auto-logout on unrecoverable 401
    deployment-helpers.ts  Shared constants and helpers: DEPLOYMENT_STATUS_META, ENVIRONMENT_META, VARIABLE_SCOPE_META, domain helpers, formatting utilities
    utils.ts              cn() utility (clsx + tailwind-merge)
  store/
    auth.ts               Zustand store for current user/auth status only (tokens stay in HttpOnly cookies)
    organization.ts       Persisted selected workspace/org id
  types/
    api.ts                TypeScript interfaces matching Go models (includes AutoBuildConfig, ProtectionStatus, DeploymentListFilters, DeploymentListResponse, DomainLogEvent, VerifyDomainResponse)

.claude/skills/
  deployik-howto/         User-facing dashboard help skill (project-scoped)
    SKILL.md              Router: triggers, when-to-use, tone, guide vs action mode, safety rules
    click-paths.md        7 v1 recipes — goal-indexed (route + sidebar + click steps + "stuck?" footer)
    api-actions.md        Endpoint catalog with safety tiers (read silent, mutate confirm, destructive typed-confirm)
    helpers/deployik      Bash wrapper for Bearer-auth API calls; reads ~/.config/deployik/config
```

## Database Schema

SQLite with 17 migrations. Tables:

| Table | Key Fields | Notes |
|---|---|---|
| `users` | id (ULID), github_id (unique), username, github_token (encrypted), role (admin/user) | `ADMIN_GITHUB_USERS` provides explicit admin bootstrap; first user only auto-promotes when no admin list is configured |
| `organizations` | id (ULID), name, slug (unique), is_personal, personal_owner_user_id (nullable unique FK) | Every user gets a personal workspace; shared orgs use memberships |
| `organization_memberships` | organization_id (FK), user_id (FK), role (owner/member) | Grants workspace visibility |
| `projects` | id (ULID), name (unique slug), github_repo, github_owner, branch, user_id (creator FK), organization_id (nullable FK), framework, package_manager, root_directory, output_directory, build_command, install_command, node_version, status, preview_password (encrypted), production_password (encrypted), host_network_access (bool), data_volume_enabled (bool), data_mount_path (default `/app/data`), port (default 3000) | Soft-delete via status='deleted'; password fields added in migration 010; runtime fields added in migrations 011-013 |
| `deployments` | id (ULID), project_id (FK), environment, status, commit_sha, commit_message, branch, container_id, container_name, image_tag, build_duration, triggered_by, trigger_source (manual/webhook/api), triggered_by_username, screenshot_path, error_message, finished_at | trigger/screenshot fields added in migration 008 |
| `build_logs` | id (auto), deployment_id (FK), line_number, content, stream (stdout/stderr) | |
| `domains` | id (ULID), project_id (FK), domain (unique), environment, is_auto, is_primary (partial unique per project+environment), dns_verified, ssl_status (pending/active/error), ssl_expires_at | Auto-domains cannot be deleted or moved; `is_primary` drives canonical URL selection per environment |
| `env_variables` | id (ULID), project_id (FK), environment (shared/preview/production), kind (env/secret), key, value (encrypted) | UNIQUE(project_id, environment, key) |
| `project_analytics` | project_id (PK/FK), audience_enabled, tracking_mode, audience_status, umami_website_id, last_event_at, verified_at | One linked Umami website per project; stores audience analytics health/provisioning state |
| `project_email_settings` | project_id (PK/FK), provider, smtp_host/port/security/user, from identity, contact recipients, recaptcha site key/mode/threshold, status, last_tested_at/error | Non-secret email onboarding metadata; SMTP password and reCAPTCHA secret live in encrypted `env_variables` secrets |
| `refresh_tokens` | id (ULID), user_id (FK), token_hash, expires_at, last_used_at, revoked_at | Opaque refresh tokens are hashed at rest and rotated on use |
| `audit_logs` | id (auto), user_id (nullable FK), action, resource_type, resource_id, project_id, deployment_id, metadata | Records login/refresh/logout and sensitive mutating actions |
| `api_tokens` | id (ULID), user_id (FK), name, token_hash, last_used_at, expires_at, revoked_at | SHA-256 hashed at rest; raw token shown once at creation; routed by `dpk_` prefix in middleware |
| `auto_build_configs` | id (ULID), project_id (unique FK), enabled, production_branch, preview_branches, webhook_id, webhook_secret (encrypted) | One config per project; webhook_id links to GitHub webhook |
| `webhook_events` | id (auto), project_id (FK), github_delivery_id (unique), event_type, branch, commit_sha, commit_message, pusher, deployment_id (nullable FK), status (received/processed/ignored/failed), error_message | Idempotency via unique github_delivery_id |

## API Endpoints

### Public
- `GET  /api/health` -- Health check; response includes `version` block (git SHA, GitHub Actions run id, commit_url, run_url) for the SPA's sidebar build badge
- `GET  /api/auth/github` -- Redirects to GitHub OAuth
- `GET  /api/auth/github/callback?code=&state=` -- Verifies OAuth state, sets session cookies, returns user
- `POST /api/auth/refresh` -- Rotates refresh cookie, returns user
- `POST /api/auth/logout` -- Revokes refresh session and clears cookies
- `POST /api/auth/dev-login` -- DEV_MODE only: creates/returns a dev user without GitHub OAuth `{username?}`
- `POST /api/webhooks/github` -- GitHub webhook receiver: validates HMAC signature, triggers auto-build deployments
- `POST /api/site-auth/verify` -- Password protection: verifies password and issues signed site-auth cookie (called by nginx proxy)
- `GET  /api/site-auth/check` -- Password protection: validates site-auth cookie (called by nginx auth_request)

### Protected (access cookie or Bearer token required)
- `GET  /api/auth/me` -- Current user
- `GET  /api/organizations` -- Organizations/workspaces current user can access
- `GET  /api/platform` -- Platform info (VPS IP for DNS setup guides)

**GitHub:**
- `GET  /api/github/repos` -- User's GitHub repos (dev-mode: returns mock repos)
- `GET  /api/github/branches?owner=&repo=` -- Repo branches (dev-mode: returns mock branches)
- `GET /api/github/repos/{owner}/{repo}/inspect?branch=` -- Detect monorepo structure (pnpm/npm/yarn/bun workspaces, Turborepo, Nx) and per-app build profiles. Rate-limited to 20/min/IP.

**Projects:**
- `GET    /api/projects?organization_id=` -- List projects with latest deployment join, optionally filtered to one workspace
- `POST   /api/projects` -- Create project (auto-creates preview domain; defaults to personal workspace if no `organization_id`)
- `GET    /api/projects/{id}` -- Get project
- `PATCH  /api/projects/{id}` -- Update project
- `DELETE /api/projects/{id}` -- Soft-delete project (stops containers, removes nginx configs, cleans domains)
- `GET    /api/projects/{id}/analytics?environment=&range=&timezone=` -- Combined project analytics payload (Umami audience + Loki runtime)
- `POST   /api/projects/{id}/analytics/verify?environment=&range=&timezone=` -- Force an analytics refresh / verification cycle
- `GET    /api/projects/{id}/email` -- Email setup payload with Webglobe SMTP defaults, required env/secret status, and AI install prompt
- `PUT    /api/projects/{id}/email` -- Save Webglobe SMTP + reCAPTCHA settings; writes shared env vars and encrypted secrets
- `POST   /api/projects/{id}/email/test-smtp` -- Send audited SMTP test email using stored settings/secrets

**Deployments:**
- `GET  /api/projects/{id}/deployments?branch=&environment=&status=&triggered_by=&from=&to=&limit=&offset=` -- List deployments with filtering and pagination
- `POST /api/projects/{id}/deployments` -- Trigger deployment `{environment, branch?, create_tag?, tag_name?}`
- `GET  /api/projects/{id}/deployments/{did}` -- Get deployment
- `GET  /api/deployments/{did}/logs` -- Get build logs
- `GET  /api/deployments/{did}/screenshot` -- Serve deployment screenshot PNG

**Auto-Build:**
- `GET    /api/projects/{id}/auto-build` -- Get auto-build configuration
- `PUT    /api/projects/{id}/auto-build` -- Create/update auto-build config `{enabled, production_branch, preview_branches}` (creates GitHub webhook)
- `DELETE /api/projects/{id}/auto-build` -- Delete auto-build config and remove GitHub webhook

**Password Protection:**
- `GET  /api/projects/{id}/protection` -- Get protection status `{preview_enabled, production_enabled}`
- `PUT  /api/projects/{id}/protection` -- Enable/disable protection `{environment, enabled}` (returns generated password when enabling)
- `POST /api/projects/{id}/protection/regenerate` -- Regenerate password `{environment}` (returns new password)

**Volumes (only meaningful when `data_volume_enabled=true` on the project):**
- `GET    /api/projects/{id}/volumes` -- List preview + production volumes with on-disk size (via `/system/df`) and in_use flag (true when the env's container is currently running)
- `DELETE /api/projects/{id}/volumes/{env}` -- Delete the volume (returns 409 if in use by a running container)
- `POST   /api/projects/{id}/volumes/{env}/recreate` -- Remove + recreate (returns 409 if in use; never reports success unless the old volume was actually removed first)

**Domains:**
- `GET    /api/projects/{id}/domains` -- List domains
- `POST   /api/projects/{id}/domains` -- Add domain `{domain, environment}`
- `PATCH  /api/projects/{id}/domains/{did}` -- Update domain `{environment?, is_primary?}`; move re-provisions the proxy target and rejects `is_auto=1` rows
- `DELETE /api/projects/{id}/domains/{did}` -- Delete domain (not auto-domains)
- `POST   /api/projects/{id}/domains/{did}/verify` -- Verify DNS + provision SSL (async, streams logs via WebSocket)

**Personal Access Tokens (PATs):**
- `GET    /api/me/tokens` -- List the caller's tokens (raw values never returned)
- `POST   /api/me/tokens` -- Create a token `{name}`; returns `{id, name, token}` once
- `DELETE /api/me/tokens/{id}` -- Revoke a token (soft delete; 404 for foreign tokens)

**Environment Variables:**
- `GET    /api/projects/{id}/env?environment=` -- List env vars (values masked)
- `PUT    /api/projects/{id}/env` -- Bulk set (replace all) `{environment, variables: [{key, value}]}`
- `POST   /api/projects/{id}/env` -- Single upsert (additive) `{key, value, environment}`
- `DELETE /api/projects/{id}/env/{key}?environment=` -- Delete env var

**Secrets:**
- `GET    /api/projects/{id}/secrets?environment=` -- List secrets (values masked)
- `PUT    /api/projects/{id}/secrets` -- Bulk set
- `POST   /api/projects/{id}/secrets` -- Single upsert
- `DELETE /api/projects/{id}/secrets/{key}?environment=` -- Delete secret

### WebSocket
- `GET /ws/deployments/{did}/logs` -- Real-time build log streaming (access cookie or Bearer token)
- `GET /ws/domains/{did}/logs` -- Real-time domain verification log streaming (access cookie or Bearer token)

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
- **Install with AI:** Integrations -> Analytics embeds `projects/project-integration.tsx`, which surfaces a generated AI-install prompt and exact manual Umami snippet. The prompt is built from project settings (`framework`, `package_manager`, `root_directory`) plus the linked website ID so users can hand it to any coding AI instead of relying on brittle proxy injection.
- **Reusable analytics UI:** `components/analytics/stat-card.tsx` and `components/analytics/metric-chart.tsx` are the shared primitives for KPI cards and chart cards. Reuse them for future dashboard work instead of building one-off chart wrappers inside each page.
- **IDs:** ULIDs generated via `db.NewID()` (time-sortable, no collision risk)
- **Encryption:** All sensitive values (GitHub tokens, env vars, secrets, webhook secrets, passwords) encrypted with AES-256-GCM before storage. Key derived from `ENCRYPTION_KEY` env var via SHA-256.
- **Migrations:** Embedded SQL files in `internal/db/migrations/`, applied in order, tracked in `_migrations` table. Each migration runs in a transaction.
- **Error handling:** Handlers return JSON `{"error": "message"}` with appropriate HTTP status codes. No panics in handlers -- pipeline panics are recovered in the goroutine wrapper.
- **Project names:** Must match `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$` (used as DNS subdomain).
- **Soft deletes:** Projects use `status='deleted'` rather than actual row deletion. Domains use hard delete (but auto-domains are protected by `is_auto=0` check).
- **Workspace model:** Users always get a personal organization via `EnsurePersonalOrganization()`. Shared organizations are represented by `organizations + organization_memberships`, and the frontend persists the currently selected workspace in `store/organization.ts`.
- **Build metadata:** `cmd/server/main.go` declares `gitSHA`, `buildTime`, `ghRunID`, `ghRepo` package vars set at link time via `go build -ldflags="-X main.<name>=<value>"`. CI (`.github/workflows/ci.yml`) passes these as Docker `build-args` to `docker/build-push-action@v6`, which the `Dockerfile` `go-builder` stage forwards to the `RUN go build` line. The result is wrapped in `internal/version.Info` and surfaced via the `/api/health` JSON response.
- **Monorepo inspection:** `internal/monorepo` is a dependency-free detection package that scans a GitHub repo via a small `RepoInspector` interface (mockable in tests). It detects `pnpm-workspace.yaml`, `turbo.json`, `nx.json`, and root `package.json` `workspaces` (npm/yarn/bun share the same field), expands workspace globs against the GitHub Trees API, and derives per-app framework/output/build defaults. The `internal/api/handlers/InspectHandler` adapts `*github.Client` (via `gh.ErrNotFound` → `monorepo.ErrFileNotFound` translation) to expose this at `GET /api/github/repos/{owner}/{repo}/inspect?branch=...` for the new-project flow.
- **Auto-deploy on creation:** `ProjectHandler.Create` fires two best-effort side-effects after persisting the project + auto-domain: `setupAutoBuildBestEffort` (calls the shared `provisionWebhook` helper extracted from `autobuild.go` to create a GitHub webhook + insert `auto_build_configs` with defaults `production_branch=project.Branch, preview_branches="*", enabled=true`) and `dispatchInitialDeployBestEffort` (inserts a `deployments` row with `environment="preview"`, `trigger_source="api"` and calls `Pipeline.Dispatch`). Failures (insufficient OAuth scope, no admin access, decrypt error, nil pipeline) log and return — the project is never rolled back. Both side-effects are recorded as audit events with `metadata={"source":"project_create"}`.

### Auto-Build System

GitHub webhooks trigger automatic deployments:

1. **Setup:** User enables auto-build via `PUT /api/projects/{id}/auto-build`. Deployik creates a GitHub webhook using the user's OAuth token (requires `admin:repo_hook` scope). The webhook secret is encrypted and stored in `auto_build_configs`.
2. **Webhook flow:** GitHub sends push events to `POST /api/webhooks/github`. The handler:
   - Validates HMAC-SHA256 signature against the stored (decrypted) webhook secret
   - Checks idempotency via `github_delivery_id` in `webhook_events`
   - Matches branch against `production_branch` (exact) or `preview_branches` (comma-separated list or `*` for all)
   - Creates a deployment with `trigger_source: "webhook"` and `triggered_by_username` from the pusher
   - Records the event in `webhook_events` (status: processed/ignored/failed)
3. **Lifecycle:** Disabling auto-build toggles the webhook on GitHub. Deleting auto-build removes the GitHub webhook and the config row.

### Password Protection

Per-environment password protection for deployed sites:

1. **Enable:** `PUT /api/projects/{id}/protection {environment, enabled: true}` generates a random 16-char base64url password, encrypts it, stores in `projects.preview_password` or `projects.production_password`, regenerates nginx configs with `auth_request` blocks, and returns the plaintext password.
2. **Nginx flow:** Protected domains get additional nginx location blocks:
   - `/_deployik/auth-check` (internal) proxies to Deployik's `/api/site-auth/check` with project/environment headers
   - `/_deployik/verify` proxies to Deployik's `/api/site-auth/verify` for the login form POST
   - Main `location /` uses `auth_request /_deployik/auth-check` to gate access
   - On 401, nginx serves a static Czech-language auth page (`auth.html`) generated by `domain.WriteAuthPage()`
3. **Site-auth cookie:** On successful password verification, Deployik issues an HMAC-SHA256 signed `deployik_site_auth` cookie (24h TTL) scoped to the project+environment.
4. **Regenerate:** `POST /api/projects/{id}/protection/regenerate` generates a new password (invalidates existing cookies on next expiry check).

### Screenshot Capture

After a successful deployment, the pipeline asynchronously captures a screenshot:

1. Waits 5 seconds for the deployed site to stabilize
2. Finds the first active SSL domain for the deployment's environment
3. Runs a `zenika/alpine-chrome` Docker container with `--screenshot` flag
4. Stores the PNG at `{SCREENSHOT_DIR}/{deployment_id}.png`
5. Updates `deployments.screenshot_path` in the database
6. Screenshots are served via `GET /api/deployments/{did}/screenshot`

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
| Mutation modes | Bulk set (PUT, replaces all) or single upsert (POST, additive) | Same |

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
9. **Volume ensure** -- If `data_volume_enabled=true`, `EnsureVolume` creates `deployik-{project}-{env}-data` (idempotent); the volume is bound to the container at `data_mount_path` (default `/app/data`)
10. **Container start** -- Temporary name (`{canonical}-{deploy_id_prefix}`); `RunContainerOptions` carries `ExtraHosts: ["host.docker.internal:host-gateway"]` when `host_network_access=true`, `VolumeBinds` from the step above, and `BindHostPort=true` under `PROXY_TYPE=host-port` (binds container 3000 to a random `127.0.0.1:<random>`)
11. **Health check** -- Polls container state; healthy after 60s timeout
12. **Upstream resolution** -- docker mode: `{containerName}:3000` over the `proxy` Docker network. host-port mode: `GetHostPort()` reads the live port from `inspect.NetworkSettings.Ports["3000/tcp"]`
13. **Domain provisioning** -- For each domain in the environment: DNS verify -> SSL (certbot per-domain, or skip entirely when `PROXY_SSL_CERT` is configured) -> proxy config (nginx or Apache depending on `PROXY_CONFIG_FORMAT`, with password protection if enabled) -> `ReloadProxy()`
14. **Blue-green swap** -- Stop old container, rename new to canonical name
15. **Finalize** -- Mark previous live as "replaced", new as "live"
16. **Screenshot** -- Async: capture headless Chrome screenshot of the deployed site (5s delay)

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
- **Nginx config:** Generated from Go template, written to shared conf.d directory, nginx tested (`-t`) then reloaded (`-s reload`). Supports password protection blocks (`auth_request` + auth page).
- **DNS verification:** Looks up domain's A records, checks if VPS IP is among them. Real-time verification logs streamed via WebSocket.
- **Variant handling:** Production custom domains get apex as primary with optional www redirect.

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

- **Layout:** Sidebar-based layout using shadcn sidebar primitives. Three layout levels:
  - `AppLayout` -- SidebarProvider wrapper (outermost protected shell)
  - `WorkspaceLayout` -- Sidebar (workspace nav) + header with breadcrumb for dashboard pages
  - `ProjectLayout` -- Sidebar (project nav with collapsible Settings) + header with breadcrumb for project pages
- **Sidebar navigation:** `AppSidebar` is context-aware: renders workspace items or project items based on `context` prop. Project context includes Overview, Deployments, Analytics, Integration, and Settings (with sub-items: Build, Domains, Environments, Protection). Footer has workspace switcher dropdown.
- **Project picker:** `ProjectPicker` in sidebar header provides command-based project switching with search.
- **Routing:** TanStack Router with nested layout routes in `app/app.tsx`. Route tree: `root -> protected -> [workspaceLayout -> index, newProject, projectLayout -> [overview, deployments, deploymentDetail, analytics, integration, settings, settingsDomains, settingsEnv, settingsProtection]]`.
- **Data fetching:** TanStack Query with 30s stale time, 1 retry. Auto-refetch on active deployments (3s interval).
- **Auth hydration:** `hydrateAuthState()` in route `beforeLoad` calls `/api/auth/me` once to bootstrap session state. Deduplicates concurrent calls via shared promise.
- **Auth store:** Zustand with `persist` middleware (localStorage key: `deployik-auth`). On 401 API response, auto-logout.
- **API client:** Class-based `ApiClient` in `lib/api.ts` with typed methods. Uses cookie auth with `credentials: 'include'`.
- **WebSocket:** `useBuildLogs` hook for deployment build logs; `useDomainVerification` hook for domain provisioning logs. Both connect with cookie/header auth.
- **Shared helpers:** `lib/deployment-helpers.ts` exports `DEPLOYMENT_STATUS_META`, `ENVIRONMENT_META`, `VARIABLE_SCOPE_META`, domain utility functions (`isDomainReady`, `getEnvironmentDomains`, `getPrimaryEnvironmentUrl`), and formatting helpers (`formatRelativeDate`, `buildReleaseTagName`, `formatCompactNumber`).
- **Build settings:** Reusable `BuildSettingsFields` component used in both NewProject and ProjectSettings. Framework change auto-syncs dependent fields.
- **Variable store:** `VariableStore` component provides Vercel-style individual add/edit/delete for env vars and secrets. Supports .env file import, scope badges (shared/preview/production), and inline editing.
- **UI theme:** Dark theme (zinc palette), shadcn/ui new-york variant.
- **Path alias:** `@/` maps to `web/src/` in both tsconfig and Vite config.
- **New-project flow:** `web/src/pages/NewProject.tsx` is a 3-step state machine: (A) pick repo → (B) pick app from monorepo (skipped transparently for single-app repos) → (C) configure build settings. Step B reuses `<PickApp>` and is driven by `api.inspectRepo()` (TanStack Query, 5-min staleTime). Picking an app pre-fills `framework`, `package_manager`, `root_directory`, `output_directory`, `build_command`. Inspect errors fall through silently to manual configuration.

### Testing

- Go tests use in-memory SQLite (`db.OpenMemory()`), no external dependencies needed
- Tests cover: DB CRUD operations, migration idempotency, authorization boundaries (foreign user rejection, admin cross-tenant access), env var validation (key format, cross-store conflicts, NEXT_PUBLIC_ secret rejection, value masking), Dockerfile generation (monorepo paths, static runtime, env var quoting), Next.js config patching (typed configs, wrapped configs, idempotency), SSL cert commands (bind mounts, flags), nginx reload (config test before reload), WebSocket auth
- Frontend: No test framework currently set up (relies on TypeScript strict mode + build verification)
- Dev-mode login endpoint (`POST /api/auth/dev-login`) enables Playwright E2E testing without GitHub OAuth

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
| `JWT_SECRET` | Signs JWT tokens and site-auth cookies |
| `ENCRYPTION_KEY` | Derives AES-256-GCM key for encrypting env vars, secrets, GitHub tokens, webhook secrets, passwords |
| `GITHUB_CLIENT_ID` | GitHub OAuth App client ID |
| `GITHUB_CLIENT_SECRET` | GitHub OAuth App client secret |

### Optional

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Server port |
| `DATABASE_PATH` | `data/deployik.db` | SQLite database path |
| `DATA_DIR` | `data` | Base data directory |
| `ALLOWED_GITHUB_USERS` | _(all)_ | Comma-separated allowed GitHub usernames |
| `ADMIN_GITHUB_USERS` | _(none)_ | Comma-separated admin GitHub usernames (explicit admin bootstrap) |
| `FRONTEND_URL` | `http://localhost:5173` | Frontend URL for OAuth callback |
| `ALLOWED_ORIGINS` | _(derived from FRONTEND_URL)_ | Additional allowed CORS origins (comma-separated) |
| `NGINX_CONF_DIR` | `/opt/nginx-proxy/conf.d` | Where to write nginx configs |
| `PROXY_CONTAINER_NAME` | `nginx-proxy` | Nginx container name for reload commands |
| `PROXY_CERTS_DIR` | `/opt/nginx-proxy/certs` | Host path to Let's Encrypt certs |
| `PROXY_HTML_DIR` | `/opt/nginx-proxy/html` | Host path for ACME challenges |
| `SSL_EMAIL` | `admin@example.com` | Let's Encrypt registration email |
| `BUILD_DIR` | `/tmp/deployik-builds` | Temp dir for builds (cleaned after each deploy) |
| `VPS_HOST` | `203.0.113.10` | Expected IP for DNS verification |
| `WEBHOOK_URL` | `{FRONTEND_URL}/api/webhooks/github` | Public URL for GitHub webhook callbacks |
| `SCREENSHOT_DIR` | `{DATA_DIR}/screenshots` | Directory to store deployment screenshots |
| `PROXY_TYPE` | `docker` | `docker` (nginx-proxy container on the same Docker network, containers reachable by name) or `host-port` (each deployed container binds to a random localhost port; the host proxy reaches it via `127.0.0.1:<port>`) |
| `PROXY_CONFIG_FORMAT` | `nginx` | `nginx` server blocks or `apache` VirtualHost blocks written into `NGINX_CONF_DIR` |
| `PROXY_RELOAD_CMD` | _(unset)_ | Shell command run via `sh -c` to reload the proxy when `PROXY_TYPE=host-port` (e.g. `apachectl graceful`, `sudo -n systemctl reload nginx`, `nsenter -t 1 -m -- apachectl graceful`) |
| `PROXY_SSL_CERT` | _(unset)_ | Path to a pre-existing wildcard cert — when set, per-domain certbot runs are skipped |
| `PROXY_SSL_KEY` | _(unset)_ | Matching wildcard key |
| `VITE_ALLOWED_HOSTS` | _(unset)_ | Comma-separated hostnames the Vite dev server will accept (for public HMR behind a reverse proxy) |
| `DEV_MODE` | _(unset)_ | Set to `true` to allow startup without required env vars; enables dev-login endpoint and mock GitHub data |

## Design Decisions

- **SQLite over Postgres:** Single binary deployment, no separate database container. WAL mode provides good concurrent read performance. Adequate for the expected scale (single VPS, handful of projects).
- **Programmatic Dockerfile generation over templates:** The original `templates/nextjs.Dockerfile.tmpl` was replaced by Go string builders in `dockerfile.go` for better control over monorepo paths, package manager detection, and conditional stages.
- **AES-256-GCM encryption:** All env vars and secrets encrypted at rest. Separate nonces per encryption ensure identical values produce different ciphertexts.
- **Env vars vs secrets as separate stores:** Keys cannot exist in both stores for the same project. Secrets are runtime-only (never in Dockerfile `ENV` lines). `NEXT_PUBLIC_*` keys are forbidden in secrets because they need build-time embedding.
- **Shared scope for variables:** Variables set in the "shared" scope apply to both preview and production unless overridden by an environment-specific value. This reduces duplication for common config.
- **Blue-green deploy:** New container starts with a temp name, gets health-checked, then the old container is stopped and the new one renamed. No traffic disruption since nginx resolves by container name on the Docker network.
- **Sidebar layout over top-nav tabs:** The monolithic `ProjectDetail.tsx` (2000+ lines with tabs) was decomposed into 7+ separate page files with nested TanStack Router routes, navigated via a sidebar with collapsible Settings section. This improves code organization, enables deep-linking, and gives more room for future navigation items.
- **Password protection via nginx auth_request:** Instead of proxying all traffic through Deployik, password-protected sites use nginx's `auth_request` directive pointing to Deployik's `/api/site-auth/check`. This keeps the fast path (authenticated requests) entirely in nginx while only the initial auth check hits Deployik.
- **Webhook HMAC validation per project:** Each project's auto-build config stores its own encrypted webhook secret, so a single webhook endpoint can serve multiple projects by iterating configs and validating signatures independently.
- **Proxy abstraction (ReloadProxy + ContainerUpstream):** Replacing direct `ReloadNginx()` calls with a `ReloadProxy()` dispatcher let one install serve docker-nginx-proxy, host nginx, or host Apache without branching hotpaths through the codebase. `ContainerUpstream` on `NginxConfig`/`ApacheConfig` decouples the upstream string from the container name so `PROXY_TYPE=host-port` can insert `127.0.0.1:<port>` without touching callers.
- **`sh -c` for PROXY_RELOAD_CMD:** The host-port reload command is passed to `sh -c` rather than parsed with `strings.Fields`. This is operator-controlled config (not user input), so giving the operator the full shell vocabulary (sudo, pipes, `nsenter -t 1 -m -- apachectl graceful`, configtest chains) is strictly better than a fragile whitespace-split parser.
- **Volumes via `/system/df`:** `VolumeInspect` and `VolumeList` leave `UsageData` nil, so the only way to show real on-disk sizes is `DiskUsage`. The volumes handler calls it once per request and keys the result by volume name for both preview and production.
- **Volume name keyed by project.Name (for now):** Volume naming is `deployik-{project.Name}-{env}-data` to match the container. Because `project.Name` is mutable via `PATCH /api/projects/{id}`, renaming would silently orphan the data. The Update handler therefore rejects rename requests when `data_volume_enabled=true` (409); a follow-up will re-key by `project.ID`.
- **Host-port port instability is documented, not masked:** Each deployed container binds to a random localhost port via `nat.PortBinding{HostIP: "127.0.0.1", HostPort: "0"}`. Docker re-assigns on restart, so the reconcile path reads the live port from `inspect.NetworkSettings.Ports` on boot and skips writing targets whose container isn't up yet — better to have no vhost than a vhost pointing nowhere. Restart-time port drift is called out in README Known Limitations with a TODO for deterministic ports.
- **`deployik-howto` skill is project-scoped, not global:** the skill lives at `.claude/skills/deployik-howto/` and is committed to the repo. UI changes to `web/src/pages/*` and skill updates ship in the same PR — no doc-vs-code drift across deploys. Action mode requires a Personal Access Token (see migration `017_api_tokens.sql`); guide mode works without one. v1 covers seven goals: GitHub repo connection, custom domain, env vars/secrets, auto-deploy, password protection, contact-form email (Webglobe SMTP + reCAPTCHA v3 + AI install prompt), and rollback. Tone is intentionally non-technical — every recipe ends with "Stuck on any of these steps? Tell me which one and I'll walk through it with you."
