import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import {
  ArrowRight,
  Building2,
  CircleDot,
  Clock,
  ExternalLink,
  GitBranch,
  GitCommit,
  Globe2,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import { queryKeys, staleTimes } from "@/lib/queryKeys";
import {
  ACTIVE_DEPLOYMENT_STATUSES,
  DEPLOYMENT_STATUS_META,
  ENVIRONMENT_META,
  formatRelativeDate,
  getEnvironmentDomains,
  getLatestEnvironmentDeployment,
  isDomainReady,
} from "@/lib/deployment-helpers";
import { formatFrameworkLabel } from "@/components/projects/build-settings";
import { DeployMenu } from "@/components/projects/deploy-menu";
import { DeploymentThumbnail } from "@/components/projects/deployment-thumbnail";
import { EditableProjectName } from "@/components/projects/editable-project-name";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { LoadingState } from "@/components/ui/spinner";
import { BranchLink, CommitLink } from "@/components/ui/github-link";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { cn } from "@/lib/utils";
import type { Deployment, PreviewInstance } from "@/types/api";

export function ProjectOverview() {
  const { id } = useParams({ strict: false }) as { id: string };
  const navigate = useNavigate();

  const { data: project, isLoading } = useQuery({
    queryKey: queryKeys.project(id),
    queryFn: () => api.getProject(id),
  });

  const { data: deployments } = useQuery({
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

  const { data: domains } = useQuery({
    queryKey: queryKeys.domains(id),
    queryFn: () => api.listDomains(id),
  });

  const { data: previewInstances } = useQuery({
    queryKey: queryKeys.previewInstances(id),
    queryFn: () => api.listPreviewInstances(id),
  });

  if (isLoading) {
    return (
      <LoadingState
        title="Loading project..."
        description="Fetching project details."
        className="min-h-[420px]"
      />
    );
  }

  if (!project) {
    return (
      <div>
        <p>Project not found</p>
        <Link to="/" className="mt-2 text-sm text-primary hover:underline">
          Back to projects
        </Link>
      </div>
    );
  }

  const allDomains = domains ?? [];
  const readyDomains = allDomains.filter(isDomainReady);
  const maxDomainsShown = 3;
  const visibleDomains = readyDomains.slice(0, maxDomainsShown);
  const extraDomainCount = readyDomains.length - visibleDomains.length;
  const githubRepoUrl = `https://github.com/${encodeURIComponent(
    project.github_owner,
  )}/${encodeURIComponent(project.github_repo)}`;

  const allDeployments = deployments ?? [];
  const latestPreview = getLatestEnvironmentDeployment(
    allDeployments,
    "preview",
  );
  const latestProduction = getLatestEnvironmentDeployment(
    allDeployments,
    "production",
  );

  const recentDeployments = allDeployments.slice(0, 6);

  return (
    <div className="space-y-8">
      {/* Project Header */}
      <div className="space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <Badge
            variant="outline"
            className={cn(
              "border-white/10 bg-white/5 text-slate-200",
              project.status === "active" &&
                "border-emerald-400/25 bg-emerald-400/12 text-emerald-100",
            )}
          >
            <CircleDot className="mr-1 size-3 fill-current" />
            {project.status}
          </Badge>
          <Badge
            variant="outline"
            className="border-primary/20 bg-primary/10 font-mono text-primary"
          >
            {formatFrameworkLabel(project.framework)}
          </Badge>
        </div>
        <EditableProjectName project={project} />
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted-foreground">
          <a
            href={githubRepoUrl}
            target="_blank"
            rel="noopener noreferrer"
            aria-label={`Open ${project.github_owner}/${project.github_repo} on GitHub`}
            className="inline-flex min-w-0 items-center gap-1 truncate transition-colors hover:text-foreground hover:underline"
          >
            {project.github_owner}/{project.github_repo}
            <ExternalLink className="h-3 w-3 shrink-0" />
          </a>
          <span className="flex items-center gap-1">
            <GitBranch className="h-3.5 w-3.5" />
            <BranchLink
              owner={project.github_owner}
              repo={project.github_repo}
              branch={project.branch}
            />
          </span>
          {project.organization_name ? (
            <span className="flex items-center gap-1">
              <Building2 className="h-3.5 w-3.5" />
              {project.organization_name}
            </span>
          ) : null}
        </div>
      </div>

      {/* Domain Strip */}
      {readyDomains.length > 0 && (
        <div className="flex flex-wrap items-center gap-2 rounded-lg border bg-muted/30 px-4 py-2.5">
          <Globe2 className="h-4 w-4 shrink-0 text-muted-foreground" />
          {visibleDomains.map((d) => (
            <a
              key={d.id}
              href={`https://${d.domain}`}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 rounded-md border bg-background px-2 py-1 text-sm text-foreground transition-colors hover:bg-accent"
            >
              {d.domain}
              <ExternalLink className="h-3 w-3 text-muted-foreground" />
            </a>
          ))}
          {extraDomainCount > 0 && (
            <button
              type="button"
              onClick={() =>
                navigate({
                  to: "/projects/$id/settings/domains",
                  params: { id },
                })
              }
              className="rounded-md border bg-background px-2 py-1 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            >
              +{extraDomainCount} more
            </button>
          )}
        </div>
      )}

      {/* Unified Environments */}
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold text-foreground">
            Environments
          </h2>
          <DeployMenu
            projectId={id}
            productionBranch={project.branch}
            defaultBranch={project.branch}
          />
        </div>

        <div className="divide-y divide-border rounded-lg border">
          <EnvironmentRow
            environment="preview"
            deployment={latestPreview}
            projectId={id}
            githubOwner={project.github_owner}
            githubRepo={project.github_repo}
            envDomainCount={getEnvironmentDomains(allDomains, "preview").length}
            onNavigate={navigate}
          />
          <EnvironmentRow
            environment="production"
            deployment={latestProduction}
            projectId={id}
            githubOwner={project.github_owner}
            githubRepo={project.github_repo}
            envDomainCount={getEnvironmentDomains(allDomains, "production").length}
            onNavigate={navigate}
          />
        </div>
      </div>

      <PreviewInstancesPanel
        projectId={id}
        githubOwner={project.github_owner}
        githubRepo={project.github_repo}
        instances={previewInstances ?? []}
      />

      {/* Recent Deployments */}
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold text-foreground">
            Recent Deployments
          </h2>
          <Button variant="outline" size="sm" asChild>
            <Link to="/projects/$id/deployments" params={{ id }}>
              See All
              <ArrowRight className="ml-1.5 h-3.5 w-3.5" />
            </Link>
          </Button>
        </div>

        {recentDeployments.length === 0 ? (
          <div className="rounded-lg border border-dashed border-border/70 px-5 py-10 text-center text-sm text-muted-foreground">
            No deployments yet. Deploy a preview or release to production to get
            started.
          </div>
        ) : (
          <div className="divide-y divide-border rounded-lg border">
            {recentDeployments.map((d) => {
              const meta = DEPLOYMENT_STATUS_META[d.status];
              const envMeta = ENVIRONMENT_META[d.environment];
              const goToDeployment = () =>
                navigate({
                  to: "/projects/$id/deployments/$did",
                  params: { id, did: d.id },
                });
              return (
                <div
                  key={d.id}
                  role="button"
                  tabIndex={0}
                  className="flex w-full cursor-pointer items-center gap-3 px-4 py-2.5 text-left transition-colors first:rounded-t-lg last:rounded-b-lg hover:bg-accent"
                  onClick={goToDeployment}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" || event.key === " ") {
                      event.preventDefault();
                      goToDeployment();
                    }
                  }}
                >
                  <span
                    className={cn(
                      "h-2 w-2 shrink-0 rounded-full",
                      meta?.dotClass ?? "bg-slate-500",
                    )}
                  />
                  <GitCommit className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                  <span className="font-mono text-xs text-foreground">
                    {d.commit_sha ? (
                      <CommitLink
                        owner={project.github_owner}
                        repo={project.github_repo}
                        sha={d.commit_sha}
                      />
                    ) : (
                      d.id.slice(0, 7)
                    )}
                  </span>
                  <span className="min-w-0 flex-1 truncate text-sm text-muted-foreground">
                    {d.commit_message || meta?.label || d.status}
                  </span>
                  <Badge
                    variant="outline"
                    className={cn("shrink-0 text-xs", envMeta?.badgeClass)}
                  >
                    {envMeta?.label ?? d.environment}
                  </Badge>
                  <span className="shrink-0 text-xs text-muted-foreground">
                    {formatRelativeDate(d.created_at)}
                  </span>
                </div>
              );
            })}
          </div>
	        )}
	      </div>
	    </div>
	  );
}

