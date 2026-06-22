import { Link } from "@tanstack/react-router";
import {
  ExternalLink,
  GitBranch,
  KeyRound,
  Rocket,
  Settings as SettingsIcon,
  Tag,
  Terminal,
  Workflow,
} from "lucide-react";

import { buildGithubRepoUrl } from "@/lib/deployment-helpers";
import type { AppHealthMember } from "@/types/api";

const chipClass =
  "inline-flex items-center gap-1.5 rounded-md border bg-background px-2.5 py-1 text-xs font-medium text-foreground transition-colors hover:bg-accent";

/**
 * When every member shares one GitHub repo (the common monorepo-bundle case),
 * return it so we can show a single bundle-level "Repo" chip. Mixed repos →
 * null (the per-member source link in the matrix covers that case instead).
 */
function sharedRepo(
  members: AppHealthMember[],
): { owner: string; repo: string } | null {
  const repos = new Map<string, { owner: string; repo: string }>();
  for (const member of members) {
    const owner = member.project.github_owner;
    const repo = member.project.github_repo;
    if (owner && repo) repos.set(`${owner}/${repo}`, { owner, repo });
  }
  if (repos.size !== 1) return null;
  const [only] = [...repos.values()];
  return only ?? null;
}

/**
 * Quick Links bar: one-click access to the bundle's source, app sections, and
 * live logs. Member site URLs live in the matrix rows (they are per-member and
 * per-environment), so this bar covers everything else.
 */
export function QuickLinksBar({
  appId,
  members,
  onOpenLogs,
}: {
  appId: string;
  members: AppHealthMember[];
  onOpenLogs?: () => void;
}) {
  const repo = sharedRepo(members);

  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="mr-1 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
        Quick links
      </span>
      {repo ? (
        <a
          href={buildGithubRepoUrl(repo.owner, repo.repo)}
          target="_blank"
          rel="noopener noreferrer"
          className={chipClass}
        >
          <GitBranch className="h-3.5 w-3.5" /> Repo
          <ExternalLink className="h-3 w-3 text-muted-foreground" />
        </a>
      ) : null}
      <Link to="/apps/$appId/deployments" params={{ appId }} className={chipClass}>
        <Rocket className="h-3.5 w-3.5" /> Deployments
      </Link>
      <Link to="/apps/$appId/releases" params={{ appId }} className={chipClass}>
        <Tag className="h-3.5 w-3.5" /> Releases
      </Link>
      <Link to="/apps/$appId/topology" params={{ appId }} className={chipClass}>
        <Workflow className="h-3.5 w-3.5" /> Topology
      </Link>
      <Link to="/apps/$appId/variables" params={{ appId }} className={chipClass}>
        <KeyRound className="h-3.5 w-3.5" /> Variables
      </Link>
      <Link to="/apps/$appId/settings" params={{ appId }} className={chipClass}>
        <SettingsIcon className="h-3.5 w-3.5" /> Settings
      </Link>
      {onOpenLogs ? (
        <button type="button" onClick={onOpenLogs} className={chipClass}>
          <Terminal className="h-3.5 w-3.5" /> Logs
        </button>
      ) : null}
    </div>
  );
}
