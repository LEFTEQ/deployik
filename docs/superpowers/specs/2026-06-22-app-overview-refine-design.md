# App Overview Refinement — Quick Links, Dual-Environment Matrix, Live Container Logs

**Date:** 2026-06-22
**Status:** Approved design (pre-implementation)
**Surface:** App Bundle overview page — `web/src/pages/AppOverview.tsx` (route `/apps/$appId`)

## Context & Goals

The App Bundle overview shows one environment at a time behind a dropdown that
defaults to **Production**, and offers no fast path to the things a developer
reaches for constantly. Day-to-day work happens in **development (preview)**, so
the current default is backwards, and jumping to a member's running site, its
repo, or its logs takes too many clicks.

Three goals, from the user:

1. **Quick Links** — one click to the running sites, source, app sections, and integrations.
2. **Both environments at once** — see development *and* production together, with **development front-and-center** (no env dropdown).
3. **Live container logs** — watch the realtime stdout/stderr of a running member container, to see what's happening inside it.

## Current State (what exists today)

- `AppOverview.tsx`: hero (name/status/counts) + single env `Select` (defaults `production`) + Refresh + "Deploy together"; a domain strip for the selected env; 4 KPI cards; two columns (Architecture + Members list / Recent deployments + Releases).
- `api.getAppHealth(appId, environment)` → `AppHealth { app, environment, combined_status, members: AppHealthMember[] }`. Each `AppHealthMember` already carries `project`, `live_status`, `primary_domain`, and `latest_deployment` (which includes `commit_sha`, `container_name`, `branch`).
- **Runtime container log streaming already exists** for sidecar services:
  - `internal/services/postgres.go` → `Logs(ctx, spec, w)` runs `docker logs --follow --tail 200 <container>` into `w`.
  - `internal/ws/services_logs.go` → `ServiceLogsHandler(db, lookupSpec, streamLogs, jwtSecret, allowedOrigins)`: auth → load project → resolve container → upgrade WS → pipe `docker logs` line-by-line as **text** frames; client disconnect cancels the context (kills the subprocess). One consumer per connection, no Hub.
  - Wired at `/ws/projects/{id}/services/{env}/logs` in `internal/api/router.go` under the `ws_service_logs` rate limiter.
- shadcn primitives already present: `web/src/components/ui/sheet.tsx`, `tabs.tsx`, `scroll-area.tsx`, plus `web/src/components/BuildLog.tsx` (log viewer) and `web/src/hooks/useBuildLogs.ts` (WS hook, reads **JSON** frames).

## Non-Goals (explicitly out of scope)

- Custom/user-pinned quick links (option E) — no new "pin a URL" data model.
- A dedicated full-page Logs route. Logs live in the side panel only for now.
- Log search/filter/regex, log persistence/history, multi-line stack folding.
- Per-branch rows in the matrix. The matrix stays env-level (dev primary instance + prod); branch granularity lives only in the logs panel.
- Changing deploy/release behavior.

## Feature 1 — Quick Links bar

A slim horizontal bar of chips directly under the hero, derived entirely from
existing data (no new persistence):

