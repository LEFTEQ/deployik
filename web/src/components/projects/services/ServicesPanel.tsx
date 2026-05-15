import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  Database,
  Plus,
  RefreshCw,
  RotateCcw,
  ScrollText,
  Settings2,
  Trash2,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import type { ServiceStatus } from "@/types/api";

import { CredentialsDialog } from "./CredentialsDialog";

const STATUS_TONE: Record<ServiceStatus, string> = {
  pending: "bg-amber-500/20 text-amber-200",
  running: "bg-green-500/20 text-green-200",
  stopped: "bg-zinc-500/20 text-zinc-200",
  failed: "bg-red-500/20 text-red-200",
};

export function ServicesPanel({
  projectId,
  projectName,
}: {
  projectId: string;
  projectName: string;
}) {
  const queryClient = useQueryClient();
  const [credentialsEnv, setCredentialsEnv] = useState<string | null>(null);

  const services = useQuery({
    queryKey: queryKeys.projectServices(projectId),
    queryFn: () => api.listServices(projectId),
  });

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: queryKeys.projectServices(projectId),
    });

  const attach = useMutation({
    mutationFn: (environment: "preview" | "production") =>
      api.attachService(projectId, { environment, type: "postgres" }),
    onSuccess: () => {
      invalidate();
      toast.success("Postgres attached. It'll start on the next deploy.");
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const detach = useMutation({
    mutationFn: (environment: string) =>
      api.detachService(projectId, environment),
    onSuccess: () => {
      invalidate();
      toast.success("Service detached.");
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const restart = useMutation({
    mutationFn: (environment: string) =>
      api.restartService(projectId, environment),
    onSuccess: () => {
      invalidate();
      toast.success("Postgres restarted.");
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const handleReset = async (environment: string) => {
    const expected = `${projectName}-${environment}`;
    const got = window.prompt(
      `Type "${expected}" to wipe ${environment}'s database (DESTRUCTIVE — data is gone forever):`,
    );
    if (got !== expected) {
      if (got !== null) toast.error("Confirm string did not match; reset cancelled.");
      return;
    }
    try {
      await api.resetService(projectId, environment, got);
      invalidate();
      toast.success(`${environment} database reset.`);
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const handleDetach = (environment: string) => {
    if (
      !window.confirm(
        `Detach Postgres from ${environment}? This also deletes the data volume.`,
      )
    ) {
      return;
    }
    detach.mutate(environment);
  };

  const byEnv = new Map(services.data?.map((s) => [s.environment, s]) ?? []);

  return (
    <>
      <div className="grid gap-4 md:grid-cols-2">
        {(["preview", "production"] as const).map((environment) => {
          const svc = byEnv.get(environment);
          return (
            <div
              key={environment}
              className="rounded-2xl border border-white/10 bg-black/10 p-4 space-y-3"
            >
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Database className="size-4" />
                  <span className="font-medium capitalize">{environment}</span>
                </div>
                {svc ? (
                  <Badge className={STATUS_TONE[svc.status]}>{svc.status}</Badge>
                ) : (
                  <Badge variant="outline">not attached</Badge>
                )}
              </div>

              {svc ? (
                <>
                  <div className="text-xs text-muted-foreground space-y-1">
                    <div>
                      image: <code>{svc.image}</code>
                    </div>
                    <div>
                      db: <code>{svc.db_name}</code> · user:{" "}
                      <code>{svc.db_user}</code>
                    </div>
                    <div>
                      host port:{" "}
                      <code>{svc.host_port > 0 ? svc.host_port : "—"}</code>
                    </div>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => setCredentialsEnv(environment)}
                    >
                      <Settings2 className="mr-1.5 size-3.5" />
                      Connect
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => restart.mutate(environment)}
                      disabled={restart.isPending}
                    >
                      <RotateCcw className="mr-1.5 size-3.5" />
                      Restart
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => handleReset(environment)}
                    >
                      <RefreshCw className="mr-1.5 size-3.5" />
                      Reset
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled
                      title="Logs panel coming in Phase 3"
                    >
                      <ScrollText className="mr-1.5 size-3.5" />
                      Logs
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      className="text-red-300"
                      onClick={() => handleDetach(environment)}
                    >
                      <Trash2 className="mr-1.5 size-3.5" />
                      Detach
                    </Button>
                  </div>
                </>
              ) : (
                <Button
                  size="sm"
                  onClick={() => attach.mutate(environment)}
                  disabled={attach.isPending}
                >
                  <Plus className="mr-1.5 size-3.5" />
                  Attach Postgres
                </Button>
              )}
            </div>
          );
        })}
      </div>

      {credentialsEnv && (
        <CredentialsDialog
          projectId={projectId}
          environment={credentialsEnv}
          onClose={() => setCredentialsEnv(null)}
        />
      )}
    </>
  );
}
