import { ExternalLink, GitCommit } from "lucide-react";

import {
  ACTIVE_DEPLOYMENT_STATUSES,
  DEPLOYMENT_STATUS_META,
  ENVIRONMENT_META,
  formatRelativeDate,
} from "@/lib/deployment-helpers";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { Deployment } from "@/types/api";

interface DeploymentCardProps {
  deployment: Deployment;
  liveUrl?: string | null;
  onOpen: () => void;
  /** Extra action rendered on the card's bottom row (e.g. a Logs link). */
  action?: React.ReactNode;
}

/**
 * Touch-friendly deployment row used in place of wide tables below `md`.
 * The whole card is tappable; secondary actions must stopPropagation.
 */
export function DeploymentCard({
  deployment,
  liveUrl,
  onOpen,
  action,
}: DeploymentCardProps) {
  const statusMeta = DEPLOYMENT_STATUS_META[deployment.status];

  return (
    <div
      data-testid="deployment-card"
      role="link"
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onOpen();
        }
      }}
      className={cn(
        "cursor-pointer rounded-lg border border-white/8 bg-card p-4 transition-colors active:bg-white/[0.06]",
        deployment.status === "live" && "bg-white/[0.03]",
      )}
    >
      <div className="flex items-center gap-2">
        <span
          className={cn(
            "h-2.5 w-2.5 shrink-0 rounded-full",
            statusMeta.dotClass,
            ACTIVE_DEPLOYMENT_STATUSES.has(deployment.status) &&
              "animate-pulse",
          )}
        />
        <Badge
          variant="outline"
          className={ENVIRONMENT_META[deployment.environment].badgeClass}
        >
          {ENVIRONMENT_META[deployment.environment].label}
        </Badge>
        <Badge variant="outline" className={statusMeta.badgeClass}>
          {statusMeta.label}
        </Badge>
        <span className="ml-auto shrink-0 text-xs text-muted-foreground">
          {formatRelativeDate(deployment.created_at)}
        </span>
      </div>

      <div className="mt-3 flex items-start gap-2">
        <GitCommit className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <p className="min-w-0 flex-1 truncate text-sm font-medium text-foreground">
          {deployment.commit_message ||
            deployment.error_message ||
            statusMeta.label}
        </p>
      </div>

      <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
        <span className="truncate font-mono">{deployment.branch}</span>
        {deployment.commit_sha ? (
          <span className="font-mono">{deployment.commit_sha.slice(0, 7)}</span>
        ) : null}
        {deployment.build_duration > 0 ? (
          <span>{deployment.build_duration}s</span>
        ) : null}
      </div>

      {(liveUrl || action) && (
        <div className="mt-3 flex items-center gap-2">
          {liveUrl ? (
            <Button asChild size="sm" variant="outline" className="h-9">
              <a
                href={liveUrl}
                target="_blank"
                rel="noopener noreferrer"
                onClick={(event) => event.stopPropagation()}
              >
                <ExternalLink className="mr-1.5 h-3.5 w-3.5" />
                Open
              </a>
            </Button>
          ) : null}
          {action}
        </div>
      )}
    </div>
  );
}
