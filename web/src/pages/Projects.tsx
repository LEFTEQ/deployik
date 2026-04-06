import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Plus, GitBranch, Clock, Circle } from "lucide-react";
import { formatDistanceToNow } from "date-fns";
import { api } from "@/lib/api";
import { useOrganizations } from "@/hooks/use-organizations";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { LoadingState } from "@/components/ui/spinner";
import type { Project } from "@/types/api";

function ProjectCard({ project }: { project: Project }) {
  return (
    <Link
      to="/projects/$id"
      params={{ id: project.id }}
    >
      <Card className="transition-all hover:-translate-y-0.5 hover:border-primary/35">
        <CardHeader className="pb-3">
          <div className="flex items-start justify-between">
            <div>
              <CardTitle className="text-base">{project.name}</CardTitle>
              <CardDescription className="mt-1">
                {project.github_owner}/{project.github_repo}
              </CardDescription>
              {project.organization_name ? (
                <Badge
                  variant="outline"
                  className="mt-2 border-white/10 bg-white/5 text-xs text-muted-foreground"
                >
                  {project.organization_name}
                </Badge>
              ) : null}
            </div>
            <Badge
              variant={project.status === "active" ? "default" : "secondary"}
            >
              <Circle className="mr-1 h-2 w-2 fill-current" />
              {project.status}
            </Badge>
          </div>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-4 text-sm text-muted-foreground">
            <span className="flex items-center gap-1">
              <GitBranch className="h-3.5 w-3.5" />
              {project.branch}
            </span>
            <span className="flex items-center gap-1">
              <Clock className="h-3.5 w-3.5" />
              {formatDistanceToNow(new Date(project.updated_at), {
                addSuffix: true,
              })}
            </span>
          </div>
        </CardContent>
      </Card>
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
    queryKey: ["projects", selectedOrganizationId ?? "all"],
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
        <Card className="flex flex-col items-center justify-center py-16">
          <p className="text-lg font-medium">No workspaces found</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Sign in again if your organization memberships were just changed.
          </p>
        </Card>
      ) : !projects?.length ? (
        <Card className="flex flex-col items-center justify-center py-16">
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
        </Card>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {projects.map((project) => (
            <ProjectCard key={project.id} project={project} />
          ))}
        </div>
      )}
    </div>
  );
}
