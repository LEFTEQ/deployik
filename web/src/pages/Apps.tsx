import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Boxes, Plus, Layers } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import type { App } from "@/types/api";
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
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";

export function Apps() {
  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");

  const { data: apps, isLoading } = useQuery({
    queryKey: queryKeys.apps(),
    queryFn: () => api.listApps(),
  });

  const createMutation = useMutation({
    mutationFn: () => api.createApp({ name: name.trim() }),
    onSuccess: (app) => {
      toast.success(`Created app '${app.name}'`);
      queryClient.invalidateQueries({ queryKey: queryKeys.apps() });
      setCreateOpen(false);
      setName("");
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to create app"),
  });

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight flex items-center gap-2">
            <Boxes className="h-6 w-6" /> Apps
          </h1>
          <p className="text-sm text-muted-foreground mt-1">
            Bundle several projects into one app — shared network, app-level env, and coordinated deploys.
          </p>
        </div>
        <Button onClick={() => setCreateOpen(true)}>
          <Plus className="h-4 w-4" /> New App
        </Button>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner />
        </div>
      ) : !apps || apps.length === 0 ? (
        <Card className="border-dashed">
          <CardContent className="flex flex-col items-center justify-center gap-3 py-12 text-center">
            <Layers className="h-8 w-8 text-muted-foreground" />
            <div>
              <p className="font-medium">No apps yet</p>
              <p className="text-sm text-muted-foreground">
                Create an app, then add member projects to deploy them together.
              </p>
            </div>
            <Button variant="outline" onClick={() => setCreateOpen(true)}>
              <Plus className="h-4 w-4" /> New App
            </Button>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {apps.map((app: App) => (
            <Link key={app.id} to="/apps/$appId" params={{ appId: app.id }}>
              <Card className="transition-colors hover:border-primary/50">
                <CardHeader>
                  <CardTitle className="flex items-center gap-2 text-base">
                    <Boxes className="h-4 w-4" /> {app.name}
                  </CardTitle>
                  <CardDescription>
                    {app.project_count} project{app.project_count === 1 ? "" : "s"}
                    {app.deploy_ordered ? " · ordered deploy" : ""}
                  </CardDescription>
                </CardHeader>
              </Card>
            </Link>
          ))}
        </div>
      )}

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>New App</DialogTitle>
            <DialogDescription>
              An app groups projects in your workspace. You can add members after creating it.
            </DialogDescription>
          </DialogHeader>
          <Input
            autoFocus
            placeholder="App name (e.g. Forge acme)"
            value={name}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && name.trim()) createMutation.mutate();
            }}
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button
              disabled={!name.trim() || createMutation.isPending}
              onClick={() => createMutation.mutate()}
            >
              {createMutation.isPending ? "Creating…" : "Create app"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
