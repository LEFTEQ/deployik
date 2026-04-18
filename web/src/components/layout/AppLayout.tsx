import { Suspense } from "react";
import { Outlet } from "@tanstack/react-router";

import { LoadingState } from "@/components/ui/spinner";
import { SidebarProvider } from "@/components/ui/sidebar";

export function AppLayout() {
  return (
    <SidebarProvider
      style={
        {
          "--sidebar-width": "16rem",
          "--sidebar-width-icon": "3rem",
        } as React.CSSProperties
      }
    >
      <Suspense fallback={<LoadingState title="Loading…" />}>
        <Outlet />
      </Suspense>
    </SidebarProvider>
  );
}
