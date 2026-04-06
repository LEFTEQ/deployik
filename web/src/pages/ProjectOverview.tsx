import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import {
  Activity,
  BarChart3,
  CircleDot,
  Copy,
  ExternalLink,
  GitBranch,
  GitCommit,
  Globe2,
  GlobeLock,
  Link2,
  Rocket,
  Building2,
} from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import {
  ACTIVE_DEPLOYMENT_STATUSES,
  DEPLOYMENT_STATUS_META,
  ENVIRONMENT_META,
  formatCompactNumber,
  formatRelativeDate,
  getLatestEnvironmentDeployment,
  getLatestLiveEnvironmentDeployment,
  getPrimaryEnvironmentUrl,
  isDomainReady,
} from "@/lib/deployment-helpers";
import { formatFrameworkLabel } from "@/components/projects/build-settings";
import { AUDIENCE_STATUS_META } from "@/components/projects/project-analytics-meta";
import { OverviewStatCard } from "@/components/projects/overview-stat-card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { LoadingState } from "@/components/ui/spinner";
import { cn } from "@/lib/utils";
import type { Deployment, Domain } from "@/types/api";

export function ProjectOverview() {
  const { id } = useParams({ strict: false }) as { id: string };
  const navigate = useNavigate();

  const { data: project, isLoading } = useQuery({
    queryKey: ["project", id],
    queryFn: () => api.getProject(id),
  });

  const { data: deployments, isLoading: deploymentsLoading } = useQuery({
    queryKey: ["deployments", id],
    queryFn: () => api.listDeployments(id),
    refetchInterval: (query) => {
      const items = query.state.data ?? [];
      return items.some((d) => ACTIVE_DEPLOYMENT_STATUSES.has(d.status))
        ? 3000
        : false;
    },
  });

  const { data: domains, isLoading: domainsLoading } = useQuery({
    queryKey: ["domains", id],
    queryFn: () => api.listDomains(id),
  });

  const timezone =
    Intl.DateTimeFormat().resolvedOptions().timeZone?.trim() || "UTC";

  const { data: analytics, isLoading: analyticsLoading } = useQuery({
    queryKey: ["project-overview-analytics", id, timezone],
    queryFn: () =>
      api.getProjectAnalytics(id, {
        environment: "all",
        range: "24h",
        timezone,
      }),
  });

  if (isLoading) {
    return (
      <LoadingState
        title="Loading project..."
        description="Fetching project details, deployments, and public endpoints."
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

  const latestDeployment = deployments?.[0] ?? null;
  const latestRelease = getLatestLiveEnvironmentDeployment(
    deployments,
    "production",
  );
  const latestPreview = getLatestEnvironmentDeployment(deployments, "preview");
  const latestProduction = getLatestEnvironmentDeployment(
    deployments,
    "production",
  );
  const previewUrl = getPrimaryEnvironmentUrl(domains, "preview");
  const productionUrl = getPrimaryEnvironmentUrl(domains, "production");
  const readyDomainCount = (domains ?? []).filter(isDomainReady).length;

  const overviewAudienceMeta = AUDIENCE_STATUS_META[
    analytics?.audience.status ?? ""
  ] ??
    AUDIENCE_STATUS_META.ready_to_install ?? {
      label: "Ready to install",
      badgeClass: "border-primary/25 bg-primary/12 text-primary",
      description:
        "The website exists. Add the tracker to start collecting audience data.",
    };

  const copyUrl = async (value: string, label: string) => {
    try {
      await navigator.clipboard.writeText(value);
      toast.success(`${label} copied`);
    } catch {
      toast.error(`Couldn't copy ${label.toLowerCase()}`);
    }
  };

  return (
    <div className="space-y-4">
      {/* Project header card */}
      <Card className="@container/card">
        <CardHeader>
          <div className="min-w-0 space-y-3">
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
            <div className="min-w-0">
              <CardTitle className="text-xl tracking-tight sm:text-2xl">
                {project.name}
              </CardTitle>
              <CardDescription className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1">
                <span className="min-w-0 truncate">
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
              </CardDescription>
            </div>
          </div>
        </CardHeader>
      </Card>

      {/* Live endpoints */}
      <Card className="@container/card">
        <CardHeader>
          <div className="flex flex-wrap items-center gap-2">
            <CardTitle className="text-base">Live Endpoints</CardTitle>
            <CardDescription>
              Quick public links to the current preview and production endpoints.
            </CardDescription>
          </div>
          <CardAction>
            <Badge variant="outline" className="hidden sm:inline-flex">
              {project.name}.preview.example.com
            </Badge>
          </CardAction>
        </CardHeader>
        <CardContent>
          <div className="grid gap-3 md:grid-cols-2">
            <LiveEndpointChip
              environment="preview"
              url={previewUrl}
              deployment={latestPreview}
              onCopy={copyUrl}
            />
            <LiveEndpointChip
              environment="production"
              url={productionUrl}
              deployment={latestProduction}
              onCopy={copyUrl}
            />
          </div>
        </CardContent>
      </Card>

      {/* Summary stat cards */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
        <OverviewStatCard
          label="Preview Health"
          value={
            latestPreview
              ? DEPLOYMENT_STATUS_META[latestPreview.status].label
              : "Not deployed"
          }
          icon={<Globe2 className="h-4 w-4" />}
          hint={
            previewUrl
              ? "Preview has an active public endpoint."
              : "Deploy preview to create a public staging URL."
          }
        />
        <OverviewStatCard
          label="Production Health"
          value={
            latestProduction
              ? DEPLOYMENT_STATUS_META[latestProduction.status].label
              : "Not released"
          }
          icon={<GlobeLock className="h-4 w-4" />}
          hint={
            productionUrl
              ? "Production has a verified domain."
              : "Release once production domains are ready."
          }
        />
        <OverviewStatCard
          label="Latest Release"
          value={latestRelease ? latestRelease.commit_sha.slice(0, 7) : "None"}
          icon={<Rocket className="h-4 w-4" />}
          hint={
            latestRelease
              ? `Released ${formatRelativeDate(latestRelease.created_at)}`
              : "No successful production release yet."
          }
        />
        <OverviewStatCard
          label="Active Domains"
          value={readyDomainCount.toString()}
          icon={<Link2 className="h-4 w-4" />}
          hint="Verified domains with active SSL."
        />
        <OverviewStatCard
          label="Traffic"
          value={
            analyticsLoading
              ? "--"
              : formatCompactNumber(analytics?.runtime.summary.requests ?? 0)
          }
          icon={<Activity className="h-4 w-4" />}
          hint="Requests over the last 24 hours."
        />
        <OverviewStatCard
          label="Analytics Status"
          value={analytics ? overviewAudienceMeta.label : "Loading"}
          icon={<BarChart3 className="h-4 w-4" />}
          hint={
            analytics?.audience.status === "ready_to_install"
              ? "Setup is still pending."
              : "Audience analytics status from Umami."
          }
        />
      </div>

      {/* Latest deployment card */}
      <Card className="@container/card overflow-hidden">
        <CardHeader>
          <div className="space-y-2">
            <div className="flex flex-wrap items-center gap-2">
              <Badge
                variant="outline"
                className={
                  latestDeployment
                    ? DEPLOYMENT_STATUS_META[latestDeployment.status].badgeClass
                    : "border-white/10 bg-white/5 text-slate-200"
                }
              >
                {latestDeployment
                  ? DEPLOYMENT_STATUS_META[latestDeployment.status].label
                  : "No deployments yet"}
              </Badge>
              {latestDeployment ? (
                <Badge
                  variant="outline"
                  className={
                    ENVIRONMENT_META[latestDeployment.environment].badgeClass
                  }
                >
                  {ENVIRONMENT_META[latestDeployment.environment].label}
                </Badge>
              ) : null}
            </div>
            <CardTitle className="text-base">Latest Deployment</CardTitle>
            <CardDescription>
              The newest build is the fastest way to read the current state of
              this project.
            </CardDescription>
          </div>
          <CardAction className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              onClick={() =>
                navigate({
                  to: "/projects/$id/deployments",
                  params: { id },
                })
              }
            >
              See All
            </Button>
            {analytics?.audience.status === "ready_to_install" ? (
              <Button
                onClick={() =>
                  navigate({
                    to: "/projects/$id/integration",
                    params: { id },
                  })
                }
              >
                Setup Analytics
              </Button>
            ) : null}
          </CardAction>
        </CardHeader>
        <CardContent>
          {deploymentsLoading || domainsLoading ? (
            <LoadingState
              title="Loading deployments..."
              description="Preparing the latest deployment and endpoint activity."
              className="min-h-[280px]"
            />
          ) : latestDeployment ? (
            <div className="grid gap-4 xl:grid-cols-[minmax(0,1.1fr)_minmax(280px,0.9fr)]">
              <div className="rounded-xl border bg-muted/30 p-5">
                <div className="flex items-start justify-between gap-4">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2 text-sm font-medium text-foreground">
                      <GitCommit className="h-4 w-4 text-muted-foreground" />
                      {latestDeployment.commit_sha
                        ? latestDeployment.commit_sha.slice(0, 7)
                        : "pending"}
                    </div>
                    <p
                      className="mt-3 truncate text-lg font-semibold text-foreground"
                      title={
                        latestDeployment.commit_message ||
                        latestDeployment.error_message
                      }
                    >
                      {latestDeployment.commit_message ||
                        latestDeployment.error_message ||
                        "Waiting for commit metadata"}
                    </p>
                  </div>
                </div>
                <div className="mt-5 grid gap-3 sm:grid-cols-3">
                  <MiniMeta label="Branch" value={latestDeployment.branch} />
                  <MiniMeta
                    label="Started"
                    value={formatRelativeDate(latestDeployment.created_at)}
                  />
                  <MiniMeta
                    label="Duration"
                    value={
                      latestDeployment.build_duration > 0
                        ? `${latestDeployment.build_duration}s`
                        : "--"
                    }
                  />
                </div>
              </div>

              <div className="space-y-3">
                <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">
                  Previous deployments
                </p>
                {(deployments ?? []).slice(1, 4).length ? (
                  (deployments ?? []).slice(1, 4).map((deployment) => (
                    <button
                      type="button"
                      key={deployment.id}
                      onClick={() =>
                        navigate({
                          to: "/projects/$id/deployments/$did",
                          params: { id, did: deployment.id },
                        })
                      }
                      className="flex w-full items-center justify-between gap-3 rounded-xl border bg-muted/30 px-4 py-3 text-left transition-colors hover:bg-accent"
                    >
                      <div className="min-w-0">
                        <p className="text-sm font-medium text-foreground">
                          {deployment.commit_sha
                            ? deployment.commit_sha.slice(0, 7)
                            : deployment.id.slice(0, 8)}
                        </p>
                        <p className="truncate text-xs text-muted-foreground">
                          {deployment.commit_message ||
                            DEPLOYMENT_STATUS_META[deployment.status].label}
                        </p>
                      </div>
                      <div className="text-right">
                        <p className="text-xs text-muted-foreground">
                          {ENVIRONMENT_META[deployment.environment].label}
                        </p>
                        <p className="mt-1 text-xs text-muted-foreground">
                          {formatRelativeDate(deployment.created_at)}
                        </p>
                      </div>
                    </button>
                  ))
                ) : (
                  <div className="rounded-xl border border-dashed border-border/70 px-4 py-8 text-sm text-muted-foreground">
                    No previous deployments yet.
                  </div>
                )}
              </div>
            </div>
          ) : (
            <div className="rounded-xl border border-dashed border-border/70 px-5 py-12 text-sm text-muted-foreground">
              No deployments yet. Deploy a preview or release to production to
              get started.
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function LiveEndpointChip({
  environment,
  url,
  deployment,
  onCopy,
}: {
  environment: Domain["environment"];
  url: string | null;
  deployment: Deployment | undefined;
  onCopy: (value: string, label: string) => void;
}) {
  const isLive = Boolean(url);

  return (
    <div className="flex items-center justify-between gap-3 rounded-xl border bg-muted/30 px-4 py-3">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <span
            className={cn(
              "h-2.5 w-2.5 rounded-full",
              isLive ? "bg-emerald-400" : "bg-slate-500",
            )}
          />
          <p className="text-sm font-medium text-foreground">
            {ENVIRONMENT_META[environment].label}
          </p>
        </div>
        <p className="mt-1 truncate text-sm text-muted-foreground">
          {url || "Not live yet"}
        </p>
        <p className="mt-1 text-xs text-muted-foreground">
          {deployment
            ? DEPLOYMENT_STATUS_META[deployment.status].label
            : "No deployment yet"}
        </p>
      </div>
      <div className="flex shrink-0 gap-2">
        {url ? (
          <>
            <Button asChild size="sm" variant="ghost">
              <a href={url} target="_blank" rel="noopener noreferrer">
                <ExternalLink className="mr-1.5 h-3.5 w-3.5" />
                Open
              </a>
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={() =>
                onCopy(url, `${ENVIRONMENT_META[environment].label} URL`)
              }
            >
              <Copy className="h-3.5 w-3.5" />
            </Button>
          </>
        ) : (
          <Badge
            variant="outline"
            className="border-white/10 bg-white/5 text-slate-200"
          >
            Pending
          </Badge>
        )}
      </div>
    </div>
  );
}

function MiniMeta({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl border bg-background px-3 py-3">
      <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">
        {label}
      </p>
      <p className="mt-2 truncate text-sm font-medium text-foreground">
        {value}
      </p>
    </div>
  );
}
