import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { LoaderCircle, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { VARIABLE_SCOPE_META } from "@/lib/deployment-helpers";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { LoadingState } from "@/components/ui/spinner";
import type { VariableScope } from "@/types/api";

export interface VariableStoreProps {
  projectId: string;
  kind: "env" | "secret";
}

export function VariableStore({ projectId, kind }: VariableStoreProps) {
  const queryClient = useQueryClient();
  const [scope, setScope] = useState<VariableScope>("shared");
  const [rows, setRows] = useState<{ key: string; value: string }[]>([
    { key: "", value: "" },
  ]);

  useEffect(() => {
    setRows([{ key: "", value: "" }]);
  }, [kind, scope]);

  const isSecret = kind === "secret";
  const storeTitle = isSecret ? "Secrets" : "Env Vars";
  const storeDescription = isSecret
    ? "Encrypted at rest, never exposed in the build, and injected only at runtime."
    : "Configuration values for your app. Use NEXT_PUBLIC_* only for values that are safe to expose to the client bundle.";
  const scopeDescription = isSecret
    ? "Shared secrets apply to both preview and production. Scope-specific secrets override shared ones with the same key."
    : "Shared env vars apply to both preview and production. Scope-specific env vars override shared ones with the same key.";
  const emptyState = isSecret
    ? "No secrets stored for this scope yet."
    : "No environment variables stored for this scope yet.";
  const replaceDescription = isSecret
    ? "Saving here replaces all secrets for the selected scope."
    : "Saving here replaces all env vars for the selected scope.";

  const { data: existingVars, isLoading } = useQuery({
    queryKey: ["project-variables", kind, projectId, scope],
    queryFn: () =>
      isSecret
        ? api.listSecrets(projectId, scope)
        : api.listEnvVars(projectId, scope),
  });

  const saveMutation = useMutation({
    mutationFn: () => {
      const variables = rows.filter((row) => row.key.trim() !== "");
      return isSecret
        ? api.bulkSetSecrets(projectId, { environment: scope, variables })
        : api.bulkSetEnvVars(projectId, { environment: scope, variables });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["project-variables", kind, projectId],
      });
      toast.success(isSecret ? "Secrets saved" : "Environment variables saved");
      setRows([{ key: "", value: "" }]);
    },
    onError: (err) => toast.error(err.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (key: string) =>
      isSecret
        ? api.deleteSecret(projectId, key, scope)
        : api.deleteEnvVar(projectId, key, scope),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["project-variables", kind, projectId],
      });
      toast.success(
        isSecret ? "Secret deleted" : "Environment variable deleted",
      );
    },
    onError: (err) => toast.error(err.message),
  });

  const addRow = () => setRows([...rows, { key: "", value: "" }]);
  const updateRow = (idx: number, field: "key" | "value", value: string) => {
    const nextRows = [...rows];
    nextRows[idx] = { ...nextRows[idx]!, [field]: value };
    setRows(nextRows);
  };
  const removeRow = (idx: number) =>
    setRows(rows.filter((_, index) => index !== idx));

  return (
    <div className="space-y-6">
      {/* Scope selector */}
      <div className="space-y-3">
        <h3 className="text-base font-semibold">{storeTitle}</h3>
        <p className="text-sm text-muted-foreground">
          {storeDescription} {scopeDescription}
        </p>
        <div className="flex flex-wrap gap-2">
          {(Object.keys(VARIABLE_SCOPE_META) as VariableScope[]).map(
            (value) => (
              <Button
                key={value}
                size="sm"
                variant={scope === value ? "default" : "outline"}
                onClick={() => setScope(value)}
              >
                {VARIABLE_SCOPE_META[value].label}
              </Button>
            ),
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
          <Badge
            variant="outline"
            className={VARIABLE_SCOPE_META[scope].badgeClass}
          >
            {VARIABLE_SCOPE_META[scope].label}
          </Badge>
          <span>{VARIABLE_SCOPE_META[scope].description}</span>
        </div>
      </div>

      {/* Current variables */}
      <div className="space-y-2">
        <div className="flex flex-col gap-0.5">
          <h3 className="text-base font-semibold">
            Current {storeTitle} ({VARIABLE_SCOPE_META[scope].label})
          </h3>
          <p className="text-sm text-muted-foreground">
            Values are masked after save. Delete a key here or replace the full
            scope below.
          </p>
        </div>
        {isLoading ? (
          <LoadingState
            title={`Loading ${storeTitle.toLowerCase()}...`}
            description={`Fetching stored ${storeTitle.toLowerCase()} for the selected scope.`}
            className="min-h-[220px]"
          />
        ) : existingVars?.length ? (
          <div className="divide-y rounded-lg border font-mono text-sm">
            {existingVars.map((variable) => {
              const deleting =
                deleteMutation.isPending &&
                deleteMutation.variables === variable.key;

              return (
                <div
                  key={variable.id}
                  className="flex flex-col gap-3 px-4 py-3 md:flex-row md:items-center md:justify-between"
                >
                  <div className="space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-foreground">{variable.key}</span>
                      <Badge
                        variant="outline"
                        className={
                          VARIABLE_SCOPE_META[variable.environment].badgeClass
                        }
                      >
                        {VARIABLE_SCOPE_META[variable.environment].label}
                      </Badge>
                    </div>
                    <p className="text-xs text-muted-foreground">
                      {variable.value}
                    </p>
                  </div>

                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => deleteMutation.mutate(variable.key)}
                    disabled={deleteMutation.isPending}
                  >
                    {deleting ? (
                      <LoaderCircle className="h-4 w-4 animate-spin" />
                    ) : (
                      <Trash2 className="h-4 w-4" />
                    )}
                  </Button>
                </div>
              );
            })}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">{emptyState}</p>
        )}
      </div>

      {/* Replace / bulk edit — keeps Card for visual separation */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            Replace {storeTitle} ({VARIABLE_SCOPE_META[scope].label})
          </CardTitle>
          <CardDescription>{replaceDescription}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-2">
          {rows.map((row, idx) => (
            <div key={idx} className="flex gap-2">
              <Input
                placeholder="KEY"
                value={row.key}
                onChange={(e) =>
                  updateRow(idx, "key", e.target.value.toUpperCase())
                }
                className="font-mono"
              />
              <Input
                placeholder={isSecret ? "secret value" : "value"}
                type={isSecret ? "password" : "text"}
                value={row.value}
                onChange={(e) => updateRow(idx, "value", e.target.value)}
                className="font-mono"
              />
              <Button
                variant="ghost"
                size="icon"
                onClick={() => removeRow(idx)}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          ))}
          <div className="flex flex-wrap gap-2 pt-2">
            <Button variant="outline" size="sm" onClick={addRow}>
              Add Row
            </Button>
            <Button
              size="sm"
              onClick={() => saveMutation.mutate()}
              disabled={saveMutation.isPending}
            >
              {saveMutation.isPending
                ? "Saving..."
                : isSecret
                  ? "Save Secrets"
                  : "Save Env Vars"}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
