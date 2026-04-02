import { Link, useMatchRoute } from "@tanstack/react-router";
import {
  Blocks,
  Building2,
  ChevronsUpDown,
  FolderKanban,
  LogOut,
  Plus,
} from "lucide-react";

import { useAuthStore } from "@/store/auth";
import { useOrganizationStore } from "@/store/organization";
import { useOrganizations } from "@/hooks/use-organizations";
import { api } from "@/lib/api";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  useSidebar,
} from "@/components/ui/sidebar";

const navItems = [
  {
    to: "/" as const,
    label: "Projects",
    icon: FolderKanban,
    matchFuzzy: true,
  },
  {
    to: "/new" as const,
    label: "New Project",
    icon: Plus,
    matchFuzzy: false,
  },
];

export function AppSidebar() {
  const { organizations, selectedOrganizationId, setSelectedOrganizationId } =
    useOrganizations();

  return (
    <Sidebar collapsible="icon" variant="inset" className="border-r-0">
      <SidebarHeader className="gap-3">
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton asChild size="lg" tooltip="Deployik">
              <Link to="/">
                <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-primary/12 text-primary">
                  <Blocks className="size-4" />
                </div>
                <div className="grid flex-1 text-left text-sm leading-tight">
                  <span className="truncate font-mono text-[13px] font-semibold tracking-[0.16em]">
                    /deployik
                  </span>
                  <span className="truncate text-xs text-muted-foreground">
                    Release Workspace
                  </span>
                </div>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>

        <SidebarGroup className="pt-0">
          <SidebarGroupLabel>Workspace</SidebarGroupLabel>
          <SidebarGroupContent>
            <div className="group-data-[collapsible=icon]:hidden">
              <Select
                value={selectedOrganizationId ?? undefined}
                onValueChange={setSelectedOrganizationId}
              >
                <SelectTrigger
                  className="h-9 w-full bg-sidebar-accent/30"
                  aria-label="Select workspace"
                >
                  <SelectValue placeholder="Workspace" />
                </SelectTrigger>
                <SelectContent>
                  {organizations.map((organization) => (
                    <SelectItem key={organization.id} value={organization.id}>
                      {organization.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <SidebarMenu className="hidden group-data-[collapsible=icon]:flex">
              <SidebarMenuItem>
                <SidebarMenuButton tooltip="Workspace">
                  <Building2 />
                  <span>Workspace</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Navigation</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {navItems.map(({ to, label, icon: Icon, matchFuzzy }) => (
                <PrimaryNavItem
                  key={to}
                  to={to}
                  label={label}
                  icon={Icon}
                  matchFuzzy={matchFuzzy}
                />
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter>
        <NavUser />
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}

function PrimaryNavItem({
  to,
  label,
  icon: Icon,
  matchFuzzy,
}: {
  to: "/" | "/new";
  label: string;
  icon: typeof FolderKanban;
  matchFuzzy: boolean;
}) {
  const matchRoute = useMatchRoute();
  const isActive = Boolean(matchRoute({ to, fuzzy: matchFuzzy }));

  return (
    <SidebarMenuItem>
      <SidebarMenuButton asChild isActive={isActive} tooltip={label}>
        <Link to={to}>
          <Icon />
          <span>{label}</span>
        </Link>
      </SidebarMenuButton>
    </SidebarMenuItem>
  );
}

function NavUser() {
  const { user, clearAuth } = useAuthStore();
  const { isMobile } = useSidebar();

  const handleLogout = async () => {
    try {
      await api.logout();
    } finally {
      useOrganizationStore.getState().clearSelection();
      clearAuth();
    }

    window.location.href = "/login";
  };

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton
              size="lg"
              className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
              tooltip={user?.username ?? "Account"}
            >
              <Avatar className="h-8 w-8 rounded-lg">
                <AvatarImage src={user?.avatar_url} alt={user?.username} />
                <AvatarFallback className="rounded-lg">
                  {user?.username?.[0]?.toUpperCase() ?? "D"}
                </AvatarFallback>
              </Avatar>
              <div className="grid flex-1 text-left text-sm leading-tight">
                <span className="truncate font-medium">{user?.username}</span>
                <span className="truncate text-xs text-muted-foreground">
                  {user?.role ?? "member"}
                </span>
              </div>
              <ChevronsUpDown className="ml-auto size-4" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            className="w-(--radix-dropdown-menu-trigger-width) min-w-56 rounded-lg"
            side={isMobile ? "bottom" : "right"}
            align="end"
            sideOffset={4}
          >
            <DropdownMenuLabel className="p-0 font-normal">
              <div className="flex items-center gap-2 px-2 py-2 text-left text-sm">
                <Avatar className="h-8 w-8 rounded-lg">
                  <AvatarImage src={user?.avatar_url} alt={user?.username} />
                  <AvatarFallback className="rounded-lg">
                    {user?.username?.[0]?.toUpperCase() ?? "D"}
                  </AvatarFallback>
                </Avatar>
                <div className="grid flex-1 text-left text-sm leading-tight">
                  <span className="truncate font-medium">{user?.username}</span>
                  <span className="truncate text-xs text-muted-foreground">
                    {user?.role ?? "member"}
                  </span>
                </div>
              </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <Link to="/">
                <FolderKanban />
                Projects
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem asChild>
              <Link to="/new">
                <Plus />
                New Project
              </Link>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={handleLogout}>
              <LogOut />
              Log Out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
