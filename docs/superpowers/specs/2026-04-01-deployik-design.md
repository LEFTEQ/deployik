# Deployik — Self-Hosted Deployment Platform

## Summary

Deployik is a self-hosted Vercel alternative running on the lovinka VPS (203.0.113.10). It enables git-push deployment of Next.js applications with automatic domain assignment, SSL, environment management, and blue-green zero-downtime deploys. Built as a single Go binary serving both the API and embedded React SPA, it integrates with the existing infra-repo nginx reverse proxy.

## Decisions

| Question | Decision | Rationale |
|----------|----------|-----------|
| Deploy model | Git-push deploy (manual trigger) | Production deploys should be intentional. Webhook support architectured for later (dev/preview auto-deploy). |
| App types | Next.js only | Focused MVP. Single optimized build pipeline. Can expand to other frameworks later. |
| Build location | On VPS | Self-contained, no external CI dependency. Clone → build → run all on the same machine. |
| Domains | `{app}.preview.example.com` + custom domains | Wildcard DNS for previews, per-domain Let's Encrypt for custom. DNS verification before SSL issuance. |
| Stack (backend) | Go | Low memory (~30MB), single binary, excellent Docker SDK, fast startup. |
| Stack (frontend) | React + Vite + TanStack Router + shadcn/ui | Same proven stack as lovinka-dashboard. Bun as package manager. |
| Database | SQLite (embedded) | Zero-ops, single file, easy backup. Pure Go driver (modernc.org/sqlite). |
| Auth | GitHub OAuth | Allowlist-based. Natural fit since repos are on GitHub. JWT tokens for session. |
| Environments | Preview + Production | Preview auto-assigned subdomain. Production manual-deploy only with custom domain support. |
| Architecture | Monolith Go binary | Single binary, single container. Builds run as goroutines with concurrency semaphore. Evolve to worker process if needed. |
| Nginx management | Dynamic nginx templates | Deployik generates .conf files, writes to conf.d/, reloads nginx. Fits existing infra-repo patterns. |
| Zero-downtime | Blue-green with health check | Start new container → health check → update nginx → stop old. No disruption on deploy. |
| Packaging | Single binary (Go embed) | Go embeds built React SPA. One container, one port. Serves API at /api/*, SPA for everything else. |
| Log streaming | WebSocket | Real-time build log streaming during deployment. Same pattern as lovinka-dashboard. |

## Architecture

```
Internet
  │
  ▼
Nginx Reverse Proxy (existing infra-repo)
  │
  ├── deployik.example.com ──► Deployik (Go binary, port 8080)
  │                              ├── /api/* → REST API
  │                              ├── /ws/*  → WebSocket (build logs)
  │                              └── /*     → React SPA (embedded)
  │
  ├── {app}.preview.example.com ──► App container (managed by Deployik)
  ├── vaclav.cz ──────────────────► App container (custom domain)
  └── ... existing lovinka apps ...
```

### Deploy Flow

1. User clicks "Deploy" → deployment record created (status: queued)
2. Go clones repo (`git clone --depth 1`) using GitHub OAuth token
3. Generates optimized Next.js Dockerfile (multi-stage: deps → build → standalone runtime)
4. `docker build` on VPS, streaming output via WebSocket
5. Blue-green: start new container → health check → update nginx → stop old container
6. Finalize: status → "live", cleanup temp files

### Environment Model

```
Project "vaclav-cz"
├── Preview:    vaclav-cz.preview.example.com  (manual + future webhook)
│               └─ branch: main, commit: abc123
└── Production: vaclav.cz                       (manual only)
                └─ branch: main, commit: def456 (pinned)
```

## Data Model

### users
| Column | Type | Description |
|--------|------|-------------|
| id | TEXT (ULID) | Primary key |
| github_id | INTEGER | GitHub user ID |
| username | TEXT | GitHub username |
| avatar_url | TEXT | GitHub avatar |
| github_token | TEXT (encrypted) | OAuth token for repo access |
| role | TEXT | "admin" or "user" |
| created_at | DATETIME | |

### projects
| Column | Type | Description |
|--------|------|-------------|
| id | TEXT (ULID) | Primary key |
| name | TEXT | URL-safe slug (used in subdomain) |
| github_repo | TEXT | Repository name |
| github_owner | TEXT | Repository owner/org |
| branch | TEXT | Default branch to deploy |
| user_id | TEXT | FK → users |
| framework | TEXT | "nextjs" (extensible) |
| build_command | TEXT | Override (default: "bun run build") |
| install_command | TEXT | Override (default: "bun install") |
| node_version | TEXT | Default: "22" |
| status | TEXT | "active", "paused", "deleted" |
| created_at | DATETIME | |
| updated_at | DATETIME | |

### deployments
| Column | Type | Description |
|--------|------|-------------|
| id | TEXT (ULID) | Primary key |
| project_id | TEXT | FK → projects |
| environment | TEXT | "preview" or "production" |
| commit_sha | TEXT | Git commit hash |
| commit_message | TEXT | First line of commit message |
| branch | TEXT | Branch deployed from |
| status | TEXT | queued/building/deploying/live/failed/rolled_back |
| container_id | TEXT | Docker container ID when running |
| container_name | TEXT | Docker container name |
| image_tag | TEXT | Docker image tag |
| build_duration | INTEGER | Seconds |
| triggered_by | TEXT | FK → users |
| error_message | TEXT | Error details on failure |
| created_at | DATETIME | |
| finished_at | DATETIME | |

### build_logs
| Column | Type | Description |
|--------|------|-------------|
| id | INTEGER | Autoincrement |
| deployment_id | TEXT | FK → deployments |
| line_number | INTEGER | Sequential line number |
| content | TEXT | Log line content |
| stream | TEXT | "stdout" or "stderr" |
| timestamp | DATETIME | |

### domains
| Column | Type | Description |
|--------|------|-------------|
| id | TEXT (ULID) | Primary key |
| project_id | TEXT | FK → projects |
| domain | TEXT | e.g., "vaclav.cz" |
| environment | TEXT | "preview" or "production" |
| is_auto | BOOLEAN | True for auto-assigned preview domains |
| dns_verified | BOOLEAN | DNS A record points to VPS |
| ssl_status | TEXT | "pending", "active", "error" |
| ssl_expires_at | DATETIME | Certificate expiry |
| created_at | DATETIME | |

### env_variables
| Column | Type | Description |
|--------|------|-------------|
| id | TEXT (ULID) | Primary key |
| project_id | TEXT | FK → projects |
| environment | TEXT | "preview" or "production" |
| key | TEXT | Variable name |
| value | TEXT (encrypted) | AES-256-GCM encrypted value |
| created_at | DATETIME | |

## API Endpoints

### Auth
- `GET /api/auth/github` → Redirect to GitHub OAuth
- `GET /api/auth/github/callback` → Handle callback, issue JWT
- `POST /api/auth/refresh` → Refresh JWT
- `GET /api/auth/me` → Current user info

### Projects
- `GET /api/projects` → List projects
- `POST /api/projects` → Create project (connect repo)
- `GET /api/projects/:id` → Project details
- `PATCH /api/projects/:id` → Update settings
- `DELETE /api/projects/:id` → Delete project + stop containers

### Deployments
- `GET /api/projects/:id/deployments` → List deployments
- `POST /api/projects/:id/deployments` → Trigger deploy
- `GET /api/projects/:id/deployments/:did` → Deployment details
- `POST /api/projects/:id/deployments/:did/rollback` → Rollback to this deployment
- `WS /api/projects/:id/deployments/:did/logs` → Stream build logs

### Domains
- `GET /api/projects/:id/domains` → List domains
- `POST /api/projects/:id/domains` → Add custom domain
- `DELETE /api/projects/:id/domains/:did` → Remove domain
- `POST /api/projects/:id/domains/:did/verify` → Verify DNS + trigger SSL

### Environment Variables
- `GET /api/projects/:id/env` → List (values masked)
- `PUT /api/projects/:id/env` → Bulk set
- `DELETE /api/projects/:id/env/:key` → Delete

### System
- `GET /api/system/health` → Health check
- `GET /api/system/stats` → VPS resource usage
- `GET /api/github/repos` → List user's GitHub repos

## Project Structure

```
lovinka-deployik/
├── cmd/server/main.go           # Entry point
├── internal/
│   ├── api/
│   │   ├── router.go            # chi router, middleware
│   │   ├── middleware/           # auth, cors, logging
│   │   └── handlers/            # auth, projects, deployments, domains, envvars, system
│   ├── build/
│   │   ├── pipeline.go          # Clone → build → deploy orchestrator
│   │   ├── dockerfile.go        # Next.js Dockerfile generation
│   │   └── docker.go            # Docker client (github.com/docker/docker)
│   ├── domain/
│   │   ├── nginx.go             # Nginx config generation + reload
│   │   ├── ssl.go               # Certbot/Let's Encrypt trigger
│   │   └── dns.go               # DNS A record verification
│   ├── db/
│   │   ├── sqlite.go            # Init, migrations
│   │   └── models.go            # Go structs + query methods
│   ├── github/
│   │   ├── oauth.go             # OAuth flow
│   │   └── client.go            # Repo listing, clone auth
│   └── ws/
│       └── logs.go              # WebSocket build log streaming
├── web/                         # React frontend
│   ├── src/
│   │   ├── main.tsx             # Entry
│   │   ├── app/app.tsx          # TanStack Router setup
│   │   ├── pages/               # Projects, ProjectDetail, Deployment, NewProject, Domains, EnvVars
│   │   ├── components/
│   │   │   ├── ui/              # shadcn/ui
│   │   │   ├── layout/          # Sidebar, Header
│   │   │   └── BuildLog.tsx     # Terminal-style log viewer
│   │   ├── hooks/               # useWebSocket, useBuildLogs
│   │   ├── lib/api.ts           # API client
│   │   └── store/auth.ts        # Zustand auth store
│   ├── package.json
│   ├── vite.config.ts
│   └── tsconfig.json
├── templates/
│   └── nextjs.Dockerfile.tmpl   # Dockerfile template for deployed Next.js apps
├── docker/
│   ├── Dockerfile               # Deployik's own Dockerfile (multi-stage Go + React)
│   └── docker-compose.yml       # Production config for VPS
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## UI Pages

1. **Projects List** (Dashboard) — All projects as cards with status, last deploy time, quick deploy button
2. **New Project** — Search/select GitHub repo, configure branch, build settings, node version, initial env vars
3. **Project Detail** — Tabs: Deployments | Settings | Domains | Env Vars. Deploy button with branch/commit selection
4. **Deployment View** — Real-time build log terminal (dark monospace), status timeline, commit info, duration, rollback
5. **Domain Management** — Auto-assigned preview URL, add custom domain, DNS verification status, SSL status
6. **Environment Variables** — Key-value editor per environment, masked values, bulk paste, redeploy prompt

## Infrastructure Integration

### Nginx Config Generation

Deployik generates configs at `/opt/nginx-proxy/conf.d/deployik-{project}.conf`:

```nginx
server {
    listen 80;
    server_name {domain};
    location /.well-known/acme-challenge/ { root /var/www/html; }
    location / { return 301 https://$host$request_uri; }
}

server {
    listen 443 ssl;
    http2 on;
    server_name {domain};
    ssl_certificate /etc/nginx/certs/live/{domain}/fullchain.pem;
    ssl_certificate_key /etc/nginx/certs/live/{domain}/privkey.pem;
    
    location / {
        proxy_pass http://deployik-{project}-{env}:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### Wildcard Domain Setup

- DNS: `*.preview.example.com` → `203.0.113.10` (A record)
- SSL: Wildcard certificate via Let's Encrypt DNS-01 challenge (or individual certs per subdomain)
- Nginx: Single server block matching `~^(?<app>.+)\.preview\.lovinka\.com$` with dynamic upstream

### Docker Integration

- Deployik container mounts Docker socket: `/var/run/docker.sock`
- Deployed app containers join the `proxy` network (same as all lovinka apps)
- Container naming: `deployik-{project}-{env}` (used as nginx upstream)
- Deployik also needs write access to `/opt/nginx-proxy/conf.d/` (volume mount)

## Security Considerations

- GitHub OAuth tokens encrypted at rest (AES-256-GCM)
- Environment variables encrypted at rest
- JWT tokens for session management (short-lived access + refresh)
- GitHub allowlist restricts who can log in
- Build isolation: each build runs in a temporary directory, cleaned up after
- No shell access to deployed containers from the UI (out of scope for MVP)
- Rate limiting on API endpoints

## Open Questions

1. **Wildcard SSL strategy**: DNS-01 challenge (requires DNS API access) vs individual certs per preview subdomain (simpler but more Let's Encrypt calls). Need to check if Cloudflare/DNS provider supports API-based DNS challenges.
2. **Build concurrency limit**: Default to 1 concurrent build? 2? Configurable via env var.
3. **Log retention**: How many deployments' worth of build logs to keep? Suggest last 20 per project.
4. **Resource limits**: Should deployed containers have default CPU/memory limits? Configurable per project?
5. **Monitoring**: Should Deployik expose Prometheus metrics for the deployment platform itself?
