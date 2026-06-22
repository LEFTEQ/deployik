import { useState } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  ArrowDown,
  ArrowUp,
  Boxes,
  ListOrdered,
  Pencil,
  Plus,
  Trash2,
  TriangleAlert,
  X,
} from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import {
  ACTIVE_MEMBER_STATUSES,
  MEMBER_STATUS_META,
} from "@/lib/app-helpers";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
import { cn } from "@/lib/utils";
import type { AppHealthMember } from "@/types/api";

const REVEAL =
  "animate-in fade-in slide-in-from-bottom-2 duration-500 [animation-fill-mode:both]";
const reveal = (ms: number) => ({
  className: REVEAL,
  style: { animationDelay: `${ms}ms` },
});

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
  const ordered = !!app?.deploy_ordered;

  return (
    <div className="space-y-7">
      {/* Hero */}
      <div {...reveal(0)} className={cn(REVEAL, "space-y-1.5")}>
        <div className="flex flex-wrap items-center gap-2">
          <span className="flex h-9 w-9 items-center justify-center rounded-lg border border-primary/20 bg-primary/10 text-primary">
            <Boxes className="h-5 w-5" />
          </span>
          <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
        </div>
        <p className="text-sm text-muted-foreground">
          Rename {app?.name ?? "this app"}, manage its members, and control how
          they roll out together.
        </p>
      </div>

      {/* App name */}
      <section {...reveal(60)} className={cn(REVEAL, "space-y-3")}>
        <h2 className="flex items-center gap-2 text-sm font-semibold text-foreground">
          <Pencil className="h-4 w-4 text-muted-foreground" />
          App name
        </h2>
        <div className="flex flex-wrap items-center gap-2 rounded-lg border p-4">
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
        </div>
      </section>

      {/* Deploy order + members */}
      <section {...reveal(120)} className={cn(REVEAL, "space-y-3")}>
        <div className="flex items-start justify-between gap-3">
          <div className="space-y-0.5">
            <h2 className="flex items-center gap-2 text-sm font-semibold text-foreground">
              <ListOrdered className="h-4 w-4 text-muted-foreground" />
              Deploy order
            </h2>
            <p className="text-xs text-muted-foreground">
              When on, members deploy in the order below; otherwise in parallel.
            </p>
          </div>
          <Switch
            checked={ordered}
            onCheckedChange={(v) => orderedMut.mutate(v)}
          />
        </div>
        {members.length === 0 ? (
          <p className="rounded-lg border border-dashed py-8 text-center text-sm text-muted-foreground">
            No members yet.
          </p>
        ) : (
          <div className="divide-y divide-border overflow-hidden rounded-lg border">
            {members.map((m, i) => (
              <MemberRow
                key={m.project.id}
                member={m}
                index={i}
                ordered={ordered}
                isFirst={i === 0}
                isLast={i === members.length - 1}
                reordering={reorderMut.isPending}
                onMove={(dir) => move(i, dir)}
                onRemove={() => removeMut.mutate(m.project.id)}
              />
            ))}
          </div>
        )}
        <Button variant="outline" size="sm" onClick={() => setAddOpen(true)}>
          <Plus className="h-4 w-4" /> Add projects
        </Button>
      </section>

      {/* Danger zone */}
      <section {...reveal(180)} className={cn(REVEAL, "space-y-3")}>
        <h2 className="flex items-center gap-2 text-sm font-semibold text-destructive">
          <TriangleAlert className="h-4 w-4" />
          Danger zone
        </h2>
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-destructive/40 p-4">
          <p className="text-sm text-muted-foreground">
            Deleting the app bundle keeps member projects — they become
            standalone.
          </p>
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button
                variant="outline"
                className="border-destructive/40 text-destructive hover:bg-destructive/10 hover:text-destructive"
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
        </div>
      </section>

      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add projects</DialogTitle>
            <DialogDescription>
              Only standalone projects (not already in an app) can be added.
            </DialogDescription>
          </DialogHeader>
          <div className="max-h-72 space-y-1.5 overflow-y-auto py-1">
            {addable.length === 0 ? (
              <p className="rounded-lg border border-dashed py-8 text-center text-sm text-muted-foreground">
                No standalone projects available.
              </p>
            ) : (
              addable.map((p) => {
                const checked = !!selected[p.id];
                return (
                  <label
                    key={p.id}
                    className={cn(
                      "flex cursor-pointer items-center gap-3 rounded-md border px-3 py-2.5 transition-colors",
                      checked
                        ? "border-primary/40 bg-primary/5"
                        : "border-white/8 hover:bg-white/[0.04]",
                    )}
                  >
                    <input
                      type="checkbox"
                      className="h-4 w-4 accent-primary"
                      checked={checked}
                      onChange={(e) =>
                        setSelected((s) => ({
                          ...s,
                          [p.id]: e.target.checked,
                        }))
                      }
                    />
                    <span className="truncate text-sm font-medium">
                      {p.name}
                    </span>
                    <Badge
                      variant="outline"
                      className="ml-auto border-primary/20 bg-primary/10 font-mono text-[10px] text-primary"
                    >
                      {p.framework}
                    </Badge>
                  </label>
                );
              })
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
              <Plus className="h-4 w-4" /> Add {selectedIds.length || ""}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function MemberRow({
  member,
  index,
  ordered,
  isFirst,
  isLast,
  reordering,
  onMove,
  onRemove,
}: {
  member: AppHealthMember;
  index: number;
  ordered: boolean;
  isFirst: boolean;
  isLast: boolean;
  reordering: boolean;
  onMove: (dir: -1 | 1) => void;
  onRemove: () => void;
}) {
  const meta = MEMBER_STATUS_META[member.live_status];
  const active = ACTIVE_MEMBER_STATUSES.has(member.live_status);
  return (
    <div className="flex items-center gap-3 px-4 py-3">
      <span
        className={cn(
          "h-2.5 w-2.5 shrink-0 rounded-full",
          meta.dotClass,
          active && "animate-pulse",
        )}
      />
      <span className="truncate text-sm font-medium text-foreground">
        {member.project.name}
      </span>
      <Badge
        variant="outline"
        className="border-primary/20 bg-primary/10 font-mono text-[10px] text-primary"
      >
        {member.project.framework}
      </Badge>
      {ordered ? (
        <span className="font-mono text-[11px] text-muted-foreground">
          #{index + 1}
        </span>
      ) : null}
      <div className="ml-auto flex items-center gap-1">
        {ordered && (
          <>
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-7"
              disabled={isFirst || reordering}
              onClick={() => onMove(-1)}
              title="Move up"
            >
              <ArrowUp className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-7"
              disabled={isLast || reordering}
              onClick={() => onMove(1)}
              title="Move down"
            >
              <ArrowDown className="h-4 w-4" />
            </Button>
          </>
        )}
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7 text-muted-foreground hover:text-destructive"
          onClick={onRemove}
          title="Remove from app"
        >
          <X className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}
