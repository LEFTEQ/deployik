import { useOptimistic, useState, useTransition } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { ExternalLink, GitCommit, GlobeLock, Rocket } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import { queryKeys, staleTimes } from "@/lib/queryKeys";
import { useAuthStore } from "@/store/auth";
import {
  ACTIVE_DEPLOYMENT_STATUSES,
  DEPLOYMENT_STATUS_META,
  ENVIRONMENT_META,
  buildReleaseTagName,
  formatRelativeDate,
  getPrimaryEnvironmentUrl,
} from "@/lib/deployment-helpers";
import { DeploymentCard } from "@/components/projects/deployment-card";
import { ReleasePanelContent } from "@/components/projects/release-panel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { LoadingState } from "@/components/ui/spinner";
import { BranchLink, CommitLink } from "@/components/ui/github-link";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";
import type { Deployment } from "@/types/api";

function buildOptimisticDeployment(args: {
  projectId: string;
  environment: "preview" | "production";
  branch: string;
  username: string;
  commitSha?: string;
  commitMessage?: string;
}): Deployment {
  return {
    id: `optimistic-${Date.now()}`,
    project_id: args.projectId,
    environment: args.environment,
    commit_sha: args.commitSha ?? "",
    commit_message: args.commitMessage ?? "",
    branch: args.branch,
    status: "queued",
    container_id: "",
    container_name: "",
    image_tag: "",
    build_duration: 0,
    triggered_by: "",
    trigger_source: "manual",
    triggered_by_username: args.username,
    screenshot_path: null,
    error_message: undefined,
    created_at: new Date().toISOString(),
    finished_at: null,
  };
}

