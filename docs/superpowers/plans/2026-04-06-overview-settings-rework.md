# Overview Rework + Settings Restructure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rework the Project Overview page to match Vercel's production-deployment hero layout, and restructure Settings into expandable sub-routes (Build, Domains, Environments).

**Architecture:** Frontend-only changes. The overview replaces 6 oversized stat cards with a compact hero card + info banner + 3 summary cards + active branches. Settings sidebar item becomes expandable with sub-menu items routing to dedicated pages. Domains move from top-level sidebar into Settings sub-route. Env vars/secrets get their own page under Settings.

**Tech Stack:** React 19, TanStack Router, TanStack Query, shadcn/ui (sidebar sub-menu primitives, breadcrumb), Tailwind CSS 4

---

### Task 1: Update Sidebar with Expandable Settings

**Files:**
- Modify: `web/src/components/layout/AppSidebar.tsx`

- [ ] **Step 1: Replace flat nav items with grouped items including Settings sub-menu**

Replace the `getProjectItems` function and the rendering loop. Settings becomes a collapsible parent with 3 sub-items. Remove the standalone Domains item.

```tsx
// In imports, add:
import {
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
} from "@/components/ui/sidebar";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { ChevronRight } from "lucide-react";
```

Replace the `getProjectItems` function:

```tsx
interface NavItem {
  label: string;
  icon: typeof FolderKanban;
  to: string;
  params?: Record<string, string>;
  matchPath: (pathname: string) => boolean;
  subItems?: { label: string; to: string; params?: Record<string, string>; matchPath: (pathname: string) => boolean }[];
}

function getProjectItems(projectId: string): NavItem[] {
  const base = `/projects/${projectId}`;
  const params = { id: projectId };
  return [
    {
      label: "Overview",
      icon: LayoutGrid,
      to: "/projects/$id",
      params,
      matchPath: (p) => p === base,
    },
    {
      label: "Deployments",
      icon: Rocket,
      to: "/projects/$id/deployments",
      params,
      matchPath: (p) => p === `${base}/deployments` || p.startsWith(`${base}/deployments/`),
    },
    {
      label: "Analytics",
      icon: BarChart3,
      to: "/projects/$id/analytics",
      params,
      matchPath: (p) => p === `${base}/analytics`,
    },
    {
      label: "Integration",
      icon: Sparkles,
      to: "/projects/$id/integration",
      params,
      matchPath: (p) => p === `${base}/integration`,
    },
    {
      label: "Settings",
      icon: Settings,
      to: "/projects/$id/settings",
      params,
      matchPath: (p) => p.startsWith(`${base}/settings`),
      subItems: [
        { label: "Build", to: "/projects/$id/settings", params, matchPath: (p) => p === `${base}/settings` },
        { label: "Domains", to: "/projects/$id/settings/domains", params, matchPath: (p) => p === `${base}/settings/domains` },
        { label: "Environments", to: "/projects/$id/settings/env", params, matchPath: (p) => p === `${base}/settings/env` },
      ],
    },
  ];
}
```

Replace the `SidebarMenu` rendering block inside the first `SidebarGroup`:

```tsx
<SidebarMenu>
  {items.map((item) => {
    const active = item.matchPath(pathname);

    if (item.subItems) {
      return (
        <Collapsible key={item.label} defaultOpen={active} className="group/collapsible">
          <SidebarMenuItem>
            <CollapsibleTrigger asChild>
              <SidebarMenuButton isActive={active} tooltip={item.label}>
                <item.icon />
                <span>{item.label}</span>
                <ChevronRight className="ml-auto transition-transform group-data-[state=open]/collapsible:rotate-90" />
              </SidebarMenuButton>
            </CollapsibleTrigger>
            <CollapsibleContent>
              <SidebarMenuSub>
                {item.subItems.map((sub) => (
                  <SidebarMenuSubItem key={sub.label}>
                    <SidebarMenuSubButton asChild isActive={sub.matchPath(pathname)}>
                      <Link to={sub.to} params={sub.params ?? {}}>
                        {sub.label}
                      </Link>
                    </SidebarMenuSubButton>
                  </SidebarMenuSubItem>
                ))}
              </SidebarMenuSub>
            </CollapsibleContent>
          </SidebarMenuItem>
        </Collapsible>
      );
    }

    return (
      <SidebarMenuItem key={item.label}>
        <SidebarMenuButton asChild isActive={active} tooltip={item.label}>
          <Link to={item.to} params={item.params ?? {}}>
            <item.icon />
            <span>{item.label}</span>
          </Link>
        </SidebarMenuButton>
      </SidebarMenuItem>
    );
  })}
</SidebarMenu>
```