function PreviewInstancesPanel({
  projectId,
  githubOwner,
  githubRepo,
  instances,
}: {
  projectId: string;
  githubOwner: string;
  githubRepo: string;
  instances: PreviewInstance[];
}) {
  const queryClient = useQueryClient();
  const [deleteTarget, setDeleteTarget] = useState<PreviewInstance | null>(null);

  const deleteMutation = useMutation({
    mutationFn: (instanceId: string) =>
      api.deletePreviewInstance(projectId, instanceId),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.previewInstances(projectId),
      });
      queryClient.invalidateQueries({ queryKey: queryKeys.deployments(projectId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.domains(projectId) });
      setDeleteTarget(null);
      toast.success("Preview deleted");
    },
    onError: (err: Error) => toast.error(err.message),
  });

  if (!instances.length) {
    return null;
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-semibold text-foreground">
          Branch Previews
        </h2>
      </div>

      <div className="divide-y divide-border rounded-lg border">
        {instances.map((instance) => {
          const status = instance.latest_deployment_status;
          const statusMeta = status ? DEPLOYMENT_STATUS_META[status] : null;
          return (
            <div
              key={instance.id}
              className="flex flex-wrap items-center gap-3 px-4 py-3"
            >
              <GitBranch className="h-4 w-4 shrink-0 text-muted-foreground" />
              <div className="min-w-[180px] flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-mono text-sm text-foreground">
                    <BranchLink
                      owner={githubOwner}
                      repo={githubRepo}
                      branch={instance.branch}
                    />
                  </span>
                  {instance.is_default ? (
                    <Badge variant="outline" className="text-xs">
                      Default
                    </Badge>
                  ) : null}
                  {statusMeta ? (
                    <Badge
                      variant="outline"
                      className={cn("text-xs", statusMeta.badgeClass)}
                    >
                      {statusMeta.label}
                    </Badge>
                  ) : null}
                </div>
                <p className="mt-1 truncate font-mono text-xs text-muted-foreground">
                  {instance.domain}
                </p>
              </div>
              {instance.domain ? (
                <Button asChild size="sm" variant="ghost">
                  <a
                    href={`https://${instance.domain}`}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    <ExternalLink className="h-3.5 w-3.5" />
                    Open
                  </a>
                </Button>
              ) : null}
              {!instance.is_default ? (
                <Button
                  size="icon"
                  variant="ghost"
                  onClick={() => setDeleteTarget(instance)}
                  disabled={deleteMutation.isPending}
                  aria-label={`Delete ${instance.branch} preview`}
                >
                  <Trash2 className="h-4 w-4 text-muted-foreground" />
                </Button>
              ) : null}
            </div>
          );
        })}
      </div>

      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete branch preview?</AlertDialogTitle>
            <AlertDialogDescription>
              This stops the preview container and removes its automatic
              preview domain. Deployment history remains available.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={deleteMutation.isPending || !deleteTarget}
              onClick={(event) => {
                event.preventDefault();
                if (deleteTarget) {
                  deleteMutation.mutate(deleteTarget.id);
                }
              }}
            >
              <Trash2 className="h-3.5 w-3.5" />
              Delete preview
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

/* ── Environment Row ── */

// Captures already in flight client-side; keyed by deployment id so a remount
// doesn't fan out duplicate POSTs while a capture goroutine is still running
// server-side. Cleared once the screenshot path appears or the capture errors.
const inFlightCaptures = new Set<string>();

function EnvironmentRow({
  environment,
  deployment,
  projectId,
  githubOwner,
  githubRepo,
  envDomainCount,
  onNavigate,
}: {
  environment: "preview" | "production";
  deployment: Deployment | undefined;
  projectId: string;
  githubOwner: string;
  githubRepo: string;
  envDomainCount: number;
  onNavigate: ReturnType<typeof useNavigate>;
}) {
  const queryClient = useQueryClient();
  const envMeta = ENVIRONMENT_META[environment];
  const statusMeta = deployment
    ? DEPLOYMENT_STATUS_META[deployment.status]
    : null;
  const isActive = deployment
    ? ACTIVE_DEPLOYMENT_STATUSES.has(deployment.status)
    : false;

  const isProduction = environment === "production";
  const isOptimistic = deployment?.id.startsWith("optimistic-") ?? false;
  const hasScreenshot = Boolean(deployment?.screenshot_path);
  const isLive = deployment?.status === "live";
  const eligibleForCapture =
    !!deployment && !isOptimistic && isLive && !hasScreenshot && envDomainCount > 0;

  const [isCapturing, setIsCapturing] = useState(false);
  const pollTimerRef = useRef<number | null>(null);

  // Lazy-backfill: when this env's live deployment lacks a screenshot, fire one
  // capture request and poll the deployments query until the path appears or
  // we time out. Idempotent server-side (the handler returns 200 ready when a
  // file already exists), so a duplicate fire is cheap rather than dangerous.
  useEffect(() => {
    if (!eligibleForCapture || !deployment) return;
    const captureKey = deployment.id;
    if (inFlightCaptures.has(captureKey)) return;
    inFlightCaptures.add(captureKey);

    let cancelled = false;
    setIsCapturing(true);

    const stopPolling = () => {
      if (pollTimerRef.current !== null) {
        window.clearInterval(pollTimerRef.current);
        pollTimerRef.current = null;
      }
    };

    const finish = () => {
      if (cancelled) return;
      stopPolling();
      setIsCapturing(false);
      inFlightCaptures.delete(captureKey);
    };

    api
      .captureProjectScreenshot(projectId, environment)
      .then((res) => {
        if (cancelled) return;
        if (res.status === "ready") {
          // Server says the file already exists; force a fresh fetch so the
          // deployment row picks up the screenshot_path.
          queryClient.invalidateQueries({
            queryKey: queryKeys.deployments(projectId),
          });
          finish();
          return;
        }
        // Capturing async — poll the deployments list until screenshot_path lands.
        const startedAt = Date.now();
        pollTimerRef.current = window.setInterval(() => {
          if (Date.now() - startedAt > 90_000) {
            // Capture should have either succeeded or errored server-side by now;
            // give up polling and let the user reload manually.
            finish();
            return;
          }
          queryClient.invalidateQueries({
            queryKey: queryKeys.deployments(projectId),
          });
        }, 5000);
      })
      .catch(() => finish());

    return () => {
      cancelled = true;
      stopPolling();
      // Don't release the in-flight slot here — the server-side goroutine may
      // still be running. We'll release on next mount once the screenshot path
      // appears (eligibleForCapture flips false) or on the timeout above.
    };
  }, [eligibleForCapture, deployment, projectId, environment, queryClient]);

  // Once the screenshot lands, the deployment id no longer qualifies for
  // capture. Drop it from the in-flight set so a future stale homepage can
  // re-trigger after a manual refresh + redeploy + invalidation cycle.
  useEffect(() => {
    if (deployment && hasScreenshot) {
      inFlightCaptures.delete(deployment.id);
    }
  }, [deployment, hasScreenshot]);

  const [isRefreshing, setIsRefreshing] = useState(false);

  const handleClick = () => {
    if (!deployment || isOptimistic) return;
    onNavigate({
      to: "/projects/$id/deployments/$did",
      params: { id: projectId, did: deployment.id },
    });
  };

  const canRefresh = !!deployment && !isOptimistic && envDomainCount > 0;
  const handleRefresh = canRefresh
    ? async () => {
        setIsRefreshing(true);
        try {
          const result = await api.captureProjectScreenshot(projectId, environment, {
            sync: true,
            force: true,
          });
          if (result.status === "ready") {
            queryClient.invalidateQueries({
              queryKey: queryKeys.deployments(projectId),
            });
            toast.success("Preview refreshed");
          } else if (result.status === "failed" && result.error) {
            toast.error(`Capture failed: ${result.error}`);
          }
        } catch (err) {
          toast.error(
            `Capture failed: ${err instanceof Error ? err.message : String(err)}`,
          );
        } finally {
          setIsRefreshing(false);
        }
      }
    : undefined;

  const isInteractive = deployment && !isOptimistic;

  return (
    <div
      role={isInteractive ? "button" : undefined}
      tabIndex={isInteractive ? 0 : undefined}
      onClick={isInteractive ? handleClick : undefined}
      onKeyDown={
        isInteractive
          ? (e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                handleClick();
              }
            }
          : undefined
      }
      className={cn(
        "flex w-full items-center gap-4 px-4 py-3 text-left transition-colors first:rounded-t-lg last:rounded-b-lg",
        isInteractive && "hover:bg-accent cursor-pointer",
        !isInteractive && "cursor-default",
        isProduction && "border-l-2 border-l-amber-500/50",
      )}
    >
      <DeploymentThumbnail
        deploymentId={deployment?.id ?? null}
        hasScreenshot={hasScreenshot}
        isCapturing={isCapturing}
        alt={`${envMeta.label} homepage preview`}
        size="sm"
        className="shrink-0"
        onRefresh={handleRefresh}
        refreshing={isRefreshing}
      />

      <Badge
        variant="outline"
        className={cn("shrink-0 text-xs", envMeta.badgeClass)}
      >
        {envMeta.label}
      </Badge>

      {deployment ? (
        <>
          <span className="flex shrink-0 items-center gap-1.5">
            <span
              className={cn(
                "h-2 w-2 rounded-full",
                isActive && "animate-pulse",
                statusMeta?.dotClass ?? "bg-slate-500",
              )}
            />
            <span className="text-sm font-medium">
              {statusMeta?.label ?? deployment.status}
            </span>
          </span>

          <span className="flex shrink-0 items-center gap-1.5 text-muted-foreground">
            <GitCommit className="h-3.5 w-3.5" />
            <span className="font-mono text-xs">
              {deployment.commit_sha ? (
                <CommitLink
                  owner={githubOwner}
                  repo={githubRepo}
                  sha={deployment.commit_sha}
                />
              ) : (
                "pending"
              )}
            </span>
          </span>

          <span className="min-w-0 flex-1 truncate text-sm text-muted-foreground">
            {deployment.commit_message ||
              deployment.error_message ||
              "Waiting for commit metadata"}
          </span>

          <span className="hidden shrink-0 items-center gap-1 text-xs text-muted-foreground sm:flex">
            <GitBranch className="h-3.5 w-3.5" />
            <BranchLink
              owner={githubOwner}
              repo={githubRepo}
              branch={deployment.branch}
            />
          </span>

          {deployment.build_duration > 0 && (
            <span className="hidden shrink-0 items-center gap-1 text-xs text-muted-foreground md:flex">
              <Clock className="h-3.5 w-3.5" />
              {deployment.build_duration}s
            </span>
          )}

          <span className="shrink-0 text-xs text-muted-foreground">
            {formatRelativeDate(deployment.created_at)}
          </span>
        </>
      ) : (
        <span className="flex-1 text-sm text-muted-foreground">
          No deployment yet
        </span>
      )}
    </div>
  );
}
