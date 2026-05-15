import { useQuery } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { ServicesPanel } from "@/components/projects/services/ServicesPanel";
import { LoadingState } from "@/components/ui/spinner";

export function ProjectSettingsServices() {
  const { id } = useParams({ strict: false });
  const projectId = typeof id === "string" ? id : "";

  const { data: project, isLoading, isError } = useQuery({
    queryKey: queryKeys.project(projectId),
    queryFn: () => api.getProject(projectId),
    enabled: !!projectId,
  });

  if (!projectId) {
    return (
      <div className="text-sm text-muted-foreground">Invalid project URL.</div>
    );
  }
  if (isLoading) {
    return (
      <LoadingState
        title="Loading project..."
        description="Fetching project metadata."
        className="min-h-[200px]"
      />
    );
  }
  if (isError) {
    return (
      <div className="text-sm text-muted-foreground">
        Unable to load project right now. Please try again.
      </div>
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
      <ServicesPanel projectId={projectId} projectName={project.name} />
    </div>
  );
}
