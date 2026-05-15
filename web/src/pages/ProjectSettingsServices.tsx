import { useQuery } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { ServicesPanel } from "@/components/projects/services/ServicesPanel";
import { LoadingState } from "@/components/ui/spinner";

export function ProjectSettingsServices() {
  const { id } = useParams({ strict: false }) as { id: string };

  const { data: project, isLoading } = useQuery({
    queryKey: queryKeys.project(id),
    queryFn: () => api.getProject(id),
  });

  if (isLoading) {
    return (
      <LoadingState
        title="Loading project..."
        description="Fetching project metadata."
        className="min-h-[200px]"
      />
    );
  }
  if (!project) {
    return (
      <div className="text-sm text-muted-foreground">Project not found.</div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-base font-semibold">Services</h2>
        <p className="text-sm text-muted-foreground">
          Attach a Postgres database to this project. Each environment gets its
          own container + persistent volume. Credentials are revealed on demand;
          external access is via SSH tunnel only.
        </p>
      </div>
      <ServicesPanel projectId={id} projectName={project.name} />
    </div>
  );
}
