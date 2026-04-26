import { useParams } from "@tanstack/react-router";

import { ProjectEmailTab } from "@/components/projects/project-email";

export function ProjectEmail() {
  const { id } = useParams({ strict: false }) as { id: string };

  return <ProjectEmailTab projectId={id} />;
}
