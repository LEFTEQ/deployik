import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "@tanstack/react-router";
import { Trash2 } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import {
  BuildSettingsFields,
} from "@/components/projects/build-settings";
import { VariableStore } from "@/components/projects/variable-store";
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
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { LoadingState } from "@/components/ui/spinner";

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
    <div className="space-y-6">
      <BuildSettingsSection project={project} />
      <VariableStore projectId={id} kind="env" />
      <VariableStore projectId={id} kind="secret" />
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
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Build Settings</CardTitle>
        <CardDescription>
          Source control branch, framework defaults, and package/runtime
          behavior for this project.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
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
      </CardContent>
    </Card>
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
