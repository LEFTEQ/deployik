import { useState } from "react";

import { Link, useMatchRoute } from "@tanstack/react-router";
import {
  Building2,
  LayoutDashboard,
  LogOut,
  Menu,
  Plus,
} from "lucide-react";

import { useAuthStore } from "@/store/auth";
import { useOrganizationStore } from "@/store/organization";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useOrganizations } from "@/hooks/use-organizations";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";

const navItems = [
  { to: "/" as const, label: "Projects", icon: LayoutDashboard },
  { to: "/new" as const, label: "New Project", icon: Plus },
];

export function Sidebar() {
  return (
    <aside className="hidden h-screen w-24 shrink-0 border-r border-white/6 bg-sidebar-background/85 backdrop-blur-2xl md:block lg:w-28">
      <SidebarNavigation compact />
    </aside>
  );
}

export function MobileSidebarNav() {
  const [open, setOpen] = useState(false);

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="md:hidden"
          aria-label="Open navigation"
        >
          <Menu className="h-5 w-5" />
        </Button>
      </SheetTrigger>
      <SheetContent
        side="left"
        className="w-80 border-white/10 bg-[#0b1220]/98 px-0 backdrop-blur-2xl"
      >
        <SheetHeader className="px-5">
          <SheetTitle className="font-mono text-sm tracking-[0.18em] text-slate-100">
            /deployik
          </SheetTitle>
        </SheetHeader>
        <SidebarNavigation onNavigate={() => setOpen(false)} />
      </SheetContent>
    </Sheet>
  );
}

function SidebarNavigation({
  compact = false,
  onNavigate,
}: {
  compact?: boolean;
  onNavigate?: () => void;
}) {
  const { user, clearAuth } = useAuthStore();
  const {
    organizations,
    selectedOrganizationId,
    setSelectedOrganizationId,
    isLoading: organizationsLoading,
  } = useOrganizations();
  const matchRoute = useMatchRoute();

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
    <div
      className={cn(
        "flex h-full flex-col",
        compact ? "px-3 py-4" : "px-4 py-5",
      )}
    >
      <div className={cn("space-y-3", compact ? "items-center" : "")}>
        <div
          className={cn(
            "font-mono text-[12px] tracking-[0.22em] text-slate-100",
            compact ? "px-1 text-center" : "px-1",
          )}
        >
          /deployik
        </div>

        <div className="space-y-2">
          <div
            className={cn(
              "flex items-center gap-2 px-1 text-[11px] uppercase tracking-[0.22em] text-muted-foreground",
              compact && "justify-center",
            )}
          >
            <Building2 className="h-3.5 w-3.5" />
            {!compact ? "Workspace" : null}
          </div>

          {organizationsLoading ? (
            <Skeleton
              className={cn(
                "rounded-2xl",
                compact ? "h-11 w-full" : "h-11 w-full",
              )}
            />
          ) : organizations.length ? (
            <Select
              value={selectedOrganizationId ?? undefined}
              onValueChange={setSelectedOrganizationId}
            >
              <SelectTrigger
                className={cn(
                  "rounded-2xl border-white/10 bg-white/5 text-left shadow-[inset_0_1px_0_rgba(255,255,255,0.04)]",
                  compact
                    ? "h-11 px-2 text-[11px]"
                    : "h-11 px-3 text-sm",
                )}
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
          ) : (
            <p
              className={cn(
                "text-xs text-muted-foreground",
                compact ? "px-1 text-center" : "px-1",
              )}
            >
              No workspaces.
            </p>
          )}
        </div>
      </div>

      <nav
        className={cn(
          "flex-1",
          compact ? "mt-6 space-y-2" : "mt-8 space-y-1.5",
        )}
      >
        {navItems.map(({ to, label, icon: Icon }) => {
          const isActive = matchRoute({ to, fuzzy: to !== "/" });
          const item = (
            <Link
              key={to}
              to={to}
              onClick={onNavigate}
              className={cn(
                "group flex items-center rounded-2xl transition-all",
                compact
                  ? "justify-center px-3 py-3"
                  : "gap-3 px-3.5 py-2.5 text-sm font-medium",
                isActive
                  ? "bg-primary/14 text-primary shadow-[inset_0_0_0_1px_rgba(125,153,255,0.18)]"
                  : "text-muted-foreground hover:bg-accent/80 hover:text-accent-foreground",
              )}
            >
              <Icon className="h-4 w-4" />
              {!compact ? <span>{label}</span> : null}
            </Link>
          );

          if (!compact) return item;

          return (
            <Tooltip key={to}>
              <TooltipTrigger asChild>{item}</TooltipTrigger>
              <TooltipContent side="right" sideOffset={10}>
                {label}
              </TooltipContent>
            </Tooltip>
          );
        })}
      </nav>

      <div
        className={cn(
          "rounded-2xl border border-white/6 bg-black/10",
          compact ? "p-2" : "p-3",
        )}
      >
        <div
          className={cn(
            "flex items-center gap-3",
            compact && "flex-col justify-center gap-2",
          )}
        >
          <Avatar className={cn(compact ? "h-9 w-9" : "h-9 w-9")}>
            <AvatarImage src={user?.avatar_url} alt={user?.username} />
            <AvatarFallback>
              {user?.username?.[0]?.toUpperCase()}
            </AvatarFallback>
          </Avatar>
          {!compact ? (
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium text-foreground">
                {user?.username}
              </p>
              <p className="text-xs text-muted-foreground">{user?.role}</p>
            </div>
          ) : null}
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8 shrink-0"
            onClick={handleLogout}
            aria-label="Log out"
          >
            <LogOut className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}