- **Source (B):** GitHub repo chip → `https://github.com/{github_owner}/{github_repo}` (bundle-level; per-member source stays the row's `⎘` icon — see Feature 2).
- **App sections (C):** Deployments, Releases, Topology, Variables — `<Link>`s to the existing `/apps/$appId/*` sub-routes.
- **Integrations (D):** Analytics and Logs. Analytics → the project/app analytics route; **Logs** opens the Live Logs sheet (Feature 3) with no tab preselected (user adds one).

Component: `web/src/components/apps/quick-links-bar.tsx` (new). Pure presentational; takes the `app` + member list and renders chips. Reuses existing chip/badge styling.

## Feature 2 — Dual-environment service matrix

Replaces the env `Select` **and** the standalone Members list/domain strip with a
single table. The env dropdown is removed.

**Data:** call `getAppHealth` for **both** environments and merge by `project.id`:

```ts
const dev  = useQuery(... getAppHealth(appId, "preview"));
const prod = useQuery(... getAppHealth(appId, "production"));
```

Merge into one row set keyed by project id; a member missing from an env →
render that cell as `— not deployed —`. Keep the existing `refetchInterval`
(3s while any member is in an active status) on **both** queries.

**Row layout** (component `web/src/components/apps/service-matrix.tsx`, new):

| Member | ● Development (preview) | Production | Src |
|---|---|---|---|
| name + framework badge (+ `#order` if `deploy_ordered`) | status dot · domain link · `commit_sha[:7]` · ▸ logs | status dot · domain link · `commit_sha[:7]` · ▸ logs | `⎘` GitHub |

- **Development cell** is the primary: tinted background, placed left.
- **Production cell** is muted (lower opacity), placed right.
- **Domain link** (`primary_domain`) is the member quick-link (A) — opens the running site in a new tab.
- **Commit SHA** comes from `member.latest_deployment?.commit_sha`.
- **▸** opens that container's tab in the Live Logs sheet (Feature 3). The dev ▸ targets the member's **primary preview instance**; prod ▸ targets the production live deployment.
- Status dot color from `MEMBER_STATUS_META[live_status]` (reuse existing meta), pulse while active.

The KPI cards, Architecture (topology) section, Recent deployments, and Releases
sections are **retained** as-is below the matrix.

## Feature 3 — Live container logs (shadcn Sheet, tabbed, env + branch switching)

### UX

- A right-anchored shadcn **`Sheet`** is the logs viewer.
- **Tabs** (shadcn `Tabs`): each open container is a tab labelled `{member} · {env}[/{branch}]` with its own live status dot. `+` is a no-op placeholder for now (tabs are opened from the matrix ▸); `✕` closes a tab; closing the last tab closes the Sheet.
- **Per-tab controls:**
  - **Env** segmented control (Development / Production) — re-targets the active tab's container.
  - **Branch** dropdown — **enabled only for Development** (production has a single instance). Lists the member's preview branches; selecting one re-targets the tab to that branch's container.
  - **Pause** (stop auto-scroll / freeze), **Wrap** (toggle line wrap), **Clear** (clear the on-screen buffer), **Download** (dump current buffer to a `.log` file).
- Console body reuses `BuildLog.tsx` styling (monospace, stderr highlighted, auto-scroll, `LIVE` indicator). Since the stream is plain text (not the build-log JSON shape), the viewer renders raw lines; stderr highlighting is best-effort by content (the `docker logs` stream merges stdout+stderr).

Components:
- `web/src/components/apps/live-logs-sheet.tsx` (new) — the Sheet + Tabs shell + switchers.
- `web/src/components/apps/log-console.tsx` (new, or extracted from `BuildLog.tsx`) — one tab's streaming console.
- `web/src/hooks/useContainerLogs.ts` (new) — WS hook mirroring `useBuildLogs` but reading **text** frames; reconnect on target change.
- Open-tabs state: a small module-local store (Zustand, mirroring `store/*`) `store/log-tabs.ts` holding `{ id, projectId, memberName, environment, branch? }[]` + `activeTabId`, so the matrix ▸ and the Sheet share state without prop-drilling.

### Backend

Add a **member container logs** WS endpoint that reuses the existing streamer.

- **Route:** `GET /ws/projects/{id}/logs?environment={preview|production}&branch={slug?}` registered in `router.go` under a new `ws_member_logs` rate-limit bucket (mirror `ws_service_logs`).
- **Handler:** `internal/ws/member_logs.go` → `MemberLogsHandler(db, resolveContainer, streamLogs, jwtSecret, allowedOrigins)`. Same shape as `ServiceLogsHandler`:
  - `middleware.ExtractAccessToken` → `AuthenticateToken` → `authz.LoadProject` (a member **is** a project, so project-level authz is the correct gate).
  - Resolve the **live container name** for `(project, environment, branch)`:
    - production → live production deployment's `container_name`.
    - preview → live deployment for the member's preview instance matching `branch` (default: primary instance when `branch` omitted).
  - If no live container → `404` ("no running container for this target").
  - Upgrade WS, then reuse the **existing** `services.Logs` streamer (`streamLogs(ctx, containerName, pw)`) and the same pipe/line-pump/disconnect-watcher loop as `ServiceLogsHandler`.
- **Container resolution query:** `internal/db/queries_deployments.go` — add/extend a helper, e.g. `GetLiveContainerName(projectID, environment, previewInstanceID *string) (string, bool, error)`, built on the existing `GetLiveDeployment` logic. Returns `("", false, nil)` when nothing is live (distinct from a real error).
- **Branch list (for the dropdown):** add `GET /api/projects/{id}/preview-instances` → `[{ branch, preview_instance_id, has_live_container }]`, backed by `queries_preview_instances.go`. Small read endpoint; project-authz gated.

### Data flow (logs)

```
matrix ▸  ──▶ store/log-tabs (add tab: projectId, env, branch?)
                 │
LiveLogsSheet ◀──┘  renders tab → useContainerLogs(projectId, env, branch)
                 │
   WS /ws/projects/{id}/logs?environment=&branch=
                 │  MemberLogsHandler: authz → resolve container → upgrade
                 ▼
   services.Logs(ctx, container, pipe) = `docker logs --follow --tail 200`
                 │  line-pump → WS text frames
                 ▼
   log-console renders lines (autoscroll unless Paused)
```

## Files Touched (summary)

**Backend (new/changed):**
- `internal/ws/member_logs.go` — new handler (mirrors `services_logs.go`).
- `internal/db/queries_deployments.go` — `GetLiveContainerName(...)` helper.
- `internal/db/queries_preview_instances.go` — list branches/instances for a project+env.
- `internal/api/handlers/` — small handler for `GET /api/projects/{id}/preview-instances`.
- `internal/api/router.go` — register the WS route (+ `ws_member_logs` limiter) and the preview-instances route.

**Frontend (new/changed):**
- `web/src/pages/AppOverview.tsx` — remove env `Select`; add QuickLinksBar + ServiceMatrix; mount LiveLogsSheet; fetch both envs.
- `web/src/components/apps/quick-links-bar.tsx` — new.
- `web/src/components/apps/service-matrix.tsx` — new (subsumes MemberRow + domain strip).
- `web/src/components/apps/live-logs-sheet.tsx` — new.
- `web/src/components/apps/log-console.tsx` — new (or extracted from `BuildLog.tsx`).
- `web/src/hooks/useContainerLogs.ts` — new.
- `web/src/store/log-tabs.ts` — new (open tabs + active tab).
- `web/src/lib/api.ts` + `web/src/types/api.ts` — `listPreviewInstances`, `containerLogsWsUrl`, types.

## Error Handling

- WS auth/resolve failures return proper HTTP status **before** upgrade (401/404), matching `ServiceLogsHandler`.
- No live container → 404; the console tab shows "No running container for {member} · {env}[/{branch}]" instead of an empty stream.
- WS drop / container restart → `useContainerLogs` surfaces a "stream ended — reconnecting" state and retries with backoff (do not silently swallow; show status in the tab).
- Both `getAppHealth` queries are independent: if one env errors, render the other env's cells and show an inline error marker in the failed env's column — never blank the whole matrix.
- `services.Logs` already logs non-context-cancel errors and returns them; preserve that (no bare catches).

## Testing

**Go:**
- `queries_deployments_test.go` — `GetLiveContainerName` resolves prod vs preview-by-branch; returns `("", false, nil)` when nothing live; isolates by project.
- `member_logs` handler — auth rejection (no/invalid token), foreign-project rejection via authz, 404 when no live container. (Stream copy mirrors the existing service-logs test approach with an injected `streamLogs` fake.)
- preview-instances query/handler — lists branches with `has_live_container`, project-authz enforced.

**Frontend:**
- Typecheck (`bunx tsc --noEmit`) + build.
- Unit (`bun run test`): merge-by-project logic for the dual-env matrix (member present in one env only → `— not deployed —`); `useContainerLogs` URL construction for env/branch; log-tabs store add/close/last-tab-closes-sheet.

## Phasing

1. **Matrix + Quick Links** (frontend-only, both-env fetch). Ships the "see both, dev-first" win immediately.
2. **Live logs** (backend endpoint + Sheet/tabs/console/hook + branch endpoint).

Phase 1 is independently shippable and unblocks the primary complaint; Phase 2 layers logs on top.

## Open Questions / Assumptions

- **Default dev branch per member:** assume the member's **primary** preview instance (the `is_primary` preview domain's instance). Confirm if a different default is wanted.
- **Quick Links "Analytics" target:** assume the existing project/app analytics route; if an app-level analytics view doesn't exist, link the primary member's analytics.
- **`+` tab button:** placeholder/no-op in v1 (tabs open from the matrix ▸). Promote to a member+env+branch picker later if desired.
