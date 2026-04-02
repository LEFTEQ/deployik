import { useQuery } from "@tanstack/react-query";
import { useParams, Link, useSearch } from "@tanstack/react-router";
import { ArrowLeft, Clock, GitBranch, GitCommit } from "lucide-react";
import { formatDistanceToNow } from "date-fns";
import { api } from "@/lib/api";
import { normalizeDeploymentReturnTab } from "@/lib/project-tabs";
import { useBuildLogs } from "@/hooks/useBuildLogs";
import { BuildLog } from "@/components/BuildLog";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { LoadingState } from "@/components/ui/spinner";

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
  const search = useSearch({ strict: false }) as { tab?: string };
  const returnTab = normalizeDeploymentReturnTab(search.tab);

  const { data: deployment, isLoading } = useQuery({
    queryKey: ["deployment", did],
    queryFn: () => api.getDeployment(id, did),
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      // Auto-refresh while deployment is in progress
      if (
        status === "queued" ||
        status === "building" ||
        status === "deploying"
      )
        return 3000;
      return false;
    },
  });

  const { data: historicalLogs } = useQuery({
    queryKey: ["build-logs", did],
    queryFn: () => api.getBuildLogs(did),
    enabled: !!deployment,
  });

  const isActive =
    deployment?.status === "queued" ||
    deployment?.status === "building" ||
    deployment?.status === "deploying";

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
      <div className="p-6">
        <LoadingState
          title="Loading deployment…"
          description="Fetching deployment metadata and build logs."
          className="min-h-[420px]"
        />
      </div>
    );
  }

  if (!deployment) {
    return (
      <div className="p-6">
        <p>Deployment not found</p>
      </div>
    );
  }

  return (
    <div className="p-6">
      <Link
        to="/projects/$id"
        params={{ id }}
        search={{ tab: returnTab }}
        className="mb-4 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        Back to project
      </Link>

      {/* Header */}
      <div className="flex items-start justify-between">
        <div>
          <div className="flex items-center gap-3">
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
          <div className="mt-2 flex items-center gap-4 text-sm text-muted-foreground">
            {deployment.commit_sha && (
              <span className="flex items-center gap-1">
                <GitCommit className="h-3.5 w-3.5" />
                {deployment.commit_sha.slice(0, 7)} {deployment.commit_message}
              </span>
            )}
            <span className="flex items-center gap-1">
              <GitBranch className="h-3.5 w-3.5" />
              {deployment.branch}
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
          </div>
        </div>

        {isActive && isConnected && (
          <Badge variant="outline" className="animate-pulse">
            Live
          </Badge>
        )}
      </div>

      {/* Build Logs */}
      <Card className="mt-6">
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Build Logs</CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          <BuildLog
            logs={allLogs}
            isStreaming={isActive}
            className="max-h-[calc(100vh-320px)] min-h-[400px] rounded-t-none"
          />
        </CardContent>
      </Card>
    </div>
  );
}
