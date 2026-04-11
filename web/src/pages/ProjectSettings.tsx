import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "@tanstack/react-router";
import { GitBranch, Trash2, Webhook } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import {
  BuildSettingsFields,
} from "@/components/projects/build-settings";
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
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { LoadingState } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";

export function ProjectSettings() {
  const { id } = useParams({ strict: false }) as { id: string };
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const { data: project, isLoading } = useQuery({
    queryKey: ["project", id],
    queryFn: () => api.getProject(id),
  });

  if (isLoading || !project) {
    return (
      <LoadingState
        title="Loading settings..."
        description="Fetching project configuration."
        className="min-h-[420px]"
      />
    );
  }

  return (
    <div className="space-y-8">
      <BuildSettingsSection project={project} />
      <div className="border-b" />
      <AutoBuildSection projectId={id} defaultBranch={project.branch} />
      <div className="border-b" />
      <DangerZone
        projectId={id}
        onDeleted={() => {
          queryClient.invalidateQueries({ queryKey: ["projects"] });
          toast.success("Project deleted");
          navigate({ to: "/" });
        }}
      />
    </div>
  );
}

function BuildSettingsSection({
  project,
}: {
  project: NonNullable<Awaited<ReturnType<typeof api.getProject>>>;
}) {
  const queryClient = useQueryClient();
  const [branch, setBranch] = useState(project.branch);
  const [buildSettings, setBuildSettings] = useState({
    framework: project.framework,
    packageManager: project.package_manager,
    rootDirectory: project.root_directory,
    outputDirectory: project.output_directory,
    buildCommand: project.build_command,
    installCommand: project.install_command,
    nodeVersion: project.node_version,
  });

  const updateMutation = useMutation({
    mutationFn: () =>
      api.updateProject(project.id, {
        branch,
        framework: buildSettings.framework,
        package_manager: buildSettings.packageManager,
        root_directory: buildSettings.rootDirectory,
        output_directory: buildSettings.outputDirectory,
        build_command: buildSettings.buildCommand,
        install_command: buildSettings.installCommand,
        node_version: buildSettings.nodeVersion,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["project", project.id] });
      toast.success("Settings updated");
    },
    onError: (err) => toast.error(err.message),
  });

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-base font-semibold">Build Settings</h2>
        <p className="text-sm text-muted-foreground">
          Source control branch, framework defaults, and package/runtime
          behavior for this project.
        </p>
      </div>
      <div className="space-y-2">
        <Label>Workspace</Label>
        <Input value={project.organization_name || "Personal"} disabled />
      </div>
      <div className="space-y-2">
        <Label>Branch</Label>
        <Input
          value={branch}
          onChange={(event) => setBranch(event.target.value)}
        />
      </div>
      <BuildSettingsFields
        value={buildSettings}
        onChange={setBuildSettings}
      />
      <Button
        onClick={() => updateMutation.mutate()}
        disabled={updateMutation.isPending}
      >
        {updateMutation.isPending ? "Saving..." : "Save Settings"}
      </Button>
    </div>
  );
}

