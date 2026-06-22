import { useState } from "react";
import type { ReactNode } from "react";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  Activity,
  ArrowRight,
  Boxes,
  Clock,
  ExternalLink,
  Globe2,
  History,
  RefreshCw,
  Rocket,
} from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys, staleTimes } from "@/lib/queryKeys";
import {
  ACTIVE_MEMBER_STATUSES,
  APP_STATUS_META,
  MEMBER_STATUS_META,
  RELEASE_STATUS_META,
} from "@/lib/app-helpers";
import {
  DEPLOYMENT_STATUS_META,
  ENVIRONMENT_META,
  ACTIVE_DEPLOYMENT_STATUSES,
  formatRelativeDate,
} from "@/lib/deployment-helpers";
import { TopologyMap } from "@/components/apps/topology-map";
import { AnalyticsStatCard } from "@/components/analytics/stat-card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { LoadingState } from "@/components/ui/spinner";
import { cn } from "@/lib/utils";
import type { AppDeployment, AppHealthMember } from "@/types/api";

type Environment = "preview" | "production";

const REVEAL = "animate-in fade-in slide-in-from-bottom-2 duration-500 [animation-fill-mode:both]";
const reveal = (ms: number) => ({ className: REVEAL, style: { animationDelay: `${ms}ms` } });

