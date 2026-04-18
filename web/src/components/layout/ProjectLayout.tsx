import { Suspense } from "react";
import { Link, Outlet, useParams, useRouterState } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { FolderKanban } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { AppSidebar } from "@/components/layout/AppSidebar";
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

export function ProjectLayout() {
  const { id } = useParams({ strict: false }) as { id: string };
  const routerState = useRouterState();
  const pathname = routerState.location.pathname;
  const { user } = useAuthStore();

  const { data: project } = useQuery({
    queryKey: queryKeys.project(id),
    queryFn: () => api.getProject(id),
  });

  // Derive current page name from pathname
  const base = `/projects/${id}`;
  let currentPage = "Overview";
  if (pathname.startsWith(`${base}/deployments/`)) currentPage = "Deployment";
  else if (pathname === `${base}/deployments`) currentPage = "Deployments";
  else if (pathname === `${base}/analytics`) currentPage = "Analytics";
  else if (pathname === `${base}/integration`) currentPage = "Integration";
  else if (pathname === `${base}/settings/domains`) currentPage = "Domains";
  else if (pathname === `${base}/settings/env`) currentPage = "Environments";
  else if (pathname === `${base}/settings/protection`) currentPage = "Protection";
  else if (pathname === `${base}/settings`) currentPage = "Build Settings";

  return (
    <>
      <AppSidebar context="project" projectId={id} />
      <SidebarInset>
        <header className="flex h-16 shrink-0 items-center gap-2 border-b px-4">
          <SidebarTrigger className="-ml-1" />
          <Separator orientation="vertical" className="mr-2 h-4" />
          <Link to="/" className="hidden items-center gap-2 md:flex">
            <FolderKanban className="size-4 text-primary" />
            <span className="font-mono text-[13px] font-semibold tracking-[0.16em]">
              /deployik
            </span>
          </Link>
          <Separator orientation="vertical" className="mr-2 hidden h-4 md:block" />
          <Breadcrumb>
            <BreadcrumbList>
              <BreadcrumbItem className="hidden md:block">
                <BreadcrumbLink asChild>
                  <Link to="/projects/$id" params={{ id }}>
                    {project?.name ?? "..."}
                  </Link>
                </BreadcrumbLink>
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
        </header>
        <div className="flex flex-1 flex-col gap-4 p-4">
          <div className="mx-auto w-full max-w-[1400px]">
            <ErrorBoundary scope="project">
              <Suspense
                fallback={
                  <LoadingState
                    title="Loading…"
                    className="min-h-[320px]"
                  />
                }
              >
                <Outlet />
              </Suspense>
            </ErrorBoundary>
          </div>
        </div>
      </SidebarInset>
    </>
  );
}
