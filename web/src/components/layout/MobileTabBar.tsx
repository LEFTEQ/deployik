import { Link, useRouterState } from "@tanstack/react-router";
import {
  FolderKanban,
  LayoutGrid,
  Menu,
  Plus,
  Rocket,
  Settings,
  Share2,
} from "lucide-react";

import { useSidebar } from "@/components/ui/sidebar";
import { cn } from "@/lib/utils";

interface MobileTabBarProps {
  context: "workspace" | "project" | "app";
  projectId?: string;
  appId?: string;
}

interface TabItem {
  label: string;
  icon: typeof FolderKanban;
  to: string;
  params?: Record<string, string>;
  matchPath: (pathname: string) => boolean;
}

function getTabs(
  context: "workspace" | "project" | "app",
  projectId?: string,
  appId?: string,
): TabItem[] {
  if (context === "project" && projectId) {
    const base = `/projects/${projectId}`;
    return [
      {
        label: "Overview",
        icon: LayoutGrid,
        to: "/projects/$id",
        params: { id: projectId },
        matchPath: (p) => p === base,
      },
      {
        label: "Deploys",
        icon: Rocket,
        to: "/projects/$id/deployments",
        params: { id: projectId },
        matchPath: (p) =>
          p === `${base}/deployments` || p.startsWith(`${base}/deployments/`),
      },
    ];
  }
  if (context === "app" && appId) {
    const base = `/apps/${appId}`;
    return [
      {
        label: "Overview",
        icon: LayoutGrid,
        to: "/apps/$appId",
        params: { appId },
        matchPath: (p) => p === base,
      },
      {
        label: "Deploys",
        icon: Rocket,
        to: "/apps/$appId/deployments",
        params: { appId },
        matchPath: (p) =>
          p === `${base}/deployments` || p.startsWith(`${base}/deployments/`),
      },
      {
        label: "Topology",
        icon: Share2,
        to: "/apps/$appId/topology",
        params: { appId },
        matchPath: (p) => p === `${base}/topology`,
      },
      {
        label: "Settings",
        icon: Settings,
        to: "/apps/$appId/settings",
        params: { appId },
        matchPath: (p) => p.startsWith(`${base}/settings`),
      },
    ];
  }
  return [
    {
      label: "Projects",
      icon: FolderKanban,
      to: "/",
      matchPath: (p) => p === "/",
    },
    {
      label: "New",
      icon: Plus,
      to: "/new",
      matchPath: (p) => p === "/new",
    },
  ];
}

/**
 * Fixed bottom tab bar for phones. The "More" tab opens the sidebar drawer,
 * which stays the single source of full navigation.
 */
export function MobileTabBar({ context, projectId, appId }: MobileTabBarProps) {
  const routerState = useRouterState();
  const pathname = routerState.location.pathname;
  const { setOpenMobile } = useSidebar();
  const tabs = getTabs(context, projectId, appId);

  return (
    <nav
      data-testid="mobile-tab-bar"
      className="fixed inset-x-0 bottom-0 z-40 border-t border-border bg-background/90 pb-safe backdrop-blur supports-[backdrop-filter]:bg-background/75 md:hidden"
    >
      <div className="flex h-14 items-stretch">
        {tabs.map((tab) => {
          const active = tab.matchPath(pathname);
          return (
            <Link
              key={tab.label}
              to={tab.to}
              params={tab.params ?? {}}
              data-testid={`mobile-tab-${tab.label.toLowerCase()}`}
              className={cn(
                "flex min-w-0 flex-1 flex-col items-center justify-center gap-0.5 text-[11px] font-medium transition-colors",
                active ? "text-primary" : "text-muted-foreground",
              )}
            >
              <tab.icon className="size-5" />
              <span className="truncate">{tab.label}</span>
            </Link>
          );
        })}
        <button
          type="button"
          data-testid="mobile-tab-more"
          onClick={() => setOpenMobile(true)}
          className="flex min-w-0 flex-1 flex-col items-center justify-center gap-0.5 text-[11px] font-medium text-muted-foreground transition-colors"
        >
          <Menu className="size-5" />
          <span className="truncate">More</span>
        </button>
      </div>
    </nav>
  );
}
