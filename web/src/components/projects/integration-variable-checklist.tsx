import { useMemo } from "react";

import { Link } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CheckCircle2,
  CircleAlert,
  KeyRound,
  LoaderCircle,
  Settings,
  Variable,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { LoadingState } from "@/components/ui/spinner";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { cn } from "@/lib/utils";
import type { InstallVariableSuggestion } from "@/types/api";

interface IntegrationVariableChecklistProps {
  projectId: string;
  title: string;
  description: string;
  envVariables: InstallVariableSuggestion[];
  secretVariables: InstallVariableSuggestion[];
  secretValues?: Record<string, string | undefined>;
  onApplied?: () => void;
}

export function IntegrationVariableChecklist({
  projectId,
  title,
  description,
  envVariables,
  secretVariables,
  secretValues = {},
  onApplied,
}: IntegrationVariableChecklistProps) {
  const queryClient = useQueryClient();

  const envQuery = useQuery({
    queryKey: queryKeys.projectVariables("env", projectId, "shared"),
    queryFn: () => api.listEnvVars(projectId, "shared"),
  });

  const secretQuery = useQuery({
    queryKey: queryKeys.projectVariables("secret", projectId, "shared"),
    queryFn: () => api.listSecrets(projectId, "shared"),
  });

  const existingEnvKeys = useMemo(
    () => new Set((envQuery.data ?? []).map((item) => item.key)),
    [envQuery.data],
  );
  const existingSecretKeys = useMemo(
    () => new Set((secretQuery.data ?? []).map((item) => item.key)),
    [secretQuery.data],
  );

  const rows = [
    ...envVariables.map((variable) => ({
      ...variable,
      icon: Variable,
      present: existingEnvKeys.has(variable.key),
      hasPendingValue: Boolean(variable.value?.trim()),
      missingLabel: "Needs value",
    })),
    ...secretVariables.map((variable) => ({
      ...variable,
      icon: KeyRound,
      present: existingSecretKeys.has(variable.key),
      hasPendingValue: Boolean(secretValues[variable.key]?.trim()),
      missingLabel: "Needs secret",
    })),
  ];

  const missingCount = rows.filter((row) => !row.present).length;
  const readyCount = rows.length - missingCount;

  const applyMutation = useMutation({
    mutationFn: async () => {
      const envValueMissing = envVariables.filter(
        (variable) =>
          !existingEnvKeys.has(variable.key) && !variable.value?.trim(),
      );
      if (envValueMissing.length > 0) {
        throw new Error(
          `Fill values before adding: ${envValueMissing
            .map((variable) => variable.key)
            .join(", ")}`,
        );
      }

      const secretValueMissing = secretVariables.filter(
        (variable) =>
          !existingSecretKeys.has(variable.key) &&
          !secretValues[variable.key]?.trim(),
      );
      if (secretValueMissing.length > 0) {
        throw new Error(
          `Fill secret values before adding: ${secretValueMissing
            .map((variable) => variable.key)
            .join(", ")}`,
        );
      }

      await Promise.all([
        ...envVariables
          .filter((variable) => variable.value?.trim())
          .map((variable) =>
            api.upsertEnvVar(projectId, {
              key: variable.key,
              value: variable.value?.trim() ?? "",
              environment: variable.environment,
            }),
          ),
        ...secretVariables
          .map((variable) => ({
            variable,
            value: secretValues[variable.key]?.trim(),
          }))
          .filter((entry) => entry.value)
          .map((entry) =>
            api.upsertSecret(projectId, {
              key: entry.variable.key,
              value: entry.value ?? "",
              environment: entry.variable.environment,
            }),
          ),
      ]);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.projectVariables("env", projectId),
      });
      queryClient.invalidateQueries({
        queryKey: queryKeys.projectVariables("secret", projectId),
      });
      onApplied?.();
      toast.success("Integration variables synchronized");
    },
    onError: (err) => toast.error(err.message),
  });

  const isLoading = envQuery.isLoading || secretQuery.isLoading;

  return (
    <Card>
      <CardHeader>
        <div className="min-w-0">
          <CardTitle>{title}</CardTitle>
          <CardDescription>{description}</CardDescription>
        </div>
        <CardAction>
          <Badge variant={missingCount === 0 ? "secondary" : "outline"}>
            {missingCount === 0
              ? "Ready"
              : `${readyCount}/${rows.length} added`}
          </Badge>
        </CardAction>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <LoadingState
            title="Checking variables..."
            description="Reading the shared environment and secret stores."
            className="min-h-[160px]"
          />
        ) : (
          <div className="flex flex-col gap-4">
            <div className="divide-y rounded-lg border">
              {rows.map((row) => (
                <VariableRow key={`${row.kind}:${row.key}`} row={row} />
              ))}
            </div>
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
              <p className="text-xs text-muted-foreground">
                Saved to the same shared environment stores used in Settings.
              </p>
              <div className="flex shrink-0 gap-2">
                <Button variant="outline" size="sm" asChild>
                  <Link to="/projects/$id/settings/env" params={{ id: projectId }}>
                    <Settings data-icon="inline-start" />
                    Environments
                  </Link>
                </Button>
                <Button
                  size="sm"
                  onClick={() => applyMutation.mutate()}
                  disabled={applyMutation.isPending}
                >
                  {applyMutation.isPending ? (
                    <LoaderCircle data-icon="inline-start" className="animate-spin" />
                  ) : (
                    <CheckCircle2 data-icon="inline-start" />
                  )}
                  Sync Variables
                </Button>
              </div>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function VariableRow({
  row,
}: {
  row: InstallVariableSuggestion & {
    icon: typeof Variable;
    present: boolean;
    hasPendingValue: boolean;
    missingLabel: string;
  };
}) {
  const ready = row.present || row.hasPendingValue;
  const Icon = row.icon;

  return (
    <div className="flex items-start gap-3 px-3 py-3">
      <div
        className={cn(
          "mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg border",
          row.present
            ? "border-success/30 bg-success/10 text-success"
            : ready
              ? "border-warning/30 bg-warning/10 text-warning"
              : "border-destructive/30 bg-destructive/10 text-destructive",
        )}
      >
        <Icon />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-mono text-sm font-medium">{row.key}</span>
          <Badge variant="outline" className="font-mono text-[11px]">
            {row.kind}
          </Badge>
          <Badge variant={row.present ? "secondary" : "outline"}>
            {row.present ? "Present" : ready ? "Ready to add" : row.missingLabel}
          </Badge>
        </div>
        {row.description ? (
          <p className="mt-1 text-xs text-muted-foreground">
            {row.description}
          </p>
        ) : null}
      </div>
      {ready ? (
        <CheckCircle2 className="mt-1 text-success" />
      ) : (
        <CircleAlert className="mt-1 text-destructive" />
      )}
    </div>
  );
}
