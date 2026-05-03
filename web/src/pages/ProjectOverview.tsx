import { useEffect, useOptimistic, useRef, useState, useTransition } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
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
  GlobeLock,
} from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import { queryKeys, staleTimes } from "@/lib/queryKeys";
import {
  ACTIVE_DEPLOYMENT_STATUSES,
  DEPLOYMENT_STATUS_META,
  ENVIRONMENT_META,
  buildReleaseTagName,
  formatRelativeDate,
  getEnvironmentDomains,
  getLatestEnvironmentDeployment,
  getPrimaryEnvironmentUrl,
  isDomainReady,
} from "@/lib/deployment-helpers";
import { formatFrameworkLabel } from "@/components/projects/build-settings";
import { DeploymentThumbnail } from "@/components/projects/deployment-thumbnail";
import { ReleasePanelContent } from "@/components/projects/release-panel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { LoadingState } from "@/components/ui/spinner";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/store/auth";
import type { Deployment } from "@/types/api";

function buildOptimisticDeployment(args: {
  projectId: string;
  environment: "preview" | "production";
  branch: string;
  username: string;
  commitMessage?: string;
}): Deployment {
  return {
    id: `optimistic-${Date.now()}`,
    project_id: args.projectId,
    environment: args.environment,
    commit_sha: "",
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

export function ProjectOverview() {
  const { id } = useParams({ strict: false }) as { id: string };
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const currentUser = useAuthStore((state) => state.user);
  const [releaseDialogOpen, setReleaseDialogOpen] = useState(false);
  const [createTag, setCreateTag] = useState(true);
  const [releaseTagName, setReleaseTagName] = useState(buildReleaseTagName());
  const [, startTransition] = useTransition();

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

  // Optimistically render the new deployment at the top of the list so the
  // "Recent deployments" section reflects the click instantly. Resets when the
  // server list refreshes after the mutation resolves.
  const [optimisticDeployments, addOptimistic] = useOptimistic<
    Deployment[],
    Deployment
  >(deployments ?? [], (state, pending) => [pending, ...state]);

  const deployMutation = useMutation({
    mutationFn: (payload: {
      environment: "preview" | "production";
      branch?: string;
      create_tag?: boolean;
      tag_name?: string;
    }) => api.triggerDeployment(id, payload),
    onSuccess: (deployment, variables) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.deployments(id) });
      toast.success(
        `${deployment.environment === "production" ? "Release" : "Preview deploy"} triggered`,
      );
      if (variables.environment === "production") {
        setReleaseDialogOpen(false);
      }
    },
    onError: (err) => toast.error(err.message),
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

  const latestPreview = getLatestEnvironmentDeployment(
    optimisticDeployments,
    "preview",
  );
  const latestProduction = getLatestEnvironmentDeployment(
    optimisticDeployments,
    "production",
  );

  const recentDeployments = optimisticDeployments.slice(0, 6);

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
          branch: payload.branch ?? project.branch,
          username: currentUser?.username ?? "",
          commitMessage: payload.tag_name
            ? `Release ${payload.tag_name}`
            : undefined,
        }),
      );
      deployMutation.mutate(payload);
    });
  };

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
        <h1 className="text-xl font-semibold tracking-tight sm:text-2xl">
          {project.name}
        </h1>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted-foreground">
          <span className="truncate">
            {project.github_owner}/{project.github_repo}
          </span>
          <span className="flex items-center gap-1">
            <GitBranch className="h-3.5 w-3.5" />
            {project.branch}
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
          <div className="flex gap-2">
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setReleaseTagName(buildReleaseTagName());
                setCreateTag(true);
                setReleaseDialogOpen(true);
              }}
              disabled={deployMutation.isPending}
            >
              <GlobeLock className="mr-1.5 h-3.5 w-3.5" />
              Release
            </Button>
          </div>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          <EnvironmentCard
            environment="preview"
            deployment={latestPreview}
            projectId={id}
            primaryUrl={getPrimaryEnvironmentUrl(allDomains, "preview")}
            envDomainCount={getEnvironmentDomains(allDomains, "preview").length}
            onNavigate={navigate}
          />
          <EnvironmentCard
            environment="production"
            deployment={latestProduction}
            projectId={id}
            primaryUrl={getPrimaryEnvironmentUrl(allDomains, "production")}
            envDomainCount={getEnvironmentDomains(allDomains, "production").length}
            onNavigate={navigate}
          />
        </div>
      </div>

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
              return (
                <button
                  key={d.id}
                  type="button"
                  className="flex w-full items-center gap-3 px-4 py-2.5 text-left transition-colors hover:bg-accent first:rounded-t-lg last:rounded-b-lg"
                  onClick={() =>
                    navigate({
                      to: "/projects/$id/deployments/$did",
                      params: { id, did: d.id },
                    })
                  }
                >
                  <span
                    className={cn(
                      "h-2 w-2 shrink-0 rounded-full",
                      meta?.dotClass ?? "bg-slate-500",
                    )}
                  />
                  <GitCommit className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                  <span className="font-mono text-xs text-foreground">
                    {d.commit_sha ? d.commit_sha.slice(0, 7) : d.id.slice(0, 7)}
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
                </button>
              );
            })}
          </div>
        )}
      </div>

      {/* Release dialog */}
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
              disabled={deployMutation.isPending}
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
                deployMutation.isPending
              }
            >
              <GlobeLock className="mr-1.5 h-3.5 w-3.5" />
              {deployMutation.isPending ? "Releasing..." : "Release"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

/* ── Environment Card ── */

// Captures already in flight client-side; keyed by deployment id so a remount
// doesn't fan out duplicate POSTs while a capture goroutine is still running
// server-side. Cleared once the screenshot path appears or the capture errors.
const inFlightCaptures = new Set<string>();

function EnvironmentCard({
  environment,
  deployment,
  projectId,
  primaryUrl,
  envDomainCount,
  onNavigate,
}: {
  environment: "preview" | "production";
  deployment: Deployment | undefined;
  projectId: string;
  primaryUrl: string | null;
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

  const handleClick = () => {
    if (!deployment || isOptimistic) return;
    onNavigate({
      to: "/projects/$id/deployments/$did",
      params: { id: projectId, did: deployment.id },
    });
  };

  return (
    <div
      className={cn(
        "flex flex-col overflow-hidden rounded-lg border bg-card",
        isProduction && "border-l-2 border-l-amber-500/50",
      )}
    >
      {/* Hero thumbnail */}
      <button
        type="button"
        disabled={!deployment || isOptimistic}
        onClick={handleClick}
        className={cn(
          "block w-full text-left",
          deployment && !isOptimistic
            ? "cursor-pointer hover:opacity-90"
            : "cursor-default",
        )}
        aria-label={
          deployment
            ? `${envMeta.label} preview \u2014 open deployment details`
            : `${envMeta.label} environment`
        }
      >
        <DeploymentThumbnail
          deploymentId={deployment?.id ?? null}
          hasScreenshot={hasScreenshot}
          isCapturing={isCapturing}
          alt={`${envMeta.label} homepage preview`}
          size="lg"
          className="rounded-none border-0 border-b"
        />
      </button>

      {/* Meta block */}
      <div className="flex flex-col gap-2 p-4">
        <div className="flex items-center justify-between gap-2">
          <Badge
            variant="outline"
            className={cn("shrink-0 text-xs", envMeta.badgeClass)}
          >
            {envMeta.label}
          </Badge>
          {primaryUrl ? (
            <a
              href={primaryUrl}
              target="_blank"
              rel="noopener noreferrer"
              onClick={(e) => e.stopPropagation()}
              className="inline-flex min-w-0 items-center gap-1 truncate text-xs text-muted-foreground hover:text-foreground"
            >
              <span className="truncate">{primaryUrl.replace(/^https?:\/\//, "")}</span>
              <ExternalLink className="h-3 w-3 shrink-0" />
            </a>
          ) : (
            <span className="text-xs text-muted-foreground">No domain yet</span>
          )}
        </div>

        {deployment ? (
          <>
            <div className="flex items-center gap-2 text-sm">
              <span
                className={cn(
                  "h-2 w-2 rounded-full",
                  isActive && "animate-pulse",
                  statusMeta?.dotClass ?? "bg-slate-500",
                )}
              />
              <span className="font-medium">
                {statusMeta?.label ?? deployment.status}
              </span>
              <span className="ml-auto text-xs text-muted-foreground">
                {formatRelativeDate(deployment.created_at)}
              </span>
            </div>

            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
              <span className="flex items-center gap-1">
                <GitCommit className="h-3.5 w-3.5" />
                <span className="font-mono">
                  {deployment.commit_sha
                    ? deployment.commit_sha.slice(0, 7)
                    : "pending"}
                </span>
              </span>
              <span className="flex items-center gap-1">
                <GitBranch className="h-3.5 w-3.5" />
                {deployment.branch}
              </span>
              {deployment.build_duration > 0 && (
                <span className="flex items-center gap-1">
                  <Clock className="h-3.5 w-3.5" />
                  {deployment.build_duration}s
                </span>
              )}
            </div>

            {(deployment.commit_message || deployment.error_message) && (
              <p className="line-clamp-1 text-xs text-muted-foreground">
                {deployment.commit_message || deployment.error_message}
              </p>
            )}
          </>
        ) : (
          <p className="text-sm text-muted-foreground">No deployment yet</p>
        )}
      </div>
    </div>
  );
}
