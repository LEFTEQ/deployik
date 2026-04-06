import { useParams } from "@tanstack/react-router";

import { ProjectIntegrationTab } from "@/components/projects/project-integration";

export function ProjectIntegration() {
  const { id } = useParams({ strict: false }) as { id: string };

  return <ProjectIntegrationTab projectId={id} />;
}
