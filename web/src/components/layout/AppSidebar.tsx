import { Link, useRouterState } from "@tanstack/react-router";
import {
  BarChart3,
  FolderKanban,
  Globe2,
  LayoutGrid,
  Rocket,
  Settings,
  Sparkles,
} from "lucide-react";

import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from "@/components/ui/sidebar";

interface AppSidebarProps {
  context: "workspace" | "project";
  projectId?: string;
}

interface NavItem {
  label: string;
  icon: typeof FolderKanban;
  to: string;
  params?: Record<string, string>;
  /** Check the current pathname against this pattern for active state. */
  matchPath: (pathname: string) => boolean;
}

function getWorkspaceItems(): NavItem[] {
  return [
    {
      label: "Projects",
      icon: FolderKanban,
      to: "/",
      matchPath: (p) => p === "/",
    },
  ];
}

function getProjectItems(projectId: string): NavItem[] {
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
      label: "Deployments",
      icon: Rocket,
      to: "/projects/$id/deployments",
      params: { id: projectId },
      matchPath: (p) =>
        p === `${base}/deployments` ||
        p.startsWith(`${base}/deployments/`),
    },
    {
      label: "Analytics",
      icon: BarChart3,
      to: "/projects/$id/analytics",
      params: { id: projectId },
      matchPath: (p) => p === `${base}/analytics`,
    },
    {
      label: "Integration",
      icon: Sparkles,
      to: "/projects/$id/integration",
      params: { id: projectId },
      matchPath: (p) => p === `${base}/integration`,
    },
    {
      label: "Domains",
      icon: Globe2,
      to: "/projects/$id/domains",
      params: { id: projectId },
      matchPath: (p) => p === `${base}/domains`,
    },
    {
      label: "Settings",
      icon: Settings,
      to: "/projects/$id/settings",
      params: { id: projectId },
      matchPath: (p) => p === `${base}/settings`,
    },
  ];
}

export function AppSidebar({ context, projectId }: AppSidebarProps) {
  const routerState = useRouterState();
  const pathname = routerState.location.pathname;

  const items =
    context === "project" && projectId
      ? getProjectItems(projectId)
      : getWorkspaceItems();

  const groupLabel = context === "project" ? "Project" : "Navigation";

  return (
    <Sidebar side="left" variant="sidebar" collapsible="icon">
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>{groupLabel}</SidebarGroupLabel>
          <SidebarMenu>
            {items.map((item) => {
              const active = item.matchPath(pathname);
              return (
                <SidebarMenuItem key={item.label}>
                  <SidebarMenuButton
                    asChild
                    isActive={active}
                    tooltip={item.label}
                  >
                    <Link to={item.to} params={item.params ?? {}}>
                      <item.icon />
                      <span>{item.label}</span>
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              );
            })}
          </SidebarMenu>
        </SidebarGroup>
      </SidebarContent>
      <SidebarRail />
    </Sidebar>
  );
}
