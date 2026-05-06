# Deployik

Self-hosted deployment platform for the [Lovinka](https://example.com) VPS. A lightweight Vercel alternative that deploys Next.js and static web apps from GitHub repositories with automatic domains, SSL certificates, environment variables, auto-build previews, opt-in production auto-deploy, and blue-green zero-downtime deployments.

## Features

- **GitHub Integration** -- Import repos, select branches, deploy with one click
- **Framework Support** -- Next.js (standalone), Vite, Astro, and generic static sites
- **Blue-Green Deploys** -- Zero-downtime container swaps with automatic health checks
- **Automatic Domains** -- Preview URLs generated per project and branch (`{name}.preview.example.com`, `{name}-{branch}.preview.example.com`)
- **Custom Domains** -- Add your own domain with DNS verification and auto-provisioned SSL (Let's Encrypt)
- **Auto-Build on Push** -- GitHub webhooks deploy previews automatically; production can opt in to track pushes to the production branch without creating release tags
- **Environment Variables and Secrets** -- Separate stores with shared/preview/production scoping, encrypted at rest (AES-256-GCM)
- **Build Settings** -- Configurable framework preset, root directory (monorepo support), output directory, install/build commands, Node.js version
- **Real-time Build Logs** -- WebSocket streaming during builds, persisted for later review
- **Monorepo Support** -- Root directory support for apps inside monorepos, automatic lock file detection from repo root or app directory
- **Dockerfile Override** -- Bring your own Dockerfile; Deployik only generates one when none exists
- **Host Network Access** -- Per-project toggle lets deployed containers reach host services (Redis, MySQL, etc.) via `host.docker.internal`
- **Persistent Volumes** -- Named Docker volumes per project-environment that survive redeployments, with UI to manage, recreate, and delete
- **Integrations Guidance** -- Analytics, Webglobe SMTP/reCAPTCHA email, and Multi Locale setup pages provide install prompts and concrete implementation steps
- **Configurable Proxy** -- Works with Docker nginx-proxy (default) or host-based proxies (Apache, nginx) via `PROXY_TYPE=host-port`
- **Apache Support** -- Generates Apache VirtualHost configs alongside nginx, with wildcard SSL cert support
- **Single Binary** -- Go backend embeds the React SPA via `go:embed`, ships as one container
- **Safe SQLite Backups** -- Companion `deployik-backup` binary creates verified live snapshots for systemd/offsite backup jobs

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Go 1.25, chi router, Docker SDK |
| Database | SQLite (WAL mode, modernc.org/sqlite -- pure Go, no CGO) |
| Frontend | React 19, Vite 7, TanStack Router + Query, Zustand, shadcn/ui, Tailwind CSS 4 |
| Auth | GitHub OAuth, JWT cookies (access + refresh), Personal Access Tokens (`dpk_...`) |
| Encryption | AES-256-GCM (env vars and secrets encrypted at rest) |
| SSL | Let's Encrypt via Certbot (Docker-based) or existing wildcard certs |
| Proxy | Nginx reverse proxy (Docker or host) or Apache (host) |
| CI/CD | GitHub Actions -- test, build Docker image, push to GHCR, deploy to VPS |
| IDs | ULIDs (time-sortable, URL-safe) |

## Architecture

### Docker nginx-proxy mode (default)

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
│  │ (Docker) │    │            │    │  app containers  │  │
│  │  :80/443 │    │  :8080     │    │  :3000 each     │  │
│  └──────────┘    └──────┬─────┘    └─────────────────┘  │
│       ▲                 │            ▲ (Docker network)  │
│       └─── Docker DNS ──┘────────────┘                   │
│                    ┌─────────┐                           │
│                    │ SQLite  │                           │
│                    └─────────┘                           │
└──────────────────────────────────────────────────────────┘
```

nginx-proxy container is on the same Docker network as deployed apps, so it reaches them by container name.

### Host proxy mode (Apache or nginx on host)

```
┌──────────────────────────────────────────────────────────┐
│                     Hetzner VPS                          │
│                                                          │
│  ┌──────────┐    ┌────────────┐    ┌─────────────────┐  │
│  │ Apache/  │───▶│  Deployik  │───▶│ Docker Engine   │  │
│  │  nginx   │    │  (Go+SPA)  │    │                 │  │
│  │ (host)   │    │            │    │  app containers  │  │
│  │  :80/443 │    │  :8080     │    │  :3000 → :RAND  │  │
│  └──────────┘    └──────┬─────┘    └─────────────────┘  │
│       ▲                 │            ▲ (host port bind)  │
│       └── 127.0.0.1:PORT┘───────────┘                   │
│                    ┌─────────┐                           │
│                    │ SQLite  │                           │
│                    └─────────┘                           │
└──────────────────────────────────────────────────────────┘
```

Each container's port 3000 is bound to a random localhost port. Deployik writes Apache/nginx configs pointing to `127.0.0.1:{port}` and reloads the proxy.

## Setup

### Prerequisites

- Go 1.25+
- Bun (frontend package manager)
- Docker
- A reverse proxy: Docker nginx-proxy (default) or Apache/nginx on the host

### 1. Clone and install dependencies

```bash
git clone https://github.com/LEFTEQ/lovinka-deployik.git
cd lovinka-deployik

go mod download
cd web && bun install && cd ..
```

### 2. Create a GitHub OAuth App

1. Go to **GitHub Settings > Developer settings > OAuth Apps > New OAuth App**
2. Set the **Authorization callback URL** to `https://your-domain.com/auth/callback` (or `http://localhost:5173/auth/callback` for local dev)
3. Note the **Client ID** and **Client Secret**

### 3. Generate secrets and write `.env`

```bash
# Generate random secrets
JWT_SECRET=$(openssl rand -hex 32)
ENCRYPTION_KEY=$(openssl rand -hex 32)

cat > .env <<EOF
JWT_SECRET=$JWT_SECRET
ENCRYPTION_KEY=$ENCRYPTION_KEY

GITHUB_CLIENT_ID=your_client_id_here
GITHUB_CLIENT_SECRET=your_client_secret_here

DEV_MODE=true
DATABASE_PATH=data/deployik.db
FRONTEND_URL=https://your-domain.com
EOF
```

### 4. Choose your proxy mode

#### Option A: Docker nginx-proxy (default)

No extra config needed. Deployik expects a Docker nginx container on the `proxy` network:

```bash
docker network create proxy
# Run your nginx-proxy container connected to this network
```

#### Option B: Host Apache

Add these to your `.env`:

```bash
# Proxy: host Apache mode
PROXY_TYPE=host-port
PROXY_CONFIG_FORMAT=apache
PROXY_RELOAD_CMD=apachectl graceful
NGINX_CONF_DIR=/etc/apache2/deployik-vhosts

# Optional: use an existing wildcard cert (skips per-domain certbot)
PROXY_SSL_CERT=/etc/letsencrypt/live/yourdomain.com/fullchain.pem
PROXY_SSL_KEY=/etc/letsencrypt/live/yourdomain.com/privkey.pem
```

Then set up Apache:

```bash
mkdir -p /etc/apache2/deployik-vhosts
echo 'IncludeOptional /etc/apache2/deployik-vhosts/*.conf' >> /etc/apache2/apache2.conf
apachectl graceful
```

Required Apache modules: `proxy`, `proxy_http`, `proxy_wstunnel`, `rewrite`, `ssl`, `headers`.

#### Option C: Host nginx

```bash
PROXY_TYPE=host-port
PROXY_CONFIG_FORMAT=nginx
PROXY_RELOAD_CMD=nginx -s reload
NGINX_CONF_DIR=/etc/nginx/conf.d
```

### 5. Create data directory and run

```bash
mkdir -p data

# Terminal 1: Go API server
make dev-api

# Terminal 2: React frontend (Vite dev server, proxies /api and /ws to Go)
make dev-web
```

The frontend runs on `http://localhost:5173` (or your `FRONTEND_URL` via a reverse proxy).

### 6. Optional settings

Add to `.env` as needed:

```bash
# Restrict login to specific GitHub users
ALLOWED_GITHUB_USERS=user1,user2,user3

# VPS IP for DNS verification of custom domains
VPS_HOST=1.2.3.4

# Let's Encrypt email (for certbot, when not using wildcard certs)
SSL_EMAIL=admin@yourdomain.com
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
| `PORT` | `8080` | HTTP server port — serves both the API and the embedded React SPA (no separate frontend needed in production) |
| `DATABASE_PATH` | `data/deployik.db` | Path to SQLite database file |
| `DEV_MODE` | _(unset)_ | Set to `true` to allow startup without all required env vars |
| `ALLOWED_GITHUB_USERS` | _(empty = all)_ | Comma-separated GitHub usernames allowed to log in |
| `FRONTEND_URL` | `http://localhost:5173` | Frontend URL for OAuth redirect |
| `BUILD_DIR` | `/tmp/deployik-builds` | Temporary directory for git clones and Docker builds |
| `VPS_HOST` | `203.0.113.10` | VPS IP address for DNS verification |

### Proxy Configuration

| Variable | Default | Description |
|---|---|---|
| `PROXY_TYPE` | `docker` | `docker` (container name upstream) or `host-port` (random localhost port) |
| `PROXY_CONFIG_FORMAT` | `nginx` | `nginx` (server blocks) or `apache` (VirtualHost blocks) |
| `PROXY_RELOAD_CMD` | _(empty)_ | Shell command to reload proxy in host-port mode (e.g. `apachectl graceful`) |
| `PROXY_SSL_CERT` | _(empty)_ | Path to existing wildcard SSL cert (skips per-domain certbot) |
| `PROXY_SSL_KEY` | _(empty)_ | Path to existing wildcard SSL key |
| `NGINX_CONF_DIR` | `/opt/nginx-proxy/conf.d` | Directory where proxy configs are written |
| `PROXY_CONTAINER_NAME` | `nginx-proxy` | Docker container to reload (docker mode only) |
| `PROXY_CERTS_DIR` | `/opt/nginx-proxy/certs` | Host path to Let's Encrypt certs (docker mode only) |
| `PROXY_HTML_DIR` | `/opt/nginx-proxy/html` | Host path for ACME challenges (docker mode only) |
| `SSL_EMAIL` | `admin@example.com` | Email for Let's Encrypt registration |

See `.env.example` for a complete template with comments.

## Build

```bash
# Build production binary (frontend build + Go embed)
make build

# Build Docker image
make docker-build

# Run Docker image locally
make docker-run
```

## Tests

```bash
# Go tests
go test ./...

# Frontend typecheck
cd web && bunx tsc --noEmit

# Frontend unit tests
cd web && bun run test

# Frontend E2E tests
cd web && bun run test:e2e

# Create a verified SQLite snapshot locally
go run ./cmd/backup create --database data/deployik.db --output /tmp/deployik-backup.sqlite3
```

## Production Deployment

### Running with host-based proxy (Apache/nginx on host)

When using `PROXY_TYPE=host-port` with a host-based proxy, Deployik needs to run commands on the host (e.g. `apachectl graceful`) and write config files to host paths (e.g. `/etc/apache2/deployik-vhosts/`). This creates a challenge: if Deployik runs inside a Docker container, those commands execute inside the container where the host proxy doesn't exist.

#### Option 1: Run Deployik directly on the host (recommended for host-proxy mode)

Build and run the Go binary on the host. No container boundary to cross -- `apachectl graceful` runs natively, config files are written directly to the host filesystem.

```bash
make build
./bin/deployik
```

Use systemd to manage the process. Templates live in `scripts/examples/`, with `@DEPLOYIK_HOME@`, `@DEPLOYIK_USER@`, and (for the web service) `@BUN_BIN@` placeholders you substitute before installing:

```bash
# Pick values that match your host
export DEPLOYIK_HOME=/opt/deployik
export DEPLOYIK_USER=deploy       # user in the `docker` group
export BUN_BIN=/home/deploy/.bun/bin/bun
export BUN_BIN_DIR=/home/deploy/.bun/bin

# API server (Go binary; suffices for production)
sed "s|@DEPLOYIK_HOME@|$DEPLOYIK_HOME|g; s|@DEPLOYIK_USER@|$DEPLOYIK_USER|g" \
  "$DEPLOYIK_HOME/scripts/examples/deployik.service" \
  | sudo tee /etc/systemd/system/deployik.service >/dev/null

# Frontend dev server (Vite) — only for dev/staging where you want HMR
sed "s|@DEPLOYIK_HOME@|$DEPLOYIK_HOME|g; s|@DEPLOYIK_USER@|$DEPLOYIK_USER|g; s|@BUN_BIN@|$BUN_BIN|g; s|@BUN_BIN_DIR@|$BUN_BIN_DIR|g" \
  "$DEPLOYIK_HOME/scripts/examples/deployik-web.service" \
  | sudo tee /etc/systemd/system/deployik-web.service >/dev/null

sudo systemctl daemon-reload
sudo systemctl enable --now deployik
# Optional (dev/staging only):
# sudo systemctl enable --now deployik-web
```

For production, you typically only need `deployik.service` since the Go binary serves the embedded SPA. If you do run the Vite dev server behind a public hostname (via a reverse proxy), set `VITE_ALLOWED_HOSTS=dev.example.com,staging.example.com` in its environment so Vite accepts the host header.

#### Option 2: Run Deployik inside Docker (requires elevated privileges)

If you prefer the Docker deployment model, mount the host proxy config directory and use `nsenter` to execute reload commands in the host's namespace:

```yaml
# docker-compose.yml additions
services:
  app:
    volumes:
      - /etc/apache2/deployik-vhosts:/etc/apache2/deployik-vhosts
    privileged: true  # or cap_add: [SYS_ADMIN]
    pid: host
    environment:
      - PROXY_RELOAD_CMD=nsenter -t 1 -m -- apachectl graceful
```

This works but requires `privileged` mode or `SYS_ADMIN` capability, which weakens container isolation. Option 1 is the cleaner approach for host-proxy setups.

### Running with Docker nginx-proxy (default mode)

When using the default `PROXY_TYPE=docker`, Deployik runs inside Docker alongside nginx-proxy on the same Docker network. No special permissions needed -- this is the original architecture and works out of the box with `docker-compose.yml`.

### CI/CD

Deployik is deployed via GitHub Actions on every push to `main`:

1. **Test** -- Go tests, frontend typecheck, frontend unit tests, frontend build
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

## How Deployments Work

1. **Trigger** -- User clicks "Deploy" in the UI, specifying environment (preview/production) and branch
2. **Queue** -- Deployment record created, waits for build slot (semaphore, max 1 concurrent build)
3. **Clone** -- Shallow clone of the GitHub repo using the user's OAuth token
4. **Resolve Settings** -- `projectconfig.Resolve()` merges project fields with framework defaults
5. **Patch** -- For Next.js projects, injects `output: 'standalone'` into `next.config.*` if not present
6. **Variables** -- Loads env vars and secrets for the deployment environment (shared + scoped, decrypted)
7. **Dockerfile** -- Generates a multi-stage Dockerfile (or uses existing one from repo), with lock-file detection for bun/pnpm/npm
8. **Build** -- Docker image build with build-time env vars (`NEXT_PUBLIC_*`) baked in
9. **Volume** -- If data volume is enabled, ensures a named Docker volume exists (`deployik-{project}-{env}-data`)
10. **Run** -- New container started with temporary name, runtime env vars passed as Docker env. If host-port mode, port 3000 is bound to a random localhost port. If host network access is enabled, `host.docker.internal` resolves to the host.
11. **Health Check** -- Polls container until running for 5+ seconds (or timeout at 60s)
12. **Domain Provisioning** -- For each domain: verify DNS, request SSL cert (or use wildcard), write proxy config (nginx or Apache), reload proxy
13. **Swap** -- Old container stopped, new container renamed to canonical name (blue-green)
14. **Finalize** -- Previous live deployment marked as "replaced", new deployment marked as "live"

## Auto-Build on Push

Projects can provision a GitHub webhook from the new-project wizard or Project Settings.

- Preview auto-build is the default for imported projects and follows the preview branch rules (`*` by default). Each matching branch gets its own persistent preview URL.
- Production auto-deploy is explicit opt-in. When enabled, a push to the configured production branch creates a production deployment from the same commit.
- A single GitHub delivery can create both preview and production deployments; idempotency is tracked per delivery, project, and environment.
- Webhook-triggered production deployments do **not** create git tags. Tags remain tied to manual release actions.
- Before deploying code with a new SQLite migration, create a live backup on the VPS:

```bash
ssh deploy@203.0.113.10 "/opt/scripts/deployik-backup.sh backup"
```

Build logs are streamed in real-time via WebSocket and persisted to SQLite for later viewing.

## Known Limitations

- **Apache + password protection**: The Apache VirtualHost template does not include `auth_request` (nginx-specific). Password protection only works with `PROXY_CONFIG_FORMAT=nginx`.
- **Volume permissions**: Docker volumes are created as root. If the container runs as a non-root user, you may need to adjust permissions in your Dockerfile.
- **Host-port mode + container restart**: In `PROXY_TYPE=host-port` mode each container binds to a random host port. If the container restarts (Docker's `unless-stopped` policy, or `docker restart`) it picks up a *new* random port and the existing proxy config is stale until Deployik itself restarts — startup reconcile re-reads the live port on boot. Workarounds: redeploy instead of restarting, or restart Deployik to force a reconcile. A future change will move to deterministic per-project ports so container restart stays transparent.
- **Renaming a project with a volume**: The volume and container names are derived from `project.Name`, so renaming would orphan the data. The API blocks the rename (HTTP 409) while `data_volume_enabled=true`. Disable the volume first (this abandons the data), or wait for the follow-up that keys volumes by `project.ID`.

## Project Structure

```
cmd/server/              Go entry point, embeds React SPA via go:embed
cmd/backup/              CLI helper that creates/verifies consistent SQLite snapshots for backup jobs
internal/
  backup/
    sqlite.go            VACUUM INTO-based live snapshot + integrity_check verification helpers
  api/
    handlers/            REST handlers (auth, projects, deployments, domains, envvars, volumes)
    middleware/           JWT auth middleware, CORS, rate limiting
    router.go            chi router with all route definitions
    spa.go               Embedded SPA serving with client-side routing fallback
  auth/                  JWT generation/validation, context helpers
  authz/                 Authorization (project ownership checks, admin bypass)
  build/
    pipeline.go          Full deployment orchestration (clone -> build -> deploy -> swap)
    clone.go             Git clone with token auth
    docker.go            Docker SDK wrapper (build, run, stop, health check, volumes, host port)
    dockerfile.go        Dockerfile generation (Next.js standalone + static site)
    nextjs.go            Next.js config patching (injects output: 'standalone')
    variables.go         Build-time vs runtime variable resolution
    semaphore.go         Concurrent build limiter
  config/                Environment variable loading
  crypto/                AES-256-GCM encryption/decryption, value masking
  db/
    sqlite.go            SQLite connection with WAL pragmas
    migrations.go        Embedded SQL migration runner
    migrations/          SQL migration files (001-018)
    models.go            Go structs (User, Project, Deployment, Domain, ProjectVariable, AutoBuildConfig, APIToken)
    queries_*.go         Query functions per entity
  domain/
    ssl.go               Domain manager (certbot, proxy config, DNS verification, ReloadProxy)
    nginx.go             Nginx config generation from template
    apache.go            Apache VirtualHost config generation
    reconcile.go         Startup reconciliation of active domain configs
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
    pages/               Page components (Login, Projects, NewProject, ProjectDetail, ProjectSettings, DeploymentDetail)
    components/
      layout/            AppLayout shell, navigation
      projects/          BuildSettingsFields reusable component
      ui/                shadcn/ui components
      BuildLog.tsx       Build log viewer with auto-scroll
    hooks/               useBuildLogs WebSocket hook
    lib/                 API client, utils
    store/               Zustand auth store (persisted to localStorage)
    types/               TypeScript API types
docker/
  Dockerfile             Multi-stage build (Bun frontend -> Go binary -> Alpine runtime)
  docker-compose.yml     Production compose with Docker socket mount, proxy volumes, data volume
scripts/
  deploy.sh              Manual deploy script
  deploy-vps.sh          CI deploy script (used by GitHub Actions)
templates/
  nextjs.Dockerfile.tmpl Legacy Go template (superseded by programmatic generation in dockerfile.go)
```
