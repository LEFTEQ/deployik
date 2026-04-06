# Deployik Vercel-Style Redesign

**Date:** 2026-04-06
**Scope:** Full-stack redesign -- frontend UI overhaul + backend webhook auto-build + screenshot capture + deployment filtering

## Context

Deployik currently uses a top-nav shell with command palette and a tabbed project detail page. The user wants to match Vercel's UI/UX: sidebar navigation, project-scoped pages, richer deployment list with filters, domain management page, and auto-build on commit via GitHub webhooks. This is a single comprehensive redesign covering frontend, backend, and infrastructure.

## 1. Navigation & Layout

### Global Layout Structure

```
+-----------------------------------------------------------+
|  [Logo] lefteq's projects > project-name    [Search] [+]  |  ← Top bar (slim)
+--------+--------------------------------------------------+
|        |                                                  |
| Sidebar|  Main Content Area                               |
|        |                                                  |
| Overview|                                                 |
| Deploy |                                                  |
| Analytics|                                                |
| Domains|                                                  |
| Settings|                                                 |
|        |                                                  |
+--------+--------------------------------------------------+
```

**Top bar:**
- Left: Logo + workspace breadcrumb (clickable, navigates to dashboard when clicking workspace name)
- Center/right: Command palette trigger (Cmd+K), user avatar/menu, "Add New..." dropdown button
- Height: ~48px, dark background, sticky

**Left sidebar:**
- Width: ~220px, collapsible on mobile
- Context-aware: shows different items at workspace vs project level
- Active item highlight with left border accent

**Workspace-level sidebar items:**
- Projects (default active at `/`)
- Settings (workspace settings, future)

**Project-level sidebar items:**
- Overview
- Deployments
- Analytics
- Integration
- Domains
- Settings

### Route Structure

```
/                                → Projects dashboard (workspace sidebar)
/new                             → New project wizard (no sidebar)
/projects/:id                   → Project Overview (project sidebar)
/projects/:id/deployments       → Deployments list
/projects/:id/deployments/:did  → Deployment detail (build logs)
/projects/:id/analytics         → Analytics
/projects/:id/integration       → Integration setup
/projects/:id/domains           → Domain management
/projects/:id/settings          → Project settings
```

### Layout Components

**Files to create/modify:**
- `components/layout/Sidebar.tsx` -- New sidebar component (reuse existing shadcn sidebar primitives from `ui/sidebar.tsx`)
- `components/layout/ProjectLayout.tsx` -- New layout wrapper for project-scoped pages with sidebar
- `components/layout/TopBar.tsx` -- Replace current `SiteHeader.tsx` with slimmer breadcrumb-style top bar
- `components/layout/AppLayout.tsx` -- Modify to include sidebar + top bar
- `app/app.tsx` -- Restructure routes from tab-based to page-based

**Sidebar behavior:**
- Desktop: Always visible, collapsible via toggle
- Mobile: Drawer overlay, triggered by hamburger in top bar
- Reuse `use-mobile.ts` hook for breakpoint detection

## 2. Projects Dashboard

**Route:** `/`
**File:** `pages/Projects.tsx` (rewrite)

### Layout

```
+-----------------------------------------------------------+
| [Search Projects...]                    [Filter] [Grid|List] [Add New... v] |
+-----------------------------------------------------------+
|                                                           |
|  +------------------+  +------------------+  +----------+ |
|  | [icon] bioar     |  | [icon] ticket-   |  | ...      | |
|  | centrumbioar.cz  |  | market-resell    |  |          | |
|  |                  |  | ticket-market-   |  |          | |
|  | ◉ LEFTEQ/Bioar  |  | resell.app       |  |          | |
|  |                  |  |                  |  |          | |
|  | ref: next.js upd |  | feat: czech      |  |          | |
|  | 12/14/25 ↗ main  |  | 11/19/25 ↗ main  |  |          | |
|  +------------------+  +------------------+  +----------+ |
|                                                           |
+-----------------------------------------------------------+
```

### Project Card Content
- **Top-left:** Framework icon (Next.js, Vite, Astro, or generic)
- **Top-right:** Quick action buttons (redeploy icon, three-dot menu)
- **Title:** Project name (bold)
- **Subtitle:** Primary domain (muted text, clickable external link)
- **Badge:** GitHub repo in a chip (`owner/repo`)
- **Footer:** Latest commit message + relative date + branch with git-branch icon

### Search & View
- Full-width search input with filter icon
- Grid/list view toggle (grid default)
- List view: compact rows instead of cards

