# Deployik

Self-hosted deployment platform for the [Lovinka](https://example.com) VPS. A lightweight Vercel alternative that deploys Next.js and static web apps from GitHub repositories with automatic domains, SSL certificates, environment variables, and blue-green zero-downtime deployments.

## Features

- **GitHub Integration** -- Import repos, select branches, deploy with one click
- **Framework Support** -- Next.js (standalone), Vite, Astro, and generic static sites
- **Blue-Green Deploys** -- Zero-downtime container swaps with automatic health checks
- **Automatic Domains** -- Preview URLs generated per project (`{name}.preview.example.com`)
- **Custom Domains** -- Add your own domain with DNS verification and auto-provisioned SSL (Let's Encrypt)
- **Environment Variables and Secrets** -- Separate stores with shared/preview/production scoping, encrypted at rest (AES-256-GCM)
- **Build Settings** -- Configurable framework preset, root directory (monorepo support), output directory, install/build commands, Node.js version
- **Real-time Build Logs** -- WebSocket streaming during builds, persisted for later review
- **Monorepo Support** -- Root directory support for apps inside monorepos, automatic lock file detection from repo root or app directory
- **Dockerfile Override** -- Bring your own Dockerfile; Deployik only generates one when none exists
- **Single Binary** -- Go backend embeds the React SPA via `go:embed`, ships as one container
- **Safe SQLite Backups** -- Companion `deployik-backup` binary creates verified live snapshots for systemd/offsite backup jobs

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Go 1.25, chi router, Docker SDK |
| Database | SQLite (WAL mode, modernc.org/sqlite -- pure Go, no CGO) |
| Frontend | React 19, Vite 7, TanStack Router + Query, Zustand, shadcn/ui, Tailwind CSS 4 |
| Auth | GitHub OAuth, JWT (access + refresh tokens) |
| Encryption | AES-256-GCM (env vars and secrets encrypted at rest) |
| SSL | Let's Encrypt via Certbot (Docker-based) |
| Proxy | Nginx reverse proxy (shared with other Lovinka apps) |
| CI/CD | GitHub Actions -- test, build Docker image, push to GHCR, deploy to VPS |
| IDs | ULIDs (time-sortable, URL-safe) |

## Architecture

```
                   ┌──────────────────────┐
                   │   GitHub Actions CI  │
                   │  test → build → push │
                   └──────────┬───────────┘
                              │ docker pull + restart
                              ▼
┌──────────────────────────────────────────────────────────┐
│                     Hetzner VPS                          │
│                                                          │
│  ┌──────────┐    ┌────────────┐    ┌─────────────────┐  │
│  │  nginx   │───▶│  Deployik  │───▶│ Docker Engine   │  │
│  │  proxy   │    │  (Go+SPA)  │    │                 │  │
│  │          │    │            │    │  app containers  │  │
│  │  :80/443 │    │  :8080     │    │  :3000 each     │  │
│  └──────────┘    └──────┬─────┘    └─────────────────┘  │
│                         │                                │
│                    ┌────┴────┐                           │
│                    │ SQLite  │                           │
│                    │  (WAL)  │                           │
│                    └─────────┘                           │
└──────────────────────────────────────────────────────────┘
```

## Development

### Prerequisites

- Go 1.25+
- Bun (frontend package manager)
- Docker (for running deployments locally)

### Local Development

Run the Go API and React frontend in separate terminals:

```bash
# Terminal 1: Go API server (dev mode, relaxed config)
make dev-api

# Terminal 2: React frontend (Vite dev server, proxies /api and /ws to Go)
make dev-web
```

The frontend dev server runs on `http://localhost:5173` and proxies API calls to the Go server on `:8080`.

### Build

```bash
# Build production binary (frontend build + Go embed)
make build

# Build Docker image
make docker-build

# Run Docker image locally
make docker-run
```

### Tests

```bash
# Go tests
go test ./...

# Frontend typecheck
cd web && bunx tsc --noEmit

# Create a verified SQLite snapshot locally
go run ./cmd/backup create --database data/deployik.db --output /tmp/deployik-backup.sqlite3
```

## Configuration

### Required Environment Variables

| Variable | Description |
|---|---|
| `JWT_SECRET` | Secret for signing JWT tokens |
| `ENCRYPTION_KEY` | Key for AES-256-GCM encryption of env vars, secrets, and GitHub tokens |
| `GITHUB_CLIENT_ID` | GitHub OAuth App client ID |
| `GITHUB_CLIENT_SECRET` | GitHub OAuth App client secret |

### Optional Environment Variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP server port |
| `DATABASE_PATH` | `data/deployik.db` | Path to SQLite database file |
| `ALLOWED_GITHUB_USERS` | _(empty = all)_ | Comma-separated GitHub usernames allowed to log in |
| `FRONTEND_URL` | `http://localhost:5173` | Frontend URL for OAuth redirect |
| `NGINX_CONF_DIR` | `/opt/nginx-proxy/conf.d` | Directory for generated nginx configs |
| `PROXY_CONTAINER_NAME` | `nginx-proxy` | Docker container name of the nginx reverse proxy |
| `PROXY_CERTS_DIR` | `/opt/nginx-proxy/certs` | Host path to Let's Encrypt certs (bind-mounted to certbot) |
| `PROXY_HTML_DIR` | `/opt/nginx-proxy/html` | Host path for ACME challenge files |
| `SSL_EMAIL` | `admin@example.com` | Email for Let's Encrypt certificate registration |
| `BUILD_DIR` | `/tmp/deployik-builds` | Temporary directory for git clones and Docker builds |
| `VPS_HOST` | `203.0.113.10` | VPS IP address for DNS verification |

See `.env.example` for a complete template.

## Production Deployment

Deployik is deployed via GitHub Actions on every push to `main`:

1. **Test** -- Go tests, frontend typecheck, frontend build
2. **Build and Push** -- Multi-stage Docker build, push to GHCR (`ghcr.io/lefteq/lovinka-deployik`)
3. **Deploy to VPS** -- SSH into VPS, pull new image, restart container, health check

### Required GitHub Secrets

| Secret | Description |
|---|---|
| `VPS_DEPLOY_SSH_KEY` | SSH private key for the `deploy` user on the VPS |
| `GHCR_USERNAME` | GitHub Container Registry username (optional, defaults to repo owner) |
| `GHCR_TOKEN` | GitHub token with `packages:write` scope (optional, defaults to `GITHUB_TOKEN`) |

### Manual Deploy

```bash
# Deploy latest image
./scripts/deploy.sh

# Deploy specific tag
./scripts/deploy.sh abc1234
```

## Project Structure

```
cmd/server/              Go entry point, embeds React SPA via go:embed
cmd/backup/              CLI helper that creates/verifies consistent SQLite snapshots for backup jobs
internal/
  backup/
    sqlite.go            VACUUM INTO-based live snapshot + integrity_check verification helpers
  api/
    handlers/            REST handlers (auth, projects, deployments, domains, envvars)
    middleware/           JWT auth middleware, CORS
    router.go            chi router with all route definitions
    spa.go               Embedded SPA serving with client-side routing fallback
  auth/                  JWT generation/validation, context helpers
  authz/                 Authorization (project ownership checks, admin bypass)
  build/
    pipeline.go          Full deployment orchestration (clone → build → deploy → swap)
    clone.go             Git clone with token auth
    docker.go            Docker SDK wrapper (build image, run/stop container, health check)
    dockerfile.go        Dockerfile generation (Next.js standalone + static site)
    nextjs.go            Next.js config patching (injects output: 'standalone')
    variables.go         Build-time vs runtime variable resolution
    semaphore.go         Concurrent build limiter
  config/                Environment variable loading
  crypto/                AES-256-GCM encryption/decryption, value masking
  db/
    sqlite.go            SQLite connection with WAL pragmas
    migrations.go        Embedded SQL migration runner
    migrations/          SQL migration files (001_initial, 002_variable_kinds, 003_build_settings)
    models.go            Go structs (User, Project, Deployment, Domain, ProjectVariable)
    queries_*.go         Query functions per entity
  domain/
    ssl.go               Domain manager (certbot, nginx, DNS verification orchestration)
    nginx.go             Nginx config generation from template
    dns.go               DNS A-record verification
  github/
    client.go            GitHub API client (repos, branches, commits)
    oauth.go             GitHub OAuth flow (authorize, exchange code, get user)
  projectconfig/
    defaults.go          Framework presets, build settings resolution, path normalization
  ws/
    hub.go               WebSocket pub/sub hub for build log streaming
    logs.go              WebSocket handler with JWT auth
web/
  src/
    app/app.tsx          TanStack Router setup, route tree, providers
    pages/               Page components (Login, Projects, NewProject, ProjectDetail, DeploymentDetail, AuthCallback)
    components/
      layout/            AppLayout shell, Sidebar
      projects/          BuildSettingsFields reusable component
      ui/                shadcn/ui components
      BuildLog.tsx       Build log viewer with auto-scroll
    hooks/               useBuildLogs WebSocket hook
    lib/                 API client, utils
    store/               Zustand auth store (persisted to localStorage)
    types/               TypeScript API types
docker/
  Dockerfile             Multi-stage build (Bun frontend → Go binary → Alpine runtime)
  docker-compose.yml     Production compose with Docker socket mount, nginx volume, data volume
scripts/
  deploy.sh              Manual deploy script
  deploy-vps.sh          CI deploy script (used by GitHub Actions)
templates/
  nextjs.Dockerfile.tmpl Legacy Go template (superseded by programmatic generation in dockerfile.go)
```

## How Deployments Work

1. **Trigger** -- User clicks "Deploy" in the UI, specifying environment (preview/production) and branch
2. **Queue** -- Deployment record created, waits for build slot (semaphore, max 1 concurrent build)
3. **Clone** -- Shallow clone of the GitHub repo using the user's OAuth token
4. **Resolve Settings** -- `projectconfig.Resolve()` merges project fields with framework defaults
5. **Patch** -- For Next.js projects, injects `output: 'standalone'` into `next.config.*` if not present
6. **Variables** -- Loads env vars and secrets for the deployment environment (shared + scoped, decrypted)
7. **Dockerfile** -- Generates a multi-stage Dockerfile (or uses existing one from repo), with lock-file detection for bun/pnpm/npm
8. **Build** -- Docker image build with build-time env vars (`NEXT_PUBLIC_*`) baked in
9. **Run** -- New container started with temporary name, runtime env vars passed as Docker env
10. **Health Check** -- Polls container until running for 5+ seconds (or timeout at 60s)
11. **Domain Provisioning** -- For each domain in the deployment's environment: verify DNS, request SSL cert via certbot, write nginx config, reload nginx
12. **Swap** -- Old container stopped, new container renamed to canonical name (blue-green)
13. **Finalize** -- Previous live deployment marked as "replaced", new deployment marked as "live"

Build logs are streamed in real-time via WebSocket and persisted to SQLite for later viewing.
