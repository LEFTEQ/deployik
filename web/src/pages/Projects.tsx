import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { ChevronRight, Plus } from "lucide-react";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { useOrganizations } from "@/hooks/use-organizations";
import {
  DEPLOYMENT_STATUS_META,
  ENVIRONMENT_META,
  formatRelativeDate,
} from "@/lib/deployment-helpers";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { LoadingState } from "@/components/ui/spinner";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";
import type { DeploymentStatus, Project } from "@/types/api";

function ProjectTableRow({ project }: { project: Project }) {
  const navigate = useNavigate();
  const status = project.latest_deployment_status as DeploymentStatus | null;
  const statusMeta =
    status && status in DEPLOYMENT_STATUS_META
      ? DEPLOYMENT_STATUS_META[status]
      : null;
  const environment = project.latest_deployment_environment as
    | keyof typeof ENVIRONMENT_META
    | null;
  const environmentMeta = environment ? ENVIRONMENT_META[environment] : null;

  const open = () => navigate({ to: "/projects/$id", params: { id: project.id } });

  return (
    <TableRow
      className="cursor-pointer border-white/8 transition-colors hover:bg-white/[0.04] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-primary/40"
      role="link"
      tabIndex={0}
      onClick={open}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          open();
        }
      }}
    >
      <TableCell className="pl-6">
        <div className="flex items-center gap-3">
          <span
            className={cn(
              "h-2 w-2 shrink-0 rounded-full",
              project.status === "active" ? "bg-emerald-400" : "bg-slate-500",
            )}
          />
          <span className="truncate text-sm font-semibold text-foreground">
            {project.name}
          </span>
        </div>
      </TableCell>
      <TableCell className="text-xs text-muted-foreground">
        <span className="font-mono">
          {project.github_owner}/{project.github_repo}
        </span>
        <span className="mx-1.5">·</span>
        <span className="font-mono">{project.branch}</span>
      </TableCell>
      <TableCell>
        {environmentMeta && statusMeta ? (
          <div className="flex flex-wrap items-center gap-1.5">
            <Badge variant="outline" className={environmentMeta.badgeClass}>
              {environmentMeta.label}
            </Badge>
            <Badge variant="outline" className={statusMeta.badgeClass}>
              {statusMeta.label}
            </Badge>
          </div>
        ) : (
          <span className="text-xs text-muted-foreground">—</span>
        )}
      </TableCell>
      <TableCell className="text-xs text-muted-foreground">
        {formatRelativeDate(project.updated_at)}
      </TableCell>
      <TableCell className="pr-6 text-right">
        <ChevronRight className="ml-auto h-4 w-4 text-muted-foreground" />
      </TableCell>
    </TableRow>
  );
}

export function Projects() {
  const {
    organizations,
    selectedOrganization,
    projectsView,
    setProjectsView,
    isLoading: organizationsLoading,
  } = useOrganizations();

  const projectsViewIsValid = useMemo(() => {
    if (projectsView === "all") return true;
    return organizations.some((organization) => organization.id === projectsView);
  }, [organizations, projectsView]);
  const activeView = projectsViewIsValid ? projectsView : "all";

  const { data: projects, isLoading } = useQuery({
    queryKey: queryKeys.projects(activeView),
    queryFn: () =>
      api.listProjects(activeView === "all" ? undefined : activeView),
    enabled: !organizationsLoading,
  });

  const subtitle =
    activeView === "all"
      ? "Across every workspace you can access"
      : `${selectedOrganization?.name ?? "Workspace"} workspace`;

  return (
    <div className="p-6">
      <div className="mb-6 flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Projects</h1>
          <p className="text-sm text-muted-foreground">{subtitle}</p>
        </div>
        <Link to="/new">
          <Button>
            <Plus className="mr-2 h-4 w-4" />
            New Project
          </Button>
        </Link>
      </div>

      {organizations.length > 0 && (
        <Tabs
          value={activeView}
          onValueChange={(value) => setProjectsView(value)}
          className="mb-4"
        >
          <TabsList variant="line">
            <TabsTrigger value="all">All</TabsTrigger>
            {organizations.map((organization) => (
              <TabsTrigger key={organization.id} value={organization.id}>
                {organization.name}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      )}

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
          <p className="text-lg font-medium">
            {activeView === "all"
              ? "No projects yet across your workspaces"
              : "No projects in this workspace"}
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            {activeView === "all"
              ? "Create your first project to get started."
              : `Create a project in ${selectedOrganization?.name ?? "this workspace"} to get started.`}
          </p>
          <Link to="/new" className="mt-4">
            <Button>
              <Plus className="mr-2 h-4 w-4" />
              New Project
            </Button>
          </Link>
        </div>
      ) : (
        <Card className="overflow-hidden">
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow className="border-white/8 hover:bg-transparent">
                  <TableHead className="pl-6">Project</TableHead>
                  <TableHead>Repository</TableHead>
                  <TableHead>Last Deploy</TableHead>
                  <TableHead>Updated</TableHead>
                  <TableHead className="w-10 pr-6" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {projects.map((project) => (
                  <ProjectTableRow key={project.id} project={project} />
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