function SectionHeader({ title, sub, action }: { title: string; sub?: string; action?: ReactNode }) {
  return (
    <div className="flex items-end justify-between gap-2">
      <div>
        <h2 className="text-sm font-semibold text-foreground">{title}</h2>
        {sub ? <p className="mt-0.5 text-xs text-muted-foreground">{sub}</p> : null}
      </div>
      {action}
    </div>
  );
}

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
    queryFn: () => api.listAppDeployments(appId, environment, 6),
    staleTime: staleTimes.activeDeployments,
    refetchInterval: (query) =>
      (query.state.data ?? []).some((d) => ACTIVE_DEPLOYMENT_STATUSES.has(d.status)) ? 3000 : false,
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
      queryClient.invalidateQueries({ queryKey: queryKeys.appDeployments(appId, environment) });
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
  const recent = deployments ?? [];
  const lastDeploy = recent[0];
  const domains = members
    .map((m) => m.primary_domain)
    .filter((d): d is string => !!d)
    .slice(0, 4);

  return (
    <div className="space-y-8">
      {/* Hero */}
      <div {...reveal(0)} className={cn(REVEAL, "space-y-3")}>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="space-y-2">
            <div className="flex flex-wrap items-center gap-2">
              <span className="flex h-9 w-9 items-center justify-center rounded-lg border border-primary/20 bg-primary/10 text-primary">
                <Boxes className="h-5 w-5" />
              </span>
              <h1 className="text-2xl font-semibold tracking-tight">{app?.name}</h1>
              <Badge variant="outline" className={cn("ml-1 gap-1.5", combinedMeta.badgeClass)}>
                <span
                  className={cn(
                    "h-2 w-2 rounded-full",
                    combinedMeta.dotClass,
                    combined === "deploying" && "animate-pulse",
                  )}
                />
                {combinedMeta.label}
              </Badge>
            </div>
            <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted-foreground">
              <span>{members.length} member{members.length === 1 ? "" : "s"}</span>
              <span className="flex items-center gap-1.5">
                <span className={cn("h-1.5 w-1.5 rounded-full", app?.deploy_ordered ? "bg-primary" : "bg-muted-foreground/50")} />
                {app?.deploy_ordered ? "Ordered deploy" : "Parallel deploy"}
              </span>
              <span>
                <span className="font-medium text-foreground">{liveCount}</span>/{members.length} live
              </span>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Select value={environment} onValueChange={(v) => setEnvironment(v as Environment)}>
              <SelectTrigger className="w-[150px]"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="production">Production</SelectItem>
                <SelectItem value="preview">Preview</SelectItem>
              </SelectContent>
            </Select>
            <Button
              variant="outline"
              size="icon"
              title="Refresh"
              onClick={() => queryClient.invalidateQueries({ queryKey: queryKeys.appHealth(appId, environment) })}
            >
              <RefreshCw className="h-4 w-4" />
            </Button>
            <Button onClick={() => deployMutation.mutate()} disabled={deployMutation.isPending || members.length === 0}>
              <Rocket className="h-4 w-4" /> Deploy together
            </Button>
          </div>
        </div>

        {/* Member domain strip */}
        {domains.length > 0 ? (
          <div className="flex flex-wrap items-center gap-2 rounded-lg border bg-muted/30 px-4 py-2.5">
            <Globe2 className="h-4 w-4 shrink-0 text-muted-foreground" />
            {domains.map((d) => (
              <a
                key={d}
                href={`https://${d}`}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex min-w-0 max-w-full items-center gap-1 rounded-md border bg-background px-2 py-1 text-sm text-foreground transition-colors hover:bg-accent"
              >
                <span className="truncate">{d}</span>
                <ExternalLink className="h-3 w-3 shrink-0 text-muted-foreground" />
              </a>
            ))}
          </div>
        ) : null}
      </div>

      {/* KPI cards */}
      <div {...reveal(60)} className={cn(REVEAL, "grid grid-cols-2 gap-4 lg:grid-cols-4")}>
        <AnalyticsStatCard
          label="Live members"
          value={`${liveCount}/${members.length}`}
          icon={<Activity className="h-4 w-4" />}
          hint={combined === "healthy" ? "All members reporting healthy" : combinedMeta.label}
        />
        <AnalyticsStatCard
          label="App status"
          value={combinedMeta.label}
          icon={<span className={cn("inline-block h-2.5 w-2.5 rounded-full", combinedMeta.dotClass)} />}
          hint={app?.deploy_ordered ? "Coordinated, ordered rollout" : "Coordinated, parallel rollout"}
        />
        <AnalyticsStatCard
          label="Releases"
          value={String(releases?.length ?? 0)}
          icon={<History className="h-4 w-4" />}
          hint={`Coordinated ${environment} deploys`}
        />
        <AnalyticsStatCard
          label="Last deploy"
          value={lastDeploy ? formatRelativeDate(lastDeploy.created_at) : "—"}
          icon={<Clock className="h-4 w-4" />}
          hint={lastDeploy ? `${lastDeploy.project_name} · ${lastDeploy.commit_sha ? lastDeploy.commit_sha.slice(0, 7) : "pending"}` : "No deploys yet"}
        />
      </div>

      {/* Two columns */}
      <div className="grid grid-cols-1 gap-x-8 gap-y-8 lg:grid-cols-[1fr_340px] lg:items-start">
        {/* Main */}
        <div className="space-y-8">
          <section {...reveal(120)} className={cn(REVEAL, "space-y-3")}>
            <SectionHeader
              title="Architecture"
              sub={`Auto-derived from env wiring · ${environment}`}
              action={
                <Button asChild variant="outline" size="sm">
                  <Link to="/apps/$appId/topology" params={{ appId }}>
                    Expand <ArrowRight className="ml-1 h-3.5 w-3.5" />
                  </Link>
                </Button>
              }
            />
            <TopologyMap topology={topology} members={members} compact />
          </section>

          <section {...reveal(180)} className={cn(REVEAL, "space-y-3")}>
            <SectionHeader
              title="Members"
              action={
                <Button asChild variant="ghost" size="sm">
                  <Link to="/apps/$appId/settings" params={{ appId }}>Manage</Link>
                </Button>
              }
            />
            {members.length === 0 ? (
              <div className="rounded-lg border border-dashed border-border/70 px-5 py-8 text-center text-sm text-muted-foreground">
                No members yet.
              </div>
            ) : (
              <div className="divide-y divide-border overflow-hidden rounded-lg border">
                {members.map((m) => (
                  <MemberRow
                    key={m.project.id}
                    member={m}
                    ordered={!!app?.deploy_ordered}
                    onOpen={() => navigate({ to: "/projects/$id", params: { id: m.project.id } })}
                  />
                ))}
              </div>
            )}
          </section>
        </div>

        {/* Sticky pulse rail */}
        <div className="space-y-8 lg:sticky lg:top-4">
          <section {...reveal(240)} className={cn(REVEAL, "space-y-3")}>
            <SectionHeader
              title="Recent deployments"
              action={
                <Link
                  to="/apps/$appId/deployments"
                  params={{ appId }}
                  className="inline-flex items-center text-sm text-primary transition-colors hover:underline"
                >
                  See all <ArrowRight className="ml-1 h-3.5 w-3.5" />
                </Link>
              }
            />
            {recent.length === 0 ? (
              <div className="rounded-lg border border-dashed border-border/70 px-4 py-6 text-center text-sm text-muted-foreground">
                No deployments yet.
              </div>
            ) : (
              <div className="divide-y divide-border overflow-hidden rounded-lg border">
                {recent.map((d) => (
                  <DeployRow
                    key={d.id}
                    d={d}
                    onOpen={() => navigate({ to: "/projects/$id/deployments/$did", params: { id: d.project_id, did: d.id } })}
                  />
                ))}
              </div>
            )}
          </section>

          <section {...reveal(300)} className={cn(REVEAL, "space-y-3")}>
            <SectionHeader
              title="Releases"
              action={
                <Link to="/apps/$appId/releases" params={{ appId }} className="text-sm text-primary transition-colors hover:underline">
                  All
                </Link>
              }
            />
            {(releases ?? []).length === 0 ? (
              <div className="rounded-lg border border-dashed border-border/70 px-4 py-6 text-center text-sm text-muted-foreground">
                No releases yet.
              </div>
            ) : (
              <div className="divide-y divide-border overflow-hidden rounded-lg border">
                {(releases ?? []).slice(0, 4).map((r) => {
                  const meta = RELEASE_STATUS_META[r.status];
                  return (
                    <div key={r.id} className="flex items-center gap-2 px-3 py-2.5 text-sm">
                      <span className={cn("h-1.5 w-1.5 shrink-0 rounded-full", meta.dotClass)} />
                      <Badge variant="outline" className={cn("text-[10px]", meta.badgeClass)}>{meta.label}</Badge>
                      <span className="ml-auto font-mono text-[11px] text-muted-foreground">{r.id.slice(0, 8)}</span>
                      <span className="text-[11px] text-muted-foreground">{formatRelativeDate(r.created_at)}</span>
                    </div>
                  );
                })}
              </div>
            )}
          </section>
        </div>
      </div>
    </div>
  );
}

