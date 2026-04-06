import { useNavigate, useParams } from "@tanstack/react-router";

import { ProjectAnalyticsTab } from "@/components/projects/project-analytics";

export function ProjectAnalytics() {
  const { id } = useParams({ strict: false }) as { id: string };
  const navigate = useNavigate();

  return (
    <ProjectAnalyticsTab
      projectId={id}
      onSetupAnalytics={() =>
        navigate({ to: "/projects/$id/integration", params: { id } })
      }
    />
  );
}