### Data
- Uses existing `listProjects()` API
- Needs latest deployment info per project -- either:
  - Extend `listProjects` API response to include latest deployment (commit, status, date)
  - Or fetch deployments separately (N+1, avoid this)
- **Backend change:** Add `latest_deployment` join to projects list query

## 3. Project Overview

**Route:** `/projects/:id`
**File:** `pages/ProjectOverview.tsx` (new, replaces overview tab in ProjectDetail)

### Production Deployment Hero

```
+-----------------------------------------------------------+
| Production Deployment              [Repository] [Rollback] [Visit v] |
+-----------------------------------------------------------+
|                          |                                |
|  +--------------------+  | Deployment                     |
|  |                    |  | deployik-abc123...              |
|  |  [Screenshot of    |  |                                |
|  |   deployed site]   |  | Domains ⊕                      |
|  |                    |  | myapp.example.com ↗             |
|  |                    |  |                                |
|  +--------------------+  | Status    Created              |
|                          | ● Ready   04/06/26 by user 👤  |
|                          |                                |
|                          | Source                          |
|                          | ↗ main                         |
|                          | ⊙ abc1234 feat: latest change  |
+-----------------------------------------------------------+
```

**Content:**
- Left column: Screenshot thumbnail (from screenshot capture system)
- Right column: Deployment metadata
  - Deployment URL/ID
  - Domains list with external link icons and "+" to add
  - Status dot + label + creation date + deployer
  - Source: branch (git icon) + commit SHA (short) + commit message

**Action buttons (top-right):**
- Repository: links to GitHub repo
- Instant Rollback: triggers rollback to previous live deployment
- Visit: opens primary domain, dropdown for other domains

### Info Banner
- "To update your Production Deployment, push to the main branch." (when auto-build enabled)
- Deployments button linking to full list

### Summary Cards Row (3 cards)

```
+-------------------+-------------------+-------------------+
| Analytics    >    | Domains      >    | Auto-Build   >    |
|                   |                   |                   |
| Pageviews: 1.2k  | 3 domains         | ● Enabled         |
| Visitors: 450    | All verified ✓    | main → production |
|                   |                   | * → preview       |
+-------------------+-------------------+-------------------+
```

Each card is clickable, navigating to the corresponding sidebar page.

### Active Branches Section

```
+-----------------------------------------------------------+
| Active Branches                                           |
+-----------------------------------------------------------+
| [Search...]                              [Status filter]  |
+-----------------------------------------------------------+
| ↗ feature/new-auth    ● Preview  ● Ready   user  04/05  ⋯|
| ↗ fix/header-bug      ● Preview  ● Building user  04/05  ⋯|
+-----------------------------------------------------------+
```

- Lists branches with active preview deployments
- Shows environment badge, status, deployer, date
- Click to see deployment detail
- Search + status filter
- **Backend:** New API endpoint or extend deployments list to group by branch

## 4. Deployments List

**Route:** `/projects/:id/deployments`
**File:** `pages/ProjectDeployments.tsx` (new)

### Filter Bar

```
+-----------------------------------------------------------+
| [All Branches v] [All Authors v] [All Environments v] [Date Range] [Status ●●● 5/6 v] |
+-----------------------------------------------------------+
```

**Filters:**
- Branch: dropdown with all branches that have deployments
- Author: dropdown with all users who triggered deployments
- Environment: All / Preview / Production
- Date range: date picker (from/to)
- Status: multi-select chips with colored dots (Ready, Error, Building, Queued, Cancelled)

### Deployment Rows

```
+-----------------------------------------------------------+
| 6t3Qi82y          ● Ready    ↗ main                        |
| Production ⊙ Cur  56s       ⊙ d26537f ref: next.js update |
|                                          04/05/26 LEFTEQ 👤 ⋯ |
+-----------------------------------------------------------+
| EWiu6enw          ● Ready    ↗ feature/new-auth             |
| Preview ⊙         1m 16s    ⊙ 3bc715d Fix React Server... |
|                                          04/05/26 vercel 👤 ⋯ |
+-----------------------------------------------------------+
```

**Row content:**
- Deployment short ID (first 8 chars of ULID)
- Environment badge + "Current" tag for live deployment
- Status dot + label (Ready/Error/Building/Queued)
- Build duration (clock icon)
- Branch with git icon
- Commit SHA (7 chars) + commit message
- Relative date + author username + avatar
- Three-dot menu: View logs, Redeploy, Promote to production, Rollback

### Backend Changes

