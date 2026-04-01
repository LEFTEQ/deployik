import { Link, useMatchRoute } from "@tanstack/react-router";
import { Building2, LayoutDashboard, Plus, LogOut } from "lucide-react";
import { useAuthStore } from "@/store/auth";
import { useOrganizationStore } from "@/store/organization";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { useOrganizations } from "@/hooks/use-organizations";
import { cn } from "@/lib/utils";
import { api } from "@/lib/api";

const navItems = [
  { to: "/" as const, label: "Projects", icon: LayoutDashboard },
  { to: "/new" as const, label: "New Project", icon: Plus },
];

export function Sidebar() {
  const { user, clearAuth } = useAuthStore();
  const {
    organizations,
    selectedOrganizationId,
    selectedOrganization,
    setSelectedOrganizationId,
    isLoading: organizationsLoading,
  } = useOrganizations();
  const matchRoute = useMatchRoute();

  return (
    <aside className="hidden h-screen w-72 flex-col border-r border-white/6 bg-sidebar-background/80 backdrop-blur-2xl md:flex">
      {/* Logo */}
      <div className="flex h-16 items-center gap-3 px-5">
        <div className="flex h-10 w-10 items-center justify-center rounded-2xl bg-[linear-gradient(135deg,rgba(87,123,255,0.95),rgba(56,189,248,0.88))] text-sm font-bold text-primary-foreground shadow-[0_14px_34px_-18px_rgba(59,130,246,0.95)]">
          D
        </div>
        <div>
          <p className="text-lg font-semibold tracking-tight">Deployik</p>
          <p className="text-xs text-muted-foreground">Dark control plane</p>
        </div>
      </div>

      <Separator />

      {/* Nav */}
      <nav className="flex-1 space-y-1.5 p-3">
        <div className="mb-4 rounded-2xl border border-white/8 bg-black/10 p-3">
          <div className="mb-3 flex items-center gap-2 text-xs font-medium uppercase tracking-[0.22em] text-muted-foreground">
            <Building2 className="h-3.5 w-3.5" />
            Workspace
          </div>
          {organizationsLoading ? (
            <Skeleton className="h-10 w-full rounded-xl" />
          ) : organizations.length ? (
            <div className="space-y-2">
              <Select
                value={selectedOrganizationId ?? undefined}
                onValueChange={setSelectedOrganizationId}
              >
                <SelectTrigger className="h-11 rounded-xl border-white/10 bg-white/5 text-left">
                  <SelectValue placeholder="Select workspace" />
                </SelectTrigger>
                <SelectContent>
                  {organizations.map((organization) => (
                    <SelectItem key={organization.id} value={organization.id}>
                      {organization.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {selectedOrganization ? (
                <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
                  <span>
                    {selectedOrganization.project_count}{" "}
                    {selectedOrganization.project_count === 1
                      ? "project"
                      : "projects"}
                  </span>
                  <Badge variant="outline" className="border-white/10 bg-white/5">
                    {selectedOrganization.is_personal ? "Personal" : "Organization"}
                  </Badge>
                </div>
              ) : null}
            </div>
          ) : (
            <p className="text-xs text-muted-foreground">
              No workspaces available.
            </p>
          )}
        </div>

        {navItems.map(({ to, label, icon: Icon }) => {
          const isActive = matchRoute({ to, fuzzy: to !== "/" });
          return (
            <Link
              key={to}
              to={to}
              className={cn(
                "flex items-center gap-3 rounded-xl px-3.5 py-2.5 text-sm font-medium transition-all",
                isActive
                  ? "bg-primary/14 text-primary shadow-[inset_0_0_0_1px_rgba(125,153,255,0.18)]"
                  : "text-muted-foreground hover:bg-accent/80 hover:text-accent-foreground",
              )}
            >
              <Icon className="h-4 w-4" />
              {label}
            </Link>
          );
        })}
      </nav>

      <Separator />

      {/* User */}
      <div className="m-3 flex items-center gap-3 rounded-2xl border border-white/6 bg-black/10 p-3">
        <Avatar className="h-8 w-8">
          <AvatarImage src={user?.avatar_url} alt={user?.username} />
          <AvatarFallback>{user?.username?.[0]?.toUpperCase()}</AvatarFallback>
        </Avatar>
        <div className="flex-1 truncate">
          <p className="truncate text-sm font-medium">{user?.username}</p>
          <p className="text-xs text-muted-foreground">{user?.role}</p>
        </div>
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8 shrink-0"
          onClick={async () => {
            try {
              await api.logout();
            } finally {
              useOrganizationStore.getState().clearSelection();
              clearAuth();
            }
            window.location.href = "/login";
          }}
        >
          <LogOut className="h-4 w-4" />
        </Button>
      </div>
    </aside>
  );
}