- [ ] **Step 2: Add shadcn collapsible component if not present**

Run: `cd web && bunx --bun shadcn@latest add collapsible -y`

If it already exists, skip this step.

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd web && bunx tsc --noEmit`
Expected: zero errors

- [ ] **Step 4: Commit**

```bash
git add web/src/components/layout/AppSidebar.tsx
git commit -m "Add expandable Settings sub-menu to project sidebar"
```

---

### Task 2: Add Settings Sub-Routes and Update Breadcrumb

**Files:**
- Modify: `web/src/app/app.tsx`
- Create: `web/src/pages/ProjectSettingsDomains.tsx`
- Create: `web/src/pages/ProjectSettingsEnv.tsx`
- Modify: `web/src/components/layout/ProjectLayout.tsx`

- [ ] **Step 1: Create ProjectSettingsDomains page**

Create `web/src/pages/ProjectSettingsDomains.tsx`. This is the same content as `ProjectDomains.tsx` but renamed. Copy the entire file and change only the export name:

```tsx
// Copy all contents of ProjectDomains.tsx, rename the export:
export function ProjectSettingsDomains() {
  // ... exact same implementation as ProjectDomains ...
}
```

- [ ] **Step 2: Create ProjectSettingsEnv page**

Create `web/src/pages/ProjectSettingsEnv.tsx`:

```tsx
import { useParams } from "@tanstack/react-router";
import { VariableStore } from "@/components/projects/variable-store";

export function ProjectSettingsEnv() {
  const { id } = useParams({ strict: false }) as { id: string };

  return (
    <div className="space-y-6">
      <VariableStore projectId={id} kind="env" />
      <VariableStore projectId={id} kind="secret" />
    </div>
  );
}
```

- [ ] **Step 3: Update route tree in app.tsx**

Replace the imports and route definitions:

```tsx
// Replace import:
// import { ProjectDomains } from "@/pages/ProjectDomains";
// Add imports:
import { ProjectSettingsDomains } from "@/pages/ProjectSettingsDomains";
import { ProjectSettingsEnv } from "@/pages/ProjectSettingsEnv";

// Remove projectDomainsRoute definition entirely

// Add new settings sub-routes:
const projectSettingsDomainsRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: "/settings/domains",
  component: ProjectSettingsDomains,
});

const projectSettingsEnvRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: "/settings/env",
  component: ProjectSettingsEnv,
});

