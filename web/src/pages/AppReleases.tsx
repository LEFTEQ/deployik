import { useState } from "react";
import { useParams } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { RotateCcw } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { RELEASE_STATUS_META } from "@/lib/app-helpers";
import { formatRelativeDate } from "@/lib/deployment-helpers";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { LoadingState } from "@/components/ui/spinner";

type Environment = "preview" | "production";

export function AppReleases() {
  const { appId } = useParams({ strict: false }) as { appId: string };
  const queryClient = useQueryClient();
  const [environment, setEnvironment] = useState<Environment>("production");

  const { data: releases, isLoading } = useQuery({
    queryKey: queryKeys.appReleases(appId, environment),
    queryFn: () => api.listAppReleases(appId, environment),
  });
  const rollback = useMutation({
    mutationFn: (releaseId: string) => api.rollbackApp(appId, environment, releaseId),
    onSuccess: () => {
      toast.success(`Rolling back ${environment}`);
      queryClient.invalidateQueries({ queryKey: queryKeys.appReleases(appId, environment) });
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to roll back"),
  });

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">Releases</h2>
          <p className="text-sm text-muted-foreground">Coordinated {environment} deploys. Roll back to redeploy every member to that release.</p>
        </div>
        <Select value={environment} onValueChange={(v) => setEnvironment(v as Environment)}>
          <SelectTrigger className="w-[150px]"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="production">Production</SelectItem>
            <SelectItem value="preview">Preview</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {isLoading ? (
        <LoadingState title="Loading releases…" />
      ) : !releases?.length ? (
        <Card><CardContent className="py-12 text-center text-sm text-muted-foreground">No {environment} releases yet.</CardContent></Card>
      ) : (
        <div className="divide-y divide-border rounded-lg border">
          {releases.map((r) => {
            const meta = RELEASE_STATUS_META[r.status];
            return (
              <div key={r.id} className="flex items-center justify-between gap-3 px-4 py-3">
                <div className="flex items-center gap-3">
                  <Badge variant="outline" className={meta.badgeClass}>{meta.label}</Badge>
                  <span className="font-mono text-xs text-muted-foreground">{r.id.slice(0, 12)}</span>
                  <span className="text-xs text-muted-foreground">{formatRelativeDate(r.created_at)}</span>
                  {r.members?.length ? <span className="text-xs text-muted-foreground">{r.members.length} member(s)</span> : null}
                </div>
                {(r.status === "succeeded" || r.status === "rolled_back") && (
                  <Button variant="ghost" size="sm" disabled={rollback.isPending} onClick={() => rollback.mutate(r.id)}>
                    <RotateCcw className="h-3.5 w-3.5" /> Roll back
                  </Button>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
