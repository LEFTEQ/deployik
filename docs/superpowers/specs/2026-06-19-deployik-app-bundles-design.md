# Deployik App Bundles — Design

> **Status:** approved design, pre-implementation (2026-06-19)
> **Scope:** Deployik Go control plane (`internal/db`, `internal/build`, `internal/domain`, `internal/services`, `internal/api`, `cmd/server`) + MCP surface.
> **One-liner:** Introduce a first-class **App** that bundles several independently-deployed projects (each its own container/build/domain) into one cohesive unit — shared private network, app-level env/secrets, an app-owned Postgres, coordinated deploys with single-rollback, a unified view, and changed-path build filtering. **Inert by default**: every existing project keeps `app_id = NULL` and behaves exactly as today.

## Problem

Deployik binds each project to a GitHub repo + a `root_directory`. A monorepo therefore becomes **N projects on one repo**, and a push to that repo webhook-fans a build to **all N**, regardless of which app's files changed. Observed 2026-06-19: commit `e4a8bd1` (touching only `apps/acme-api`) triggered builds on `forge`, `acme-studio`, `acme-app`, `acme-app-api`, and `acme` — five projects for a one-app change (two of them failed).

Two distinct pains:
1. **Fan-out** — unrelated projects rebuild on every push (trigger-time problem).
2. **No cohesion** — `acme-app` (web) and `acme-app-api` are one logical application (+ a database), but Deployik has no way to treat them as one: separate env (DATABASE_URL hand-duplicated), public-internet hops between them (CORS), independent uncoordinated deploys, no combined view, no single rollback.

## The reframe (why not docker-compose)

The first instinct — collapse api+web+db into one docker-compose project — was rejected after verifying the source. A Deployik project is **one image → one container per environment** (`Deployment` carries a single `ContainerID`/`ImageTag`, `internal/db/models.go:315-335`; `RunContainer` is called once, `internal/build/pipeline.go:377`; blue-green *swaps/renames*, never adds, `pipeline.go:419-426`). Services are postgres-only, hard-blocked at three layers (SQL `CHECK`, enum, API 400). Ingress is **one-upstream-per-project** (the `domains` row has no service identifier; every domain proxies the same container — `models.go:346-358`, `internal/domain/reconcile.go:45-55`, `internal/domain/nginx.go:157-184`). Compose would require rebuilding the runtime, service, *and* ingress models — large and risky on a live control plane.

Reference platforms:
- **Vercel** uses one project per app (same as Deployik) and solves fan-out with an "Ignored Build Step" (`turbo-ignore`) — changed-path build skipping. It does **not** merge apps.
- **Railway** is the model adopted here: a project holds multiple **services** (each its own container/domain) + databases on a **shared private network**, managed as one unit, each independently deployable.

This design takes the Railway shape **without** the compose runtime change: each member stays its own one-container project; the **App** is a cohesion + shared-network + shared-env + coordinated-deploy layer on top, plus an app-owned database. The existing "group" primitive cannot serve this role — it is the renamed `organizations` table (a workspace/access-control boundary, `internal/db/models.go:45-56`, `migrations/006_organizations.sql`) and holds *all* of a workspace's projects; it carries no runtime/deploy behavior.

## Goals

1. A project can belong to an **App** (≤1). An App lives inside a workspace (org).
2. **Path-based build filtering** — a push only rebuilds members whose files changed.
3. **Shared private network** — members reach each other by internal hostname (no public/CORS hop).
4. **App-level env/secrets** — set once on the App, inherited by all members.
5. **App-owned Postgres** — the database is a resource of the App, shared by members, its credentials injected as app-level env.
6. **Deploy together** — coordinated, ordered, health-gated rollout of an App's members, with **single-rollback** to a known-good set.
7. **Unified view** — one composite read of an App: members + per-member status/health + release history.

## Non-goals (explicit)

- **No migration of existing live data.** Promoting Postgres to app-level changes the *model and provisioning*; it does **not** move the running `acme-app-api` sidecar or its data. New Apps provision an app-level DB from creation; re-homing the existing demo DB into an app-owned resource is a separate, **backup-gated** migration documented for later, out of scope here.
- **No new service types.** Services stay postgres-only; this work changes the service *owner* (project → app), not the kind. Redis/MySQL remain reserved for follow-ups.
- **No docker-compose / multi-container-per-project runtime.** Each member remains one image → one container.
- **No multi-upstream-per-domain.** Each member keeps its own domain → its own container (the ingress model is untouched).
- **No cross-app dependency graph.** Member ordering is a simple integer, not a DAG.
- **No true distributed-atomic deploy.** "Atomic" is realized as coordinated + health-gated + best-effort set-rollback (see §6).

