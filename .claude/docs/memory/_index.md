# Project Memory Index
> Always loaded. Keep under 100 lines. Last validated: 2026-04-16

## Deployik - Self-hosted Vercel alternative for the Lovinka VPS

### Tech Stack
Go 1.25 (chi, Docker SDK, modernc sqlite) + React 19 (Vite 7, TanStack Router/Query, Zustand, shadcn new-york + Tailwind 4) + SQLite WAL | Single binary: `//go:embed web_dist` | AES-256-GCM at rest | GitHub OAuth → JWT (1h access / 7d refresh)

### Domain Map

| Domain | Core Tech | Entry Point | L1 Trail |
|--------|-----------|-------------|----------|
| backend | chi v5 | `internal/api/router.go` (`NewRouter`) | middleware chain, handler struct pattern, loadAuthorized* helpers, writeJSON, rate limiter groups |
| database | SQLite + migrations | `internal/db/sqlite.go`, `models.go`, `migrations/*.sql` | ULIDs via `db.NewID()`, embedded SQL migrations, query file split per entity, soft-delete convention |
| auth | GitHub OAuth + JWT + cookies | `internal/auth/jwt.go`, `internal/api/handlers/auth.go` | Cookie-only tokens, refresh rotation + hashing, DEV_MODE dev-login, GetClaims context helper |
| build | Docker SDK pipeline | `internal/build/pipeline.go` (`Pipeline.Deploy`) | 11-step blue-green deploy, variable split (build vs runtime), Dockerfile generation, semaphore(1) |
| domains | nginx + certbot + auth_request | `internal/domain/ssl.go` (`Manager.ProvisionDomain`) | DNS verify → certbot → nginx template → reload, password auth_request blocks, variant (apex+www) |
| autobuild | GitHub webhooks | `internal/api/handlers/webhooks.go`, `autobuild.go` | HMAC per-project, idempotency via delivery_id, 404=no admin / 403=insufficient scope mapping |
| analytics | Umami + Loki | `internal/analytics/service.go` | Best-effort Umami website provisioning, AI-install prompt, shared stat-card + metric-chart primitives |
| frontend | React 19 + TanStack | `web/src/app/app.tsx` | Route tree with nested layouts, cookie auth via `credentials: 'include'`, class-based ApiClient, hydrateAuthState |
| ui-layout | shadcn sidebar primitives | `web/src/components/layout/AppSidebar.tsx` | Context-aware sidebar (workspace vs project), ProjectPicker command, CommandPalette Cmd+K, release dialog content |
| variables | env + secrets in one table | `internal/api/handlers/envvars.go` (`VariableHandler`) | `kind` column splits env/secret, shared→scoped merge, `NEXT_PUBLIC_*` rules, masked in API |
| workspaces | orgs + memberships | `internal/db/queries_organizations.go` | Personal org auto-bootstrapped per user, shared orgs via memberships, `organization_id` filter on projects |
| deploy-ops | Docker + GH Actions | `docker/Dockerfile`, `.github/workflows/ci.yml` | Multi-stage image (Bun→Go→Alpine), SSH-based VPS deploy, seed-dev.sh, backup binary `cmd/backup` |

### Golden Rules
> Learned from corrections and mandated by project conventions. Violations cause rework.

- **Never commit directly** — user always reviews diffs; leave work staged/unstaged unless explicitly told to commit.
- **All sensitive values go through `crypto.Encryptor`** — GitHub tokens, env vars, secrets, webhook secrets, protection passwords. Plaintext is never persisted.
- **Project-scoped handlers MUST call `loadAuthorizedProject`/`loadAuthorizedDeployment`** from `handlers/access.go` before any mutation. No ad-hoc DB lookups bypassing authz.
- **Cookies, not localStorage** for tokens. Frontend must use `credentials: 'include'` via the `ApiClient`; never store JWT/refresh tokens in JS.
- **Database backups before migrations** on prod — `pnpm db:backup` in lovinka project, never `docker volume rm` or `docker compose down -v`.
- **Auto-domains (`is_auto=1`) cannot be deleted** — enforced in queries + UI.

### Key Commands

| Action | Command |
|--------|---------|
| Dev API (Go, DEV_MODE) | `make dev-api` |
| Dev Frontend (Vite :5173) | `make dev-web` |
| Seed dev data | `make dev-seed` |
| Go tests | `go test ./...` |
| Frontend typecheck | `cd web && bunx tsc --noEmit` |
| Production build | `make build` |
| Docker image | `make docker-build` |
| Manual deploy to VPS | `./scripts/deploy.sh [tag]` |
