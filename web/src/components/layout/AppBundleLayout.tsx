import { Suspense } from "react";
import {
  Link,
  Outlet,
  useParams,
  useRouterState,
} from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { FolderKanban } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { AppSidebar } from "@/components/layout/AppSidebar";
import { MobileTabBar } from "@/components/layout/MobileTabBar";
import { CommandPalette } from "@/components/layout/CommandPalette";
import { ErrorBoundary } from "@/components/layout/ErrorBoundary";
import { LoadingState } from "@/components/ui/spinner";
import { useAuthStore } from "@/store/auth";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Separator } from "@/components/ui/separator";
import { SidebarInset, SidebarTrigger } from "@/components/ui/sidebar";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";

export function AppBundleLayout() {
  const { appId } = useParams({ strict: false }) as { appId: string };
  const routerState = useRouterState();
  const pathname = routerState.location.pathname;
  const { user } = useAuthStore();

  const { data: health } = useQuery({
    queryKey: queryKeys.appHealth(appId, "production"),
    queryFn: () => api.getAppHealth(appId, "production"),
  });

  // Derive current page name from pathname
  const base = `/apps/${appId}`;
  let currentPage = "Overview";
  if (pathname.startsWith(`${base}/deployments`)) currentPage = "Deployments";
  else if (pathname === `${base}/topology`) currentPage = "Topology";
  else if (pathname === `${base}/variables`) currentPage = "Variables";
  else if (pathname === `${base}/releases`) currentPage = "Releases";
  else if (pathname.startsWith(`${base}/settings`)) currentPage = "Settings";

  return (
    <>
      <AppSidebar context="app" appId={appId} />
      <SidebarInset>
        <header className="shrink-0 border-b px-4 pt-safe">
          <div className="flex h-14 items-center gap-2 md:h-16">
            <SidebarTrigger className="-ml-1" />
            <Separator orientation="vertical" className="mr-2 h-4" />
            <Link to="/" className="hidden items-center gap-2 md:flex">
              <FolderKanban className="size-4 text-primary" />
              <span className="font-mono text-[13px] font-semibold tracking-[0.16em]">
                /deployik
              </span>
            </Link>
            <Separator
              orientation="vertical"
              className="mr-2 hidden h-4 md:block"
            />
            <Breadcrumb>
              <BreadcrumbList>
                <BreadcrumbItem className="hidden md:block">
                  <BreadcrumbLink asChild>
                    <Link to="/apps">Apps</Link>
                  </BreadcrumbLink>
                </BreadcrumbItem>
                <BreadcrumbSeparator className="hidden md:block" />
                <BreadcrumbItem className="hidden md:block">
                  <BreadcrumbPage>{health?.app.name ?? "..."}</BreadcrumbPage>
                </BreadcrumbItem>
                <BreadcrumbSeparator className="hidden md:block" />
                <BreadcrumbItem>
                  <BreadcrumbPage>{currentPage}</BreadcrumbPage>
                </BreadcrumbItem>
              </BreadcrumbList>
            </Breadcrumb>
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
          <div className="mx-auto w-full max-w-[1400px]">
            <ErrorBoundary scope="app">
              <Suspense
                fallback={
                  <LoadingState title="Loading…" className="min-h-[320px]" />
                }
              >
                <Outlet />
              </Suspense>
            </ErrorBoundary>
          </div>
        </div>
        <MobileTabBar context="app" appId={appId} />
      </SidebarInset>
    </>
  );
}