## Data model

All additions are backward-compatible: existing rows get `app_id = NULL` and the current code paths are unchanged. New migrations continue the numbered sequence (next is `026_...`; the latest today is `025_push_subscriptions.sql`).

### New tables

**`apps`** (`026_apps.sql`)
| column | type | notes |
|---|---|---|
| `id` | TEXT PK | ULID, like other entities |
| `organization_id` | TEXT NOT NULL FK → `organizations(id)` ON DELETE CASCADE | the workspace it lives in |
| `name` | TEXT NOT NULL | |
| `slug` | TEXT NOT NULL | unique within org: `UNIQUE(organization_id, slug)` |
| `deploy_ordered` | INTEGER NOT NULL DEFAULT 0 | 0 = parallel deploy; 1 = honor member `deploy_order` |
| `display_order` | INTEGER NOT NULL DEFAULT 0 | dashboard ordering, mirrors `024` |
| `created_at`, `updated_at` | TEXT | |

**`app_variables`** (`026_apps.sql`) — mirrors `env_variables` (`migrations/002_project_variable_kinds.sql:1-10`) but app-scoped.
`(id, app_id FK→apps ON DELETE CASCADE, environment ∈ {shared,preview,production}, kind ∈ {env,secret}, key, value)`, `UNIQUE(app_id, environment, key)`.

**`app_releases`** + **`app_release_members`** (`027_app_releases.sql`) — snapshot of a coordinated deploy, for single-rollback.
- `app_releases(id, app_id FK, environment, status, created_at)`
- `app_release_members(release_id FK→app_releases ON DELETE CASCADE, project_id FK→projects, deployment_id FK→deployments)` — the exact deployment each member ran in that release.

### Extended tables

- **`projects`**: add
  - `app_id TEXT NULL REFERENCES apps(id) ON DELETE SET NULL` (indexed). A project in ≤1 app; `NULL` = standalone.
  - `deploy_order INTEGER NOT NULL DEFAULT 0` — used only when its App's `deploy_ordered = 1` (low deploys first; equal = parallel).
  - `build_filter_enabled INTEGER NOT NULL DEFAULT 0` — opt-in path filtering (see §1). Default 0 → today's "always build".
  - `watch_paths TEXT NULL` — JSON array of globs for shared deps (see §1).
  - The `Project` struct (`internal/db/models.go:150`, where `OrganizationID` already lives) gains `AppID *string`, `DeployOrder int`, `BuildFilterEnabled bool`, `WatchPaths []string`.

- **`project_services`** (`migrations/023_project_services.sql`): add `app_id TEXT NULL REFERENCES apps(id) ON DELETE CASCADE`. A service is owned by **either** a project (today: `project_id` set, `app_id` null) **or** an App (new: `app_id` set). Exactly one owner. The `service_type` CHECK stays `'postgres'` (`023:25`) — owner change only.

## Capability designs

### 1. Path-based build filtering (the fan-out fix)

Trigger-time, in the GitHub push webhook handler. Compute the union of changed paths across `commits[].added/modified/removed` in the push payload. For each project bound to the repo+branch:

