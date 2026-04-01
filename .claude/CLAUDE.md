# Deployik

Self-hosted Vercel alternative for the lovinka VPS. Deploys Next.js apps with automatic domains, SSL, and zero-downtime.

## Stack

- **Backend:** Go (chi router, SQLite via modernc.org/sqlite, Docker SDK)
- **Frontend:** React 19 + Vite + TanStack Router/Query + Zustand + shadcn/ui (new-york, zinc)
- **Package Manager:** Bun (frontend)
- **Database:** SQLite (embedded, WAL mode)
- **Deployment:** Single Go binary embeds React SPA via `//go:embed`

## Project Structure

```
cmd/server/          Go entry point + embedded SPA
internal/
  api/               chi router, handlers, middleware
  build/             Deploy pipeline (clone, docker build, container mgmt)
  config/            Environment config loading
  crypto/            AES-256-GCM encryption
  db/                SQLite, migrations, models, queries
  domain/            Nginx config gen, SSL, DNS verification
  github/            OAuth, repo listing
  ws/                WebSocket log streaming
web/                 React frontend (Vite + Bun)
docker/              Dockerfile + docker-compose
templates/           Dockerfile templates for deployed apps
migrations/          SQL migration files
```

## Development

```bash
# Start Go API (dev mode)
make dev-api

# Start React frontend (Vite dev server with proxy to Go API)
make dev-web

# Build production binary (frontend + backend)
make build

# Docker
make docker-build
```

## Key Patterns

- **SPA embedding:** `web/dist` → `cmd/server/web_dist` → Go `//go:embed` → served at `/*`
- **API routes:** `/api/*` via chi router, `/ws/*` for WebSocket
- **Auth:** GitHub OAuth → JWT (access + refresh tokens)
- **Deploy flow:** Manual trigger → clone → docker build → blue-green swap → nginx reload
- **Next.js patching:** `internal/build/nextjs.go` injects `output: 'standalone'` into plain, typed, and wrapped `next.config.*` variants before Docker builds so `.next/standalone` exists for the runtime image
- **Preview domains:** successful preview deploys must provision `/opt/nginx-proxy/conf.d` and request a dedicated cert for `{project}.preview.example.com`; `*.example.com` does not cover nested `*.preview.example.com`
- **Env vars:** AES-256-GCM encrypted at rest, masked in API responses
- **Frontend state:** Zustand for auth, TanStack Query for server state

## Design Spec

Full design at `docs/superpowers/specs/2026-04-01-deployik-design.md`
