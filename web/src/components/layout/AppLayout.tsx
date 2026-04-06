import { Outlet } from "@tanstack/react-router";

import { SidebarProvider } from "@/components/ui/sidebar";
import { TopBar } from "@/components/layout/TopBar";

export function AppLayout() {
  return (
    <SidebarProvider
      className="flex-col"
      style={
        {
          "--sidebar-width": "16rem",
          "--sidebar-width-icon": "3rem",
        } as React.CSSProperties
      }
    >
      <TopBar />
      <div className="flex flex-1">
        <Outlet />
      </div>
    </SidebarProvider>
  );
}
