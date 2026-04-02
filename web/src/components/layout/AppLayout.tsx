import { Outlet } from "@tanstack/react-router";

import { AppSidebar } from "./Sidebar";
import { SiteHeader } from "./SiteHeader";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";

export function AppLayout() {
  return (
    <SidebarProvider defaultOpen={false}>
      <AppSidebar />
      <SidebarInset className="min-w-0 bg-transparent md:m-0 md:ml-0 md:rounded-none md:shadow-none">
        <SiteHeader />
        <main className="flex-1 overflow-y-auto">
          <div className="mx-auto w-full max-w-[1600px] px-4 py-4 sm:px-6 lg:px-8">
            <Outlet />
          </div>
        </main>
      </SidebarInset>
    </SidebarProvider>
  );
}
