# Deployik PWA + Mobile Responsivity — Design

Approved 2026-06-12. Goal: operate Deployik from an iPhone Air on the go — installable
PWA with offline shell, push notifications, and full phone-first responsivity.

Three phases, three sequential PRs, each independently shippable.

## Phase 1 — Responsive overhaul (`feat/mobile-responsive`, frontend only)

Shared primitives:

- **`MobileTabBar`** — fixed bottom bar, visible below `md`, safe-area bottom padding.
  Context-aware like `AppSidebar`: project context = Overview / Deploys / More;
  workspace context = Projects / New / More. **More** calls the shadcn sidebar's
  `setOpenMobile(true)` — the drawer stays the single source of full navigation.
  Active tab derives from current route.
- **Mobile card-list for tables** — deployments (and similar wide rows) render as
  stacked cards below `md`: commit message title, status badge, branch + env +
  relative time metadata, whole card tappable. Desktop tables untouched. Extract a
  reusable component only where the shape repeats 3+ times (deployments do).
- **Safe areas** — `viewport-fit=cover` in the viewport meta; `env(safe-area-inset-*)`
  utilities on header, tab bar, drawer.
- **Touch sizing** — ≥44px tap targets on mobile; dialogs `max-h-[85dvh]` with
  internal scroll.

Page sweep (~12 pages): Projects, NewProject (3 steps stack), ProjectOverview,
ProjectDeployments (cards + filters collapse to a sheet), DeploymentDetail (log
viewer full-bleed, smaller mono font, horizontal scroll, sticky status header),
Analytics (KPI cards 2-up), Email, Settings Build/Domains/Env/Protection (single
column, env rows wrap), Login.

Verification: Playwright E2E at 390×844 (tab bar nav, card list, dialog fit, log
scroll) + `bunx tsc --noEmit` + existing tests stay green.

## Phase 2 — Installable PWA + offline shell (`feat/pwa-shell`)

- **`vite-plugin-pwa`**: manifest (name Deployik, `display: standalone`,
  theme/background `#09090b`, start `/`); icons from `favicon.svg`: 192/512 PNG,
  512 maskable, 180px `apple-touch-icon` link tag; iOS meta tags
  (`apple-mobile-web-app-capable`, `black-translucent` status bar).
- **Workbox SW**: precache built shell with content-hash versioning;
  `registerType: 'autoUpdate'` + `skipWaiting` + `clientsClaim` (silent updates —
  Deployik deploys often, shell must never go stale); **`NetworkOnly` for `/api/*`
  and `/ws/*`** — zero data caching ever (lesson from the protection auth-page SW
  cache loop); navigation fallback to cached `index.html` excluding `/api`, `/ws`;
  `devOptions.enabled: false`.
- **`spa.go`**: serve `sw.js` with `Cache-Control: no-cache` so browsers revalidate
  the worker every launch.
- **Offline**: app shell opens from cache; offline banner driven by
  `navigator.onLine` + failed-fetch signal in `ApiClient`; TanStack Query retry on
  reconnect. Deliberately no offline data caching.

## Phase 3 — Push notifications (`feat/push-notifications`)

- **SW switches to `injectManifest`** — custom `sw.ts` with Workbox precache (Phase 2
  behavior unchanged) + `push` + `notificationclick` handlers; click deep-links
  (e.g. failed deploy → `/projects/{id}/deployments/{did}`).
- **VAPID keys**: auto-generated on first boot, persisted to `{DATA_DIR}/vapid.json`
  (env override possible); subject from `SSL_EMAIL`. If unreadable/unwritable, push
  endpoints 503; rest of app unaffected.
- **Migration `020_push_subscriptions.sql`** — one row per device: id (ULID),
  user_id FK, endpoint (unique), p256dh, auth, device_label, per-device event-type
  toggles `notify_deploy_outcomes` / `notify_build_starts` / `notify_ssl_issues`
  (default true), created_at, failed_at.
- **`internal/push`** using `webpush-go`: `Notifier.Send(projectID, eventType,
  payload)` — recipients via the same access rule as `authz` (creator + owning-org
  members), filtered by per-subscription toggles, sent best-effort in a goroutine;
  `404/410 Gone` deletes the dead subscription. Never blocks or fails the caller.
- **Hook points**: pipeline finalize (deploy succeeded/failed), webhook handler
  after creating a deployment (build started: branch + pusher), SSL provisioning
  failure in `domain.Manager`.
- **API (protected)**: `GET /api/push/vapid-key`; `GET/POST /api/push/subscriptions`;
  `PATCH/DELETE /api/push/subscriptions/{id}`.
- **Frontend**: "Notifications" page from the More drawer / sidebar footer —
  "Enable on this device" (calls `Notification.requestPermission()` inside the tap
  handler; iOS requires a user gesture), three event-type switches, device list
  with revoke, and an iOS-not-installed hint (push needs home-screen install).

## Testing & error handling

- Go: `internal/push` with injectable mock sender (email-service pattern) —
  recipient/authz resolution, toggle filtering, Gone-cleanup, VAPID persistence;
  handler tests incl. foreign-user rejection; migration idempotency.
- Frontend: tsc + unit tests + Playwright mobile suite (dev-login).
- On-device manual checklist: install, standalone launch, OAuth login inside the
  installed app, receive real push, deep-link tap-through.
- Push/screenshot-style failures are fire-and-forget; SW registration failure
  degrades to a normal web app; mutations fail loudly offline (no queueing).

## Accepted risks

- iOS standalone OAuth quirks — fallback: log in via Safari first, then install.
- 7-day refresh TTL ≈ weekly phone re-login (unchanged in v1).
- Push works only on installed PWA over HTTPS — dev testing via desktop Chrome.
- **Phase 3 deploy requires a production DB backup first** (SQLite migration on VPS).

## Decisions log

- PWA depth: installable + offline shell + push (user picked max depth).
- Push events: deploy outcomes, webhook build starts, domain/SSL problems.
- Push controls: per event-type toggles (per device), no per-project matrix.
- Mobile nav: hybrid bottom tab bar (Overview/Deploys/More) + existing drawer.
- Responsive scope: every page, phone-first audit.
- Tooling: vite-plugin-pwa over hand-rolled SW; native Web Push from Go
  (`webpush-go`) over ntfy/Telegram relay; shared primitives + page sweep over
  separate mobile views.
