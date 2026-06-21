import { useState } from "react";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ArrowRight, Boxes, ExternalLink, RefreshCw, Rocket } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys, staleTimes } from "@/lib/queryKeys";
import {
  APP_STATUS_META,
  MEMBER_STATUS_META,
  RELEASE_STATUS_META,
  ACTIVE_MEMBER_STATUSES,
} from "@/lib/app-helpers";
import { DEPLOYMENT_STATUS_META, ENVIRONMENT_META, formatRelativeDate } from "@/lib/deployment-helpers";
import { TopologyMap } from "@/components/apps/topology-map";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { LoadingState } from "@/components/ui/spinner";
import { cn } from "@/lib/utils";
import type { AppDeployment, AppHealthMember } from "@/types/api";

type Environment = "preview" | "production";

export function AppOverview() {
  const { appId } = useParams({ strict: false }) as { appId: string };
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [environment, setEnvironment] = useState<Environment>("production");

  const { data: health, isLoading } = useQuery({
    queryKey: queryKeys.appHealth(appId, environment),
    queryFn: () => api.getAppHealth(appId, environment),
    staleTime: staleTimes.activeDeployments,
    refetchInterval: (query) =>
      (query.state.data?.members ?? []).some((m) => ACTIVE_MEMBER_STATUSES.has(m.live_status)) ? 3000 : false,
  });
  const { data: topology } = useQuery({
    queryKey: queryKeys.appTopology(appId, environment),
    queryFn: () => api.getAppTopology(appId, environment),
  });
  const { data: deployments } = useQuery({
    queryKey: queryKeys.appDeployments(appId, environment),
    queryFn: () => api.listAppDeployments(appId, environment, 5),
    staleTime: staleTimes.activeDeployments,
  });
  const { data: releases } = useQuery({
    queryKey: queryKeys.appReleases(appId, environment),
    queryFn: () => api.listAppReleases(appId, environment),
  });

  const deployMutation = useMutation({
    mutationFn: () => api.deployApp(appId, environment),
    onSuccess: (r) => {
      toast.success(`Deploying ${r.member_count} member(s) to ${environment}`);
      queryClient.invalidateQueries({ queryKey: queryKeys.appReleases(appId, environment) });
      queryClient.invalidateQueries({ queryKey: queryKeys.appHealth(appId, environment) });
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : "Failed to start deploy"),
  });

  if (isLoading) {
    return <LoadingState title="Loading app…" className="min-h-[320px]" />;
  }
  const app = health?.app;
  const members = health?.members ?? [];
  const combined = health?.combined_status ?? "none";
  const combinedMeta = APP_STATUS_META[combined];
  const liveCount = members.filter((m) => m.live_status === "healthy").length;

  return (
    <div className="space-y-6">
      {/* hero */}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-2">
          <h1 className="flex items-center gap-2 text-2xl font-semibold tracking-tight">
            <Boxes className="h-6 w-6" /> {app?.name}
            <Badge variant="outline" className={cn("ml-1 gap-1.5", combinedMeta.badgeClass)}>
              <span className={cn("h-2 w-2 rounded-full", combinedMeta.dotClass)} />
              {combinedMeta.label}
            </Badge>
          </h1>
          <p className="text-sm text-muted-foreground">
            {members.length} member{members.length === 1 ? "" : "s"}
            {app?.deploy_ordered ? " · ordered deploy" : " · parallel deploy"} · {liveCount}/{members.length} live
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Select value={environment} onValueChange={(v) => setEnvironment(v as Environment)}>
            <SelectTrigger className="w-[150px]"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="production">Production</SelectItem>
              <SelectItem value="preview">Preview</SelectItem>
            </SelectContent>
          </Select>
          <Button variant="outline" size="icon" title="Refresh"
            onClick={() => queryClient.invalidateQueries({ queryKey: queryKeys.appHealth(appId, environment) })}>
            <RefreshCw className="h-4 w-4" />
          </Button>
          <Button onClick={() => deployMutation.mutate()} disabled={deployMutation.isPending || members.length === 0}>
            <Rocket className="h-4 w-4" /> Deploy together
          </Button>
        </div>
      </div>

      {/* two columns */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[1fr_320px] lg:items-start">
        {/* main */}
        <div className="space-y-6">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-3">
              <CardTitle className="text-base">Architecture</CardTitle>
              <Link to="/apps/$appId" params={{ appId }} className="text-sm text-primary hover:underline">
                Expand
              </Link>
            </CardHeader>
            <CardContent>
              <TopologyMap topology={topology} members={members} compact />
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-3">
              <CardTitle className="text-base">Members</CardTitle>
              <Link to="/apps/$appId" params={{ appId }} className="text-sm text-primary hover:underline">
                Manage
              </Link>
            </CardHeader>
            <CardContent className="space-y-2">
              {members.length === 0 ? (
                <p className="py-4 text-center text-sm text-muted-foreground">No members yet.</p>
              ) : (
                members.map((m) => <MemberRow key={m.project.id} member={m} ordered={!!app?.deploy_ordered} onOpen={() => navigate({ to: "/projects/$id", params: { id: m.project.id } })} />)
              )}
            </CardContent>
          </Card>
        </div>

        {/* sticky rail */}
        <div className="space-y-4 lg:sticky lg:top-4">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-3">
              <CardTitle className="text-base">Health</CardTitle>
              <Badge variant="outline" className={cn("gap-1.5", combinedMeta.badgeClass)}>
                <span className={cn("h-2 w-2 rounded-full", combinedMeta.dotClass)} />
                {combinedMeta.label}
              </Badge>
            </CardHeader>
            <CardContent className="grid grid-cols-2 gap-2">
              <Kpi label="Live" value={`${liveCount}/${members.length}`} />
              <Kpi label="Members" value={String(members.length)} />
              <Kpi label="Releases" value={String(releases?.length ?? 0)} />
              <Kpi label="Mode" value={app?.deploy_ordered ? "Ordered" : "Parallel"} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-3">
              <CardTitle className="text-base">Recent deployments</CardTitle>
              <Link to="/apps/$appId" params={{ appId }} className="inline-flex items-center text-sm text-primary hover:underline">
                See all <ArrowRight className="ml-1 h-3.5 w-3.5" />
              </Link>
            </CardHeader>
            <CardContent className="space-y-1">
              {(deployments ?? []).length === 0 ? (
                <p className="py-3 text-center text-sm text-muted-foreground">No deployments yet.</p>
              ) : (
                (deployments ?? []).map((d) => <DeployRow key={d.id} d={d} onOpen={() => navigate({ to: "/projects/$id/deployments/$did", params: { id: d.project_id, did: d.id } })} />)
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-3">
              <CardTitle className="text-base">Releases</CardTitle>
              <Link to="/apps/$appId" params={{ appId }} className="text-sm text-primary hover:underline">All</Link>
            </CardHeader>
            <CardContent className="space-y-1">
              {(releases ?? []).slice(0, 3).length === 0 ? (
                <p className="py-3 text-center text-sm text-muted-foreground">No releases yet.</p>
              ) : (
                (releases ?? []).slice(0, 3).map((r) => {
                  const meta = RELEASE_STATUS_META[r.status];
                  return (
                    <div key={r.id} className="flex items-center justify-between rounded-md border px-3 py-2 text-sm">
                      <Badge variant="outline" className={meta.badgeClass}>{meta.label}</Badge>
                      <span className="font-mono text-xs text-muted-foreground">{r.id.slice(0, 10)}</span>
                      <span className="text-xs text-muted-foreground">{formatRelativeDate(r.created_at)}</span>
                    </div>
                  );
                })
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}

function Kpi({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border bg-muted/20 px-3 py-2">
      <p className="text-[10px] uppercase tracking-wide text-muted-foreground">{label}</p>
      <p className="mt-0.5 text-lg font-semibold">{value}</p>
    </div>
  );
}

function MemberRow({ member, ordered, onOpen }: { member: AppHealthMember; ordered: boolean; onOpen: () => void }) {
  const meta = MEMBER_STATUS_META[member.live_status];
  return (
    <div className="flex items-center justify-between rounded-md border px-3 py-2">
      <div className="flex min-w-0 items-center gap-3">
        <span className={cn("h-2 w-2 shrink-0 rounded-full", meta.dotClass)} />
        <span className="truncate font-medium">{member.project.name}</span>
        <Badge variant="secondary" className="text-xs">{member.project.framework}</Badge>
        {ordered && <span className="text-xs text-muted-foreground">order {member.project.deploy_order}</span>}
      </div>
      <div className="flex shrink-0 items-center gap-3">
        {member.primary_domain ? (
          <a href={`https://${member.primary_domain}`} target="_blank" rel="noopener noreferrer"
            className="hidden items-center gap-1 text-xs text-muted-foreground hover:text-foreground sm:flex"
            onClick={(e) => e.stopPropagation()}>
            {member.primary_domain} <ExternalLink className="h-3 w-3" />
          </a>
        ) : null}
        <Button variant="ghost" size="sm" onClick={onOpen}>Open</Button>
      </div>
    </div>
  );
}

function DeployRow({ d, onOpen }: { d: AppDeployment; onOpen: () => void }) {
  const meta = DEPLOYMENT_STATUS_META[d.status];
  const envMeta = ENVIRONMENT_META[d.environment];
  return (
    <button type="button" onClick={onOpen}
      className="flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm transition-colors hover:bg-accent">
      <span className={cn("h-2 w-2 shrink-0 rounded-full", meta?.dotClass)} />
      <span className="font-medium">{d.project_name}</span>
      <Badge variant="outline" className={cn("text-[10px]", envMeta?.badgeClass)}>{envMeta?.label}</Badge>
      <span className="ml-auto font-mono text-xs text-muted-foreground">{d.commit_sha ? d.commit_sha.slice(0, 7) : "—"}</span>
      <span className="text-xs text-muted-foreground">{formatRelativeDate(d.created_at)}</span>
    </button>
  );
}
