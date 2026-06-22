import { ExternalLink, GitBranch, Terminal } from "lucide-react";

import { ACTIVE_MEMBER_STATUSES, MEMBER_STATUS_META } from "@/lib/app-helpers";
import {
  buildGithubCommitUrl,
  buildGithubRepoUrl,
  formatRelativeDate,
} from "@/lib/deployment-helpers";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import type { AppHealthMember, Project } from "@/types/api";

/** A container the live-logs sheet can stream (member + environment + branch). */
export interface LogTarget {
  projectId: string;
  projectName: string;
  environment: "preview" | "production";
  /** Preview branch slug/name; omitted for production (single instance). */
  branch?: string;
}

/** One matrix row: a member project with its preview and/or production health. */
export interface MatrixRow {
  project: Project;
  preview?: AppHealthMember;
  production?: AppHealthMember;
}

/**
 * Merge per-environment member lists into one row set keyed by project id,
 * preserving the order members first appear (preview order, then any
 * production-only members appended).
 */
export function buildMatrixRows(
  preview: AppHealthMember[] = [],
  production: AppHealthMember[] = [],
): MatrixRow[] {
  const byId = new Map<string, MatrixRow>();
  const order: string[] = [];
  const add = (member: AppHealthMember, env: "preview" | "production") => {
    let row = byId.get(member.project.id);
    if (!row) {
      row = { project: member.project };
      byId.set(member.project.id, row);
      order.push(member.project.id);
    }
    row[env] = member;
  };
  preview.forEach((member) => add(member, "preview"));
  production.forEach((member) => add(member, "production"));
  return order.map((id) => byId.get(id) as MatrixRow);
}

const GRID = "grid grid-cols-[minmax(140px,1.1fr)_1.4fr_1fr] items-center gap-3";

export function ServiceMatrix({
  rows,
  ordered,
  onOpenLogs,
}: {
  rows: MatrixRow[];
  ordered: boolean;
  onOpenLogs?: (target: LogTarget) => void;
}) {
  if (rows.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-border/70 px-5 py-8 text-center text-sm text-muted-foreground">
        No members yet.
      </div>
    );
  }
  return (
    <div className="overflow-hidden rounded-lg border">
      <div
        className={cn(
          GRID,
          "border-b bg-muted/30 px-4 py-2 text-[10px] uppercase tracking-wide text-muted-foreground",
        )}
      >
        <span>Member</span>
        <span className="flex items-center gap-1.5 text-emerald-300/80">
          <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" /> Development
        </span>
        <span>Production</span>
      </div>
      <div className="divide-y divide-border">
        {rows.map((row) => (
          <RowView
            key={row.project.id}
            row={row}
            ordered={ordered}
            onOpenLogs={onOpenLogs}
          />
        ))}
      </div>
    </div>
  );
}

function RowView({
  row,
  ordered,
  onOpenLogs,
}: {
  row: MatrixRow;
  ordered: boolean;
  onOpenLogs?: (target: LogTarget) => void;
}) {
  const { project } = row;
  const hasRepo = !!project.github_owner && !!project.github_repo;
  return (
    <div className={cn(GRID, "px-4 py-3")}>
      <div className="flex min-w-0 items-center gap-2">
        <span className="truncate text-sm font-medium text-foreground">
          {project.name}
        </span>
        <Badge
          variant="outline"
          className="shrink-0 border-primary/20 bg-primary/10 font-mono text-[10px] text-primary"
        >
          {project.framework}
        </Badge>
        {ordered ? (
          <span className="shrink-0 font-mono text-[11px] text-muted-foreground">
            #{project.deploy_order}
          </span>
        ) : null}
        {hasRepo ? (
          <a
            href={buildGithubRepoUrl(project.github_owner, project.github_repo)}
            target="_blank"
            rel="noopener noreferrer"
            title="Source on GitHub"
            className="ml-auto shrink-0 text-muted-foreground/60 transition-colors hover:text-foreground"
          >
            <GitBranch className="h-3.5 w-3.5" />
          </a>
        ) : null}
      </div>
      <EnvCell
        member={row.preview}
        environment="preview"
        project={project}
        primary
        onOpenLogs={onOpenLogs}
      />
      <EnvCell
        member={row.production}
        environment="production"
        project={project}
        onOpenLogs={onOpenLogs}
      />
    </div>
  );
}

function EnvCell({
  member,
  environment,
  project,
  primary,
  onOpenLogs,
}: {
  member?: AppHealthMember;
  environment: "preview" | "production";
  project: Project;
  primary?: boolean;
  onOpenLogs?: (target: LogTarget) => void;
}) {
  const cellBase = "flex items-center gap-2 rounded-md px-2.5 py-1.5 text-xs";

  if (!member) {
    return (
      <div className={cn(cellBase, "opacity-50")}>
        <span className="font-mono text-muted-foreground">— not deployed —</span>
      </div>
    );
  }

  const meta = MEMBER_STATUS_META[member.live_status];
  const active = ACTIVE_MEMBER_STATUSES.has(member.live_status);
  const deployment = member.latest_deployment;
  const sha = deployment?.commit_sha ? deployment.commit_sha.slice(0, 7) : null;
  const hasRepo = !!project.github_owner && !!project.github_repo;

  const openLogs = () =>
    onOpenLogs?.({
      projectId: project.id,
      projectName: project.name,
      environment,
      branch:
        environment === "preview"
          ? (deployment?.branch ?? undefined)
          : undefined,
    });

  return (
    <div
      className={cn(
        cellBase,
        primary
          ? "border border-emerald-400/20 bg-emerald-400/[0.07]"
          : "opacity-70",
      )}
    >
      <span
        className={cn(
          "h-2 w-2 shrink-0 rounded-full",
          meta.dotClass,
          active && "animate-pulse",
        )}
        title={meta.label}
      />
      {member.primary_domain ? (
        <a
          href={`https://${member.primary_domain}`}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex min-w-0 items-center gap-1 font-mono text-foreground transition-colors hover:text-primary"
        >
          <span className="truncate">{member.primary_domain}</span>
          <ExternalLink className="h-3 w-3 shrink-0 text-muted-foreground" />
        </a>
      ) : (
        <span className="font-mono text-muted-foreground">internal</span>
      )}
      {sha ? (
        hasRepo && deployment?.commit_sha ? (
          <a
            href={buildGithubCommitUrl(
              project.github_owner,
              project.github_repo,
              deployment.commit_sha,
            )}
            target="_blank"
            rel="noopener noreferrer"
            className="shrink-0 font-mono text-[10px] text-muted-foreground/70 hover:text-foreground"
          >
            {sha}
          </a>
        ) : (
          <span className="shrink-0 font-mono text-[10px] text-muted-foreground/70">
            {sha}
          </span>
        )
      ) : null}
      <span className="ml-auto flex shrink-0 items-center gap-2">
        {deployment?.created_at ? (
          <span className="text-[10px] text-muted-foreground">
            {formatRelativeDate(deployment.created_at)}
          </span>
        ) : null}
        {onOpenLogs ? (
          <button
            type="button"
            onClick={openLogs}
            title="Live logs"
            className="rounded border border-border/60 px-1 py-0.5 text-muted-foreground transition-colors hover:border-primary hover:text-primary"
          >
            <Terminal className="h-3 w-3" />
          </button>
        ) : null}
      </span>
    </div>
  );
}
