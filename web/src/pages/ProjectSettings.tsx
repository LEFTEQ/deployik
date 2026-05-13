import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "@tanstack/react-router";
import {
  GitBranch,
  HardDrive,
  Network,
  RefreshCw,
  Trash2,
  Webhook,
} from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import { resolveAutoDeploySourceBranch } from "@/lib/autodeploy";
import { queryKeys } from "@/lib/queryKeys";
import type {
  AutoBuildConfig,
  UpdateAutoBuildConfigPayload,
  VolumeInfo,
} from "@/types/api";
import { BuildSettingsFields } from "@/components/projects/build-settings";
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
    queryKey: queryKeys.project(id),
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
      <AutoBuildSection
        projectId={id}
        defaultBranch={project.branch}
        githubOwner={project.github_owner}
        githubRepo={project.github_repo}
      />
      <div className="border-b" />
      <RuntimeSettingsSection project={project} />
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
    port: project.port || 3000,
    startCommand: "",
    healthPath: "",
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
        port: buildSettings.port,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.project(project.id) });
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
  githubOwner,
  githubRepo,
}: {
  projectId: string;
  defaultBranch: string;
  githubOwner: string;
  githubRepo: string;
}) {
  const queryClient = useQueryClient();
  const [needsReauth, setNeedsReauth] = useState(false);
  const [noAdminAccess, setNoAdminAccess] = useState<string | null>(null);

  const { data: config } = useQuery({
    queryKey: queryKeys.autoBuild(projectId),
    queryFn: () => api.getAutoBuildConfig(projectId).catch(() => null),
  });

  const enabled = config?.enabled ?? false;

  const [productionBranch, setProductionBranch] = useState(
    config?.production_branch || defaultBranch || "main",
  );
  const [previewBranches, setPreviewBranches] = useState(
    config?.preview_branches || "*",
  );
  const [autoProductionEnabled, setAutoProductionEnabled] = useState(
    config?.auto_production_enabled ?? false,
  );
  const sourceBranch = resolveAutoDeploySourceBranch(
    productionBranch,
    defaultBranch,
  );

  useEffect(() => {
    if (!config) return;
    setProductionBranch(config.production_branch || defaultBranch || "main");
    setPreviewBranches(config.preview_branches || "*");
    setAutoProductionEnabled(
      config.enabled && (config.auto_production_enabled ?? false),
    );
  }, [config, defaultBranch]);

  const updateMutation = useMutation({
    mutationFn: (data: UpdateAutoBuildConfigPayload) =>
      api.updateAutoBuildConfig(projectId, data),
    onSuccess: (updatedConfig: AutoBuildConfig) => {
      queryClient.setQueryData(queryKeys.autoBuild(projectId), updatedConfig);
      setProductionBranch(
        updatedConfig.production_branch || defaultBranch || "main",
      );
      setPreviewBranches(updatedConfig.preview_branches || "*");
      setAutoProductionEnabled(
        updatedConfig.enabled && updatedConfig.auto_production_enabled,
      );
      setNeedsReauth(false);
      setNoAdminAccess(null);
      toast.success("Auto-build updated");
    },
    onError: (err) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.autoBuild(projectId),
      });
      setAutoProductionEnabled(
        config ? config.enabled && config.auto_production_enabled : false,
      );
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
    const nextAutoProductionEnabled = checked ? autoProductionEnabled : false;
    updateMutation.mutate({
      enabled: checked,
      production_branch: productionBranch,
      preview_branches: previewBranches,
      auto_production_enabled: nextAutoProductionEnabled,
    });
  };

  const handleProductionToggle = (checked: boolean) => {
    if (!enabled) return;
    updateMutation.mutate({
      enabled: true,
      production_branch: productionBranch,
      preview_branches: previewBranches,
      auto_production_enabled: checked,
    });
  };

  const handleSave = () => {
    updateMutation.mutate({
      enabled: true,
      production_branch: productionBranch,
      preview_branches: previewBranches,
      auto_production_enabled: autoProductionEnabled,
    });
  };

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <Webhook className="h-4 w-4 text-muted-foreground" />
            <h2 id="auto-build-heading" className="text-base font-semibold">
              Auto-Build on Push
            </h2>
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
            Automatically deploy previews from GitHub pushes to matching source
            branches. Production can opt in to release from the selected/default
            branch.
          </p>
        </div>
        <Switch
          id="auto-build-on-push"
          aria-labelledby="auto-build-heading"
          checked={enabled}
          onCheckedChange={handleToggle}
          disabled={updateMutation.isPending}
        />
      </div>

      <div className="flex items-center justify-between gap-4 rounded-lg border p-4">
        <div className="space-y-1">
          <Label htmlFor="production-auto-deploy">
            Production auto-deploy
          </Label>
          <p className="text-xs text-muted-foreground">
            Preview auto-deploy uses the preview branch rules. Enable this only
            when pushes to <span className="font-mono">{sourceBranch}</span>{" "}
            should release production.
          </p>
        </div>
        <Switch
          id="production-auto-deploy"
          checked={enabled && autoProductionEnabled}
          onCheckedChange={handleProductionToggle}
          disabled={!enabled || updateMutation.isPending}
        />
      </div>

      {noAdminAccess && (
        <div className="rounded-lg border border-rose-500/25 bg-rose-500/5 p-4 space-y-2">
          <p className="text-sm font-medium text-rose-200">
            Cannot create webhook on{" "}
            <span className="font-mono">{githubOwner}/{githubRepo}</span>
          </p>
          <p className="text-sm text-muted-foreground">
            GitHub requires <strong>admin access</strong> to create webhooks.
            Your account doesn't have admin permissions on this repository.
          </p>
          <p className="text-sm text-muted-foreground">To fix this:</p>
          <ul className="list-disc pl-5 text-sm text-muted-foreground space-y-1">
            <li>
              Ask the repo owner to add you as an{" "}
              <strong>admin collaborator</strong> in{" "}
              <a
                href={`https://github.com/${githubOwner}/${githubRepo}/settings/access`}
                target="_blank"
                rel="noopener noreferrer"
                className="text-primary hover:underline"
              >
                GitHub Settings → Collaborators
              </a>
            </li>
            <li>Or have the repo owner log into Deployik and enable auto-build themselves</li>
          </ul>
        </div>
      )}

      {needsReauth && (
        <div className="rounded-lg border border-amber-500/25 bg-amber-500/5 p-4 space-y-2">
          <p className="text-sm font-medium text-amber-200">
            GitHub permissions need updating
          </p>
          <p className="text-sm text-muted-foreground">
            Your account was authorized before webhook support was added.
            Re-authorize to grant the <strong>admin:repo_hook</strong> permission.
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
                Production Source Branch
              </Label>
              <Input
                value={productionBranch}
                onChange={(e) => setProductionBranch(e.target.value)}
                placeholder="main"
              />
              <p className="text-xs text-muted-foreground">
                Used by production auto-deploy when enabled. Usually the
                selected/default branch.
              </p>
            </div>
            <div className="space-y-2">
              <Label className="flex items-center gap-1.5">
                <GitBranch className="h-3.5 w-3.5" />
                Preview Branches
              </Label>
              <div className="flex items-center justify-between gap-3 rounded-md border px-3 py-2">
                <div className="space-y-0.5">
                  <p className="text-sm font-medium">All branches</p>
                  <p className="text-xs text-muted-foreground">
                    Every push to any branch deploys a preview.
                  </p>
                </div>
                <Switch
                  id="preview-all-branches"
                  checked={previewBranches.trim() === "*"}
                  onCheckedChange={(checked) =>
                    setPreviewBranches(
                      checked ? "*" : (defaultBranch || "main"),
                    )
                  }
                  disabled={updateMutation.isPending}
                />
              </div>
              {previewBranches.trim() !== "*" && (
                <>
                  <Input
                    value={previewBranches}
                    onChange={(e) => setPreviewBranches(e.target.value)}
                    placeholder="develop,staging"
                  />
                  <p className="text-xs text-muted-foreground">
                    Comma-separated branch names. Pushes to other branches
                    won't deploy a preview.
                  </p>
                </>
              )}
            </div>
          </div>
          <Button
            size="sm"
            onClick={handleSave}
            disabled={updateMutation.isPending}
          >
            {updateMutation.isPending ? "Saving..." : "Save Auto-Build Settings"}
          </Button>
        </div>
      )}
    </div>
  );
}