function MemberRow({ member, ordered, onOpen }: { member: AppHealthMember; ordered: boolean; onOpen: () => void }) {
  const meta = MEMBER_STATUS_META[member.live_status];
  const active = ACTIVE_MEMBER_STATUSES.has(member.live_status);
  const lastAt = member.latest_deployment?.created_at;
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onOpen();
        }
      }}
      className="group flex cursor-pointer items-center gap-3 px-4 py-3 transition-colors hover:bg-accent"
    >
      <span className={cn("h-2.5 w-2.5 shrink-0 rounded-full", meta.dotClass, active && "animate-pulse")} />
      <span className="truncate text-sm font-medium text-foreground">{member.project.name}</span>
      <Badge variant="outline" className="border-primary/20 bg-primary/10 font-mono text-[10px] text-primary">
        {member.project.framework}
      </Badge>
      {ordered ? (
        <span className="font-mono text-[11px] text-muted-foreground">#{member.project.deploy_order}</span>
      ) : null}
      <span className="ml-auto flex items-center gap-3">
        {member.primary_domain ? (
          <a
            href={`https://${member.primary_domain}`}
            target="_blank"
            rel="noopener noreferrer"
            className="hidden items-center gap-1 text-xs text-muted-foreground transition-colors hover:text-foreground sm:flex"
            onClick={(e) => e.stopPropagation()}
          >
            <span className="max-w-[180px] truncate">{member.primary_domain}</span>
            <ExternalLink className="h-3 w-3" />
          </a>
        ) : null}
        {lastAt ? <span className="hidden text-xs text-muted-foreground md:inline">{formatRelativeDate(lastAt)}</span> : null}
        <ArrowRight className="h-4 w-4 text-muted-foreground/40 transition-transform group-hover:translate-x-0.5 group-hover:text-muted-foreground" />
      </span>
    </div>
  );
}

function DeployRow({ d, onOpen }: { d: AppDeployment; onOpen: () => void }) {
  const meta = DEPLOYMENT_STATUS_META[d.status];
  const envMeta = ENVIRONMENT_META[d.environment];
  const active = ACTIVE_DEPLOYMENT_STATUSES.has(d.status);
  return (
    <button
      type="button"
      onClick={onOpen}
      className="flex w-full items-center gap-2 px-3 py-2.5 text-left text-sm transition-colors hover:bg-accent"
    >
      <span className={cn("h-2 w-2 shrink-0 rounded-full", meta?.dotClass, active && "animate-pulse")} />
      <span className="truncate font-medium text-foreground">{d.project_name}</span>
      <Badge variant="outline" className={cn("shrink-0 text-[10px]", envMeta?.badgeClass)}>{envMeta?.label}</Badge>
      <span className="ml-auto shrink-0 font-mono text-xs text-muted-foreground">
        {d.commit_sha ? d.commit_sha.slice(0, 7) : "—"}
      </span>
      <span className="shrink-0 text-xs text-muted-foreground">{formatRelativeDate(d.created_at)}</span>
    </button>
  );
}
