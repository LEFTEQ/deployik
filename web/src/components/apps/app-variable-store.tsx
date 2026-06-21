import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Plus, Trash2 } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

type Scope = "shared" | "preview" | "production";

export function AppVariableStore({ appId, kind }: { appId: string; kind: "env" | "secret" }) {
  const queryClient = useQueryClient();
  const [scope, setScope] = useState<Scope>("shared");
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");

  const { data: variables } = useQuery({
    queryKey: queryKeys.appVariables(appId, kind, scope),
    queryFn: () => api.listAppVariables(appId, kind, scope),
  });

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: queryKeys.appVariables(appId, kind, scope) });

  const upsert = useMutation({
    mutationFn: () => api.upsertAppVariable(appId, kind, { key: key.trim(), value, environment: scope }),
    onSuccess: () => {
      toast.success(`${kind === "secret" ? "Secret" : "Variable"} saved`);
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
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to remove"),
  });

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <Select value={scope} onValueChange={(v) => setScope(v as Scope)}>
          <SelectTrigger className="w-[160px]"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="shared">Shared</SelectItem>
            <SelectItem value="preview">Preview</SelectItem>
            <SelectItem value="production">Production</SelectItem>
          </SelectContent>
        </Select>
        <span className="text-xs text-muted-foreground">
          Inherited by every member at deploy time (member vars override).
        </span>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Input className="w-[220px] font-mono" placeholder="KEY" value={key} onChange={(e) => setKey(e.target.value)} />
        <Input className="min-w-[220px] flex-1 font-mono" type={kind === "secret" ? "password" : "text"}
          placeholder={kind === "secret" ? "secret value" : "value"} value={value} onChange={(e) => setValue(e.target.value)} />
        <Button onClick={() => upsert.mutate()} disabled={!key.trim() || !value || upsert.isPending}>
          <Plus className="h-4 w-4" /> Add
        </Button>
      </div>

      <div className="divide-y divide-border rounded-lg border">
        {(variables ?? []).length === 0 ? (
          <p className="px-3 py-6 text-center text-sm text-muted-foreground">No {kind === "secret" ? "secrets" : "variables"} in {scope}.</p>
        ) : (
          (variables ?? []).map((v) => (
            <div key={v.id} className="flex items-center justify-between gap-3 px-3 py-2">
              <span className="font-mono text-sm">{v.key}</span>
              <div className="flex items-center gap-3">
                <Badge variant="secondary" className="font-mono text-xs">{v.value}</Badge>
                <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => remove.mutate(v.key)} title="Remove">
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