function RuntimeSettingsSection({
  project,
}: {
  project: NonNullable<Awaited<ReturnType<typeof api.getProject>>>;
}) {
  const queryClient = useQueryClient();
  const [hostNetworkAccess, setHostNetworkAccess] = useState(
    project.host_network_access,
  );
  const [dataVolumeEnabled, setDataVolumeEnabled] = useState(
    project.data_volume_enabled,
  );
  const [dataMountPath, setDataMountPath] = useState(
    project.data_mount_path || "/app/data",
  );

  const updateMutation = useMutation({
    mutationFn: () =>
      api.updateProject(project.id, {
        host_network_access: hostNetworkAccess,
        data_volume_enabled: dataVolumeEnabled,
        data_mount_path: dataMountPath,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.project(project.id) });
      toast.success("Runtime settings updated");
    },
    onError: (err) => toast.error(err.message),
  });

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-base font-semibold">Runtime</h2>
        <p className="text-sm text-muted-foreground">
          Container networking and persistent storage for deployed environments.
        </p>
      </div>

      <div className="space-y-4">
        <div className="flex items-center justify-between rounded-lg border px-4 py-3">
          <div className="flex items-center gap-3">
            <Network className="h-4 w-4 text-muted-foreground" />
            <div className="space-y-0.5">
              <div className="text-sm font-medium">Host network access</div>
              <div className="text-xs text-muted-foreground">
                Connect to host services (Redis, MySQL, etc.) via{" "}
                <code className="rounded bg-muted px-1 font-mono text-xs">
                  host.docker.internal
                </code>
              </div>
            </div>
          </div>
          <Switch
            checked={hostNetworkAccess}
            onCheckedChange={setHostNetworkAccess}
          />
        </div>

        <div className="space-y-3 rounded-lg border p-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <HardDrive className="h-4 w-4 text-muted-foreground" />
              <div className="space-y-0.5">
                <div className="text-sm font-medium">
                  Persistent data volume
                </div>
                <div className="text-xs text-muted-foreground">
                  Mount a named Docker volume so data survives redeployments
                </div>
              </div>
            </div>
            <Switch
              checked={dataVolumeEnabled}
              onCheckedChange={setDataVolumeEnabled}
            />
          </div>

          {dataVolumeEnabled && (
            <div className="space-y-1.5 pl-7">
              <Label htmlFor="data_mount_path" className="text-xs">
                Mount path
              </Label>
              <Input
                id="data_mount_path"
                value={dataMountPath}
                onChange={(e) => setDataMountPath(e.target.value)}
                placeholder="/app/data"
                className="font-mono text-sm"
              />
              <p className="text-xs text-muted-foreground">
                Path inside the container where the volume is mounted. Volume
                name:{" "}
                <code className="font-mono">
                  deployik-{project.name}-{"<env>"}-data
                </code>
              </p>
            </div>
          )}
        </div>
      </div>

      <Button
        onClick={() => updateMutation.mutate()}
        disabled={updateMutation.isPending}
      >
        {updateMutation.isPending ? "Saving..." : "Save Runtime Settings"}
      </Button>

      {dataVolumeEnabled && <VolumesSection projectId={project.id} />}
    </div>
  );
}

