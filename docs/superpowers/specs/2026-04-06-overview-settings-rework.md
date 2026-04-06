# Overview Rework + Settings Restructure

**Date:** 2026-04-06
**Scope:** Frontend-only -- rework Project Overview to match Vercel hero layout, restructure Settings into sub-routes (Build, Domains, Environments)

## Context

The current Project Overview page has 6 large stat cards (Preview Health, Production Health, Latest Release, Active Domains, Traffic, Analytics Status) that feel oversized and don't prioritize key information. The Vercel design uses a compact Production Deployment hero card with metadata, a banner, and 3 small summary cards -- much more information-dense.

Additionally, Settings currently houses build config, env vars, secrets, and danger zone in one page. Domains is a separate top-level sidebar item. The user wants to restructure: Settings becomes expandable with sub-routes (Build, Domains, Environments).

## 1. Sidebar Navigation

Updated project sidebar items:
- Overview
- Deployments
- Analytics
- Integration
- Settings (expandable with sub-items using `SidebarMenuSub`)
  - Build (default, `/projects/:id/settings`)
  - Domains (`/projects/:id/settings/domains`)
  - Environments (`/projects/:id/settings/env`)

Uses `SidebarMenuSub` / `SidebarMenuSubButton` / `SidebarMenuSubItem` from `components/ui/sidebar.tsx`.

## 2. Routes

```
/projects/:id                    → Overview (reworked)
/projects/:id/deployments        → Deployments list
/projects/:id/deployments/:did   → Deployment detail
/projects/:id/analytics          → Analytics
/projects/:id/integration        → Integration
/projects/:id/settings           → Build Settings (default settings page)
/projects/:id/settings/domains   → Domain Management
/projects/:id/settings/env       → Environment Variables & Secrets
```

## 3. Overview Page Rework

### Production Deployment Hero Card

Two-column layout inside a card:
- **Left column:** Screenshot thumbnail (`api.getDeploymentScreenshotUrl(deployment.id)`) with framework icon fallback
- **Right column:**
  - "Deployment" label + short deployment ID
  - "Domains" label with `+` icon + list of domains as external links
  - "Status" + "Created" on same line: status dot + label + relative date + "by username" with avatar
  - "Source" label: branch with git icon + commit SHA (7 chars) + commit message

**Top-right action buttons:** Repository (external link), Instant Rollback (button), Visit (dropdown with domain options)

If no production deployment exists, show an empty state card with "No production deployment yet. Deploy to production to see it here."

### Info Banner

Below the hero card:
- Collapsible "Deployment Settings" section (default collapsed)
- Text: "To update your Production Deployment, push to the main branch."
- Right-aligned: "Deployments" button linking to deployments page

### Three Compact Summary Cards

Three equal-width cards in a row (~120px height each), clickable:

1. **Domains** → navigates to `/projects/:id/settings/domains`
   - Shows domain count + "All verified" or "X pending"
   - Arrow indicator on right

2. **Analytics** → navigates to `/projects/:id/analytics`
   - If analytics configured: shows pageviews/visitors summary
   - If not: "Track visitors and page views" + "Enable" CTA

3. **Auto-Build** → navigates to `/projects/:id/settings` (build settings)
   - Shows enabled/disabled status
   - If enabled: branch config (e.g., "main → production")

### Active Branches Section

Below the summary cards:
- "Active Branches" heading
- Search input + status filter dropdown
- List of branches with active preview deployments
- Each row: branch name, environment badge, status dot, deployer, date, three-dot menu

## 4. Build Settings Page

**Route:** `/projects/:id/settings`
**File:** `pages/ProjectSettings.tsx` (rewrite)

Sections:
- **General:** Project name, GitHub repo link, default branch (read-only)
- **Build & Development:** Reuses `BuildSettingsFields` component
- **Auto-Build:** Switch toggle, production branch, preview branches, save button
- **Danger Zone:** Delete project with confirmation dialog

Does NOT include env vars, secrets, or domains (moved to sub-routes).

## 5. Domains Settings Page

**Route:** `/projects/:id/settings/domains`
**File:** `pages/ProjectSettingsDomains.tsx` (new, content moved from `ProjectDomains.tsx`)

Same content as current `ProjectDomains.tsx`: search, add domain, domain list with verify/edit/delete.

## 6. Environments Settings Page

**Route:** `/projects/:id/settings/env`
**File:** `pages/ProjectSettingsEnv.tsx` (new)

Two stacked sections:
- **Environment Variables** heading + `VariableStore` component with `kind="env"`
- **Secrets** heading + `VariableStore` component with `kind="secret"`

## 7. Files

### Create
| File | Purpose |
|------|---------|
| `pages/ProjectSettingsDomains.tsx` | Domain management (moved from ProjectDomains) |
| `pages/ProjectSettingsEnv.tsx` | Env vars + secrets page |

### Rewrite
| File | Changes |
|------|---------|
| `pages/ProjectOverview.tsx` | Hero card + banner + 3 compact cards + active branches |
| `pages/ProjectSettings.tsx` | Build settings + auto-build + danger zone only (remove env/secrets) |
| `components/layout/AppSidebar.tsx` | Settings with expandable sub-items |

### Update
| File | Changes |
|------|---------|
| `app/app.tsx` | Add settings sub-routes, remove `/projects/:id/domains` |
| `components/layout/ProjectLayout.tsx` | Update breadcrumb for settings sub-pages |

### Delete
| File | Reason |
|------|--------|
| `pages/ProjectDomains.tsx` | Replaced by ProjectSettingsDomains |

## 8. Verification

- `cd web && bunx tsc --noEmit` -- zero errors
- `bun run build` -- succeeds
- Navigate all routes locally via Playwright:
  - `/` → dashboard with projects
  - `/projects/:id` → overview with hero card
  - `/projects/:id/settings` → build settings
  - `/projects/:id/settings/domains` → domain management
  - `/projects/:id/settings/env` → env vars + secrets
- Sidebar Settings item expands to show Build/Domains/Environments sub-items
- Breadcrumb updates correctly for settings sub-pages
- Summary cards navigate to correct pages when clicked
