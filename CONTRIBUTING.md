# Contributing to Deployik

Thanks for your interest in improving Deployik — a self-hostable, Vercel-style
deployment platform. This guide covers local setup, tests, and the PR process.

## Development Environment

Deployik is a Go 1.25 backend (chi, Docker SDK, SQLite via `modernc.org/sqlite`)
with a React 19 + Vite 7 frontend in `web/`, plus an MCP server in `mcp/`. The
frontend uses [Bun](https://bun.sh).

1. **Clone the repo:**

   ```bash
   git clone https://github.com/lefteq/lovinka-deployik.git
   cd lovinka-deployik
   ```

2. **Create your env file and set the required secrets:**

   ```bash
   cp .env.example .env
   ```

   Set these four required values in `.env`:

   - `JWT_SECRET` — signs JWTs and site-auth cookies (use a strong random value)
   - `ENCRYPTION_KEY` — derives the AES-256-GCM key for secrets at rest
   - `GITHUB_CLIENT_ID` — GitHub OAuth App client ID
   - `GITHUB_CLIENT_SECRET` — GitHub OAuth App client secret

   For local development you can generate the random secrets with
   `openssl rand -hex 32`.

3. **Run the API and the frontend (two terminals):**

   ```bash
   make dev-api   # Go API with DEV_MODE=true
   make dev-web   # Vite dev server on :5173, proxies API to :8080
   ```

4. **(Optional) Seed local data:**

   ```bash
   make dev-seed
   ```

**DEV_MODE shortcuts:** `make dev-api` sets `DEV_MODE=true`, which bypasses
GitHub OAuth. You can authenticate locally via `POST /api/auth/dev-login` and
the app serves mock GitHub repos/branches, so you can develop and run E2E tests
without configuring a real GitHub OAuth App.

## Running the Tests

Please run the relevant suites locally and make sure they pass **before opening
a PR**.

**Backend (Go):**

```bash
go test ./...
```

**Frontend (from `web/`):**

```bash
cd web && bunx tsc --noEmit   # typecheck
cd web && bun run test        # unit tests
cd web && bun run test:e2e    # end-to-end tests
```

## Commit Messages

This repository uses [Conventional Commits](https://www.conventionalcommits.org/).
Format your commit titles as `type(scope): summary`, for example:

- `feat(domains): add wildcard certificate support`
- `fix(build): correct package-manager detection for monorepos`
- `docs(readme): clarify proxy setup`
- `refactor(api): extract authz helpers`
- `test(envvars): cover NEXT_PUBLIC_ rejection`
- `chore(ci): bump Go toolchain`

## Pull Request Process

1. Branch from `main`.
2. Keep changes **surgical** — touch only what the change requires; don't
   refactor or reformat unrelated code.
3. Run the test suites above and make sure they pass.
4. Open the PR **ready for review** (not a draft) with a clear description.
5. Ensure CI is **green** before requesting review. Maintainers may ask for
   changes; please keep the discussion focused on the change at hand.

## Code Style

- **Go:** keep code `gofmt`-clean (`gofmt -w` / `go fmt ./...`). Match existing
  patterns — handlers return JSON errors with appropriate status codes, IDs are
  ULIDs, sensitive values are encrypted at rest, and migrations are append-only.
- **React/TypeScript:** match the existing component and hook idioms in `web/`,
  keep TypeScript strict-mode clean, and reuse the shared helpers and shadcn/ui
  primitives rather than building one-off variants.

By contributing, you agree that your contributions are licensed under the
project's Apache-2.0 license.