function formatVolumeSize(bytes: number): string {
  if (bytes <= 0) return "Volume exists";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let v = bytes;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 10 || i === 0 ? 0 : 1)} ${units[i]}`;
}

function VolumesSection({ projectId }: { projectId: string }) {
  const queryClient = useQueryClient();

  const { data: volumes } = useQuery({
    queryKey: queryKeys.volumes(projectId),
    queryFn: () => api.listVolumes(projectId),
  });

  const deleteMutation = useMutation({
    mutationFn: (env: "preview" | "production") =>
      api.deleteVolume(projectId, env),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.volumes(projectId) });
      toast.success("Volume deleted");
    },
    onError: (err) => toast.error(err.message),
  });

  const recreateMutation = useMutation({
    mutationFn: (env: "preview" | "production") =>
      api.recreateVolume(projectId, env),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.volumes(projectId) });
      toast.success("Volume recreated");
    },
    onError: (err) => toast.error(err.message),
  });

  return (
    <div className="space-y-3">
      <div className="text-sm font-medium">Volumes</div>
      {(volumes ?? []).map((v: VolumeInfo) => (
        <div
          key={v.environment}
          className="flex items-center justify-between rounded-lg border px-4 py-3"
        >
          <div className="space-y-0.5">
            <div className="flex items-center gap-2">
              <Badge
                variant="outline"
                className={
                  v.environment === "production"
                    ? "border-amber-500/25 text-amber-200"
                    : ""
                }
              >
                {v.environment}
              </Badge>
              <code className="text-xs font-mono text-muted-foreground">
                {v.mount_path}
              </code>
            </div>
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <span>
                {v.exists
                  ? formatVolumeSize(v.size_bytes)
                  : "Created automatically on next deploy"}
              </span>
              {v.in_use && (
                <Badge
                  variant="outline"
                  className="border-emerald-500/25 text-emerald-300"
                >
                  in use
                </Badge>
              )}
            </div>
          </div>
          {v.exists && (
            <div className="flex items-center gap-1">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  if (
                    confirm(
                      `Recreate volume for ${v.environment}? All existing data will be wiped.`,
                    )
                  ) {
                    recreateMutation.mutate(
                      v.environment as "preview" | "production",
                    );
                  }
                }}
                disabled={recreateMutation.isPending || v.in_use}
                title={
                  v.in_use
                    ? "Stop the deployment before recreating this volume"
                    : undefined
                }
              >
                <RefreshCw className="mr-1 h-3 w-3" />
                Recreate
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="text-destructive hover:text-destructive"
                onClick={() => {
                  if (
                    confirm(
                      `Delete volume for ${v.environment}? All data will be permanently lost.`,
                    )
                  ) {
                    deleteMutation.mutate(
                      v.environment as "preview" | "production",
                    );
                  }
                }}
                disabled={deleteMutation.isPending || v.in_use}
                title={
                  v.in_use
                    ? "Stop the deployment before deleting this volume"
                    : undefined
                }
              >
                <Trash2 className="mr-1 h-3 w-3" />
                Delete
              </Button>
            </div>
          )}
        </div>
      ))}
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
