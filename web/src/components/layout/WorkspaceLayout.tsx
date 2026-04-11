import { Link, Outlet } from "@tanstack/react-router";
import { FolderKanban } from "lucide-react";

import { AppSidebar } from "@/components/layout/AppSidebar";
import { CommandPalette } from "@/components/layout/CommandPalette";
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
        <header className="flex h-16 shrink-0 items-center gap-2 border-b px-4">
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
        </header>
        <div className="flex flex-1 flex-col gap-4 p-4">
          <div className="mx-auto w-full max-w-[1600px]">
            <Outlet />
          </div>
        </div>
      </SidebarInset>
    </>
  );
}