**New migration (`008_deployment_enhancements.sql`):**
```sql
ALTER TABLE deployments ADD COLUMN trigger_source TEXT NOT NULL DEFAULT 'manual'
    CHECK (trigger_source IN ('manual', 'webhook', 'api'));
ALTER TABLE deployments ADD COLUMN triggered_by_username TEXT NOT NULL DEFAULT '';
```

**API changes to `GET /api/projects/{id}/deployments`:**
- Add query params: `branch`, `environment`, `status`, `triggered_by`, `from`, `to`, `limit`, `offset`
- Return total count for pagination
- Include deployer username in response

**Backend query changes (`queries_deployments.go`):**
- Build dynamic WHERE clause from filter params
- Join with users table to get username + avatar for `triggered_by`
- Add pagination (LIMIT/OFFSET)

## 5. Domains Page

**Route:** `/projects/:id/domains`
**File:** `pages/ProjectDomains.tsx` (new, replaces domains section in settings tab)

### Layout

```
+-----------------------------------------------------------+
| [Search any domain...]                    [Add Domain]    |
+-----------------------------------------------------------+
|                                                           |
| ✓ www.myapp.com              → 308 → myapp.com           |
|   Valid Configuration          ⊙ Production    [Refresh] [Edit] |
|                                                           |
| ✓ myapp.com                                               |
|   Valid Configuration          ⊙ Production    [Refresh] [Edit] |
|                                                           |
| ✓ myapp.preview.example.com                                |
|   Valid Configuration          ⊙ Preview       [Refresh]  |
|   (auto-generated)                                        |
+-----------------------------------------------------------+
```

### Domain Entry (collapsed)
- Status icon (checkmark for valid, warning for pending)
- Domain name (bold) + redirect indicator if applicable
- "Valid Configuration" / "Pending DNS" / "SSL Error" status text
- Environment badge
- Action buttons: Refresh (re-verify DNS+SSL), Edit

### Domain Entry (expanded/edit mode)
- Domain input field
- Environment selector (radio: Preview / Production)
- DNS instructions text with VPS IP (show the VPS_HOST IP for A-record setup)
- Remove button (destructive), Cancel, Save
- **Note:** Domain redirect config (301/302/307/308 to another domain) is a future feature. The current DB schema and nginx generation don't support redirects. For now, just show domain status and environment assignment.

### Backend
- No new API endpoints needed -- existing domain CRUD is sufficient
- The Domains page is a UI restructure only (moving from settings tab to its own page)

## 6. Settings Page

**Route:** `/projects/:id/settings`
**File:** `pages/ProjectSettings.tsx` (new, consolidates settings tab content)

### Sections

**General:**
- Project name (read-only display)
- GitHub repository link
- Default branch

**Build & Development:**
- Reuse existing `BuildSettingsFields` component
- Framework, package manager, install/build commands, output dir, root dir, node version

**Auto-Build (new):**
- Enable/disable toggle
- Production branch (e.g., `main`) -- pushes to this branch auto-deploy to production
- Preview branches: pattern or "all other branches"
- Status indicator showing webhook health

**Environment Variables:**
- Reuse existing env var management UI
- Scope tabs: shared / preview / production
- Bulk editor with key-value pairs

**Secrets:**
- Same as env vars but for secrets
- Masked values

**Danger Zone:**
- Delete project (with confirmation dialog)

## 7. Auto-Build System (Backend)

### Database Schema

**New migration (`009_auto_build.sql`):**

```sql
CREATE TABLE IF NOT EXISTS auto_build_configs (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL UNIQUE REFERENCES projects(id) ON DELETE CASCADE,
    enabled INTEGER NOT NULL DEFAULT 0,
    production_branch TEXT NOT NULL DEFAULT 'main',
    preview_branches TEXT NOT NULL DEFAULT '*',  -- '*' = all, or comma-separated list
    webhook_id INTEGER,                          -- GitHub's webhook ID
    webhook_secret TEXT NOT NULL,                -- Encrypted with AES-256-GCM
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS webhook_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    github_delivery_id TEXT NOT NULL UNIQUE,
    event_type TEXT NOT NULL,
    branch TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    commit_message TEXT NOT NULL DEFAULT '',
    pusher TEXT NOT NULL DEFAULT '',
    deployment_id TEXT REFERENCES deployments(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'received' CHECK (status IN ('received', 'processed', 'ignored', 'failed')),
    error_message TEXT,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
```

### OAuth Scope Update

**File:** `internal/github/oauth.go`

Change OAuth scopes from `repo,read:user` to `repo,read:user,admin:repo_hook`.

