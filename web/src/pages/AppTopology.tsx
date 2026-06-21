import { useState } from "react";
import { useParams } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { ArrowRight, Network, Workflow } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { TopologyMap } from "@/components/apps/topology-map";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { LoadingState } from "@/components/ui/spinner";
import { cn } from "@/lib/utils";

type Environment = "preview" | "production";

const REVEAL = "animate-in fade-in slide-in-from-bottom-2 duration-500 [animation-fill-mode:both]";
const reveal = (ms: number) => ({ className: REVEAL, style: { animationDelay: `${ms}ms` } });

export function AppTopology() {
  const { appId } = useParams({ strict: false }) as { appId: string };
  const [environment, setEnvironment] = useState<Environment>("production");

  const { data: health, isLoading } = useQuery({
    queryKey: queryKeys.appHealth(appId, environment),
    queryFn: () => api.getAppHealth(appId, environment),
  });
  const { data: topology } = useQuery({
    queryKey: queryKeys.appTopology(appId, environment),
    queryFn: () => api.getAppTopology(appId, environment),
  });

  if (isLoading) return <LoadingState title="Loading topology…" className="min-h-[320px]" />;

  const members = health?.members ?? [];
  const confirmed = (topology?.edges ?? []).filter((e) => e.confirmed);
  const nameOf = (id: string) => members.find((m) => m.project.id === id)?.project.name ?? id;

  return (
    <div className="space-y-7">
      {/* Header */}
      <div {...reveal(0)} className={cn(REVEAL, "flex flex-wrap items-start justify-between gap-3")}>
        <div className="space-y-1.5">
          <div className="flex items-center gap-2">
            <span className="flex h-8 w-8 items-center justify-center rounded-lg border border-primary/20 bg-primary/10 text-primary">
              <Network className="h-4 w-4" />
            </span>
            <h1 className="text-2xl font-semibold tracking-tight">Topology</h1>
          </div>
          <p className="max-w-xl text-sm text-muted-foreground">
            Auto-derived from env wiring. Solid = a member&apos;s variable points at a sibling; faint = reachable on the private network.
          </p>
        </div>
        <Select value={environment} onValueChange={(v) => setEnvironment(v as Environment)}>
          <SelectTrigger className="w-[150px]"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="production">Production</SelectItem>
            <SelectItem value="preview">Preview</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {/* Architecture map */}
      <Card {...reveal(80)} className={cn(REVEAL, "overflow-hidden bg-gradient-to-t from-primary/5 to-card")}>
        <CardHeader className="flex flex-row items-center justify-between pb-3">
          <div>
            <CardTitle className="text-base">Architecture map</CardTitle>
            <p className="mt-0.5 text-xs text-muted-foreground">
              Service pipeline ordered by deploy order · {environment}
            </p>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          <TopologyMap topology={topology} members={members} />
          {/* Legend */}
          <div className="flex flex-wrap items-center gap-x-5 gap-y-2 px-1 text-[11px] text-muted-foreground">
            <span className="flex items-center gap-1.5">
              <span className="inline-block h-px w-6 rounded-full bg-gradient-to-r from-primary/15 via-primary/70 to-primary/40" />
              Solid line — referenced via env wiring
            </span>
            <span className="flex items-center gap-1.5">
              <span className="inline-block h-px w-6 rounded-full border-t border-dashed border-border/70" />
              Dashed line — reachable on the private network
            </span>
          </div>
        </CardContent>
      </Card>

      {/* Detected connections */}
      <Card {...reveal(160)} className={REVEAL}>
        <CardHeader className="flex flex-row items-center justify-between pb-3">
          <CardTitle className="text-base">Detected connections</CardTitle>
          {confirmed.length > 0 ? (
            <Badge variant="outline" className="font-mono text-[10px]">
              {confirmed.length} edge{confirmed.length === 1 ? "" : "s"}
            </Badge>
          ) : null}
        </CardHeader>
        <CardContent>
          {confirmed.length === 0 ? (
            <div className="flex flex-col items-center justify-center gap-2 rounded-lg border border-dashed border-border/70 py-10 text-center">
              <Workflow className="h-6 w-6 text-muted-foreground/60" />
              <p className="max-w-md text-sm text-muted-foreground">
                No internal references detected. Members still reach each other through the
                injected <code className="font-mono text-foreground">&lt;NAME&gt;_URL</code> vars on the private network.
              </p>
            </div>
          ) : (
            <div className="divide-y divide-border rounded-lg border">
              {confirmed.map((e, i) => (
                <div
                  key={`${e.source}-${e.target}-${e.via}-${i}`}
                  className="flex flex-wrap items-center gap-3 px-4 py-3"
                >
                  <span className="flex min-w-0 items-center gap-2 text-sm">
                    <span className="truncate font-medium text-foreground">{nameOf(e.source)}</span>
                    <ArrowRight className="h-4 w-4 shrink-0 text-primary" />
                    <span className="truncate font-medium text-foreground">{nameOf(e.target)}</span>
                  </span>
                  <span className="ml-auto flex shrink-0 items-center gap-2">
                    <span className="rounded-full border border-primary/30 bg-primary/10 px-2 py-0.5 font-mono text-[10px] text-primary">
                      {e.via}
                    </span>
                    {e.kind ? (
                      <Badge variant="secondary" className="text-[10px]">{e.kind}</Badge>
                    ) : null}
                  </span>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