function AutoBuildSection({
  projectId,
  defaultBranch,
}: {
  projectId: string;
  defaultBranch: string;
}) {
  const queryClient = useQueryClient();
  const [needsReauth, setNeedsReauth] = useState(false);
  const [noAdminAccess, setNoAdminAccess] = useState<string | null>(null);

  const { data: config } = useQuery({
    queryKey: ["auto-build", projectId],
    queryFn: () => api.getAutoBuildConfig(projectId).catch(() => null),
  });

  const enabled = config?.enabled ?? false;

  const [productionBranch, setProductionBranch] = useState(
    config?.production_branch || defaultBranch || "main",
  );
  const [previewBranches, setPreviewBranches] = useState(
    config?.preview_branches || "*",
  );

  const updateMutation = useMutation({
    mutationFn: (data: {
      enabled: boolean;
      production_branch: string;
      preview_branches: string;
    }) => api.updateAutoBuildConfig(projectId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["auto-build", projectId] });
      setNeedsReauth(false);
      toast.success("Auto-build updated");
    },
    onError: (err) => {
      const msg = err.message || String(err);
      if (msg.includes("insufficient_scope")) {
        setNeedsReauth(true);
      } else if (msg.includes("no_admin_access") || msg.includes("admin access")) {
        setNoAdminAccess(msg);
      } else {
        toast.error(msg);
      }
    },
  });

  const handleToggle = (checked: boolean) => {
    updateMutation.mutate({
      enabled: checked,
      production_branch: productionBranch,
      preview_branches: previewBranches,
    });
  };

  const handleSave = () => {
    updateMutation.mutate({
      enabled: true,
      production_branch: productionBranch,
      preview_branches: previewBranches,
    });
  };

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <Webhook className="h-4 w-4 text-muted-foreground" />
            <h2 className="text-base font-semibold">Auto-Build on Push</h2>
            {enabled && (
              <Badge
                variant="outline"
                className="border-emerald-400/25 bg-emerald-400/12 text-emerald-100"
              >
                Active
              </Badge>
            )}
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            Automatically deploy when you push to GitHub. Configure which
            branches trigger preview and production deployments.
          </p>
        </div>
        <Switch
          checked={enabled}
          onCheckedChange={handleToggle}
          disabled={updateMutation.isPending}
        />
      </div>

      {noAdminAccess && (
        <div className="rounded-lg border border-rose-500/25 bg-rose-500/5 p-4">
          <p className="text-sm font-medium text-rose-200">
            No admin access to this repository
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            {noAdminAccess}
          </p>
        </div>
      )}

      {needsReauth && (
        <div className="rounded-lg border border-amber-500/25 bg-amber-500/5 p-4">
          <p className="text-sm font-medium text-amber-200">
            GitHub permissions need updating
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            Your account was authorized before webhook support was added.
            Re-authorize to grant webhook permissions.
          </p>
          <Button size="sm" className="mt-3" asChild>
            <a href="/api/auth/github">Re-authorize with GitHub</a>
          </Button>
        </div>
      )}

      {enabled && (
        <div className="space-y-4 rounded-lg border p-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label className="flex items-center gap-1.5">
                <GitBranch className="h-3.5 w-3.5" />
                Production Branch
              </Label>
              <Input
                value={productionBranch}
                onChange={(e) => setProductionBranch(e.target.value)}
                placeholder="main"
              />
              <p className="text-xs text-muted-foreground">
                Pushes to this branch trigger a production deployment.
              </p>
            </div>
            <div className="space-y-2">
              <Label className="flex items-center gap-1.5">
                <GitBranch className="h-3.5 w-3.5" />
                Preview Branches
              </Label>
              <Input
                value={previewBranches}
                onChange={(e) => setPreviewBranches(e.target.value)}
                placeholder="* (all other branches)"
              />
              <p className="text-xs text-muted-foreground">
                Use * for all branches, or comma-separated names.
              </p>
            </div>
          </div>
          <Button
            size="sm"
            onClick={handleSave}
            disabled={updateMutation.isPending}
          >
            {updateMutation.isPending ? "Saving..." : "Save Branch Config"}
          </Button>
        </div>
      )}
    </div>
  );
}

function DangerZone({
  projectId,
  onDeleted,
}: {
  projectId: string;
  onDeleted: () => void;
}) {
  const deleteMutation = useMutation({
    mutationFn: () => api.deleteProject(projectId),
    onSuccess: onDeleted,
    onError: (err) => toast.error(err.message),
  });

  return (
    <Card className="border-destructive/50">
      <CardHeader>
        <CardTitle className="text-base text-destructive">
          Danger Zone
        </CardTitle>
      </CardHeader>
      <CardContent>
        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button variant="destructive">
              <Trash2 className="mr-1.5 h-3.5 w-3.5" />
              Delete Project
            </Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Delete project?</AlertDialogTitle>
              <AlertDialogDescription>
                This will stop all running containers and remove the project.
                This action cannot be undone.
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancel</AlertDialogCancel>
              <AlertDialogAction onClick={() => deleteMutation.mutate()}>
                Delete
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </CardContent>
    </Card>
  );
}
