import { useRef } from "react";

import { useParams } from "@tanstack/react-router";

import { ProjectAnalyticsTab } from "@/components/projects/project-analytics";
import { ProjectIntegrationTab } from "@/components/projects/project-integration";

export function ProjectAnalytics() {
  const { id } = useParams({ strict: false }) as { id: string };
  const setupRef = useRef<HTMLDivElement | null>(null);

  return (
    <div className="flex flex-col gap-8">
      <ProjectAnalyticsTab
        projectId={id}
        onSetupAnalytics={() =>
          setupRef.current?.scrollIntoView({
            behavior: "smooth",
            block: "start",
          })
        }
      />
      <div ref={setupRef}>
        <ProjectIntegrationTab projectId={id} embedded />
      </div>
    </div>
  );
}
