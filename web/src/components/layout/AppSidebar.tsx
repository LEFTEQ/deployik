import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useRouterState } from "@tanstack/react-router";
import {
  ArrowLeft,
  BarChart3,
  BellRing,
  Building2,
  ChevronRight,
  Cpu,
  Database,
  FolderKanban,
  Globe2,
  KeyRound,
  Languages,
  LayoutGrid,
  LogOut,
  Mail,
  Plus,
  Rocket,
  Settings,
  Shield,
  Sparkles,
  UserPlus,
  Wrench,
} from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { useAuthStore } from "@/store/auth";
import { useGroupStore } from "@/store/group";
import { useOrganizationStore } from "@/store/organization";
import { useOrganizations } from "@/hooks/use-organizations";
import { VersionRow } from "@/components/layout/VersionRow";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
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
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  SidebarRail,
} from "@/components/ui/sidebar";

interface AppSidebarProps extends React.ComponentProps<typeof Sidebar> {
  context: "workspace" | "project";
  projectId?: string;
}

interface NavSubItem {
  label: string;
  icon: typeof FolderKanban;
  to: string;
  params?: Record<string, string>;
  testId?: string;
  matchPath: (pathname: string) => boolean;
}

interface NavItem {
  label: string;
  icon: typeof FolderKanban;
  to: string;
  params?: Record<string, string>;
  matchPath: (pathname: string) => boolean;
  subItems?: NavSubItem[];
}

function getWorkspaceItems(): NavItem[] {
  return [
    {
      label: "Projects",
      icon: FolderKanban,
      to: "/",
      matchPath: (p) => p === "/",
    },
    {
      label: "Access tokens",
      icon: KeyRound,
      to: "/account/tokens",
      matchPath: (p) => p === "/account/tokens",
    },
    {
      label: "Notifications",
      icon: BellRing,
      to: "/account/notifications",
      matchPath: (p) => p === "/account/notifications",
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
      label: "Integrations",
      icon: Sparkles,
      to: "/projects/$id/integrations/analytics",
      params: { id: projectId },
      matchPath: (p) => p.startsWith(`${base}/integrations`),
      subItems: [
        {
          label: "Analytics",
          icon: BarChart3,
          to: "/projects/$id/integrations/analytics",
          params: { id: projectId },
          testId: "sidebar-integrations-analytics",
          matchPath: (p) => p === `${base}/integrations/analytics`,
        },
        {
          label: "Email",
          icon: Mail,
          to: "/projects/$id/integrations/email",
          params: { id: projectId },
          testId: "sidebar-integrations-email",
          matchPath: (p) => p === `${base}/integrations/email`,
        },
        {
          label: "Multi Locale",
          icon: Languages,
          to: "/projects/$id/integrations/multi-locale",
          params: { id: projectId },
          testId: "sidebar-integrations-multi-locale",
          matchPath: (p) => p === `${base}/integrations/multi-locale`,
        },
      ],
    },
    {
      label: "Settings",
      icon: Settings,
      to: "/projects/$id/settings",
      params: { id: projectId },
      matchPath: (p) => p.startsWith(`${base}/settings`),
      subItems: [
        {
          label: "Build",
          icon: Wrench,
          to: "/projects/$id/settings",
          params: { id: projectId },
          matchPath: (p) => p === `${base}/settings`,
        },
        {
          label: "Domains",
          icon: Globe2,
          to: "/projects/$id/settings/domains",
          params: { id: projectId },
          matchPath: (p) => p === `${base}/settings/domains`,
        },
        {
          label: "Environments",
          icon: KeyRound,
          to: "/projects/$id/settings/env",
          params: { id: projectId },
          matchPath: (p) => p === `${base}/settings/env`,
        },
        {
          label: "Services",
          icon: Database,
          to: "/projects/$id/settings/services",
          params: { id: projectId },
          testId: "sidebar-settings-services",
          matchPath: (p) => p === `${base}/settings/services`,
        },
        {
          label: "Protection",
          icon: Shield,
          to: "/projects/$id/settings/protection",
          params: { id: projectId },
          matchPath: (p) => p === `${base}/settings/protection`,
        },
        {
          label: "Resources",
          icon: Cpu,
          to: "/projects/$id/settings/resources",
          params: { id: projectId },
          testId: "sidebar-settings-resources",
          matchPath: (p) => p === `${base}/settings/resources`,
        },
      ],
    },
  ];
}

