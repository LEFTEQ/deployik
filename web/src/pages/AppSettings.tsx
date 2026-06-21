import { useState } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ArrowDown, ArrowUp, Plus, Trash2, X } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { LoadingState } from "@/components/ui/spinner";
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

export function AppSettings() {
  const { appId } = useParams({ strict: false }) as { appId: string };
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [addOpen, setAddOpen] = useState(false);
  const [selected, setSelected] = useState<Record<string, boolean>>({});

  const { data: health, isLoading } = useQuery({
    queryKey: queryKeys.appHealth(appId, "production"),
    queryFn: () => api.getAppHealth(appId, "production"),
  });
  const { data: allProjects } = useQuery({
    queryKey: queryKeys.projects("all"),
    queryFn: () => api.listProjects(),
  });

  const [name, setName] = useState("");
  const app = health?.app;
  const members = health?.members ?? [];
  const memberIds = new Set(members.map((m) => m.project.id));
  const addable = (allProjects ?? []).filter(
    (p) => !p.app_id && !memberIds.has(p.id),
  );

  const invalidate = () => {
    queryClient.invalidateQueries({
      queryKey: queryKeys.appHealth(appId, "production"),
    });
    queryClient.invalidateQueries({ queryKey: queryKeys.apps() });
    queryClient.invalidateQueries({ queryKey: ["projects"] });
  };

  const renameMut = useMutation({
    mutationFn: () => api.updateApp(appId, { name: name.trim() }),
    onSuccess: () => {
      toast.success("Renamed");
      invalidate();
    },
    onError: (e) =>
      toast.error(e instanceof Error ? e.message : "Failed to rename"),
  });
  const orderedMut = useMutation({
    mutationFn: (v: boolean) => api.updateApp(appId, { deploy_ordered: v }),
    onSuccess: () => invalidate(),
    onError: (e) =>
      toast.error(e instanceof Error ? e.message : "Failed to update"),
  });
  const reorderMut = useMutation({
    mutationFn: (ids: string[]) => api.reorderAppMembers(appId, ids),
    onSuccess: () => invalidate(),
    onError: (e) =>
      toast.error(e instanceof Error ? e.message : "Failed to reorder"),
  });
  const addMut = useMutation({
    mutationFn: (ids: string[]) => api.addProjectsToApp(appId, ids),
    onSuccess: () => {
      toast.success("Projects added");
      setAddOpen(false);
      setSelected({});
      invalidate();
    },
    onError: (e) =>
      toast.error(e instanceof Error ? e.message : "Failed to add"),
  });
  const removeMut = useMutation({
    mutationFn: (projectId: string) =>
      api.removeProjectFromApp(appId, projectId),
    onSuccess: () => {
      toast.success("Removed");
      invalidate();
    },
    onError: (e) =>
      toast.error(e instanceof Error ? e.message : "Failed to remove"),
  });
  const deleteMut = useMutation({
    mutationFn: () => api.deleteApp(appId),
    onError: (e) =>
      toast.error(e instanceof Error ? e.message : "Failed to delete"),
  });

  if (isLoading) return <LoadingState title="Loading settings…" />;

  const orderedIds = members.map((m) => m.project.id);
  const move = (index: number, dir: -1 | 1) => {
    const next = [...orderedIds];
    const target = index + dir;
    if (target < 0 || target >= next.length) return;
    const current = next[index];
    const swapped = next[target];
    if (current === undefined || swapped === undefined) return;
    next[index] = swapped;
    next[target] = current;
    reorderMut.mutate(next);
  };
  const selectedIds = Object.keys(selected).filter((id) => selected[id]);

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">App name</CardTitle>
        </CardHeader>
        <CardContent className="flex items-center gap-2">
          <Input
            className="max-w-sm"
            placeholder={app?.name}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          <Button
            onClick={() => renameMut.mutate()}
            disabled={!name.trim() || renameMut.isPending}
          >
            Save
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div>
            <CardTitle className="text-base">Deploy order</CardTitle>
            <p className="text-sm text-muted-foreground">
              When on, members deploy in the order below; otherwise in parallel.
            </p>
          </div>
          <Switch
            checked={!!app?.deploy_ordered}
            onCheckedChange={(v) => orderedMut.mutate(v)}
          />
        </CardHeader>
        <CardContent className="space-y-2">
          {members.length === 0 ? (
            <p className="py-4 text-center text-sm text-muted-foreground">
              No members yet.
            </p>
          ) : (
            members.map((m, i) => (
              <div
                key={m.project.id}
                className="flex items-center justify-between rounded-md border px-3 py-2"
              >
                <div className="flex items-center gap-3">
                  {app?.deploy_ordered && (
                    <span className="font-mono text-xs text-muted-foreground">
                      {i + 1}
                    </span>
                  )}
                  <span className="font-medium">{m.project.name}</span>
                  <Badge variant="secondary" className="text-xs">
                    {m.project.framework}
                  </Badge>
                </div>
                <div className="flex items-center gap-1">
                  {app?.deploy_ordered && (
                    <>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7"
                        disabled={i === 0 || reorderMut.isPending}
                        onClick={() => move(i, -1)}
                        title="Move up"
                      >
                        <ArrowUp className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7"
                        disabled={
                          i === members.length - 1 || reorderMut.isPending
                        }
                        onClick={() => move(i, 1)}
                        title="Move down"
                      >
                        <ArrowDown className="h-4 w-4" />
                      </Button>
                    </>
                  )}
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7"
                    onClick={() => removeMut.mutate(m.project.id)}
                    title="Remove from app"
                  >
                    <X className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            ))
          )}
          <Button variant="outline" size="sm" onClick={() => setAddOpen(true)}>
            <Plus className="h-4 w-4" /> Add projects
          </Button>
        </CardContent>
      </Card>

      <Card className="border-destructive/40">
        <CardHeader>
          <CardTitle className="text-base text-destructive">
            Danger zone
          </CardTitle>
        </CardHeader>
        <CardContent>
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button
                variant="outline"
                className="border-destructive/40 text-destructive"
              >
                <Trash2 className="h-4 w-4" /> Delete app
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Delete this app?</AlertDialogTitle>
                <AlertDialogDescription>
                  The app bundle is removed. Member projects survive and become
                  standalone.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction
                  onClick={() =>
                    deleteMut.mutate(undefined, {
                      onSuccess: () => {
                        toast.success("App deleted");
                        queryClient.invalidateQueries({
                          queryKey: queryKeys.apps(),
                        });
                        navigate({ to: "/apps" });
                      },
                    })
                  }
                >
                  Delete
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </CardContent>
      </Card>

      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add projects</DialogTitle>
            <DialogDescription>
              Only standalone projects (not already in an app) can be added.
            </DialogDescription>
          </DialogHeader>
          <div className="max-h-72 space-y-1 overflow-y-auto">
            {addable.length === 0 ? (
              <p className="py-4 text-center text-sm text-muted-foreground">
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
              disabled={selectedIds.length === 0 || addMut.isPending}
              onClick={() => addMut.mutate(selectedIds)}
            >
              Add {selectedIds.length || ""}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
