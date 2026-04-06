import { Link, useRouterState } from "@tanstack/react-router";
import {
  BarChart3,
  Building2,
  FolderKanban,
  Globe2,
  LayoutGrid,
  LogOut,
  Plus,
  Rocket,
  Settings,
  Sparkles,
} from "lucide-react";

import { useAuthStore } from "@/store/auth";
import { useOrganizationStore } from "@/store/organization";
import { useOrganizations } from "@/hooks/use-organizations";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from "@/components/ui/sidebar";

interface AppSidebarProps extends React.ComponentProps<typeof Sidebar> {
  context: "workspace" | "project";
  projectId?: string;
}

interface NavItem {
  label: string;
  icon: typeof FolderKanban;
  to: string;
  params?: Record<string, string>;
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
        p === `${base}/deployments` || p.startsWith(`${base}/deployments/`),
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

export function AppSidebar({ context, projectId, ...props }: AppSidebarProps) {
  const routerState = useRouterState();
  const pathname = routerState.location.pathname;
  const { user, clearAuth } = useAuthStore();
  const {
    organizations,
    selectedOrganization,
    selectedOrganizationId,
    setSelectedOrganizationId,
  } = useOrganizations();

  const items =
    context === "project" && projectId
      ? getProjectItems(projectId)
      : getWorkspaceItems();

  const groupLabel = context === "project" ? "Project" : "Navigation";

  const handleLogout = async () => {
    try {
      const { api } = await import("@/lib/api");
      await api.logout();
    } finally {
      useOrganizationStore.getState().clearSelection();
      clearAuth();
    }
    window.location.href = "/login";
  };

  return (
    <Sidebar variant="sidebar" collapsible="icon" {...props}>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild>
              <Link to="/">
                <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
                  <FolderKanban className="size-4" />
                </div>
                <div className="flex flex-col gap-0.5 leading-none">
                  <span className="font-semibold">Deployik</span>
                  <span className="text-xs">
                    {selectedOrganization?.name ?? "Workspace"}
                  </span>
                </div>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

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

        {/* Quick action */}
        <SidebarGroup>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton asChild tooltip="New Project">
                <Link to="/new">
                  <Plus />
                  <span>New Project</span>
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <SidebarMenuButton
                  size="lg"
                  className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
                >
                  <Avatar className="h-8 w-8 rounded-lg">
                    <AvatarImage
                      src={user?.avatar_url}
                      alt={user?.username}
                    />
                    <AvatarFallback className="rounded-lg">
                      {user?.username?.[0]?.toUpperCase() ?? "D"}
                    </AvatarFallback>
                  </Avatar>
                  <div className="grid flex-1 text-left text-sm leading-tight">
                    <span className="truncate font-medium">
                      {user?.username}
                    </span>
                    <span className="truncate text-xs text-muted-foreground">
                      {selectedOrganization?.name ?? "Workspace"}
                    </span>
                  </div>
                </SidebarMenuButton>
              </DropdownMenuTrigger>
              <DropdownMenuContent
                className="w-[--radix-dropdown-menu-trigger-width] min-w-56 rounded-lg"
                side="top"
                align="start"
                sideOffset={4}
              >
                <DropdownMenuLabel>Workspaces</DropdownMenuLabel>
                <DropdownMenuRadioGroup
                  value={selectedOrganizationId ?? ""}
                  onValueChange={setSelectedOrganizationId}
                >
                  {organizations.map((org) => (
                    <DropdownMenuRadioItem key={org.id} value={org.id}>
                      <Building2 className="size-4" />
                      {org.name}
                    </DropdownMenuRadioItem>
                  ))}
                </DropdownMenuRadioGroup>
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={handleLogout}>
                  <LogOut className="size-4" />
                  Log Out
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}