export function AppSidebar({ context, projectId, ...props }: AppSidebarProps) {
  const routerState = useRouterState();
  const pathname = routerState.location.pathname;
  const { user, clearAuth } = useAuthStore();
  const queryClient = useQueryClient();
  const [invitesOpen, setInvitesOpen] = useState(false);
  const { organizations, selectedOrganizationId, setSelectedOrganizationId } =
    useOrganizations();
  const invitesQuery = useQuery({
    queryKey: queryKeys.myGroupInvites(),
    queryFn: () => api.listMyGroupInvites(),
  });
  const inviteCount = invitesQuery.data?.length ?? 0;

  const acceptInviteMutation = useMutation({
    mutationFn: (inviteId: string) => api.acceptGroupInvite(inviteId),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.myGroupInvites() }),
        queryClient.invalidateQueries({ queryKey: queryKeys.groups() }),
        queryClient.invalidateQueries({ queryKey: queryKeys.organizations() }),
        queryClient.invalidateQueries({ queryKey: ["projects"] }),
      ]);
      setInvitesOpen(false);
    },
  });

  const declineInviteMutation = useMutation({
    mutationFn: (inviteId: string) => api.declineGroupInvite(inviteId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.myGroupInvites(),
      });
    },
  });

  const items =
    context === "project" && projectId
      ? getProjectItems(projectId)
      : getWorkspaceItems();

  const groupLabel = context === "project" ? "Project" : "Navigation";

  const handleLogout = async () => {
    try {
      await api.logout();
    } finally {
      useGroupStore.getState().clearSelection();
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
            <SidebarMenuButton size="lg" asChild tooltip="Deployik">
              <Link to="/">
                <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
                  <FolderKanban className="size-4" />
                </div>
                <div className="flex flex-col gap-0.5 leading-none">
                  <span className="font-semibold">Deployik</span>
                  <span className="text-xs text-muted-foreground">Home</span>
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
              if (item.subItems) {
                return (
                  <Collapsible
                    key={item.label}
                    asChild
                    defaultOpen
                    className="group/collapsible"
                  >
                    <SidebarMenuItem>
                      <CollapsibleTrigger asChild>
                        <SidebarMenuButton
                          isActive={active}
                          tooltip={item.label}
                        >
                          <item.icon />
                          <span>{item.label}</span>
                          <ChevronRight className="ml-auto transition-transform duration-200 group-data-[state=open]/collapsible:rotate-90" />
                        </SidebarMenuButton>
                      </CollapsibleTrigger>
                      <CollapsibleContent>
                        <SidebarMenuSub>
                          {item.subItems.map((sub) => {
                            const subActive = sub.matchPath(pathname);
                            return (
                              <SidebarMenuSubItem key={sub.label}>
                                <SidebarMenuSubButton
                                  asChild
                                  isActive={subActive}
                                >
                                  <Link
                                    to={sub.to}
                                    params={sub.params ?? {}}
                                    data-testid={sub.testId}
                                  >
                                    <sub.icon />
                                    <span>{sub.label}</span>
                                  </Link>
                                </SidebarMenuSubButton>
                              </SidebarMenuSubItem>
                            );
                          })}
                        </SidebarMenuSub>
                      </CollapsibleContent>
                    </SidebarMenuItem>
                  </Collapsible>
                );
              }
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

        {/* Quick actions */}
        <SidebarGroup>
          <SidebarMenu>
            {context === "project" && (
              <SidebarMenuItem>
                <SidebarMenuButton asChild tooltip="All Projects">
                  <Link to="/">
                    <ArrowLeft />
                    <span>All Projects</span>
                  </Link>
                </SidebarMenuButton>
              </SidebarMenuItem>
            )}
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
        <VersionRow />
        <SidebarMenu>
          <SidebarMenuItem>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <SidebarMenuButton
                  size="lg"
                  className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
                >
                  <Avatar className="h-8 w-8 rounded-lg">
                    <AvatarImage src={user?.avatar_url} alt={user?.username} />
                    <AvatarFallback className="rounded-lg">
                      {user?.username?.[0]?.toUpperCase() ?? "D"}
                    </AvatarFallback>
                  </Avatar>
                  <div className="grid flex-1 text-left text-sm leading-tight">
                    <span className="truncate font-medium">
                      {user?.username}
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
                <DropdownMenuLabel>Groups</DropdownMenuLabel>
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
                <DropdownMenuItem
                  onSelect={(event) => {
                    event.preventDefault();
                    setInvitesOpen(true);
                  }}
                >
                  <UserPlus className="size-4" />
                  Group invitations
                  {inviteCount > 0 ? (
                    <Badge variant="outline" className="ml-auto">
                      {inviteCount}
                    </Badge>
                  ) : null}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={handleLogout}>
                  <LogOut className="size-4" />
                  Log Out
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </SidebarMenuItem>
        </SidebarMenu>
        <Dialog open={invitesOpen} onOpenChange={setInvitesOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Group invitations</DialogTitle>
              <DialogDescription>
                Review pending invitations for your GitHub account.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-3">
              {invitesQuery.isLoading ? (
                <div className="py-8 text-center text-sm text-muted-foreground">
                  Loading invitations…
                </div>
              ) : invitesQuery.data?.length ? (
                invitesQuery.data.map((invite) => (
                  <div
                    key={invite.id}
                    className="flex flex-col gap-3 rounded-md border p-3 sm:flex-row sm:items-center"
                  >
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm font-medium">
                        {invite.group_name}
                      </div>
                      <div className="text-xs text-muted-foreground">
                        Invited as {invite.role}
                      </div>
                    </div>
                    <div className="flex gap-2">
                      <Button
                        size="sm"
                        onClick={() => acceptInviteMutation.mutate(invite.id)}
                        disabled={
                          acceptInviteMutation.isPending ||
                          declineInviteMutation.isPending
                        }
                      >
                        Accept
                      </Button>
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => declineInviteMutation.mutate(invite.id)}
                        disabled={
                          acceptInviteMutation.isPending ||
                          declineInviteMutation.isPending
                        }
                      >
                        Decline
                      </Button>
                    </div>
                  </div>
                ))
              ) : (
                <div className="py-8 text-center text-sm text-muted-foreground">
                  No pending invitations.
                </div>
              )}
            </div>
          </DialogContent>
        </Dialog>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}
