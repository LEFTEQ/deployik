import { useMemo, useState } from "react";
import { Link, useParams } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  ArrowLeft,
  Boxes,
  Plus,
  Rocket,
  RotateCcw,
  Trash2,
  X,
} from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import type { AppRelease } from "@/types/api";
import { formatRelativeDate } from "@/lib/deployment-helpers";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Spinner } from "@/components/ui/spinner";

type Environment = "preview" | "production";

const RELEASE_BADGE: Record<AppRelease["status"], string> = {
  succeeded: "bg-emerald-500/15 text-emerald-400 border-emerald-500/30",
  failed: "bg-red-500/15 text-red-400 border-red-500/30",
  rolled_back: "bg-amber-500/15 text-amber-400 border-amber-500/30",
  pending: "bg-zinc-500/15 text-zinc-400 border-zinc-500/30",
};

export function AppDetail() {
  const { appId } = useParams({ strict: false }) as { appId: string };
  const queryClient = useQueryClient();
  const [environment, setEnvironment] = useState<Environment>("production");
  const [addOpen, setAddOpen] = useState(false);
  const [selected, setSelected] = useState<Record<string, boolean>>({});

  const { data: health, isLoading } = useQuery({
    queryKey: queryKeys.appHealth(appId),
    queryFn: () => api.getAppHealth(appId),
  });
  const { data: releases } = useQuery({
    queryKey: queryKeys.appReleases(appId, environment),
    queryFn: () => api.listAppReleases(appId, environment),
  });
  const { data: allProjects } = useQuery({
    queryKey: queryKeys.projects("all"),
    queryFn: () => api.listProjects(),
  });

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: queryKeys.appHealth(appId) });
    queryClient.invalidateQueries({ queryKey: queryKeys.apps() });
    queryClient.invalidateQueries({ queryKey: ["projects"] });
  };

  const members = health?.members ?? [];
  const memberIds = useMemo(
    () => new Set(members.map((m) => m.project.id)),
    [members],
  );
  const addable = (allProjects ?? []).filter(
    (p) => !p.app_id && !memberIds.has(p.id),
  );

  const addMutation = useMutation({
    mutationFn: (ids: string[]) => api.addProjectsToApp(appId, ids),
    onSuccess: () => {
      toast.success("Projects added");
      setAddOpen(false);
      setSelected({});
      invalidate();
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to add projects"),
  });
  const removeMutation = useMutation({
    mutationFn: (projectId: string) => api.removeProjectFromApp(appId, projectId),
    onSuccess: () => {
      toast.success("Project removed");
      invalidate();
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to remove project"),
  });
  const deployMutation = useMutation({
    mutationFn: () => api.deployApp(appId, environment),
    onSuccess: (r) => {
      toast.success(`Deploying ${r.member_count} member(s) to ${environment}`);
      queryClient.invalidateQueries({ queryKey: queryKeys.appReleases(appId, environment) });
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to start deploy"),
  });
  const rollbackMutation = useMutation({
    mutationFn: (releaseId: string) => api.rollbackApp(appId, environment, releaseId),
    onSuccess: () => {
      toast.success(`Rolling back ${environment}`);
      queryClient.invalidateQueries({ queryKey: queryKeys.appReleases(appId, environment) });
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to start rollback"),
  });
  const deleteMutation = useMutation({
    mutationFn: () => api.deleteApp(appId),
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to delete app"),
  });
  const orderedMutation = useMutation({
    mutationFn: (deployOrdered: boolean) => api.updateApp(appId, { deploy_ordered: deployOrdered }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.appHealth(appId) });
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to update app"),
  });

  if (isLoading) {
    return (
      <div className="flex justify-center py-16">
        <Spinner />
      </div>
    );
  }

  const app = health?.app;
  const selectedIds = Object.keys(selected).filter((id) => selected[id]);

  return (
    <div className="space-y-6">
      <div>
        <Link
          to="/apps"
          className="text-sm text-muted-foreground hover:text-foreground inline-flex items-center gap-1"
        >
          <ArrowLeft className="h-3.5 w-3.5" /> Apps
        </Link>
        <div className="mt-2 flex flex-wrap items-center justify-between gap-3">
          <h1 className="text-2xl font-semibold tracking-tight flex items-center gap-2">
            <Boxes className="h-6 w-6" /> {app?.name}
          </h1>
          <div className="flex items-center gap-2">
            <Select value={environment} onValueChange={(v) => setEnvironment(v as Environment)}>
              <SelectTrigger className="w-[150px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="production">Production</SelectItem>
                <SelectItem value="preview">Preview</SelectItem>
              </SelectContent>
            </Select>
            <Button
              onClick={() => deployMutation.mutate()}
              disabled={deployMutation.isPending || members.length === 0}
            >
              <Rocket className="h-4 w-4" /> Deploy together
            </Button>
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button variant="outline" size="icon" title="Delete app">
                  <Trash2 className="h-4 w-4" />
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Delete this app?</AlertDialogTitle>
                  <AlertDialogDescription>
                    The app bundle is removed. Member projects survive and become standalone.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction
                    onClick={() =>
                      deleteMutation.mutate(undefined, {
                        onSuccess: () => {
                          toast.success("App deleted");
                          queryClient.invalidateQueries({ queryKey: queryKeys.apps() });
                          window.history.back();
                        },
                      })
                    }
                  >
                    Delete
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>
        </div>
      </div>

      {/* Members */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div>
            <CardTitle className="text-base">Members</CardTitle>
            <CardDescription>
              {members.length} project{members.length === 1 ? "" : "s"}
              {app?.deploy_ordered ? " · deployed in deploy_order" : " · deployed in parallel"}
            </CardDescription>
          </div>
          <div className="flex items-center gap-4">
            <label className="flex items-center gap-2 text-sm text-muted-foreground">
              <Switch
                checked={!!app?.deploy_ordered}
                onCheckedChange={(v) => orderedMutation.mutate(v)}
              />
              Ordered deploy
            </label>
            <Button variant="outline" size="sm" onClick={() => setAddOpen(true)}>
              <Plus className="h-4 w-4" /> Add projects
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-2">
          {members.length === 0 ? (
            <p className="text-sm text-muted-foreground py-4 text-center">
              No members yet. Add projects to deploy them together.
            </p>
          ) : (
            members.map((m) => (
              <div
                key={m.project.id}
                className="flex items-center justify-between rounded-md border px-3 py-2"
              >
                <div className="flex items-center gap-3">
                  <span className="font-medium">{m.project.name}</span>
                  <Badge variant="secondary" className="text-xs">
                    {m.project.framework}
                  </Badge>
                  {app?.deploy_ordered && (
                    <span className="text-xs text-muted-foreground">
                      order {m.project.deploy_order}
                    </span>
                  )}
                </div>
                <div className="flex items-center gap-3">
                  <span className="text-xs text-muted-foreground">
                    prod{" "}
                    {m.latest_production_deploy_at
                      ? formatRelativeDate(m.latest_production_deploy_at)
                      : "—"}
                  </span>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7"
                    title="Remove from app"
                    onClick={() => removeMutation.mutate(m.project.id)}
                  >
                    <X className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            ))
          )}
        </CardContent>
      </Card>

      {/* Releases */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Release history</CardTitle>
          <CardDescription>
            Coordinated {environment} deploys. Roll back to redeploy every member to that release.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-2">
          {!releases || releases.length === 0 ? (
            <p className="text-sm text-muted-foreground py-4 text-center">
              No {environment} releases yet.
            </p>
          ) : (
            releases.map((r) => (
              <div
                key={r.id}
                className="flex items-center justify-between rounded-md border px-3 py-2"
              >
                <div className="flex items-center gap-3">
                  <Badge variant="outline" className={RELEASE_BADGE[r.status]}>
                    {r.status}
                  </Badge>
                  <span className="text-xs text-muted-foreground font-mono">
                    {r.id.slice(0, 10)}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {formatRelativeDate(r.created_at)}
                  </span>
                </div>
                {(r.status === "succeeded" || r.status === "rolled_back") && (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => rollbackMutation.mutate(r.id)}
                    disabled={rollbackMutation.isPending}
                  >
                    <RotateCcw className="h-3.5 w-3.5" /> Roll back
                  </Button>
                )}
              </div>
            ))
          )}
        </CardContent>
      </Card>

      {/* Add projects dialog */}
      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add projects to {app?.name}</DialogTitle>
            <DialogDescription>
              Only standalone projects (not already in an app) can be added.
            </DialogDescription>
          </DialogHeader>
          <div className="max-h-72 space-y-1 overflow-y-auto">
            {addable.length === 0 ? (
              <p className="text-sm text-muted-foreground py-4 text-center">
                No standalone projects available.
              </p>
            ) : (
              addable.map((p) => (
                <label
                  key={p.id}
                  className="flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2 hover:bg-accent"
                >
                  <input
                    type="checkbox"
                    checked={!!selected[p.id]}
                    onChange={(e) =>
                      setSelected((s) => ({ ...s, [p.id]: e.target.checked }))
                    }
                  />
                  <span className="font-medium">{p.name}</span>
                  <Badge variant="secondary" className="text-xs">
                    {p.framework}
                  </Badge>
                </label>
              ))
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAddOpen(false)}>
              Cancel
            </Button>
            <Button
              disabled={selectedIds.length === 0 || addMutation.isPending}
              onClick={() => addMutation.mutate(selectedIds)}
            >
              Add {selectedIds.length || ""}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
