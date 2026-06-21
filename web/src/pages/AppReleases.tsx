import { useState } from "react";
import { useParams } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { History, RotateCcw, Users } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { RELEASE_STATUS_META } from "@/lib/app-helpers";
import { formatRelativeDate } from "@/lib/deployment-helpers";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { LoadingState } from "@/components/ui/spinner";
import { cn } from "@/lib/utils";

type Environment = "preview" | "production";

const REVEAL = "animate-in fade-in slide-in-from-bottom-2 duration-500 [animation-fill-mode:both]";

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

  const list = releases ?? [];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div
        className={cn(REVEAL, "flex flex-wrap items-center justify-between gap-3")}
        style={{ animationDelay: "0ms" }}
      >
        <div className="space-y-1">
          <h2 className="text-base font-semibold">Releases</h2>
          <p className="text-sm text-muted-foreground">
            Coordinated {environment} deploys. Roll back to redeploy every member to that release.
          </p>
        </div>
        <Select value={environment} onValueChange={(v) => setEnvironment(v as Environment)}>
          <SelectTrigger className="w-[150px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="production">Production</SelectItem>
            <SelectItem value="preview">Preview</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {isLoading ? (
        <LoadingState title="Loading releases…" />
      ) : list.length === 0 ? (
        <Card className={REVEAL} style={{ animationDelay: "60ms" }}>
          <CardContent className="flex flex-col items-center gap-3 py-14 text-center">
            <span className="flex h-11 w-11 items-center justify-center rounded-full border border-border bg-muted/40 text-muted-foreground">
              <History className="h-5 w-5" />
            </span>
            <p className="text-sm text-muted-foreground">No {environment} releases yet.</p>
          </CardContent>
        </Card>
      ) : (
        <Card
          className={cn(REVEAL, "overflow-hidden bg-gradient-to-t from-primary/5 to-card")}
          style={{ animationDelay: "60ms" }}
        >
          <CardHeader className="flex flex-row items-center justify-between gap-3 pb-3">
            <div className="flex items-center gap-2">
              <History className="h-4 w-4 text-muted-foreground" />
              <CardTitle className="text-base">Release timeline</CardTitle>
            </div>
            <span className="text-xs text-muted-foreground">
              {list.length} {environment} release{list.length === 1 ? "" : "s"}
            </span>
          </CardHeader>
          <CardContent className="p-0">
            <div className="divide-y divide-border border-t">
              {list.map((r, i) => {
                const meta = RELEASE_STATUS_META[r.status];
                const memberCount = r.members?.length ?? 0;
                const canRollback = r.status === "succeeded" || r.status === "rolled_back";
                const isLast = i === list.length - 1;
                return (
                  <div
                    key={r.id}
                    className={cn(REVEAL, "flex items-center gap-4 px-6 py-4")}
                    style={{ animationDelay: `${120 + i * 40}ms` }}
                  >
                    {/* Timeline rail: connector + dot */}
                    <div className="relative flex w-2.5 shrink-0 flex-col items-center self-stretch">
                      <span
                        className={cn(
                          "h-2.5 w-2.5 shrink-0 rounded-full ring-4 ring-card",
                          meta.dotClass,
                          r.status === "pending" && "animate-pulse",
                        )}
                      />
                      {!isLast && (
                        <span className="absolute top-3 bottom-[-1rem] w-px bg-border" aria-hidden />
                      )}
                    </div>

                    <div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-1">
                      <Badge variant="outline" className={meta.badgeClass}>
                        {meta.label}
                      </Badge>
                      <span className="font-mono text-xs text-muted-foreground">
                        {r.id.slice(0, 12)}
                      </span>
                      <span className="text-xs text-muted-foreground">
                        {formatRelativeDate(r.created_at)}
                      </span>
                      {memberCount > 0 && (
                        <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
                          <Users className="h-3 w-3" />
                          {memberCount} member{memberCount === 1 ? "" : "s"}
                        </span>
                      )}
                    </div>

                    {canRollback && (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="shrink-0"
                        disabled={rollback.isPending}
                        onClick={() => rollback.mutate(r.id)}
                      >
                        <RotateCcw className="h-3.5 w-3.5" /> Roll back
                      </Button>
                    )}
                  </div>
                );
              })}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
