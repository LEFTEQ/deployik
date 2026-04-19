# Sidebar Version Badge Design

## Summary

Surface the running Deployik build in the app sidebar so an operator can see — at a glance — which commit is live and jump straight to either the GitHub commit or the GitHub Actions run that built it. Version metadata is baked into the Go binary at build time via `-ldflags`, exposed through the existing `/api/health` endpoint, and rendered as a dedicated muted row in the sidebar footer above the user/workspace dropdown.

## Decisions

| Question | Decision | Rationale |
|----------|----------|-----------|
| How is version data injected into the binary? | **Build-time `-ldflags` + Docker build args** | Single source of truth, frozen into the binary, no drift between deploy script and container restarts. CI passes `GIT_SHA`, `BUILD_TIME`, `GH_RUN_ID`, `GH_REPO` as Docker build args; Dockerfile forwards them to `go build -ldflags`. |
| How does the SPA fetch it? | **Extend `GET /api/health`** | One fewer endpoint to register/route/test. Public, cacheable, hit once at app boot via React Query (`staleTime: Infinity`). Caveat: keep the response small so external monitors aren't penalized. |
| Where does it appear in the sidebar? | **Dedicated row above user menu, two icon links** | Two distinct affordances visible without a click: `<> abc1234` → commit, `⧉ build` → Actions run. Collapses to a single tooltipped icon in icon-only sidebar mode. |

## Chosen Approach

### Backend

1. **`cmd/server/main.go`** — declare four package-level vars set by ldflags:
   ```go
   var (
       gitSHA    = "dev"
       buildTime = "unknown"
       ghRunID   = ""
       ghRepo    = "lefteq/lovinka-deployik"
   )
   ```
   Pass them into a small `version.Info` struct on startup and inject into the health handler (closure or service field — pick whichever matches existing wiring).

2. **`internal/api/handlers/`** — extend the health handler to return:
   ```json
   {
     "status": "ok",
     "version": {
       "git_sha":    "abc1234",
       "git_sha_full": "abc1234567890fedcba...",
       "build_time": "2026-04-19T10:23:11Z",
       "gh_repo":    "lefteq/lovinka-deployik",
       "gh_run_id":  "12345678",
       "commit_url": "https://github.com/lefteq/lovinka-deployik/commit/abc1234567890...",
       "run_url":    "https://github.com/lefteq/lovinka-deployik/actions/runs/12345678"
     }
   }
   ```
   `commit_url` and `run_url` are **server-built** so the SPA never has to know the URL templates. When `gh_run_id` is empty (local `make dev-api`), omit `run_url` and the SPA hides the build link.

3. **`docker/Dockerfile`** — add build args and forward to `go build`:
   ```dockerfile
   ARG GIT_SHA=dev
   ARG BUILD_TIME=unknown
   ARG GH_RUN_ID=
   ARG GH_REPO=lefteq/lovinka-deployik

   RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w \
     -X main.gitSHA=$GIT_SHA \
     -X main.buildTime=$BUILD_TIME \
     -X main.ghRunID=$GH_RUN_ID \
     -X main.ghRepo=$GH_REPO" \
     -o /deployik ./cmd/server/
   ```
   The `deployik-backup` binary build line is unaffected.

4. **`.github/workflows/ci.yml`** — pass build args from the workflow context:
   ```yaml
   - uses: docker/build-push-action@v6
     with:
       context: .
       file: docker/Dockerfile
       push: true
       tags: ${{ steps.meta.outputs.tags }}
       labels: ${{ steps.meta.outputs.labels }}
       build-args: |
         GIT_SHA=${{ github.sha }}
         BUILD_TIME=${{ github.event.head_commit.timestamp }}
         GH_RUN_ID=${{ github.run_id }}
         GH_REPO=${{ github.repository }}
   ```

### Frontend

1. **`web/src/types/api.ts`** — add `VersionInfo` and extend `HealthResponse`.

2. **`web/src/lib/api.ts`** — add `getHealth(): Promise<HealthResponse>`.

3. **`web/src/lib/queryKeys.ts`** — add `health` query key.

4. **`web/src/components/layout/AppSidebar.tsx`** — between `</SidebarContent>` and the user `DropdownMenu`, add a new `SidebarMenuItem` rendering the version row:
   - `useQuery({ queryKey: queryKeys.health, queryFn: api.getHealth, staleTime: Infinity })`
   - Two `<a target="_blank" rel="noreferrer">` links inside one row
   - Muted text (`text-muted-foreground text-xs`)
   - Wrap in `Tooltip` so icon-collapsed mode shows `v abc1234 · build #12345678`
   - Hide the `build` link entirely when `version.run_url` is missing

### Layout

```
┌─ sidebar footer ─────────────────┐
│  <> abc1234     ⧉ build          │  ← new VersionRow (muted)
│  ──────────────                  │
│  [👤 lukas             ▾]         │  ← existing user/workspace dropdown
└──────────────────────────────────┘

Collapsed (icon-only):
┌─────┐
│ <>  │  tooltip: "v abc1234 · build #12345678"
│ 👤  │
└─────┘
```

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                         BUILD TIME                                │
│                                                                  │
│  GitHub Actions (ci.yml)                                         │
│    │                                                             │
│    │  build-args: GIT_SHA, BUILD_TIME, GH_RUN_ID, GH_REPO        │
│    ▼                                                             │
│  docker/Dockerfile  ── ARG ──▶  go build -ldflags="-X main.X=…"  │
│    │                                                             │
│    ▼                                                             │
│  deployik binary  (gitSHA, buildTime, ghRunID, ghRepo frozen in) │
└──────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────┐
│                         RUN TIME                                  │
│                                                                  │
│  cmd/server/main.go                                              │
│    │  builds version.Info from ldflag vars                       │
│    │  passes to health handler                                   │
│    ▼                                                             │
│  GET /api/health  ──▶  { status, version: {…, commit_url,        │
│                                            run_url} }            │
│                                  ▲                               │
│                                  │ React Query (staleTime ∞)     │
│                                  │                               │
│  AppSidebar.tsx ── VersionRow ───┘                               │
│    │                                                             │
│    ├─▶ <a href={commit_url}>  <> abc1234                         │
│    └─▶ <a href={run_url}>     ⧉ build                            │
└──────────────────────────────────────────────────────────────────┘
```

## Open Questions

- **Commit timestamp source** — `github.event.head_commit.timestamp` is only populated for `push` events. For PR builds it's empty; we'd fall back to build time. Acceptable since PR images aren't deployed to the VPS, but worth a one-line fallback in CI (`BUILD_TIME=${{ github.event.head_commit.timestamp || github.run_started_at }}`).
- **Local dev rendering** — `make dev-api` produces `gitSHA="dev"` and empty `gh_run_id`. The frontend should detect `git_sha === "dev"` and either hide the row or show a `dev build` chip with no links. Decide during implementation; default: hide both links, show `<> dev` with no underline.
- **Cache busting on deploy** — once a user has the SPA loaded, their bundled JS still calls `/api/health` and gets the new version, but the SPA itself is the old bundle. Out of scope here; an explicit "new version available — reload" toast can be added later by polling `/api/health` and comparing against the boot-time SHA.
