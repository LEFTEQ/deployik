import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import {
  ChevronRight,
  ExternalLink,
  GitBranch,
  GitCommit,
  Search,
} from "lucide-react";

import { api } from "@/lib/api";
import {
  ACTIVE_DEPLOYMENT_STATUSES,
  DEPLOYMENT_STATUS_META,
  ENVIRONMENT_META,
  formatRelativeDate,
  getLatestLiveEnvironmentDeployment,
  getPrimaryEnvironmentUrl,
  isDomainReady,
} from "@/lib/deployment-helpers";
import { formatFrameworkLabel } from "@/components/projects/build-settings";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { LoadingState } from "@/components/ui/spinner";
import { cn } from "@/lib/utils";
import type { Deployment } from "@/types/api";

export function ProjectOverview() {
  const { id } = useParams({ strict: false }) as { id: string };
  const navigate = useNavigate();
  const [branchSearch, setBranchSearch] = useState("");

  const { data: project, isLoading } = useQuery({
    queryKey: ["project", id],
    queryFn: () => api.getProject(id),
  });

  const { data: deployments } = useQuery({
    queryKey: ["deployments", id],
    queryFn: () => api.listDeployments(id),
    refetchInterval: (query) => {
      const items = query.state.data ?? [];
      return items.some((d) => ACTIVE_DEPLOYMENT_STATUSES.has(d.status))
        ? 3000
        : false;
    },
  });

  const { data: domains } = useQuery({
    queryKey: ["domains", id],
    queryFn: () => api.listDomains(id),
  });

  const { data: autoBuild } = useQuery({
    queryKey: ["auto-build", id],
    queryFn: () => api.getAutoBuildConfig(id).catch(() => null),
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

  const liveProduction = getLatestLiveEnvironmentDeployment(
    deployments,
    "production",
  );
  const productionUrl = getPrimaryEnvironmentUrl(domains, "production");
  const allDomains = domains ?? [];
  const totalDomainCount = allDomains.length;
  const pendingDomainCount = allDomains.filter((d) => !isDomainReady(d)).length;

  return (
    <div className="space-y-4">
      {/* A: Production Deployment Hero Card */}
      <Card>
        <CardHeader className="pb-3">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <CardTitle className="text-base font-semibold">
              Production Deployment
            </CardTitle>
            <div className="flex shrink-0 gap-2">
              <Button asChild size="sm" variant="outline">
                <a
                  href={`https://github.com/${project.github_owner}/${project.github_repo}`}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  <ExternalLink className="mr-1.5 h-3.5 w-3.5" />
                  Repository
                </a>
              </Button>
              {productionUrl ? (
                <Button asChild size="sm">
                  <a
                    href={productionUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    <ExternalLink className="mr-1.5 h-3.5 w-3.5" />
                    Visit
                  </a>
                </Button>
              ) : null}
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {liveProduction ? (
            <div className="grid gap-4 md:grid-cols-[280px_1fr]">
              {/* Left: Framework icon placeholder */}
              <div className="flex flex-col items-center justify-center gap-2 rounded-xl border bg-muted/30 px-6 py-8">
                <div className="flex h-14 w-14 items-center justify-center rounded-xl border bg-background text-xl font-bold tracking-tight text-foreground">
                  {formatFrameworkLabel(project.framework)
                    .slice(0, 2)
                    .toUpperCase()}
                </div>
                <span className="text-sm font-medium text-muted-foreground">
                  {formatFrameworkLabel(project.framework)}
                </span>
              </div>

              {/* Right: Metadata */}
              <div className="space-y-4">
                <MetaRow label="Deployment ID">
                  <span className="font-mono text-sm">
                    {liveProduction.id.slice(0, 8)}
                  </span>
                </MetaRow>

                <MetaRow label="Domains">
                  <div className="flex flex-col gap-1">
                    {allDomains
                      .filter((d) => d.environment === "production")
                      .map((d) => (
                        <a
                          key={d.id}
                          href={`https://${d.domain}`}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="inline-flex items-center gap-1 text-sm text-primary hover:underline"
                        >
                          {d.domain}
                          <ExternalLink className="h-3 w-3" />
                        </a>
                      ))}
                    {allDomains.filter((d) => d.environment === "production")
                      .length === 0 ? (
                      <span className="text-sm text-muted-foreground">
                        No production domains
                      </span>
                    ) : null}
                  </div>
                </MetaRow>

                <MetaRow label="Status">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge
                      variant="outline"
                      className={
                        DEPLOYMENT_STATUS_META[liveProduction.status].badgeClass
                      }
                    >
                      {DEPLOYMENT_STATUS_META[liveProduction.status].label}
                    </Badge>
                    <span className="text-sm text-muted-foreground">
                      {formatRelativeDate(liveProduction.created_at)}
                    </span>
                  </div>
                </MetaRow>

                <MetaRow label="Source">
                  <div className="space-y-1">
                    <div className="flex items-center gap-1.5 text-sm text-foreground">
                      <GitBranch className="h-3.5 w-3.5 text-muted-foreground" />
                      {liveProduction.branch}
                    </div>
                    <div className="flex items-center gap-1.5 text-sm text-muted-foreground">
                      <GitCommit className="h-3.5 w-3.5" />
                      <span className="font-mono">
                        {liveProduction.commit_sha
                          ? liveProduction.commit_sha.slice(0, 7)
                          : "—"}
                      </span>
                      {liveProduction.commit_message ? (
                        <span className="truncate">
                          {liveProduction.commit_message}
                        </span>
                      ) : null}
                    </div>
                  </div>
                </MetaRow>
              </div>
            </div>
          ) : (
            <div className="rounded-xl border border-dashed border-border/70 px-5 py-12 text-center text-sm text-muted-foreground">
              No production deployment yet. Release to production to see it
              here.
            </div>
          )}
        </CardContent>
      </Card>

      {/* B: Info Banner */}
      <div className="flex items-center justify-between gap-4 rounded-lg border bg-muted/30 px-4 py-3">
        <p className="text-sm text-muted-foreground">
          To update your Production Deployment, push to the{" "}
          <span className="font-medium text-foreground">{project.branch}</span>{" "}
          branch.
        </p>
        <Button
          variant="outline"
          size="sm"
          className="shrink-0"
          onClick={() =>
            navigate({ to: "/projects/$id/deployments", params: { id } })
          }
        >
          Deployments
        </Button>
      </div>

      {/* C: Three Compact Summary Cards */}
      <div className="grid gap-3 md:grid-cols-3">
        {/* Domains card */}
        <button
          type="button"
          className="group flex min-h-[120px] flex-col justify-between rounded-lg border bg-card px-4 py-3 text-left transition-colors hover:bg-accent"
          onClick={() =>
            navigate({ to: "/projects/$id/settings", params: { id } })
          }
        >
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium text-foreground">Domains</span>
            <ChevronRight className="h-4 w-4 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
          </div>
          <div>
            <p className="text-2xl font-semibold tabular-nums text-foreground">
              {totalDomainCount}
            </p>
            <p className="mt-0.5 text-xs text-muted-foreground">
              {pendingDomainCount === 0
                ? "All verified"
                : `${pendingDomainCount} pending`}
            </p>
          </div>
        </button>

        {/* Analytics card */}
        <button
          type="button"
          className="group flex min-h-[120px] flex-col justify-between rounded-lg border bg-card px-4 py-3 text-left transition-colors hover:bg-accent"
          onClick={() =>
            navigate({ to: "/projects/$id/analytics", params: { id } })
          }
        >
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium text-foreground">
              Analytics
            </span>
            <ChevronRight className="h-4 w-4 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
          </div>
          <div>
            <p className="text-sm font-medium text-foreground">
              Track visitors and page views
            </p>
            <p className="mt-0.5 text-xs text-primary">Enable</p>
          </div>
        </button>

        {/* Auto-Build card */}
        <button
          type="button"
          className="group flex min-h-[120px] flex-col justify-between rounded-lg border bg-card px-4 py-3 text-left transition-colors hover:bg-accent"
          onClick={() =>
            navigate({ to: "/projects/$id/settings", params: { id } })
          }
        >
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium text-foreground">
              Auto-Build
            </span>
            <ChevronRight className="h-4 w-4 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
          </div>
          <div>
            <p className="text-sm font-medium text-foreground">
              {autoBuild?.enabled ? "Enabled" : "Disabled"}
            </p>
            <p className="mt-0.5 text-xs text-muted-foreground">
              {autoBuild?.enabled
                ? `Branch: ${autoBuild.production_branch || project.branch}`
                : "Configure webhooks"}
            </p>
          </div>
        </button>
      </div>

      {/* D: Active Branches */}
      <ActiveBranches
        deployments={deployments ?? []}
        branchSearch={branchSearch}
        onBranchSearchChange={setBranchSearch}
        onNavigate={(did) =>
          navigate({
            to: "/projects/$id/deployments/$did",
            params: { id, did },
          })
        }
      />
    </div>
  );
}

function MetaRow({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-0.5 sm:flex-row sm:items-start sm:gap-4">
      <span className="w-28 shrink-0 text-xs font-medium uppercase tracking-wide text-muted-foreground">
        {label}
      </span>
      <div className="min-w-0 flex-1">{children}</div>
    </div>
  );
}

function ActiveBranches({
  deployments,
  branchSearch,
  onBranchSearchChange,
  onNavigate,
}: {
  deployments: Deployment[];
  branchSearch: string;
  onBranchSearchChange: (value: string) => void;
  onNavigate: (did: string) => void;
}) {
  // Group by branch, keep latest per branch
  const latestByBranch = useMemo(() => {
    const map = new Map<string, Deployment>();
    for (const d of deployments) {
      if (!map.has(d.branch)) {
        map.set(d.branch, d);
      }
    }
    return Array.from(map.values());
  }, [deployments]);

  const filtered = useMemo(() => {
    if (!branchSearch.trim()) return latestByBranch;
    const q = branchSearch.toLowerCase();
    return latestByBranch.filter((d) => d.branch.toLowerCase().includes(q));
  }, [latestByBranch, branchSearch]);

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <h3 className="text-sm font-semibold text-foreground">
          Active Branches
        </h3>
        <div className="relative">
          <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            type="text"
            placeholder="Search branches..."
            value={branchSearch}
            onChange={(e) => onBranchSearchChange(e.target.value)}
            className="h-8 w-48 pl-8 text-sm"
          />
        </div>
      </div>

      {filtered.length === 0 ? (
        <div className="rounded-lg border border-dashed border-border/70 px-5 py-10 text-center text-sm text-muted-foreground">
          {deployments.length === 0
            ? "No deployments yet."
            : "No branches match your search."}
        </div>
      ) : (
        <div className="divide-y divide-border rounded-lg border">
          {filtered.map((deployment) => {
            const meta = DEPLOYMENT_STATUS_META[deployment.status];
            const envMeta = ENVIRONMENT_META[deployment.environment];
            return (
              <button
                key={deployment.id}
                type="button"
                className="flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-accent first:rounded-t-lg last:rounded-b-lg"
                onClick={() => onNavigate(deployment.id)}
              >
                <span
                  className={cn("h-2 w-2 shrink-0 rounded-full", meta.dotClass)}
                />
                <span className="min-w-0 flex-1">
                  <span className="flex items-center gap-1.5 text-sm font-medium text-foreground">
                    <GitBranch className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                    <span className="truncate">{deployment.branch}</span>
                  </span>
                  {deployment.commit_message ? (
                    <span className="mt-0.5 block truncate text-xs text-muted-foreground">
                      {deployment.commit_message}
                    </span>
                  ) : null}
                </span>
                <Badge
                  variant="outline"
                  className={cn("shrink-0 text-xs", envMeta.badgeClass)}
                >
                  {envMeta.label}
                </Badge>
                <span className="shrink-0 text-xs text-muted-foreground">
                  {formatRelativeDate(deployment.created_at)}
                </span>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
