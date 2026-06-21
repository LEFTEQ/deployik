import { useState } from "react";
import { useParams } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { TopologyMap } from "@/components/apps/topology-map";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { LoadingState } from "@/components/ui/spinner";

type Environment = "preview" | "production";

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

  if (isLoading) return <LoadingState title="Loading topology…" />;
  const members = health?.members ?? [];
  const confirmed = (topology?.edges ?? []).filter((e) => e.confirmed);
  const nameOf = (id: string) => members.find((m) => m.project.id === id)?.project.name ?? id;

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">Topology</h2>
          <p className="text-sm text-muted-foreground">Auto-derived from env wiring. Solid = a member's variable points at a sibling; faint = reachable on the private network.</p>
        </div>
        <Select value={environment} onValueChange={(v) => setEnvironment(v as Environment)}>
          <SelectTrigger className="w-[150px]"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="production">Production</SelectItem>
            <SelectItem value="preview">Preview</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <Card>
        <CardHeader><CardTitle className="text-base">Architecture map</CardTitle></CardHeader>
        <CardContent><TopologyMap topology={topology} members={members} /></CardContent>
      </Card>

      <Card>
        <CardHeader><CardTitle className="text-base">Detected connections</CardTitle></CardHeader>
        <CardContent className="space-y-2">
          {confirmed.length === 0 ? (
            <p className="py-3 text-center text-sm text-muted-foreground">No internal references detected. Members reach each other by the injected <code>&lt;NAME&gt;_URL</code> vars.</p>
          ) : (
            confirmed.map((e, i) => (
              <div key={i} className="flex items-center gap-2 rounded-md border px-3 py-2 text-sm">
                <span className="font-medium">{nameOf(e.source)}</span>
                <span className="text-primary">→</span>
                <span className="font-medium">{nameOf(e.target)}</span>
                <Badge variant="outline" className="ml-auto font-mono text-[10px]">{e.via}</Badge>
                <Badge variant="secondary" className="text-[10px]">{e.kind}</Badge>
              </div>
            ))
          )}
        </CardContent>
      </Card>
    </div>
  );
}
