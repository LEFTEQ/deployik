import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { LoaderCircle, Plus, Trash2 } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { VARIABLE_SCOPE_META } from "@/lib/deployment-helpers";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";

type Scope = "shared" | "preview" | "production";

const SCOPES: Scope[] = ["shared", "preview", "production"];

export function AppVariableStore({
  appId,
  kind,
}: {
  appId: string;
  kind: "env" | "secret";
}) {
  const queryClient = useQueryClient();
  const [scope, setScope] = useState<Scope>("shared");
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");

  const isSecret = kind === "secret";
  const noun = isSecret ? "secret" : "variable";
  const nounPlural = isSecret ? "secrets" : "variables";
  const scopeMeta = VARIABLE_SCOPE_META[scope];

  const { data: variables } = useQuery({
    queryKey: queryKeys.appVariables(appId, kind, scope),
    queryFn: () => api.listAppVariables(appId, kind, scope),
  });

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: queryKeys.appVariables(appId, kind, scope),
    });

  const upsert = useMutation({
    mutationFn: () =>
      api.upsertAppVariable(appId, kind, {
        key: key.trim(),
        value,
        environment: scope,
      }),
    onSuccess: () => {
      toast.success(`${isSecret ? "Secret" : "Variable"} saved`);
      setKey("");
      setValue("");
      invalidate();
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to save"),
  });

  const remove = useMutation({
    mutationFn: (k: string) => api.deleteAppVariable(appId, kind, k, scope),
    onSuccess: () => {
      toast.success("Removed");
      invalidate();
    },
    onError: (e) =>
      toast.error(e instanceof Error ? e.message : "Failed to remove"),
  });

  const list = variables ?? [];

  return (
    <div className="space-y-4">
      {/* Scope selector */}
      <div className="flex flex-wrap items-center gap-3">
        <Select value={scope} onValueChange={(v) => setScope(v as Scope)}>
          <SelectTrigger className="w-[160px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {SCOPES.map((s) => (
              <SelectItem key={s} value={s}>
                {VARIABLE_SCOPE_META[s].label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Badge variant="outline" className={scopeMeta.badgeClass}>
          {scopeMeta.label}
        </Badge>
        <span className="text-xs text-muted-foreground">
          {scopeMeta.description}
        </span>
      </div>

      {/* Add row */}
      <div className="flex flex-wrap items-center gap-2">
        <Input
          className="w-[220px] font-mono"
          placeholder="KEY"
          value={key}
          onChange={(e) => setKey(e.target.value)}
        />
        <Input
          className="min-w-[220px] flex-1 font-mono"
          type={isSecret ? "password" : "text"}
          placeholder={isSecret ? "secret value" : "value"}
          value={value}
          onChange={(e) => setValue(e.target.value)}
        />
        <Button
          onClick={() => upsert.mutate()}
          disabled={!key.trim() || !value || upsert.isPending}
        >
          {upsert.isPending ? (
            <LoaderCircle className="h-4 w-4 animate-spin" />
          ) : (
            <Plus className="h-4 w-4" />
          )}
          Add
        </Button>
      </div>

      {/* List */}
      <div className="divide-y divide-border rounded-lg border">
        {list.length === 0 ? (
          <p className="px-4 py-8 text-center text-sm text-muted-foreground">
            No {nounPlural} in the {scopeMeta.label.toLowerCase()} scope yet.
          </p>
        ) : (
          list.map((v) => {
            const deleting =
              remove.isPending && remove.variables === v.key;
            return (
              <div
                key={v.id}
                className="flex items-center justify-between gap-3 px-4 py-3"
              >
                <span className="min-w-0 break-all font-mono text-sm text-foreground">
                  {v.key}
                </span>
                <div className="flex shrink-0 items-center gap-2">
                  <Badge
                    variant="secondary"
                    className="max-w-[220px] truncate font-mono text-xs text-muted-foreground"
                  >
                    {v.value}
                  </Badge>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 text-muted-foreground hover:text-destructive"
                    disabled={deleting}
                    onClick={() => remove.mutate(v.key)}
                    title={`Remove ${noun}`}
                  >
                    {deleting ? (
                      <LoaderCircle className={cn("h-4 w-4 animate-spin")} />
                    ) : (
                      <Trash2 className="h-4 w-4" />
                    )}
                  </Button>
                </div>
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
