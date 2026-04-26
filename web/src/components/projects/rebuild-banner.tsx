import { useQueries, useQuery } from "@tanstack/react-query";
import { AlertTriangle, GlobeLock, Loader2, Rocket } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { useFastDeploy } from "@/hooks/useFastDeploy";
import type { ProjectVariable, VariableScope } from "@/types/api";

export interface RebuildBannerProps {
  projectId: string;
}

type VariableEnvironment = Exclude<VariableScope, "shared">;

interface ScopedFetch {
  kind: "env" | "secret";
  scope: VariableScope;
}

const SCOPED_FETCHES: ScopedFetch[] = [
  { kind: "env", scope: "shared" },
  { kind: "env", scope: "preview" },
  { kind: "env", scope: "production" },
  { kind: "secret", scope: "shared" },
  { kind: "secret", scope: "preview" },
  { kind: "secret", scope: "production" },
];

interface EnvironmentSummary {
  env: VariableEnvironment;
  changedCount: number;
  // null when the env has never had a successful deploy
  lastDeployAt: Date | null;
}

function pickListsForEnvironment(
  buckets: Record<string, ProjectVariable[]>,
  env: VariableEnvironment,
): ProjectVariable[] {
  // Shared variables flow into every environment, so they count for both.
  return [
    ...(buckets["env:shared"] ?? []),
    ...(buckets[`env:${env}`] ?? []),
    ...(buckets["secret:shared"] ?? []),
    ...(buckets[`secret:${env}`] ?? []),
  ];
}

function countChangesSince(vars: ProjectVariable[], cutoff: Date | null): number {
  if (cutoff === null) {
    // No cutoff means the env has never deployed. Every variable is pending.
    return vars.length;
  }
  let n = 0;
  for (const v of vars) {
    const updated = new Date(v.updated_at);
    if (Number.isFinite(updated.getTime()) && updated > cutoff) {
      n++;
    }
  }
  return n;
}

export function RebuildBanner({ projectId }: RebuildBannerProps) {
  const { data: project } = useQuery({
    queryKey: queryKeys.project(projectId),
    queryFn: () => api.getProject(projectId),
  });

  const variableQueries = useQueries({
    queries: SCOPED_FETCHES.map(({ kind, scope }) => ({
      queryKey: queryKeys.projectVariables(kind, projectId, scope),
      queryFn: () =>
        kind === "secret"
          ? api.listSecrets(projectId, scope)
          : api.listEnvVars(projectId, scope),
    })),
  });

  const fastDeploy = useFastDeploy(projectId);

  // Wait until project + every variable bucket has resolved at least once.
  // Without this we can flash a misleading "X changed" count while shared
  // variables are still loading.
  if (!project || variableQueries.some((q) => q.data === undefined)) {
    return null;
  }

  const buckets: Record<string, ProjectVariable[]> = {};
  SCOPED_FETCHES.forEach((spec, i) => {
    buckets[`${spec.kind}:${spec.scope}`] = variableQueries[i]?.data ?? [];
  });

  const summaries: EnvironmentSummary[] = (
    ["preview", "production"] as VariableEnvironment[]
  ).map((env) => {
    const lastDeployIso =
      env === "preview"
        ? project.latest_preview_deploy_at
        : project.latest_production_deploy_at;
    const lastDeployAt = lastDeployIso ? new Date(lastDeployIso) : null;
    const vars = pickListsForEnvironment(buckets, env);
    return {
      env,
      changedCount: countChangesSince(vars, lastDeployAt),
      lastDeployAt,
    };
  });

  const visible = summaries.filter((s) => s.changedCount > 0);
  if (visible.length === 0) {
    return null;
  }

  const productionConfirming = fastDeploy.productionState === "confirming";

  return (
    <div className="space-y-2">
      {visible.map((summary) => {
        const isProduction = summary.env === "production";
        const noun = summary.changedCount === 1 ? "variable" : "variables";
        const message = summary.lastDeployAt
          ? `${summary.changedCount} ${noun} changed since last ${summary.env} deploy`
          : `${summary.changedCount} ${noun} ready for first ${summary.env} deploy`;
        const onClick = isProduction
          ? fastDeploy.triggerProduction
          : fastDeploy.triggerPreview;
        const buttonLabel = isProduction
          ? productionConfirming
            ? "Click to confirm"
            : "Deploy production"
          : "Deploy preview";
        const icon = isProduction ? GlobeLock : Rocket;
        const Icon = icon;
        return (
          <Card
            key={summary.env}
            className="flex flex-col gap-3 border-l-4 border-yellow-500/70 bg-yellow-500/5 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
          >
            <div className="flex items-start gap-3">
              <AlertTriangle className="mt-0.5 h-4 w-4 text-yellow-600" />
              <div className="space-y-0.5">
                <p className="text-sm font-medium text-foreground">{message}</p>
                <p className="text-xs text-muted-foreground">
                  {summary.lastDeployAt
                    ? "Redeploy to apply the new values to the running container."
                    : "Trigger a deploy to roll the new variables out for the first time."}
                </p>
              </div>
            </div>
            <Button
              size="sm"
              variant={
                isProduction && productionConfirming ? "destructive" : "outline"
              }
              onClick={onClick}
              disabled={fastDeploy.isPending}
            >
              {fastDeploy.isPending ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Icon className="h-3.5 w-3.5" />
              )}
              {buttonLabel}
            </Button>
          </Card>
        );
      })}
    </div>
  );
}