- If `build_filter_enabled = 0` → build (today's behavior, default).
- Else build **iff** some changed path is under the project's `root_directory` **or** matches one of its `watch_paths` globs.

**Fail-safe = build.** If the changed-file set is unavailable (GitHub truncates pushes > 2000 files; non-push or synthetic events), build regardless — never silently skip. Every decision (`built` / `skipped: no paths under <root>+<globs>` / `built: file list unavailable`) is logged on the deployment/trigger record.

**Shared-dependency handling (decided):** manual `watch_paths` globs — e.g. both acme members list `packages/acme-shared/**` and `bun.lock`. Chosen over auto workspace-dependency-graph resolution (turbo-ignore-style) — far less code, explicit, sufficient at this scale.

Works for standalone projects too (independent of Apps); within an App it makes "deploy together" smart (only changed members rebuild).

### 2. Shared private network

A per-app, per-environment docker network named `deployik-app-<appID>-<env>`, created on demand via a new `NetworkEnsure(ctx, name)` helper beside `RunContainer` (`internal/build/docker.go` — the `network` package is already imported; `RunContainer` already takes a `networkName` and calls `NetworkConnect`, `docker.go:318,375-377`).

Each member container connects to **both**: the global `proxy` network (ingress, `ProxyNetwork` constant `cmd/server/main.go:159,181`) **and** its app network (sibling comms). After the existing `RunContainer` connect (`pipeline.go:377`), connect the app network when `project.AppID != nil` (already loaded on the struct — no extra query).

Members reach each other by container hostname on the app network (web → `http://deployik-acme-api-<env>:<port>`). The internal base URL is surfaced via app-level env (§4) so members reference it by name, not hardcode.

> Preview branches get per-instance container names (`PreviewContainerName`, `queries_preview_instances.go:56-61`). The app network is scoped per `(app, environment)`; preview-branch isolation within an app reuses the existing per-instance container naming, so siblings on the same branch instance share the network and cross-branch instances do not collide.

### 3. App-level env / secrets

At deploy time, a member's resolved variables merge with **most-specific-wins** precedence:

```
app shared  →  app <env>  →  project shared  →  project <env>
(lowest)                                          (highest)
```

Implemented by layering app vars *underneath* the existing per-project resolver (`ListResolvedProjectVariables`, `internal/db/queries_envvars.go:83-98`, merge `:61-80`). Secrets (`kind = secret`) handled identically to project secrets. This is the one place to set shared tokens — and where the app-owned DB's connection lands (§5).

### 4. App-owned Postgres

The database is a resource of the **App**, not a member.

- **Ownership:** a `project_services` row with `app_id` set (and `project_id` null). The attach API/handler (`internal/api/handlers/services.go`, which today 400s non-postgres at `:115-118`) accepts an app owner; type stays `postgres`.
- **Lifecycle:** unchanged — the sidecar container (`deployik-app-<appID>-<env>-pg`, mirroring `PostgresContainerName`) starts via the existing `EnsureServices` hook, on the **app network** (§2). Internal port 5432 (`internal/services/postgres.go:22`).
- **Credentials → app env:** the existing env injection (`postgres.go:39-59` — `DATABASE_URL`, `POSTGRES_*`, secret `POSTGRES_PASSWORD`) targets **`app_variables`** (shared scope) instead of a single project's env. Every member inherits `DATABASE_URL` automatically via §3's merge — no per-project duplication.
- **Stateful across releases:** the DB persists across app deploys/rollbacks; `app_releases` snapshot only the code services' deployment IDs, never DB data.

**Data-migration is a non-goal here** (see Non-goals): the existing live `acme-app-api` sidecar + data stays attached to that project untouched. New Apps provision an app-level DB at creation. The dogfood App (`acme-app`) therefore groups its *code* services immediately while its DB stays project-attached until a later backup-gated re-home.

### 5. Deploy together (coordinated release)

A new "deploy app" action deploys an App's members for an environment.

- **Source of members to build:** from a push, only members whose paths changed (§1); unchanged members keep running. A manual app-deploy can force-all.
- **Ordering (decided):** when `apps.deploy_ordered = 1`, members deploy in ascending `projects.deploy_order` (db/api low → web high); equal order runs in parallel. A simple integer — not a DAG. Reuses the existing single-project dispatch (`Pipeline.Dispatch`, `internal/build/pipeline.go:77`) per member; a new `ListProjectsByApp(appID, env)` query drives the loop.
- **Atomicity (decided):** coordinated + health-gated + best-effort set-rollback — *not* literal all-or-nothing. Each member is gated on its health check (the pipeline already health-checks before the blue-green swap, `pipeline.go:385,419-426`). If a member fails, halt the rollout and roll the **already-swapped** members of this release back to their previous live deployments (blue-green retains the old container until swap; rollback reuses the existing per-project promote/rollback path). The failed release is recorded with `status = failed`.
- **Release snapshot:** a successful coordinated deploy writes an `app_releases` row + one `app_release_members` row per member (its `deployment_id`).

### 6. Unified view + single rollback

- **Composite read** (`get_app_health`, mirroring `get_project_health`): App → members + each member's latest deployment status/health + a worst-of combined status + `app_releases` history + the app-owned DB service status.
- **Single rollback** (`rollback_app`): choose a prior `app_release`; redeploy each member to its recorded `deployment_id` via the existing per-project rollback/promote. The DB is not rolled (stateful).

## API / MCP surface

New endpoints + MCP tools (REST under `internal/api`, registered in `internal/api/router.go` near the existing group routes `:181-195`):

- `create_app`, `list_apps`, `get_app` / `get_app_health`, `update_app`, `delete_app`
- `add_project_to_app(app_id, project_id)`, `remove_project_from_app(project_id)`
- App env CRUD: `list_app_env_vars`, `set_app_env_var`, `delete_app_env_var` (+ secret variants), mirroring the project env tools
- `attach_service(app_id, type=postgres, environment)` — extends the existing attach to accept an app owner
- `deploy_app(app_id, environment, force_all?)`, `rollback_app(app_id, environment, release_id)`
- Project tools gain `build_filter_enabled` + `watch_paths` + `deploy_order` fields.

## Backward compatibility / inert-by-default

- Every migration is additive; existing projects get `app_id = NULL`, `build_filter_enabled = 0` → **identical behavior to today**.
- The push webhook still builds every bound project unless a project has opted into filtering.
- Standalone projects, the existing groups (orgs), per-project services, and ingress are all untouched.
- The dogfood/first App is `acme-app` + `acme-app-api` (DB stays attached to the api project per the non-goal).

## Testing

Go offline tests mirroring the existing style (`fakeRunner`, `t.TempDir`, table tests, `strings.Contains` on generated config — no live Docker/Let's Encrypt):

- **Path-filter matcher:** changed-paths × (`root_directory`, `watch_paths`) → build/skip; `build_filter_enabled = 0` always builds; missing file-list → fail-safe build. Regression-guard the default-off behavior.
- **App-env merge precedence:** app-shared → app-env → project-shared → project-env, most-specific wins; secret handling parity.
- **Network naming + connect:** `deployik-app-<id>-<env>`; member connects to both `proxy` and the app network only when `AppID != nil`.
- **Deploy-together ordering:** ascending `deploy_order`; equal = parallel; ordered only when `deploy_ordered = 1`.
- **Health-gated rollback:** a failing member halts the rollout and the already-swapped members roll back to their prior deployment IDs; release recorded `failed`.
- **Release snapshot / single-rollback:** `app_releases` + `app_release_members` capture the member→deployment set; `rollback_app` selects a prior set and redeploys each member to its recorded deployment.
- **App-owned service:** attach with `app_id` set injects DB creds into `app_variables`; per-project attach (project_id) is byte-identical to today.

## Phasing (for the implementation plan)

Each phase is independently shippable and inert-by-default:

- **P1 — Data model + App CRUD + unified view.** `apps`, `app_variables`, `projects.app_id`/`deploy_order`/`build_filter_enabled`/`watch_paths`, `project_services.app_id`; structs/queries; REST + MCP CRUD; `get_app_health`. No runtime behavior change.
- **P2 — Path-based build filtering.** Webhook changed-path computation + per-project filter + fail-safe + logging. Opt-in.
- **P3 — Shared network + app-env.** `NetworkEnsure` + dual connect; app-var merge in the resolver.
- **P4 — App-owned Postgres + deploy-together + releases + rollback.** App-owned service attach + env injection; `ListProjectsByApp`; coordinated/ordered/health-gated deploy; `app_releases`; `deploy_app`/`rollback_app`.

## Decisions log

| # | Decision | Choice |
|---|---|---|
| D1 | App model | First-class `apps` entity nested in the workspace; project `app_id` FK (≤1 app) |
| D2 | Shared-dep build filtering | Manual `watch_paths` globs (not auto dependency-graph) |
| D3 | Member deploy ordering | Simple `deploy_order` int (not a DAG) |
| D4 | Deploy atomicity | Coordinated + health-gated + best-effort set-rollback (not true atomic) |
| D5 | DB ownership | App-level (`project_services.app_id`); creds → `app_variables` |
| D6 | DB data migration | **Out of scope** — existing live DB untouched; new Apps provision app-level DB; re-home is a later backup-gated step |
| D7 | Service types | postgres-only (owner change only; no new kinds) |
| D8 | Ingress | Unchanged — one domain → one container per member |
