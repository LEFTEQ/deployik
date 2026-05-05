# Project Memory Index
> Always loaded. Keep under 100 lines. Last validated: 2026-04-30

## Deployik - Self-hosted Vercel alternative for the Lovinka VPS

### Tech Stack
Go 1.25 (chi, Docker SDK, modernc sqlite) + React 19 (Vite 7, TanStack Router/Query, Zustand, shadcn new-york + Tailwind 4) + SQLite WAL | Single binary: `//go:embed web_dist` | AES-256-GCM at rest | GitHub OAuth -> JWT cookies (1h access / 7d refresh) + `dpk_` Personal Access Tokens via Bearer

### Domain Map

| Domain | Core Tech | Entry Point | L1 Trail |
|--------|-----------|-------------|----------|
| backend | chi v5 | `internal/api/router.go` (`NewRouter`) | middleware chain, handler struct pattern, loadAuthorized* helpers, writeJSON, rate limiter groups, `/api/health` version block, `/api/me/tokens`, `/api/github/repos/{owner}/{repo}/inspect` endpoint |
| database | SQLite + migrations | `internal/db/sqlite.go`, `models.go`, `migrations/*.sql` | ULIDs via `db.NewID()`, embedded SQL migrations through 018, query file split per entity, soft-delete convention |
| auth | GitHub OAuth + JWT + PATs | `internal/auth/jwt.go`, `internal/api/middleware/auth.go` | HttpOnly cookie session, refresh rotation + hashing, `dpk_` PAT Bearer branch, DEV_MODE dev-login, GetClaims context helper |
| build | Docker SDK pipeline | `internal/build/pipeline.go` (`Pipeline.Dispatch` -> `Pipeline.Deploy`) | queued async deploys, blue-green swap, variable split (build vs runtime), Dockerfile generation, semaphore(1) |
| domains | nginx + certbot + auth_request | `internal/domain/ssl.go` (`Manager.ProvisionDomain`) | DNS verify → certbot → nginx template → reload, password auth_request blocks, variant (apex+www) |
| autobuild | GitHub webhooks | `internal/api/handlers/webhooks.go`, `autobuild.go` | HMAC per-project, idempotency by delivery_id+project+environment, opt-in production fan-out, 404=no admin / 403=insufficient scope mapping, `provisionWebhook` shared helper, auto-setup + preview auto-deploy on project creation |
| analytics | Umami + Loki | `internal/analytics/service.go` | Best-effort Umami website provisioning, AI-install prompt, shared stat-card + metric-chart primitives |
| frontend | React 19 + TanStack | `web/src/app/app.tsx` | Route tree with nested layouts, cookie auth via `credentials: 'include'`, class-based ApiClient, hydrateAuthState, account tokens, multi-locale integration, `api.inspectRepo()` + 3-step NewProject flow |
| ui-layout | shadcn sidebar primitives | `web/src/components/layout/AppSidebar.tsx` | Context-aware sidebar (workspace vs project), ProjectPicker command, CommandPalette Cmd+K, Integrations subnav (Analytics/Email/Multi Locale), VersionRow SHA+build badge in footer |
| monorepo | pure Go detection | `internal/monorepo/` (`inspect.go`, `detect.go`, `types.go`) | `RepoInspector` interface, `Report`/`App` types, `ErrFileNotFound` sentinel, pnpm/npm/yarn/bun/turbo/nx detection, concurrent per-app profiling |
| variables | env + secrets in one table | `internal/api/handlers/envvars.go` (`VariableHandler`) | `kind` column splits env/secret, shared→scoped merge, `NEXT_PUBLIC_*` rules, masked in API |
| workspaces | orgs + memberships | `internal/db/queries_organizations.go` | Personal org auto-bootstrapped per user, shared orgs via memberships, `organization_id` filter on projects |
| deploy-ops | Docker + GH Actions | `docker/Dockerfile`, `.github/workflows/ci.yml` | Multi-stage image (Bun→Go→Alpine), SSH-based VPS deploy, seed-dev.sh, backup binary `cmd/backup` |

### Golden Rules
> Learned from corrections and mandated by project conventions. Violations cause rework.

- **Never commit directly** — user always reviews diffs; leave work staged/unstaged unless explicitly told to commit.
- **All sensitive values go through `crypto.Encryptor`** — GitHub tokens, env vars, secrets, webhook secrets, protection passwords. Plaintext is never persisted.
- **Project-scoped handlers MUST call `loadAuthorizedProject`/`loadAuthorizedDeployment`** from `handlers/access.go` before any mutation. No ad-hoc DB lookups bypassing authz.
- **Cookies, not localStorage** for tokens. Frontend must use `credentials: 'include'` via the `ApiClient`; never store JWT/refresh tokens in JS.
- **Database backups before migrations** on prod — for Deployik use `ssh deploy@203.0.113.10 "/opt/scripts/deployik-backup.sh backup"` before deploying migrations; never `docker volume rm` or `docker compose down -v`.
- **Auto-domains (`is_auto=1`) cannot be deleted** — enforced in queries + UI.
- **Best-effort side-effects in `ProjectHandler.Create`** — webhook setup and initial deploy fire after the project row is committed. Preview initial deploy remains default. Production initial deploy only happens when `auto_production_enabled=true` and the auto-build config was durably saved. The project is never rolled back for missed webhook/deploy side-effects.

### Key Commands

| Action | Command |
|--------|---------|
| Dev API (Go, DEV_MODE) | `make dev-api` |
| Dev Frontend (Vite :5173) | `make dev-web` |
| Seed dev data | `make dev-seed` |
| Go tests | `go test ./...` |
| Frontend typecheck | `cd web && bunx tsc --noEmit` |
| Frontend unit tests | `cd web && bun run test` |
| Frontend E2E tests | `cd web && bun run test:e2e` |
| Production build | `make build` |
| Docker image | `make docker-build` |
| Manual deploy to VPS | `./scripts/deploy.sh [tag]` |
