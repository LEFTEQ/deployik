# App Dashboard — Design

> **Status:** approved design, pre-implementation (2026-06-21)
> **Scope:** Deployik frontend (`web/src/pages`, `web/src/components`, `web/src/app`, `web/src/lib`) + a read-model layer in the Go control plane (`internal/api/handlers`, `internal/db`, `internal/build`/`internal/services`). **No new DB migrations.**
> **One-liner:** Promote an **App** from a single plain page (two cards) into a **first-class shell** — like a Project — with its own sidebar context, sub-routes, a two-column Overview dashboard with a sticky live-pulse rail, an **auto-derived architecture/topology map** from real env wiring, **live container-health** roll-up, and a unified cross-member deployments feed with deep links into each project.

## Problem

App Bundles shipped the *capability* (coordinated deploys, app-level vars, releases, private network, sibling discovery — see `docs/superpowers/specs/2026-06-19-deployik-app-bundles-design.md`) but the UI is a single `web/src/pages/AppDetail.tsx` with two basic cards (Members list + Release history). It does not:

- Treat the App as a navigable entity (no shell, no sub-routes) the way a Project has `ProjectLayout` + Overview/Deployments/Settings pages.
- Show **per-member live status** — the current `GET /api/apps/{id}/health` returns only last-deploy timestamps (no status, no liveness), and there is no combined app status (a gap already noted as design-spec G7 "partial").
- Surface the App's defining property: that its members **communicate with each other**. The private network + injected `<SIBLING>_URL/_HOST/_PORT` env exist, but nothing visualizes the resulting architecture.
- Give a **unified deployments view** across members, or quick links to drill into a specific project.

The goal is a dashboard that makes an App feel like a cohesive, production-grade unit and visibly stands out, while reusing Deployik's existing dark visual language (the `divide-y rounded-lg border` row lists, the `Card`+`Table` deployments grid, status-dot + env-badge + mono-SHA pattern, production's amber accent).

## The reframe (why no new schema)

Everything this design surfaces is **derivable from already-shipped App Bundles data** plus a live probe:

- **Topology edges** come from scanning members' existing stored variables (`app_variables` + `env_variables`) for references to a sibling's internal host. No `depends_on` table.
- **Live health** comes from probing the already-named canonical containers (`deployik-<name>-<env>`) over the existing `proxy` network. No status table.
- **Unified deployments** is a join over existing `deployments` + `projects.app_id`. No new table.
- **Member ordering** uses the existing `projects.deploy_order`.

So this is a **read-model + UI** feature. The only persistent concept deliberately deferred is *manual* topology edges (a user-declared "uses" override), kept out of scope (see Non-goals / D-edges).

## Goals

1. **App-as-shell** — an App has its own layout, sidebar context, and sub-routes, mirroring how a Project works (`web/src/components/layout/ProjectLayout.tsx`).
2. **Overview dashboard** — a two-column page: a wide main column for structure (topology + members) and a **sticky right rail** for the live pulse (combined health + KPIs + recent-deployments feed + releases).
3. **Auto-derived topology** — an architecture map of members, with **confirmed** directed edges (solid, labeled with the variable) derived from real env wiring and **reachable** links (faint) for the shared private network.
4. **Live combined health** — real per-member container probing rolled up to a worst-of combined app status badge (closes design-spec G7).
5. **Unified deployments** — a cross-member recent-deployments feed (rail slice) and a full filterable Deployments page, each row deep-linking to that project's deployment detail.
6. **Deep links everywhere** — one click from a member or a deployment row into the owning project.

## Non-goals (explicit)

- **No new DB migrations / tables.** This is a read-model + UI layer over the shipped App Bundles schema.
- **No user-declared dependency edges (`depends_on`) in v1.** Topology is auto-derived only; a manual "uses" override is a documented later add-on (option C from brainstorming), not built now.
- **No app-owned Postgres surfacing beyond a reserved status slot.** App-owned DB remains deferred (App Bundles non-goal D6); the combined-status roll-up leaves a slot for it but no DB re-home happens here.
- **No graph/force-layout dependency.** The topology map is a hand-rolled deterministic SVG layout (layered by deploy/topological order). A graph library is a possible future upgrade, not a v1 dep.
- **No changes to deploy/rollback orchestration.** `deploy_app` / `rollback_app` (`internal/api/handlers/app_deploy.go`) are unchanged; the dashboard only reads + triggers existing actions.
- **No ingress / multi-upstream changes.** Each member keeps its own domain → its own container.