**User re-auth:** Existing users whose tokens lack the new scope will need to re-authenticate. The app should:
1. Attempt webhook creation
2. If it fails with 403/404 (insufficient scope), show a banner: "Re-authorize with GitHub to enable auto-build"
3. Re-auth flow: redirect to GitHub OAuth with updated scopes

### GitHub Client Extensions

**File:** `internal/github/client.go`

New methods:
```go
func (c *Client) CreateWebhook(owner, repo, webhookURL, secret string) (int64, error)
func (c *Client) DeleteWebhook(owner, repo string, webhookID int64) error
func (c *Client) PingWebhook(owner, repo string, webhookID int64) error
```

Webhook config sent to GitHub:
- URL: `https://deployik.example.com/api/webhooks/github`
- Content type: `application/json`
- Secret: randomly generated per project, encrypted at rest
- Events: `["push"]`
- Active: true/false based on config

### Webhook Receiver

**New file:** `internal/api/handlers/webhooks.go`
**New route:** `POST /api/webhooks/github` (public, no JWT auth)

Flow:
1. Read `X-Hub-Signature-256` header
2. Read `X-GitHub-Delivery` header (idempotency key)
3. Read raw body
4. Check idempotency: if delivery ID exists in `webhook_events`, return 200 (already processed)
5. Parse JSON to determine repo (from `repository.full_name` → `owner/repo`)
6. Look up projects by `github_owner` + `github_repo` fields in `projects` table, then check if each has an active `auto_build_configs` entry
7. For each matching config:
   a. Validate HMAC-SHA256 signature with stored secret
   b. Extract branch from `ref` field (strip `refs/heads/` prefix)
   c. Determine environment: if branch matches `production_branch` → production, else check `preview_branches` pattern → preview
   d. If no match, log as `ignored`
   e. Create deployment with `trigger_source=webhook`, `triggered_by` = project owner's user ID, `triggered_by_username` = pusher from webhook payload
8. Return 200

**Rate limiting:** Separate rate limit for webhook endpoint (e.g., 60/min per IP)

### Auto-Build API

**New routes:**
```
GET  /api/projects/{id}/auto-build  → Get auto-build config
PUT  /api/projects/{id}/auto-build  → Create/update config (registers/updates GitHub webhook)
DELETE /api/projects/{id}/auto-build → Disable + unregister webhook
```

**Handler file:** `internal/api/handlers/autobuild.go`

**PUT flow:**
1. Validate request (production_branch required, preview_branches pattern)
2. Decrypt project owner's GitHub token
3. If no existing webhook: create via GitHub API, store webhook_id
4. If existing: update active state via GitHub API
5. Upsert `auto_build_configs` row
6. Return config

## 8. Screenshot Capture

### Approach
After deployment reaches `live` status, capture a screenshot asynchronously.

### Implementation

**New migration (part of `008_deployment_enhancements.sql`):**
```sql
ALTER TABLE deployments ADD COLUMN screenshot_path TEXT;
```

**Screenshot service (`internal/build/screenshot.go`):**
```go
func CaptureScreenshot(ctx context.Context, dockerClient *client.Client, url, outputPath string) error
```

**Chosen approach:** Use Docker SDK (already available in the build pipeline) to run a Chromium container. This avoids installing a browser in the Deployik image and reuses the same Docker SDK patterns from `internal/build/docker.go`.

- Run `chromedp/headless-shell` container with a mounted volume
- Execute a small inline script via Chrome DevTools Protocol to navigate + screenshot
- Or simpler: use a purpose-built screenshot container image (e.g., `zenika/alpine-chrome` with `--screenshot` flag)
- Container is ephemeral (`--rm`), runs on the same Docker network as deployed apps
- Save to persistent path: `/data/screenshots/{deployment_id}.png` (same volume as SQLite data)

**Pipeline integration (`internal/build/pipeline.go`):**
- After health check succeeds and deployment is marked `live`:
  ```go
  go func() {
      // Wait a few seconds for the app to fully initialize
      time.Sleep(5 * time.Second)
      path, err := CaptureScreenshot(primaryDomain, screenshotDir)
      if err != nil {
          log.Printf("screenshot capture failed: %v", err)
          return
      }
      db.UpdateDeploymentScreenshot(deployment.ID, path)
  }()
  ```
- Fire-and-forget: deployment status is not affected by screenshot failure

**API endpoint:**
- `GET /api/deployments/{did}/screenshot` -- Serves the screenshot image file
- Returns 404 if no screenshot captured yet
- Content-Type: `image/png`

**Frontend usage:**
- Project overview shows screenshot in hero card
- Falls back to framework icon placeholder if no screenshot available

