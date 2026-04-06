import { Link, Outlet, useParams, useRouterState } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";

import { api } from "@/lib/api";
import { AppSidebar } from "@/components/layout/AppSidebar";
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
import { CommandPalette } from "@/components/layout/CommandPalette";

export function ProjectLayout() {
  const { id } = useParams({ strict: false }) as { id: string };
  const routerState = useRouterState();
  const pathname = routerState.location.pathname;

  const { data: project } = useQuery({
    queryKey: ["project", id],
    queryFn: () => api.getProject(id),
  });

  // Derive current page name from pathname
  const base = `/projects/${id}`;
  let currentPage = "Overview";
  if (pathname.startsWith(`${base}/deployments/`)) currentPage = "Deployment";
  else if (pathname === `${base}/deployments`) currentPage = "Deployments";
  else if (pathname === `${base}/analytics`) currentPage = "Analytics";
  else if (pathname === `${base}/integration`) currentPage = "Integration";
  else if (pathname === `${base}/domains`) currentPage = "Domains";
  else if (pathname === `${base}/settings`) currentPage = "Settings";

  return (
    <>
      <AppSidebar context="project" projectId={id} />
      <SidebarInset>
        <header className="flex h-16 shrink-0 items-center gap-2 border-b px-4">
          <SidebarTrigger className="-ml-1" />
          <Separator orientation="vertical" className="mr-2 h-4" />
          <Breadcrumb>
            <BreadcrumbList>
              <BreadcrumbItem className="hidden md:block">
                <BreadcrumbLink asChild>
                  <Link to="/">Projects</Link>
                </BreadcrumbLink>
              </BreadcrumbItem>
              <BreadcrumbSeparator className="hidden md:block" />
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
          <div className="ml-auto">
            <CommandPalette />
          </div>
        </header>
        <div className="flex flex-1 flex-col gap-4 p-4">
          <div className="mx-auto w-full max-w-[1400px]">
            <Outlet />
          </div>
        </div>
      </SidebarInset>
    </>
  );
}
