import { Link, useMatchRoute } from "@tanstack/react-router";
import { Building2, FolderKanban, LogOut, Menu, Plus } from "lucide-react";

import { useAuthStore } from "@/store/auth";
import { useOrganizationStore } from "@/store/organization";
import { useOrganizations } from "@/hooks/use-organizations";
import { api } from "@/lib/api";
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
import {
  NavigationMenu,
  NavigationMenuItem,
  NavigationMenuLink,
  NavigationMenuList,
  navigationMenuTriggerStyle,
} from "@/components/ui/navigation-menu";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";

export function SiteHeader() {
  const matchRoute = useMatchRoute();
  const { user, clearAuth } = useAuthStore();
  const {
    organizations,
    selectedOrganization,
    selectedOrganizationId,
    setSelectedOrganizationId,
  } = useOrganizations();
  const navItems = [
    {
      to: "/" as const,
      label: "Projects",
      active: Boolean(matchRoute({ to: "/", fuzzy: true })),
      icon: FolderKanban,
    },
    {
      to: "/new" as const,
      label: "New Project",
      active: Boolean(matchRoute({ to: "/new" })),
      icon: Plus,
    },
  ];

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
    <header className="sticky top-0 z-30 border-b bg-background/85 backdrop-blur supports-[backdrop-filter]:bg-background/70">
      <div className="mx-auto flex h-16 w-full max-w-[1600px] items-center gap-3 px-4 sm:px-6 lg:px-8">
        <div className="flex min-w-0 items-center gap-3">
          <Link to="/" className="flex items-center gap-3">
            <div className="flex size-9 items-center justify-center rounded-xl bg-primary/12 text-primary">
              <FolderKanban className="size-4" />
            </div>
            <p className="font-mono text-[13px] font-semibold tracking-[0.16em]">
              /deployik
            </p>
          </Link>
        </div>

        <NavigationMenu viewport={false} className="hidden lg:flex">
          <NavigationMenuList>
            {navItems.map((item) => (
              <NavigationMenuItem key={item.to}>
                <NavigationMenuLink
                  asChild
                  data-active={item.active}
                  className={cn(
                    navigationMenuTriggerStyle(),
                    item.active && "bg-accent/70 text-foreground",
                  )}
                >
                  <Link to={item.to}>{item.label}</Link>
                </NavigationMenuLink>
              </NavigationMenuItem>
            ))}
          </NavigationMenuList>
        </NavigationMenu>

        <div className="ml-auto flex min-w-0 items-center gap-2">
          <div className="hidden min-w-0 flex-1 lg:flex lg:max-w-sm">
            <CommandPalette />
          </div>

          <div className="hidden xl:block">
            <Select
              value={selectedOrganizationId ?? undefined}
              onValueChange={setSelectedOrganizationId}
            >
              <SelectTrigger
                className="h-9 min-w-[210px]"
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

          <div className="md:hidden">
            <CommandPalette compact />
          </div>

          <Button asChild size="sm" className="hidden sm:inline-flex">
            <Link to="/new">
              <Plus />
              New Project
            </Link>
          </Button>

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="outline"
                size="icon-sm"
                className="shrink-0"
                aria-label="Open menu"
              >
                <Menu className="size-4 lg:hidden" />
                <Avatar className="hidden h-7 w-7 rounded-lg lg:flex">
                  <AvatarImage src={user?.avatar_url} alt={user?.username} />
                  <AvatarFallback className="rounded-lg">
                    {user?.username?.[0]?.toUpperCase() ?? "D"}
                  </AvatarFallback>
                </Avatar>
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-72 rounded-lg">
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
                      {selectedOrganization?.name ?? "Workspace"}
                    </p>
                  </div>
                </div>
              </DropdownMenuLabel>

              <DropdownMenuSeparator className="lg:hidden" />
              <div className="px-1 py-1 lg:hidden">
                {navItems.map((item) => (
                  <DropdownMenuItem key={item.to} asChild>
                    <Link to={item.to}>
                      <item.icon className="size-4" />
                      {item.label}
                    </Link>
                  </DropdownMenuItem>
                ))}
              </div>

              <DropdownMenuSeparator />
              <DropdownMenuLabel>Workspaces</DropdownMenuLabel>
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
      </div>
    </header>
  );
}