## 9. Existing Features Migration

### Analytics (`pages/ProjectAnalytics.tsx`)
- Extract from `ProjectDetail.tsx` analytics tab into standalone page
- Same component content, just wrapped in project layout instead of tab panel
- Route: `/projects/:id/analytics`

### Integration (`pages/ProjectIntegration.tsx`)
- Extract from `ProjectDetail.tsx` integration tab into standalone page
- Same component content
- Route: `/projects/:id/integration`

### Command Palette
- Keep `CommandPalette.tsx` but update navigation targets to new routes
- Remove tab references, use direct route navigation

## 10. Files to Create

| File | Purpose |
|------|---------|
| `components/layout/Sidebar.tsx` | Sidebar navigation component |
| `components/layout/ProjectLayout.tsx` | Layout wrapper with sidebar for project pages |
| `components/layout/TopBar.tsx` | Slim breadcrumb top bar |
| `pages/ProjectOverview.tsx` | Project overview (hero + summary cards + active branches) |
| `pages/ProjectDeployments.tsx` | Deployments list with filters |
| `pages/ProjectDomains.tsx` | Domain management page |
| `pages/ProjectSettings.tsx` | Consolidated settings page |
| `pages/ProjectAnalytics.tsx` | Analytics (extracted from tab) |
| `pages/ProjectIntegration.tsx` | Integration (extracted from tab) |
| `internal/api/handlers/autobuild.go` | Auto-build config CRUD handlers |
| `internal/api/handlers/webhooks.go` | GitHub webhook receiver |
| `internal/api/handlers/screenshots.go` | Screenshot serving handler |
| `internal/build/screenshot.go` | Screenshot capture service |
| `internal/db/queries_autobuild.go` | Auto-build config DB queries |
| `internal/db/queries_webhook_events.go` | Webhook events DB queries |
| `internal/db/migrations/008_deployment_enhancements.sql` | trigger_source, triggered_by_username, screenshot_path |
| `internal/db/migrations/009_auto_build.sql` | auto_build_configs + webhook_events tables |

## 11. Files to Modify

| File | Changes |
|------|---------|
| `app/app.tsx` | New route tree with project sub-routes |
| `components/layout/AppLayout.tsx` | Add sidebar, restructure layout |
| `components/layout/SiteHeader.tsx` | Replace with TopBar (or rename) |
| `components/layout/CommandPalette.tsx` | Update navigation targets |
| `pages/Projects.tsx` | Redesign cards to Vercel style |
| `pages/ProjectDetail.tsx` | Remove (split into separate pages) |
| `pages/DeploymentDetail.tsx` | Update layout for sidebar nav |
| `internal/api/router.go` | Add webhook, auto-build, screenshot routes |
| `internal/api/handlers/deployments.go` | Add filter params, include username, refactor trigger logic |
| `internal/github/oauth.go` | Add admin:repo_hook scope |
| `internal/github/client.go` | Add webhook CRUD methods |
| `internal/build/pipeline.go` | Add screenshot capture step |
| `internal/db/models.go` | Add AutoBuildConfig, WebhookEvent models; extend Deployment |
| `internal/db/queries_deployments.go` | Add filter support, pagination, username join |
| `internal/db/queries_projects.go` | Add latest deployment join to list query |
| `types/api.ts` | Add AutoBuildConfig, WebhookEvent types; extend Deployment |
| `lib/api.ts` | Add auto-build and screenshot API methods |

## 12. Files to Delete

| File | Reason |
|------|--------|
| `pages/ProjectDetail.tsx` | Replaced by separate project pages |

## 13. Testing Strategy

### Backend Tests
- Webhook signature validation (valid, invalid, missing)
- Auto-build config CRUD (create, update, delete, get)
- Webhook event processing (push to production branch, push to preview branch, push to non-matching branch, duplicate delivery)
- Deployment filter queries (each filter individually, combined filters, pagination)
- Screenshot path storage and retrieval
- Migration idempotency

### Frontend Verification
- Navigate all new routes
- Sidebar active state highlighting
- Project cards display correctly with latest deployment data
- Deployment filters work (each filter, clear filters)
- Domain management (add, verify, edit display)
- Settings page sections all render
- Auto-build toggle and config
- Screenshot display in overview (with and without screenshot)
- Mobile responsive (sidebar drawer, card layout collapse)
- Command palette navigates to new routes

### Integration
- Full auto-build flow: enable → push to GitHub → webhook received → deployment triggered → screenshot captured
- OAuth re-auth flow for users missing webhook scope