// Update routeTree - remove projectDomainsRoute, add new sub-routes:
const routeTree = rootRoute.addChildren([
  loginRoute,
  authCallbackRoute,
  protectedRoute.addChildren([
    workspaceLayoutRoute.addChildren([indexRoute]),
    newProjectRoute,
    projectLayoutRoute.addChildren([
      projectOverviewRoute,
      projectDeploymentsRoute,
      deploymentDetailRoute,
      projectAnalyticsRoute,
      projectIntegrationRoute,
      projectSettingsRoute,
      projectSettingsDomainsRoute,
      projectSettingsEnvRoute,
    ]),
  ]),
]);
```

- [ ] **Step 4: Update breadcrumb in ProjectLayout.tsx**

In `ProjectLayout.tsx`, update the `currentPage` derivation to handle settings sub-routes:

```tsx
let currentPage = "Overview";
if (pathname.startsWith(`${base}/deployments/`)) currentPage = "Deployment";
else if (pathname === `${base}/deployments`) currentPage = "Deployments";
else if (pathname === `${base}/analytics`) currentPage = "Analytics";
else if (pathname === `${base}/integration`) currentPage = "Integration";
else if (pathname === `${base}/settings/domains`) currentPage = "Domains";
else if (pathname === `${base}/settings/env`) currentPage = "Environments";
else if (pathname === `${base}/settings`) currentPage = "Build Settings";
```

Note: the settings sub-routes must be checked before the base `/settings` match.

- [ ] **Step 5: Delete old ProjectDomains.tsx**

```bash
rm web/src/pages/ProjectDomains.tsx
```

- [ ] **Step 6: Verify TypeScript compiles**

Run: `cd web && bunx tsc --noEmit`
Expected: zero errors

- [ ] **Step 7: Commit**

```bash
git add web/src/pages/ProjectSettingsDomains.tsx web/src/pages/ProjectSettingsEnv.tsx web/src/app/app.tsx web/src/components/layout/ProjectLayout.tsx
git rm web/src/pages/ProjectDomains.tsx
git commit -m "Add settings sub-routes: Build, Domains, Environments"
```

---

### Task 3: Rewrite ProjectSettings to Remove Env/Secrets

**Files:**
- Modify: `web/src/pages/ProjectSettings.tsx`

- [ ] **Step 1: Remove VariableStore imports and usage**

Remove the two `<VariableStore>` lines from the `ProjectSettings` component. The file should render:

```tsx
return (
  <div className="space-y-6">
    <BuildSettingsSection project={project} />
    <DangerZone
      projectId={id}
      onDeleted={() => {
        queryClient.invalidateQueries({ queryKey: ["projects"] });
        toast.success("Project deleted");
        navigate({ to: "/" });
      }}
    />
  </div>
);
```

Remove the `VariableStore` import line.

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web && bunx tsc --noEmit`
Expected: zero errors

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/ProjectSettings.tsx
git commit -m "Remove env/secrets from settings page (moved to /settings/env)"
```

---

### Task 4: Rewrite ProjectOverview with Vercel Hero Layout

**Files:**
- Rewrite: `web/src/pages/ProjectOverview.tsx`

- [ ] **Step 1: Rewrite the overview page**

Replace the entire content of `ProjectOverview.tsx`. The new layout has 4 sections:

**A) Production Deployment Hero Card** -- Two-column: screenshot/icon on left, metadata on right (deployment ID, domains, status + created date, source branch + commit). Action buttons top-right: Repository, Instant Rollback, Visit. Empty state when no production deployment.

**B) Info Banner** -- "To update your Production Deployment, push to the main branch." with Deployments link button.

**C) Three Compact Summary Cards** -- Domains (count + status, links to settings/domains), Analytics (pageviews or Enable CTA, links to analytics), Auto-Build (enabled/disabled + branch config, links to settings).

**D) Active Branches** -- Search + status filter, list of branches with active preview deployments.

The complete rewrite:

```tsx
import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import {
  ArrowRight,
  BarChart3,
  Building2,
  ChevronRight,
  CircleDot,
  ExternalLink,
  GitBranch,
  GitCommit,
  Globe2,
  Rocket,
  RotateCcw,
  Settings,
} from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import {
  ACTIVE_DEPLOYMENT_STATUSES,
  DEPLOYMENT_STATUS_META,
  ENVIRONMENT_META,
  formatRelativeDate,
  getLatestLiveEnvironmentDeployment,
  getPrimaryEnvironmentUrl,
  isDomainReady,
} from "@/lib/deployment-helpers";
import { formatFrameworkLabel } from "@/components/projects/build-settings";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { LoadingState } from "@/components/ui/spinner";
import { cn } from "@/lib/utils";

