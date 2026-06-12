import { Link, Outlet } from "@tanstack/react-router";
import { FolderKanban } from "lucide-react";

import { AppSidebar } from "@/components/layout/AppSidebar";
import { CommandPalette } from "@/components/layout/CommandPalette";
import { MobileTabBar } from "@/components/layout/MobileTabBar";
import { ErrorBoundary } from "@/components/layout/ErrorBoundary";
import { useAuthStore } from "@/store/auth";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Separator } from "@/components/ui/separator";
import { SidebarInset, SidebarTrigger } from "@/components/ui/sidebar";

export function WorkspaceLayout() {
  const { user } = useAuthStore();

  return (
    <>
      <AppSidebar context="workspace" />
      <SidebarInset>
        <header className="shrink-0 border-b px-4 pt-safe">
          <div className="flex h-14 items-center gap-2 md:h-16">
            <SidebarTrigger className="-ml-1" />
            <Separator orientation="vertical" className="mr-2 h-4" />
            <Link to="/" className="flex items-center gap-2">
              <FolderKanban className="size-4 text-primary" />
              <span className="font-mono text-[13px] font-semibold tracking-[0.16em]">
                /deployik
              </span>
            </Link>
            <div className="ml-auto flex items-center gap-2">
              <CommandPalette />
              <Avatar className="h-7 w-7 rounded-lg">
                <AvatarImage src={user?.avatar_url} alt={user?.username} />
                <AvatarFallback className="rounded-lg text-xs">
                  {user?.username?.[0]?.toUpperCase() ?? "D"}
                </AvatarFallback>
              </Avatar>
            </div>
          </div>
        </header>
        <div className="flex min-w-0 flex-1 flex-col gap-4 p-4 pb-safe-tabbar md:pb-4">
          <div className="mx-auto w-full max-w-[1600px]">
            <ErrorBoundary scope="workspace">
              <Outlet />
            </ErrorBoundary>
          </div>
        </div>
        <MobileTabBar context="workspace" />
      </SidebarInset>
    </>
  );
}
