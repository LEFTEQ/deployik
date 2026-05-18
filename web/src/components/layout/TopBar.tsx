import { Link, useParams, useRouterState } from "@tanstack/react-router";
import {
  Building2,
  ChevronRight,
  FolderKanban,
  LogOut,
  Plus,
} from "lucide-react";
import { useQuery } from "@tanstack/react-query";

import { useAuthStore } from "@/store/auth";
import { useOrganizationStore } from "@/store/organization";
import { useOrganizations } from "@/hooks/use-organizations";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { useGroupStore } from "@/store/group";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { CommandPalette } from "@/components/layout/CommandPalette";
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
import { Separator } from "@/components/ui/separator";
import { SidebarTrigger } from "@/components/ui/sidebar";

export function TopBar() {
  const { user, clearAuth } = useAuthStore();
  const {
    organizations,
    selectedOrganization,
    selectedOrganizationId,
    setSelectedOrganizationId,
  } = useOrganizations();

  const routerState = useRouterState();
  const pathname = routerState.location.pathname;

  // Detect if we're on a project route
  const params = useParams({ strict: false }) as { id?: string };
  const projectId = params.id;
  const isProjectRoute = pathname.startsWith("/projects/") && projectId;

  // Fetch project name when on a project route
  const { data: project } = useQuery({
    queryKey: queryKeys.project(projectId ?? ""),
    queryFn: () => api.getProject(projectId!),
    enabled: !!isProjectRoute,
  });

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
    <header className="sticky top-0 z-30 flex h-12 items-center border-b bg-background/95 backdrop-blur px-4">
      {/* Left: SidebarTrigger + Logo + breadcrumb */}
      <div className="flex items-center gap-2">
        <SidebarTrigger className="-ml-1" />
        <Separator orientation="vertical" className="mr-1 h-4" />
        <Link to="/" className="flex items-center gap-2 text-foreground">
          <FolderKanban className="h-4 w-4" />
          <span className="hidden font-mono text-[13px] font-semibold tracking-[0.16em] sm:inline">
            /deployik
          </span>
        </Link>

        {/* Breadcrumb segments */}
        {selectedOrganization ? (
          <>
            <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
            <span className="hidden text-sm text-muted-foreground sm:inline">
              {selectedOrganization.name}
            </span>
          </>
        ) : null}

        {isProjectRoute && project ? (
          <>
            <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
            <Link
              to="/projects/$id"
              params={{ id: project.id }}
              className="text-sm font-medium text-foreground hover:underline"
            >
              {project.name}
            </Link>
          </>
        ) : null}
      </div>

      {/* Right: Command palette + Add New + user menu */}
      <div className="ml-auto flex items-center gap-2">
        <div className="hidden lg:block">
          <CommandPalette />
        </div>
        <div className="lg:hidden">
          <CommandPalette compact />
        </div>

        <Button size="sm" asChild className="hidden sm:inline-flex">
          <Link to="/new">
            <Plus className="h-4 w-4" />
            Add New
          </Link>
        </Button>

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              className="shrink-0"
              aria-label="Open user menu"
            >
              <Avatar className="h-7 w-7 rounded-lg">
                <AvatarImage src={user?.avatar_url} alt={user?.username} />
                <AvatarFallback className="rounded-lg">
                  {user?.username?.[0]?.toUpperCase() ?? "D"}
                </AvatarFallback>
              </Avatar>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-64 rounded-lg">
            <DropdownMenuLabel className="p-0 font-normal">
              <div className="flex items-center gap-3 px-2 py-2">
                <Avatar className="h-9 w-9 rounded-lg">
                  <AvatarImage src={user?.avatar_url} alt={user?.username} />
                  <AvatarFallback className="rounded-lg">
                    {user?.username?.[0]?.toUpperCase() ?? "D"}
                  </AvatarFallback>
                </Avatar>
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">
                    {user?.username}
                  </p>
                  <p className="truncate text-xs text-muted-foreground">
                    {selectedOrganization?.name ?? "Group"}
                  </p>
                </div>
              </div>
            </DropdownMenuLabel>

            <DropdownMenuSeparator />
            <DropdownMenuLabel>Groups</DropdownMenuLabel>
            <DropdownMenuRadioGroup
              value={selectedOrganizationId ?? ""}
              onValueChange={setSelectedOrganizationId}
            >
              {organizations.map((organization) => (
                <DropdownMenuRadioItem
                  key={organization.id}
                  value={organization.id}
                >
                  <Building2 className="size-4" />
                  {organization.name}
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
      </div>
    </header>
  );
}
