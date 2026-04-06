import { Outlet, useParams } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";

import { api } from "@/lib/api";
import { AppSidebar } from "@/components/layout/AppSidebar";
import { SidebarInset } from "@/components/ui/sidebar";

export function ProjectLayout() {
  const { id } = useParams({ strict: false }) as { id: string };

  // Prefetch project data so all child pages get an instant cache hit
  useQuery({
    queryKey: ["project", id],
    queryFn: () => api.getProject(id),
  });

  return (
    <>
      <AppSidebar context="project" projectId={id} />
      <SidebarInset>
        <div className="flex-1 overflow-y-auto">
          <div className="mx-auto w-full max-w-[1400px] px-4 py-4 sm:px-6 lg:px-8">
            <Outlet />
          </div>
        </div>
      </SidebarInset>
    </>
  );
}