export function ProjectOverview() {
  const { id } = useParams({ strict: false }) as { id: string };
  const navigate = useNavigate();

  const { data: project, isLoading } = useQuery({
    queryKey: ["project", id],
    queryFn: () => api.getProject(id),
  });

  const { data: deployments } = useQuery({
    queryKey: ["deployments", id],
    queryFn: () => api.listDeployments(id),
    refetchInterval: (query) => {
      const items = query.state.data ?? [];
      return items.some((d) => ACTIVE_DEPLOYMENT_STATUSES.has(d.status)) ? 3000 : false;
    },
  });

  const { data: domains } = useQuery({
    queryKey: ["domains", id],
    queryFn: () => api.listDomains(id),
  });

  const { data: autoBuild } = useQuery({
    queryKey: ["auto-build", id],
    queryFn: () => api.getAutoBuildConfig(id).catch(() => null),
  });

  const [branchSearch, setBranchSearch] = useState("");

  if (isLoading) {
    return <LoadingState title="Loading project..." description="Fetching project details." className="min-h-[420px]" />;
  }
  if (!project) {
    return <div><p>Project not found</p><Link to="/" className="mt-2 text-sm text-primary hover:underline">Back to projects</Link></div>;
  }

  const productionDeployment = getLatestLiveEnvironmentDeployment(deployments, "production");
  const readyDomainCount = (domains ?? []).filter(isDomainReady).length;
  const pendingDomainCount = (domains ?? []).length - readyDomainCount;
  const productionDomains = (domains ?? []).filter((d) => d.environment === "production" && isDomainReady(d));

  // Active branches: group deployments by branch, show latest per branch
  const activeBranches = useMemo(() => {
    const branchMap = new Map<string, (typeof deployments extends (infer T)[] | undefined ? T : never)>();
    for (const d of deployments ?? []) {
      if (!branchMap.has(d.branch)) branchMap.set(d.branch, d);
    }
    return Array.from(branchMap.values()).filter(
      (d) => !branchSearch || d.branch.toLowerCase().includes(branchSearch.toLowerCase())
    );
  }, [deployments, branchSearch]);

  return (
    <div className="space-y-4">
      {/* A: Production Deployment Hero */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="text-base">Production Deployment</CardTitle>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" asChild>
                <a href={`https://github.com/${project.github_owner}/${project.github_repo}`} target="_blank" rel="noopener noreferrer">
                  <GitBranch className="mr-1.5 h-3.5 w-3.5" /> Repository
                </a>
              </Button>
              {productionDomains[0] && (
                <Button size="sm" asChild>
                  <a href={`https://${productionDomains[0].domain}`} target="_blank" rel="noopener noreferrer">
                    Visit <ExternalLink className="ml-1.5 h-3.5 w-3.5" />
                  </a>
                </Button>
              )}
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {productionDeployment ? (
            <div className="grid gap-6 md:grid-cols-[280px_1fr]">
              {/* Screenshot / framework icon */}
              <div className="flex items-center justify-center rounded-lg border bg-muted/30 p-4 min-h-[180px]">
                <div className="text-center text-muted-foreground">
                  <Rocket className="mx-auto h-8 w-8 mb-2" />
                  <p className="text-xs font-mono">{formatFrameworkLabel(project.framework)}</p>
                </div>
              </div>
              {/* Metadata */}
              <div className="space-y-4 text-sm">
                <div>
                  <p className="text-xs text-muted-foreground">Deployment</p>
                  <p className="font-mono text-foreground">deployik-{productionDeployment.id.slice(0, 8)}</p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Domains</p>
                  <div className="flex flex-wrap gap-2 mt-1">
                    {productionDomains.map((d) => (
                      <a key={d.id} href={`https://${d.domain}`} target="_blank" rel="noopener noreferrer" className="text-foreground hover:underline inline-flex items-center gap-1">
                        {d.domain} <ExternalLink className="h-3 w-3" />
                      </a>
                    ))}
                    {!productionDomains.length && <span className="text-muted-foreground">No production domains</span>}
                  </div>
                </div>
                <div className="flex gap-8">
                  <div>
                    <p className="text-xs text-muted-foreground">Status</p>
                    <p className="flex items-center gap-2 mt-1">
                      <span className={cn("h-2 w-2 rounded-full", DEPLOYMENT_STATUS_META[productionDeployment.status]?.dotClass ?? "bg-slate-500")} />
                      {DEPLOYMENT_STATUS_META[productionDeployment.status]?.label ?? productionDeployment.status}
                    </p>
                  </div>
                  <div>
                    <p className="text-xs text-muted-foreground">Created</p>
                    <p className="mt-1">{formatRelativeDate(productionDeployment.created_at)} by {productionDeployment.triggered_by_username || "system"}</p>
                  </div>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Source</p>
                  <div className="mt-1 space-y-1">
                    <p className="flex items-center gap-1.5"><GitBranch className="h-3.5 w-3.5" /> {productionDeployment.branch}</p>
                    <p className="flex items-center gap-1.5 text-muted-foreground"><GitCommit className="h-3.5 w-3.5" /> {productionDeployment.commit_sha?.slice(0, 7) ?? "pending"} {productionDeployment.commit_message}</p>
                  </div>
                </div>
              </div>
            </div>
          ) : (
            <div className="rounded-lg border border-dashed border-border/70 px-5 py-12 text-center text-sm text-muted-foreground">
              No production deployment yet. Deploy to production to see it here.
            </div>
          )}
        </CardContent>
      </Card>

      {/* B: Info Banner */}
      <div className="flex items-center justify-between rounded-lg border bg-muted/30 px-4 py-3 text-sm">
        <p className="text-muted-foreground">
          To update your Production Deployment, push to the <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">{project.branch}</code> branch.
        </p>
        <Button variant="outline" size="sm" asChild>
          <Link to="/projects/$id/deployments" params={{ id }}>
            Deployments <ArrowRight className="ml-1.5 h-3.5 w-3.5" />
          </Link>
        </Button>
      </div>

      {/* C: Three Compact Summary Cards */}
      <div className="grid gap-4 md:grid-cols-3">
        <button type="button" onClick={() => navigate({ to: "/projects/$id/settings/domains", params: { id } })} className="rounded-lg border bg-card p-4 text-left transition-colors hover:bg-accent">
          <div className="flex items-center justify-between">
            <p className="text-sm font-medium">Domains</p>
            <ChevronRight className="h-4 w-4 text-muted-foreground" />
          </div>
          <p className="mt-2 text-2xl font-semibold">{readyDomainCount}</p>
          <p className="mt-1 text-xs text-muted-foreground">
            {pendingDomainCount > 0 ? `${pendingDomainCount} pending verification` : "All verified"}
          </p>
        </button>

        <button type="button" onClick={() => navigate({ to: "/projects/$id/analytics", params: { id } })} className="rounded-lg border bg-card p-4 text-left transition-colors hover:bg-accent">
          <div className="flex items-center justify-between">
            <p className="text-sm font-medium">Analytics</p>
            <ChevronRight className="h-4 w-4 text-muted-foreground" />
          </div>
          <div className="mt-2 flex items-center gap-2">
            <BarChart3 className="h-5 w-5 text-muted-foreground" />
            <span className="text-sm text-muted-foreground">Track visitors and page views</span>
          </div>
          <p className="mt-1 text-xs text-primary">Enable</p>
        </button>

        <button type="button" onClick={() => navigate({ to: "/projects/$id/settings", params: { id } })} className="rounded-lg border bg-card p-4 text-left transition-colors hover:bg-accent">
          <div className="flex items-center justify-between">
            <p className="text-sm font-medium">Auto-Build</p>
            <ChevronRight className="h-4 w-4 text-muted-foreground" />
          </div>
          <p className="mt-2 text-2xl font-semibold">{autoBuild?.enabled ? "Enabled" : "Disabled"}</p>
          <p className="mt-1 text-xs text-muted-foreground">
            {autoBuild?.enabled ? `${autoBuild.production_branch} → production` : "Configure auto-deploy on push"}
          </p>
        </button>
      </div>

      {/* D: Active Branches */}
      <div>
        <h2 className="text-lg font-semibold mb-3">Active Branches</h2>
        <div className="mb-3">
          <Input placeholder="Search branches..." value={branchSearch} onChange={(e) => setBranchSearch(e.target.value)} className="max-w-sm" />
        </div>
        <div className="space-y-2">
          {activeBranches.length ? activeBranches.map((d) => (
            <button key={d.id} type="button" onClick={() => navigate({ to: "/projects/$id/deployments/$did", params: { id, did: d.id } })}
              className="flex w-full items-center justify-between gap-3 rounded-lg border bg-muted/30 px-4 py-3 text-left transition-colors hover:bg-accent">
              <div className="flex items-center gap-3 min-w-0">
                <GitBranch className="h-4 w-4 shrink-0 text-muted-foreground" />
                <span className="truncate text-sm font-medium">{d.branch}</span>
                <Badge variant="outline" className={ENVIRONMENT_META[d.environment]?.badgeClass}>{ENVIRONMENT_META[d.environment]?.label}</Badge>
                <span className={cn("h-2 w-2 rounded-full shrink-0", DEPLOYMENT_STATUS_META[d.status]?.dotClass ?? "bg-slate-500")} />
              </div>
              <span className="shrink-0 text-xs text-muted-foreground">{formatRelativeDate(d.created_at)}</span>
            </button>
          )) : (
            <div className="rounded-lg border border-dashed border-border/70 px-4 py-8 text-center text-sm text-muted-foreground">
              {branchSearch ? "No branches match your search." : "No deployments yet."}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web && bunx tsc --noEmit`
Expected: zero errors

- [ ] **Step 3: Verify build succeeds**

Run: `cd web && bun run build`
Expected: build completes

- [ ] **Step 4: Commit**

```bash
git add web/src/pages/ProjectOverview.tsx
git commit -m "Rework overview to Vercel hero layout with compact summary cards"
```

---

### Task 5: Final Verification

- [ ] **Step 1: Full TypeScript check**

Run: `cd web && bunx tsc --noEmit`
Expected: zero errors

- [ ] **Step 2: Production build**

Run: `cd web && bun run build`
Expected: build succeeds

- [ ] **Step 3: Test locally with Playwright**

Start dev servers if not running: `make dev-api` + `make dev-web`

Navigate to `http://localhost:5173`, authenticate via dev-login, then verify:
1. Dashboard shows projects
2. Click a project → Overview shows hero card + banner + 3 summary cards + active branches
3. Sidebar Settings expands to show Build / Domains / Environments sub-items
4. Click Build → settings page without env/secrets
5. Click Domains → domain management page
6. Click Environments → env vars + secrets page
7. Breadcrumb updates correctly for each settings sub-page