## Frontend design

### Route tree (TanStack Router, `web/src/app/app.tsx`)

A new app-shell layout route nests the App sub-pages, mirroring the existing `projectLayout` subtree:

```
protected
└─ workspaceLayout
   ├─ /apps                         → Apps (existing list)
   └─ appShellLayout
      └─ /apps/$appId               → AppOverview      (index)
         /apps/$appId/deployments   → AppDeployments
         /apps/$appId/topology      → AppTopology
         /apps/$appId/variables     → AppVariables
         /apps/$appId/releases      → AppReleases
         /apps/$appId/settings      → AppSettings
```

`AppDetail.tsx` (the current single page) is decomposed into these pages and removed.

### Components

- **`AppShellLayout`** (`web/src/components/layout/AppShellLayout.tsx`) — mirrors `ProjectLayout`: breadcrumb header + sidebar with `context="app"`. `AppSidebar` (`web/src/components/layout/AppSidebar.tsx`) gains an app nav group (Overview / Deployments / Topology / Variables / Releases / Settings) alongside its existing workspace + project contexts.
- **`AppOverview`** — two-column grid: full-width hero (app name, combined `AppStatusBadge`, primary URL, environment switcher, **Deploy together**, refresh, overflow menu) over `grid-cols-[1fr_320px]`. Main column: `AppTopologyMap` (compact) + `AppMembersList`. Sticky rail (`position: sticky`): Health card (badge + 2×2 KPIs), `AppDeploymentFeed` (limit 5), Releases (limit 3). Collapses to a single stacked column below `md` (hero → pulse → topology → members).
- **`AppTopologyMap`** — renders `{nodes, edges}` as a deterministic layered SVG: nodes positioned in columns by topological/deploy order; **confirmed** edges solid + labeled with the variable key; **reachable** edges faint. Node shows name + framework + a live-status dot. A `compact` variant for Overview, a full interactive variant for the Topology page. Hand-rolled, no new dependency.
- **`AppMembersList`** — rich rows (reusing the `divide-y rounded-lg border` pattern): live-status dot, name, framework badge, `order N` (when `deploy_ordered`), primary domain, last-deploy relative time, **Open project →** deep link to `/projects/$pid`.
- **`AppDeploymentFeed`** (rail) + **`AppDeploymentsTable`** (full page) — both built on the unified cross-member feed, reusing `DEPLOYMENT_STATUS_META`, `ENVIRONMENT_META`, `formatRelativeDate`, and (mobile) `DeploymentCard`. Each row adds a **project** column and deep-links to `/projects/$pid/deployments/$did`. The full page mirrors `ProjectDeployments.tsx` (Card + Table, status filters, pagination).
- **`AppVariables`** — reuses `web/src/components/projects/variable-store.tsx` pointed at the app env/secret endpoints (already exist: `/api/apps/{id}/env|secrets`). `VariableStore` is parameterized (or a thin app-scoped wrapper) so the CRUD UX is byte-identical to the project store.
- **`AppReleases`** — full release history + rollback (today's Releases card, expanded; optional per-release member breakdown using `app_release_members`).
- **`AppSettings`** — name edit, `deploy_ordered` toggle, member add/remove (existing dialogs), **member reorder** (drag or up/down → batch order endpoint), delete app (existing confirm).
- **`web/src/lib/app-helpers.ts`** — `APP_STATUS_META` (Healthy / Deploying / Degraded / Down + dot/badge classes) and `MEMBER_STATUS_META`, mirroring `DEPLOYMENT_STATUS_META` in `lib/deployment-helpers.ts`. Single source of truth for status colors.

### Data fetching

TanStack Query, matching existing conventions: `health` and `deployments` use `staleTimes.activeDeployments` and `refetchInterval: 3000` while any member has an active deployment (as `ProjectOverview`/`ProjectDeployments` do); `topology` uses a longer stale time (≈30s). Deep-link navigations reuse existing project routes.

## Backend design (read-model; no migrations)

All endpoints are app-scoped and authorized via the existing `loadManagedApp` / `GetAppForUser` org-membership path used by the other app handlers.

### Endpoints

- **`GET /api/apps/{id}/health?environment=`** *(upgraded)* — unified read closing design-spec G7:
  ```
  { app, combined_status,
    members: [ { project, deploy_order, primary_domain, live_status, latest_deployment } ] }
  ```
  `combined_status` is the worst-of member `live_status`. `environment` defaults to `production` (same normalization as `ListReleases`). Replaces today's timestamps-only `GetHealth` (`internal/api/handlers/apps.go`).
- **`GET /api/apps/{id}/topology?environment=`** *(new)* — `{ nodes: [...], edges: [ { source, target, via, kind, confirmed } ] }`. `via` is the variable **key**; `kind ∈ {env, secret}`; `confirmed=false` rows are the faint reachable links. **Never returns variable values.**
- **`GET /api/apps/{id}/deployments?environment=&limit=&offset=`** *(new)* — unified cross-member feed. Backed by a new query `ListAppDeployments(appID, environment, limit, offset)` joining `deployments` to `projects WHERE app_id = ?`, newest-first, returning a `DeploymentWithProject` (deployment fields + project id/name). Powers the rail (limit 5) and the Deployments page (paginated/filtered).
- **`GET /api/apps/{id}/releases`** *(existing)* — unchanged.
- **`PATCH /api/apps/{id}/members/order { project_ids: [...] }`** *(new)* — batch-assigns `projects.deploy_order` from the array index, scoped to the app's members (rejects ids not in the app). Drives drag-reorder in Settings.

### Topology derivation (precise)

Server-side, for the selected `environment`:

1. Load members via `ListProjectsByApp(appID, environment)` (`internal/db/queries_apps.go`).
2. For each member **M**, gather M's **stored, user-authored** variables: app-level (`app_variables`) + project-level (`env_variables`), both `env` and `secret` kinds. Secret values are **decrypted in-process only** for matching. The auto-injected `<SIBLING>_URL/_HOST/_PORT` are computed at deploy time by `AppSiblingEnv` (`internal/build/app_deploy.go`) and **never stored**, so they are naturally excluded — preventing a false full mesh.
3. For each sibling **S** (S ≠ M), compute S's match tokens: its canonical internal host `deployik-<S.name>-<env>` (per `db.DeploymentContainerName`) and S's primary domain(s).
4. **Confirmed edge M→S** when any M-var *value* contains an S match token. Emit `{ source: M, target: S, via: <var key>, kind, confirmed: true }`. (First/most-specific match wins for the label.)
5. Every non-confirmed ordered pair becomes a **reachable** edge `{ confirmed: false }` (members share the per-(app,env) private network). The frontend renders these faint and may collapse them visually to reduce clutter.

In-memory string matching over members × siblings — cheap; no persistence.

### Live health probe (enterprise-grade)

A mockable `HealthProber` interface produces each member's `live_status`, computed **concurrently** (one goroutine per member, ~2s per-probe timeout) with a short **TTL cache** (~5–10s, keyed `appID:env`) so the 3s UI poll does not hammer Docker. Per member:

1. If an active deployment is in flight (status ∈ building/queued/deploying) → `deploying`.
2. Else probe the canonical container `deployik-<name>-<env>`:
   - **running** AND (Docker healthcheck `healthy` **OR**, when no healthcheck, an internal HTTP `GET http://deployik-<name>-<env>:<port><health_path>` returns `200/204/3xx/401/403`) → `healthy`;
   - running but failing the above → `degraded`;
   - not running → `down` if the last deployment was `live` (crashed after deploy), `failed` if the last deploy failed, else `none`.
3. Deployik reaches members over the existing `proxy` network (docker mode) or `127.0.0.1:<hostport>` via `GetHostPort` (host-port mode), reusing the monitoring blackbox up-semantics (`200/204/3xx/401/403` = up — so password-protected `401` stays healthy). `<port>` is `project.Port`; `<health_path>` reuses the monitoring `health_path` (default `/`).

`combined_status` = worst-of (`down`/`failed` > `degraded` > `deploying` > `healthy` > `none`), with a reserved slot for the deferred app-owned DB service. Per-member probe failures are isolated — one member's error yields its own `unknown`/`degraded` status and never fails the whole read.

## Data flow (Overview)

```
AppOverview mount
  → GET /apps/{id}/health?environment=prod   (members + live_status + combined_status; polled 3s when active)
  → GET /apps/{id}/topology?environment=prod (nodes + edges)
  → GET /apps/{id}/deployments?limit=5       (rail feed; polled 3s when active)
  → GET /apps/{id}/releases?environment=prod (rail releases)
Deploy together  → POST /apps/{id}/deploy    (existing; 202 + inflight 409 guard)
Roll back        → POST /apps/{id}/rollback  (existing)
Member "Open →"  → /projects/$pid
Deployment row   → /projects/$pid/deployments/$did
```

## Error / empty states

- **No members** → empty topology placeholder + "Add projects" CTA (to Settings); health badge shows `none`.
- **No confirmed edges** → faint reachable mesh only + hint "No internal references detected yet."
- **Per-member probe failure / timeout** → that member shows `unknown` dot + tooltip; cached/last-known used with a subtle "stale" indicator; page never breaks.
- **App not found / not authorized** → standard not-found, consistent with project pages.

## Testing

- **Backend (Go, offline table tests, existing style):**
  - Topology derivation: M-var value referencing a sibling's internal host → confirmed edge labeled with the key; injected sibling vars excluded (no false mesh); secret-value scan path; primary-domain match; no-confirmed → faint reachable only.
  - Health roll-up: a fake `HealthProber` across status permutations → correct per-member `live_status` and worst-of `combined_status`; in-flight deployment → `deploying`; crashed (not running, last deploy live) → `down`.
  - Unified `ListAppDeployments` query: only the app's members, newest-first, correct project join, pagination.
  - Member reorder: batch assigns `deploy_order` by index; rejects non-member ids.
  - Confirm secret **values** never appear in any topology/health response.
- **Frontend:** `bunx tsc --noEmit`; `bun run test` units for `APP_STATUS_META`/`MEMBER_STATUS_META` mapping and the topology layout (node ordering + confirmed/faint edge classification).

## Phasing

Each phase is independently shippable; the UI shape is stable across the health-source swap.

- **P1 — Shell + Overview + unified deployments.** `AppShellLayout` + routes + `AppSidebar` app context; upgraded `health` (deploy-status roll-up first, so Overview is live without the prober); new `ListAppDeployments` + `/apps/{id}/deployments`; **AppOverview** (two-col + sticky rail) and **AppDeployments** page; `app-helpers.ts`.
- **P2 — Live health probe.** `HealthProber` (container inspect + internal HTTP probe, concurrent, TTL cache) swapped into `health`. No UI change.
- **P3 — Topology.** `/apps/{id}/topology` derivation + `AppTopologyMap` (Overview compact + full Topology page).
- **P4 — Variables / Releases / Settings.** `AppVariables` (VariableStore), `AppReleases` page, `AppSettings` (member reorder endpoint + ordered toggle + add/remove + delete).

## Decisions log

| # | Decision | Choice |
|---|---|---|
| D-structure | App UI shape | First-class **shell** with sub-routes (Overview/Deployments/Topology/Variables/Releases/Settings), like a Project — not a single page or in-page tabs |
| D-overview | Overview layout | Two-column: wide main (topology + members) + **sticky** right rail (health + KPIs + deploy feed + releases); stacks on phones |
| D-topo-source | Topology edges | **Auto-derived** from env wiring (scan stored member vars for sibling-host references) — not illustrative-only, not user-declared |
| D-edges | Edge presentation | **Confirmed** (solid, labeled with the var key) **+ reachable** (faint) two-tier |
| D-health | Combined/member status | **Live container probe** (concurrent, internal HTTP + container-state, TTL cache), worst-of roll-up — production-grade, not deploy-status-only |
| D-schema | Persistence | **No new migrations** — pure read-model + UI on the shipped App Bundles schema |
| D-topo-render | Map rendering | Hand-rolled deterministic SVG (layered by order); no graph-lib dependency in v1 |
| D-manual-edges | User-declared `depends_on` | **Out of scope** — auto-derived only; manual "uses" override is a documented later add-on |
| D-deeplinks | Drill-in | Members and deployment rows deep-link into the owning project's pages |
```
