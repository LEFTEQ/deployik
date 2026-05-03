import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ChevronRight, Plus } from "lucide-react";
import { formatDistanceToNow } from "date-fns";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { useOrganizations } from "@/hooks/use-organizations";
import { Button } from "@/components/ui/button";
import { LoadingState } from "@/components/ui/spinner";
import { DeploymentThumbnail } from "@/components/projects/deployment-thumbnail";
import { cn } from "@/lib/utils";
import type { Project } from "@/types/api";

function ProjectRow({ project }: { project: Project }) {
  return (
    <Link
      to="/projects/$id"
      params={{ id: project.id }}
      className="flex items-center gap-4 border-b border-white/5 px-5 py-4 transition-colors last:border-b-0 hover:bg-muted/50"
    >
      <DeploymentThumbnail
        deploymentId={project.latest_deployment_id}
        hasScreenshot={Boolean(project.latest_deployment_screenshot_path)}
        alt={`${project.name} preview`}
        size="sm"
        className="shrink-0"
      />
      <span
        className={cn(
          "h-2.5 w-2.5 shrink-0 rounded-full",
          project.status === "active" ? "bg-emerald-400" : "bg-slate-500",
        )}
      />
      <div className="min-w-0 flex-1">
        <span className="block truncate text-sm font-semibold text-foreground">
          {project.name}
        </span>
        <span className="hidden truncate text-xs text-muted-foreground sm:block">
          <span className="font-mono">
            {project.github_owner}/{project.github_repo}
          </span>
          <span className="mx-1.5">·</span>
          <span className="font-mono">{project.branch}</span>
          {project.latest_deployment_environment && (
            <>
              <span className="mx-1.5">·</span>
              <span>{project.latest_deployment_environment}</span>
            </>
          )}
        </span>
      </div>
      <span className="ml-auto shrink-0 text-xs text-muted-foreground">
        {formatDistanceToNow(new Date(project.updated_at), {
          addSuffix: true,
        })}
      </span>
      <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
    </Link>
  );
}

export function Projects() {
  const {
    organizations,
    selectedOrganization,
    selectedOrganizationId,
    isLoading: organizationsLoading,
  } = useOrganizations();
  const { data: projects, isLoading } = useQuery({
    queryKey: queryKeys.projects(selectedOrganizationId),
    queryFn: () => api.listProjects(selectedOrganizationId ?? undefined),
    enabled: !organizationsLoading,
  });

  return (
    <div className="p-6">
      <div className="mb-6 flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Projects</h1>
          <p className="text-sm text-muted-foreground">
            {selectedOrganization
              ? `${selectedOrganization.name} workspace`
              : "Manage your deployed applications"}
          </p>
        </div>
        <Link to="/new">
          <Button>
            <Plus className="mr-2 h-4 w-4" />
            New Project
          </Button>
        </Link>
      </div>

      {organizationsLoading || isLoading ? (
        <LoadingState
          title="Loading projects…"
          description="Fetching your workspaces and deployed projects."
          className="min-h-[360px]"
        />
      ) : !organizations.length ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-white/10 py-16">
          <p className="text-lg font-medium">No workspaces found</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Sign in again if your organization memberships were just changed.
          </p>
        </div>
      ) : !projects?.length ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-white/10 py-16">
          <p className="text-lg font-medium">No projects in this workspace</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Create a project in {selectedOrganization?.name ?? "this workspace"}{" "}
            to get started
          </p>
          <Link to="/new" className="mt-4">
            <Button>
              <Plus className="mr-2 h-4 w-4" />
              New Project
            </Button>
          </Link>
        </div>
      ) : (
        <div className="rounded-lg border border-white/5">
          {projects.map((project) => (
            <ProjectRow key={project.id} project={project} />
          ))}
        </div>
      )}
    </div>
  );
}
