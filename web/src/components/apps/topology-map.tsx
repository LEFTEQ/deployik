import { Fragment } from "react";
import { cn } from "@/lib/utils";
import { MEMBER_STATUS_META } from "@/lib/app-helpers";
import type { AppHealthMember, AppTopology } from "@/types/api";

interface TopologyMapProps {
  topology?: AppTopology;
  members: AppHealthMember[];
  compact?: boolean;
}

// Renders members left-to-right by deploy_order with confirmed edges labeled by
// the variable that creates them and faint "reachable" links between the rest.
export function TopologyMap({ topology, members, compact }: TopologyMapProps) {
  const ordered = [...members].sort((a, b) => a.project.deploy_order - b.project.deploy_order);
  if (ordered.length === 0) {
    return <p className="py-6 text-center text-sm text-muted-foreground">No members yet — add projects to see the architecture.</p>;
  }
  const edges = topology?.edges ?? [];
  const confirmed = edges.filter((e) => e.confirmed);
  const statusFor = (projectId: string) => members.find((m) => m.project.id === projectId)?.live_status ?? "unknown";

  // edge label between two adjacent nodes (if a confirmed edge exists in either direction)
  const labelBetween = (aId: string, bId: string) => {
    const e = confirmed.find((x) => (x.source === aId && x.target === bId) || (x.source === bId && x.target === aId));
    return e?.via ?? null;
  };

  return (
    <div className={cn("flex flex-wrap items-center justify-center gap-y-3", compact ? "py-3" : "py-8")}>
      {ordered.map((m, i) => {
        const meta = MEMBER_STATUS_META[statusFor(m.project.id)];
        const next = ordered[i + 1];
        const label = next ? labelBetween(m.project.id, next.project.id) : null;
        return (
          <Fragment key={m.project.id}>
            <div className="flex items-center gap-2 rounded-lg border border-primary/40 bg-primary/10 px-3 py-2">
              <span className={cn("h-2 w-2 rounded-full", meta.dotClass)} />
              <span className="text-sm font-medium">{m.project.name}</span>
              <span className="text-[10px] text-muted-foreground">{m.project.framework}</span>
            </div>
            {next ? (
              <div className="flex min-w-[56px] flex-col items-center px-1 text-muted-foreground">
                <span className={cn("text-base leading-none", label ? "text-primary" : "text-muted-foreground/40")}>→</span>
                {label ? <span className="mt-1 font-mono text-[8px] text-primary">{label}</span> : <span className="mt-1 text-[8px] text-muted-foreground/50">reachable</span>}
              </div>
            ) : null}
          </Fragment>
        );
      })}
      {confirmed.length === 0 ? (
        <p className="mt-3 w-full text-center text-[11px] text-muted-foreground">No internal references detected yet.</p>
      ) : null}
    </div>
  );
}