export function ProjectDeployments() {
  const { id } = useParams({ strict: false }) as { id: string };
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const currentUser = useAuthStore((state) => state.user);
  const [releaseDialogOpen, setReleaseDialogOpen] = useState(false);
  const [createTag, setCreateTag] = useState(true);
  const [releaseTagName, setReleaseTagName] = useState(buildReleaseTagName());
  const [, startTransition] = useTransition();

  const { data: project } = useQuery({
    queryKey: queryKeys.project(id),
    queryFn: () => api.getProject(id),
  });

  const { data: deployments, isLoading } = useQuery({
    queryKey: queryKeys.deployments(id),
    queryFn: () => api.listDeployments(id),
    staleTime: staleTimes.activeDeployments,
    refetchInterval: (query) => {
      const items = query.state.data ?? [];
      return items.some((d) => ACTIVE_DEPLOYMENT_STATUSES.has(d.status))
        ? 3000
        : false;
    },
  });

  // Optimistic deployment list: the user sees a "queued" card the instant they
  // click Deploy, even before the API confirms. React 19 resets this automatically
  // when `deployments` (the server source of truth) updates after mutation success.
  const [optimisticDeployments, addOptimistic] = useOptimistic<
    Deployment[],
    Deployment
  >(deployments ?? [], (state, pending) => [pending, ...state]);

  const { data: domains } = useQuery({
    queryKey: queryKeys.domains(id),
    queryFn: () => api.listDomains(id),
  });

  const deploymentMutation = useMutation({
    mutationFn: (payload: {
      environment: "preview" | "production";
      branch?: string;
      create_tag?: boolean;
      tag_name?: string;
    }) => api.triggerDeployment(id, payload),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.deployments(id) });
      queryClient.invalidateQueries({
        queryKey: queryKeys.previewInstances(id),
      });
      toast.success(
        variables.environment === "production"
          ? "Release queued"
          : "Preview deployment queued",
      );
      if (variables.environment === "production") {
        setReleaseDialogOpen(false);
      }
    },
    onError: (err) => toast.error(err.message),
  });

  const openDeploymentDetails = (deploymentId: string) => {
    navigate({
      to: "/projects/$id/deployments/$did",
      params: { id, did: deploymentId },
    });
  };

  const triggerDeploy = (payload: {
    environment: "preview" | "production";
    branch?: string;
    create_tag?: boolean;
    tag_name?: string;
  }) => {
    startTransition(() => {
      addOptimistic(
        buildOptimisticDeployment({
          projectId: id,
          environment: payload.environment,
          branch: payload.branch ?? project?.branch ?? "main",
          username: currentUser?.username ?? "",
          commitMessage: payload.tag_name
            ? `Release ${payload.tag_name}`
            : undefined,
        }),
      );
      deploymentMutation.mutate(payload);
    });
  };

  return (
    <div className="space-y-8">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="text-base font-semibold">Deployments</h2>
          <p className="text-sm text-muted-foreground">
            Every deployment stays readable and row-clickable, with direct
            access to logs and live endpoints.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            size="sm"
            onClick={() => triggerDeploy({ environment: "preview" })}
            disabled={deploymentMutation.isPending}
          >
            <Rocket className="mr-1.5 h-3.5 w-3.5" />
            Deploy Preview
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              setReleaseTagName(buildReleaseTagName());
              setCreateTag(true);
              setReleaseDialogOpen(true);
            }}
            disabled={deploymentMutation.isPending}
          >
            <GlobeLock className="mr-1.5 h-3.5 w-3.5" />
            Release
          </Button>
        </div>
      </div>

      {isLoading ? (
        <LoadingState
          title="Loading deployments..."
          description="Fetching recent preview and production build history."
        />
      ) : !optimisticDeployments.length ? (
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-sm text-muted-foreground">
              No deployments yet. Click deploy to trigger your first build.
            </p>
          </CardContent>
        </Card>
      ) : (
        <>
          {/* Phones get tappable cards; the table needs more width than 390px. */}
          <div className="space-y-3 md:hidden">
            {optimisticDeployments.map((deployment) => {
              const liveUrl =
                deployment.status === "live"
                  ? getPrimaryEnvironmentUrl(
                      domains,
                      deployment.environment,
                      deployment.preview_instance_id,
                    )
                  : null;
              return (
                <DeploymentCard
                  key={deployment.id}
                  deployment={deployment}
                  liveUrl={liveUrl}
                  onOpen={() => openDeploymentDetails(deployment.id)}
                  action={
                    <Button asChild size="sm" variant="outline" className="h-9">
                      <Link
                        to="/projects/$id/deployments/$did"
                        params={{ id, did: deployment.id }}
                        onClick={(event) => event.stopPropagation()}
                      >
                        Logs
                      </Link>
                    </Button>
                  }
                />
              );
            })}
          </div>
          <Card className="hidden overflow-hidden md:block">
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow className="border-white/8 hover:bg-transparent">
                    <TableHead className="pl-6">Environment</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Commit</TableHead>
                    <TableHead>Branch</TableHead>
                    <TableHead>Started</TableHead>
                    <TableHead>Duration</TableHead>
                    <TableHead className="text-right">Open</TableHead>
                    <TableHead className="pr-6 text-right">Logs</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {optimisticDeployments.map((deployment) => {
                    const liveUrl =
                      deployment.status === "live"
                        ? getPrimaryEnvironmentUrl(
                            domains,
                            deployment.environment,
                            deployment.preview_instance_id,
                          )
                        : null;
                    const statusMeta =
                      DEPLOYMENT_STATUS_META[deployment.status];

                    return (
                      <TableRow
                        key={deployment.id}
                        className={cn(
                          "cursor-pointer border-white/8 transition-colors hover:bg-white/[0.04] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-primary/40",
                          deployment.status === "live" && "bg-white/[0.03]",
                        )}
                        role="link"
                        tabIndex={0}
                        onClick={() => openDeploymentDetails(deployment.id)}
                        onKeyDown={(event) => {
                          if (event.key === "Enter" || event.key === " ") {
                            event.preventDefault();
                            openDeploymentDetails(deployment.id);
                          }
                        }}
                      >
                        <TableCell className="pl-6">
                          <div className="flex items-center gap-3">
                            <span
                              className={cn(
                                "h-2.5 w-2.5 rounded-full",
                                statusMeta.dotClass,
                                ACTIVE_DEPLOYMENT_STATUSES.has(
                                  deployment.status,
                                ) && "animate-pulse",
                              )}
                            />
                            <div>
                              <Badge
                                variant="outline"
                                className={
                                  ENVIRONMENT_META[deployment.environment]
                                    .badgeClass
                                }
                              >
                                {ENVIRONMENT_META[deployment.environment].label}
                              </Badge>
                              <p className="mt-1 text-xs text-muted-foreground">
                                {deployment.id.slice(0, 8)}
                              </p>
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>
                          <Badge
                            variant="outline"
                            className={statusMeta.badgeClass}
                          >
                            {statusMeta.label}
                          </Badge>
                        </TableCell>
                        <TableCell className="max-w-[340px]">
                          <div className="space-y-1">
                            <div className="flex items-center gap-2 text-sm font-medium text-foreground">
                              <GitCommit className="h-3.5 w-3.5 text-muted-foreground" />
                              {deployment.commit_sha && project ? (
                                <CommitLink
                                  owner={project.github_owner}
                                  repo={project.github_repo}
                                  sha={deployment.commit_sha}
                                />
                              ) : deployment.commit_sha ? (
                                deployment.commit_sha.slice(0, 7)
                              ) : (
                                "pending"
                              )}
                            </div>
                            <p
                              className="truncate text-xs text-muted-foreground"
                              title={
                                deployment.commit_message ||
                                deployment.error_message
                              }
                            >
                              {deployment.commit_message ||
                                deployment.error_message ||
                                statusMeta.label}
                            </p>
                          </div>
                        </TableCell>
                        <TableCell className="text-sm text-muted-foreground">
                          {project ? (
                            <BranchLink
                              owner={project.github_owner}
                              repo={project.github_repo}
                              branch={deployment.branch}
                            />
                          ) : (
                            deployment.branch
                          )}
                        </TableCell>
                        <TableCell className="text-sm text-muted-foreground">
                          {formatRelativeDate(deployment.created_at)}
                        </TableCell>
                        <TableCell className="text-sm text-muted-foreground">
                          {deployment.build_duration > 0
                            ? `${deployment.build_duration}s`
                            : "--"}
                        </TableCell>
                        <TableCell className="text-right">
                          {liveUrl ? (
                            <Button asChild size="sm" variant="ghost">
                              <a
                                href={liveUrl}
                                target="_blank"
                                rel="noopener noreferrer"
                                onClick={(event) => event.stopPropagation()}
                              >
                                <ExternalLink className="mr-1.5 h-3.5 w-3.5" />
                                Open
                              </a>
                            </Button>
                          ) : (
                            <span className="text-xs text-muted-foreground">
                              --
                            </span>
                          )}
                        </TableCell>
                        <TableCell className="pr-6 text-right">
                          <Button asChild size="sm" variant="ghost">
                            <Link
                              to="/projects/$id/deployments/$did"
                              params={{ id, did: deployment.id }}
                              onClick={(event) => event.stopPropagation()}
                            >
                              Logs
                            </Link>
                          </Button>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </>
      )}

      {/* Release dialog */}
      {project && (
        <Dialog open={releaseDialogOpen} onOpenChange={setReleaseDialogOpen}>
          <DialogContent className="sm:max-w-lg">
            <DialogHeader>
              <DialogTitle>Release to Production</DialogTitle>
              <DialogDescription>
                Deploy the latest commit from{" "}
                <span className="font-mono font-medium text-foreground">
                  {project.branch}
                </span>{" "}
                to production.
              </DialogDescription>
            </DialogHeader>
            <ReleasePanelContent
              project={project}
              domains={domains}
              createTag={createTag}
              onCreateTagChange={setCreateTag}
              releaseTagName={releaseTagName}
              onReleaseTagChange={setReleaseTagName}
            />
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => setReleaseDialogOpen(false)}
                disabled={deploymentMutation.isPending}
              >
                Cancel
              </Button>
              <Button
                onClick={() =>
                  triggerDeploy({
                    environment: "production",
                    create_tag: createTag,
                    tag_name: createTag ? releaseTagName.trim() : undefined,
                  })
                }
                disabled={
                  (createTag && !releaseTagName.trim()) ||
                  deploymentMutation.isPending
                }
              >
                <GlobeLock className="mr-1.5 h-3.5 w-3.5" />
                {deploymentMutation.isPending ? "Releasing..." : "Release"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
    </div>
  );
}
