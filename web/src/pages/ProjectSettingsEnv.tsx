import { useParams } from "@tanstack/react-router";

import { RebuildBanner } from "@/components/projects/rebuild-banner";
import { VariableStore } from "@/components/projects/variable-store";

export function ProjectSettingsEnv() {
  const { id } = useParams({ strict: false }) as { id: string };

  return (
    <div className="space-y-8">
      <RebuildBanner projectId={id} />
      <VariableStore projectId={id} kind="env" />
      <div className="border-b" />
      <VariableStore projectId={id} kind="secret" />
    </div>
  );
}
