import { useOptimistic, useState, useTransition } from "react";
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
  Rocket,
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
  getLatestEnvironmentDeployment,
  isDomainReady,
} from "@/lib/deployment-helpers";
import { formatFrameworkLabel } from "@/components/projects/build-settings";
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
              onClick={() => triggerDeploy({ environment: "preview" })}
              disabled={deployMutation.isPending}
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
              disabled={deployMutation.isPending}
            >
              <GlobeLock className="mr-1.5 h-3.5 w-3.5" />
              Release
            </Button>
          </div>
        </div>

        <div className="divide-y divide-border rounded-lg border">
          <EnvironmentRow
            environment="preview"
            deployment={latestPreview}
            projectId={id}
            onNavigate={navigate}
          />
          <EnvironmentRow
            environment="production"
            deployment={latestProduction}
            projectId={id}
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

/* ── Environment Row ── */

function EnvironmentRow({
  environment,
  deployment,
  projectId,
  onNavigate,
}: {
  environment: "preview" | "production";
  deployment: Deployment | undefined;
  projectId: string;
  onNavigate: ReturnType<typeof useNavigate>;
}) {
  const envMeta = ENVIRONMENT_META[environment];
  const statusMeta = deployment
    ? DEPLOYMENT_STATUS_META[deployment.status]
    : null;
  const isActive = deployment
    ? ACTIVE_DEPLOYMENT_STATUSES.has(deployment.status)
    : false;

  const isProduction = environment === "production";

  const handleClick = () => {
    if (!deployment) return;
    onNavigate({
      to: "/projects/$id/deployments/$did",
      params: { id: projectId, did: deployment.id },
    });
  };

  return (
    <button
      type="button"
      disabled={!deployment}
      onClick={handleClick}
      className={cn(
        "flex w-full items-center gap-4 px-4 py-3 text-left transition-colors first:rounded-t-lg last:rounded-b-lg",
        deployment && "hover:bg-accent cursor-pointer",
        !deployment && "cursor-default",
        isProduction && "border-l-2 border-l-amber-500/50",
      )}
    >
      {/* Environment label */}
      <Badge
        variant="outline"
        className={cn("shrink-0 text-xs", envMeta.badgeClass)}
      >
        {envMeta.label}
      </Badge>

      {deployment ? (
        <>
          {/* Status */}
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

          {/* Commit */}
          <span className="flex shrink-0 items-center gap-1.5 text-muted-foreground">
            <GitCommit className="h-3.5 w-3.5" />
            <span className="font-mono text-xs">
              {deployment.commit_sha
                ? deployment.commit_sha.slice(0, 7)
                : "pending"}
            </span>
          </span>

          {/* Commit message */}
          <span className="min-w-0 flex-1 truncate text-sm text-muted-foreground">
            {deployment.commit_message ||
              deployment.error_message ||
              "Waiting for commit metadata"}
          </span>

          {/* Branch */}
          <span className="hidden shrink-0 items-center gap-1 text-xs text-muted-foreground sm:flex">
            <GitBranch className="h-3.5 w-3.5" />
            {deployment.branch}
          </span>

          {/* Duration */}
          {deployment.build_duration > 0 && (
            <span className="hidden shrink-0 items-center gap-1 text-xs text-muted-foreground md:flex">
              <Clock className="h-3.5 w-3.5" />
              {deployment.build_duration}s
            </span>
          )}

          {/* Time */}
          <span className="shrink-0 text-xs text-muted-foreground">
            {formatRelativeDate(deployment.created_at)}
          </span>
        </>
      ) : (
        <span className="flex-1 text-sm text-muted-foreground">
          No deployment yet
        </span>
      )}
    </button>
  );
}
