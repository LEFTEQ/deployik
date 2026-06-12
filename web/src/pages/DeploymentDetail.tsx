import { useQuery } from "@tanstack/react-query";
import { useParams, Link } from "@tanstack/react-router";
import {
  ArrowLeft,
  Clock,
  ExternalLink,
  GitBranch,
  GitCommit,
} from "lucide-react";
import { formatDistanceToNow } from "date-fns";
import { api } from "@/lib/api";
import { queryKeys, staleTimes } from "@/lib/queryKeys";
import {
  ACTIVE_DEPLOYMENT_STATUSES,
  getPreferredEnvironmentDomain,
} from "@/lib/deployment-helpers";
import { useBuildLogs } from "@/hooks/useBuildLogs";
import { BuildLog } from "@/components/BuildLog";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { LoadingState } from "@/components/ui/spinner";
import { BranchLink, CommitLink } from "@/components/ui/github-link";
import { cn } from "@/lib/utils";

const statusColor: Record<string, string> = {
  queued: "bg-muted-foreground",
  building: "bg-yellow-500",
  deploying: "bg-blue-500",
  live: "bg-green-500",
  failed: "bg-red-500",
  rolled_back: "bg-orange-500",
  replaced: "bg-muted-foreground",
};

export function DeploymentDetail() {
  const { id, did } = useParams({ strict: false }) as {
    id: string;
    did: string;
  };

  const { data: deployment, isLoading } = useQuery({
    queryKey: queryKeys.deployment(did),
    queryFn: () => api.getDeployment(id, did),
    staleTime: staleTimes.activeDeployments,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (
        status === "queued" ||
        status === "building" ||
        status === "deploying"
      )
        return 3000;
      return false;
    },
  });

  const { data: project } = useQuery({
    queryKey: queryKeys.project(id),
    queryFn: () => api.getProject(id),
  });

  const { data: historicalLogs } = useQuery({
    queryKey: queryKeys.buildLogs(did),
    queryFn: () => api.getBuildLogs(did),
    enabled: !!deployment,
  });

  const { data: domains, isLoading: isDomainsLoading } = useQuery({
    queryKey: queryKeys.domains(id),
    queryFn: () => api.listDomains(id),
    enabled: !!deployment,
  });

  const isActive = deployment
    ? ACTIVE_DEPLOYMENT_STATUSES.has(deployment.status)
    : false;

  const { logs: streamLogs, isConnected } = useBuildLogs(isActive ? did : null);

  // Merge historical + streaming logs, dedup by line number
  const allLogs = (() => {
    const historical = (historicalLogs ?? []).map((l) => ({
      line_number: l.line_number,
      content: l.content,
      stream: l.stream as "stdout" | "stderr",
    }));
    const seen = new Set(historical.map((l) => l.line_number));
    const merged = [...historical];
    for (const l of streamLogs) {
      if (!seen.has(l.line_number)) {
        merged.push(l);
        seen.add(l.line_number);
      }
    }
    return merged.sort((a, b) => a.line_number - b.line_number);
  })();

  if (isLoading) {
    return (
      <LoadingState
        title="Loading deployment..."
        description="Fetching deployment metadata and build logs."
        className="min-h-[420px]"
      />
    );
  }

  if (!deployment) {
    return <p>Deployment not found</p>;
  }

  const preferredDomain = getPreferredEnvironmentDomain(
    domains,
    deployment.environment,
    deployment.preview_instance_id,
  );
  const deploymentUrl = preferredDomain
    ? `https://${preferredDomain.domain}`
    : null;
  const canOpenDeployment = deployment.status === "live" && !!deploymentUrl;
  const domainClassName = cn(
    "inline-flex h-8 max-w-full items-center gap-1.5 rounded-md border px-2.5 text-sm font-medium transition-colors",
    canOpenDeployment
      ? "border-primary/25 bg-primary/10 text-primary hover:bg-primary/15"
      : "cursor-not-allowed border-white/10 bg-white/[0.03] text-muted-foreground/80",
  );
  const isDomainShimmering =
    (isActive || isDomainsLoading) && (preferredDomain || isDomainsLoading);
  const domainTextClassName = cn(
    "truncate",
    isDomainShimmering && "deployment-domain-text-shimmer",
  );
  const domainLabel =
    preferredDomain?.domain ??
    (isDomainsLoading ? "Loading domain" : "No domain");

  return (
    <div>
      <Link
        to="/projects/$id/deployments"
        params={{ id }}
        className="mb-6 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        Back to deployments
      </Link>

      {/* Header */}
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-3">
            <div
              className={`h-3 w-3 rounded-full ${statusColor[deployment.status] ?? "bg-muted-foreground"} ${isActive ? "animate-pulse" : ""}`}
            />
            <h1 className="text-xl font-bold tracking-tight">
              Deployment {deployment.id.slice(0, 8)}
            </h1>
            <Badge
              variant={
                deployment.status === "live"
                  ? "default"
                  : deployment.status === "failed"
                    ? "destructive"
                    : "secondary"
              }
            >
              {deployment.status}
            </Badge>
          </div>
          <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-2 text-sm text-muted-foreground">
            {deployment.commit_sha && (
              <span className="flex min-w-0 max-w-full items-center gap-1">
                <GitCommit className="h-3.5 w-3.5 shrink-0" />
                {project ? (
                  <CommitLink
                    owner={project.github_owner}
                    repo={project.github_repo}
                    sha={deployment.commit_sha}
                  />
                ) : (
                  deployment.commit_sha.slice(0, 7)
                )}{" "}
                <span className="min-w-0 truncate">
                  {deployment.commit_message}
                </span>
              </span>
            )}
            <span className="flex min-w-0 max-w-full items-center gap-1">
              <GitBranch className="h-3.5 w-3.5 shrink-0" />
              <span className="min-w-0 truncate">
                {project ? (
                  <BranchLink
                    owner={project.github_owner}
                    repo={project.github_repo}
                    branch={deployment.branch}
                  />
                ) : (
                  deployment.branch
                )}
              </span>
            </span>
            <span className="flex items-center gap-1">
              <Clock className="h-3.5 w-3.5" />
              {formatDistanceToNow(new Date(deployment.created_at), {
                addSuffix: true,
              })}
            </span>
            {deployment.build_duration > 0 && (
              <span>{deployment.build_duration}s</span>
            )}
            {canOpenDeployment ? (
              <a
                href={deploymentUrl}
                target="_blank"
                rel="noopener noreferrer"
                className={domainClassName}
              >
                <span className={domainTextClassName}>{domainLabel}</span>
                <ExternalLink className="h-3.5 w-3.5 shrink-0" />
              </a>
            ) : (
              <span className={domainClassName} aria-disabled="true">
                <span className={domainTextClassName}>{domainLabel}</span>
                {preferredDomain && (
                  <ExternalLink className="h-3.5 w-3.5 shrink-0" />
                )}
              </span>
            )}
          </div>
        </div>

        {isActive && isConnected && (
          <Badge variant="outline" className="animate-pulse">
            Live
          </Badge>
        )}
      </div>

      {/* Build Logs */}
      <Card className="-mx-4 mt-8 rounded-none border-x-0 md:mx-0 md:rounded-xl md:border-x">
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Build Logs</CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          <BuildLog
            logs={allLogs}
            isStreaming={isActive}
            className="max-h-[calc(100vh-320px)] min-h-[400px] rounded-none md:rounded-md md:rounded-t-none"
          />
        </CardContent>
      </Card>
    </div>
  );
}
