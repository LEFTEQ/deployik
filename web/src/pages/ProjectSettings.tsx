import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "@tanstack/react-router";
import { Trash2 } from "lucide-react";
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
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
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
      <PasswordProtectionSection projectId={id} />
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

function PasswordProtectionSection({ projectId }: { projectId: string }) {
  const queryClient = useQueryClient();
  const [passwordToShow, setPasswordToShow] = useState<string | null>(null);

  const { data: protection } = useQuery({
    queryKey: ["protection", projectId],
    queryFn: () => api.getProtectionStatus(projectId),
  });

  const updateMutation = useMutation({
    mutationFn: (data: { environment: string; enabled: boolean }) =>
      api.updateProtection(projectId, data),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ["protection", projectId] });
      if (result.password) {
        setPasswordToShow(result.password);
      }
      toast.success(result.enabled ? "Protection enabled" : "Protection disabled");
    },
    onError: (err) => toast.error(err.message),
  });

  const regenerateMutation = useMutation({
    mutationFn: (data: { environment: string }) =>
      api.regeneratePassword(projectId, data),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ["protection", projectId] });
      if (result.password) {
        setPasswordToShow(result.password);
      }
      toast.success("Password regenerated");
    },
    onError: (err) => toast.error(err.message),
  });

  const isPending = updateMutation.isPending || regenerateMutation.isPending;

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-base font-semibold">Password Protection</h2>
        <p className="text-sm text-muted-foreground">
          Control access to your deployed environments. When enabled, visitors
          see an "unavailable" page and must enter a password to access the
          site.
        </p>
      </div>

      <div className="divide-y rounded-lg border">
        <ProtectionRow
          environment="preview"
          enabled={protection?.preview_enabled ?? false}
          onToggle={(enabled) =>
            updateMutation.mutate({ environment: "preview", enabled })
          }
          onRegenerate={() =>
            regenerateMutation.mutate({ environment: "preview" })
          }
          isPending={isPending}
        />
        <ProtectionRow
          environment="production"
          enabled={protection?.production_enabled ?? false}
          onToggle={(enabled) =>
            updateMutation.mutate({ environment: "production", enabled })
          }
          onRegenerate={() =>
            regenerateMutation.mutate({ environment: "production" })
          }
          isPending={isPending}
        />
      </div>

      {/* Password reveal dialog */}
      <AlertDialog
        open={!!passwordToShow}
        onOpenChange={() => setPasswordToShow(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Password Generated</AlertDialogTitle>
            <AlertDialogDescription>
              Save this password — it won't be shown again.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="rounded-lg border bg-muted/30 p-4 font-mono text-lg tracking-wider text-center select-all">
            {passwordToShow}
          </div>
          <AlertDialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                navigator.clipboard.writeText(passwordToShow ?? "");
                toast.success("Password copied");
              }}
            >
              Copy
            </Button>
            <AlertDialogAction onClick={() => setPasswordToShow(null)}>
              Done
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function ProtectionRow({
  environment,
  enabled,
  onToggle,
  onRegenerate,
  isPending,
}: {
  environment: string;
  enabled: boolean;
  onToggle: (enabled: boolean) => void;
  onRegenerate: () => void;
  isPending: boolean;
}) {
  return (
    <div className="flex items-center justify-between px-4 py-3">
      <div className="flex items-center gap-3">
        <Badge
          variant="outline"
          className={
            environment === "production"
              ? "border-amber-500/25 text-amber-200"
              : ""
          }
        >
          {environment === "preview" ? "Preview" : "Production"}
        </Badge>
        <span className="text-sm text-muted-foreground">
          {enabled ? "Password required" : "Public access"}
        </span>
      </div>
      <div className="flex items-center gap-2">
        {enabled && (
          <Button
            variant="ghost"
            size="sm"
            onClick={onRegenerate}
            disabled={isPending}
          >
            Regenerate
          </Button>
        )}
        <Switch
          checked={enabled}
          onCheckedChange={onToggle}
          disabled={isPending}
        />
      </div>
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
