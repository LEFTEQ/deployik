import { Fragment } from "react";
import { ArrowRight, Boxes } from "lucide-react";

import { cn } from "@/lib/utils";
import { ACTIVE_MEMBER_STATUSES, MEMBER_STATUS_META } from "@/lib/app-helpers";
import type { AppHealthMember, AppTopology } from "@/types/api";

interface TopologyMapProps {
  topology?: AppTopology;
  members: AppHealthMember[];
  compact?: boolean;
}

// Schematic dot-grid backdrop so the map reads as an architecture diagram
// rather than a plain row of chips.
const GRID_STYLE: React.CSSProperties = {
  backgroundImage:
    "radial-gradient(circle, color-mix(in oklch, var(--foreground) 7%, transparent) 1px, transparent 1px)",
  backgroundSize: "18px 18px",
  backgroundPosition: "center",
};

/**
 * The App's architecture as a left-to-right service pipeline ordered by
 * deploy_order. Each member is a status-tinted node; confirmed env-wiring edges
 * between adjacent members render as labelled data-flow connectors, and any
 * remaining gap is a faint "reachable" link (members share a private network).
 * Non-adjacent confirmed edges are surfaced in the page-level connections panel.
 */
export function TopologyMap({ topology, members, compact }: TopologyMapProps) {
  const ordered = [...members].sort(
    (a, b) => a.project.deploy_order - b.project.deploy_order,
  );

  if (ordered.length === 0) {
    return (
      <div
        className="flex flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-border/70 py-10 text-center"
        style={GRID_STYLE}
      >
        <Boxes className="h-6 w-6 text-muted-foreground/60" />
        <p className="text-sm text-muted-foreground">
          No members yet — add projects to see the architecture.
        </p>
      </div>
    );
  }

  const confirmed = (topology?.edges ?? []).filter((e) => e.confirmed);
  const statusFor = (projectId: string) =>
    members.find((m) => m.project.id === projectId)?.live_status ?? "unknown";

  // Confirmed edge between two adjacent nodes (either direction), for the label.
  const edgeBetween = (aId: string, bId: string) =>
    confirmed.find(
      (e) =>
        (e.source === aId && e.target === bId) ||
        (e.source === bId && e.target === aId),
    ) ?? null;

  return (
    <div
      className={cn(
        "relative overflow-x-auto rounded-xl border border-border/60",
        compact ? "px-4 py-6" : "px-6 py-10",
      )}
      style={GRID_STYLE}
    >
      <div className="flex min-w-max items-stretch justify-center gap-0">
        {ordered.map((m, i) => {
          const status = statusFor(m.project.id);
          const meta = MEMBER_STATUS_META[status];
          const active = ACTIVE_MEMBER_STATUSES.has(status);
          const next = ordered[i + 1];
          const edge = next ? edgeBetween(m.project.id, next.project.id) : null;

          return (
            <Fragment key={m.project.id}>
              {/* Node */}
              <div
                className={cn(
                  "relative flex min-w-[140px] flex-col gap-1.5 rounded-xl border px-4 py-3 backdrop-blur-sm",
                  meta.badgeClass,
                  "shadow-[inset_0_1px_0_0_rgba(255,255,255,0.04)]",
                )}
              >
                <div className="flex items-center gap-2">
                  <span
                    className={cn(
                      "h-2.5 w-2.5 shrink-0 rounded-full ring-2 ring-black/20",
                      meta.dotClass,
                      active && "animate-pulse",
                    )}
                  />
                  <span className="truncate text-sm font-semibold text-foreground">
                    {m.project.name}
                  </span>
                </div>
                <div className="flex items-center justify-between gap-2">
                  <span className="font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
                    {m.project.framework}
                  </span>
                  <span className="text-[10px] font-medium opacity-80">
                    {meta.label}
                  </span>
                </div>
              </div>

              {/* Connector to next node */}
              {next ? (
                <div className="flex min-w-[76px] shrink-0 flex-col items-center justify-center px-1">
                  {edge ? (
                    <span className="mb-1 rounded-full border border-primary/30 bg-primary/10 px-1.5 py-0.5 font-mono text-[8px] leading-none text-primary">
                      {edge.via}
                    </span>
                  ) : (
                    <span className="mb-1 text-[8px] uppercase tracking-wide text-muted-foreground/50">
                      reachable
                    </span>
                  )}
                  <div className="flex w-full items-center">
                    <div
                      className={cn(
                        "h-px flex-1 rounded-full",
                        edge
                          ? "animate-pulse bg-gradient-to-r from-primary/15 via-primary/70 to-primary/40"
                          : "border-t border-dashed border-border/70",
                      )}
                    />
                    <ArrowRight
                      className={cn(
                        "-ml-1 h-3 w-3 shrink-0",
                        edge ? "text-primary" : "text-muted-foreground/40",
                      )}
                    />
                  </div>
                </div>
              ) : null}
            </Fragment>
          );
        })}
      </div>

      {confirmed.length === 0 && ordered.length > 1 ? (
        <p className="mt-4 text-center text-[11px] text-muted-foreground">
          No internal references detected yet — members still reach each other by
          their injected <code className="font-mono">&lt;NAME&gt;_URL</code> vars.
        </p>
      ) : null}
    </div>
  );
}
