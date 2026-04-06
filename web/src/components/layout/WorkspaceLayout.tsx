import { Outlet } from "@tanstack/react-router";

import { AppSidebar } from "@/components/layout/AppSidebar";
import { SidebarInset } from "@/components/ui/sidebar";

export function WorkspaceLayout() {
  return (
    <>
      <AppSidebar context="workspace" />
      <SidebarInset>
        <div className="flex-1 overflow-y-auto">
          <div className="mx-auto w-full max-w-[1600px] px-4 py-4 sm:px-6 lg:px-8">
            <Outlet />
          </div>
        </div>
      </SidebarInset>
    </>
  );
}
