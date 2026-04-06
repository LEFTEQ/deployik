import { useParams } from "@tanstack/react-router";

import { VariableStore } from "@/components/projects/variable-store";

export function ProjectSettingsEnv() {
  const { id } = useParams({ strict: false }) as { id: string };

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-lg font-semibold">Environment Variables & Secrets</h2>
        <p className="text-sm text-muted-foreground">Manage environment-specific configuration and sensitive values.</p>
      </div>
      <VariableStore projectId={id} kind="env" />
      <div className="border-b" />
      <VariableStore projectId={id} kind="secret" />
    </div>
  );
}
