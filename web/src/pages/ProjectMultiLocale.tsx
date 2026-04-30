import { useParams } from "@tanstack/react-router";

import { ProjectMultiLocaleTab } from "@/components/projects/project-multi-locale";

export function ProjectMultiLocale() {
  const { id } = useParams({ strict: false }) as { id: string };

  return <ProjectMultiLocaleTab projectId={id} />;
}
