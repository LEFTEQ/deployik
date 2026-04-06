import { useParams } from "@tanstack/react-router";

import { VariableStore } from "@/components/projects/variable-store";

export function ProjectSettingsEnv() {
  const { id } = useParams({ strict: false }) as { id: string };

  return (
    <div className="space-y-6">
      <VariableStore projectId={id} kind="env" />
      <VariableStore projectId={id} kind="secret" />
    </div>
  );
}
