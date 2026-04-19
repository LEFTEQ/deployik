import { useQuery } from "@tanstack/react-query";
import { GitCommit, ExternalLink } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { useSidebar } from "@/components/ui/sidebar";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

// Renders the running build SHA + a link to the GitHub Actions run that
// produced it. Sits in the sidebar footer above the user/workspace dropdown.
// Hidden entirely when the binary was built without version metadata
// (impossible after Task 3 since defaults are "dev" rather than "").
export function VersionRow() {
  const { state } = useSidebar();
  const collapsed = state === "collapsed";

  const { data } = useQuery({
    queryKey: queryKeys.health(),
    queryFn: () => api.getHealth(),
    staleTime: Infinity,
    gcTime: Infinity,
    retry: 1,
  });

  const version = data?.version;
  if (!version) return null;

  const isDev = !version.git_sha_full || version.git_sha_full === "dev";
  const tooltipLabel = version.gh_run_id
    ? `version ${version.git_sha} \u00b7 build #${version.gh_run_id}`
    : `version ${version.git_sha}`;

  if (collapsed) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          {version.commit_url ? (
            <a
              href={version.commit_url}
              target="_blank"
              rel="noreferrer"
              className="mx-auto flex size-8 items-center justify-center rounded-md text-muted-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
              aria-label={tooltipLabel}
            >
              <GitCommit className="size-4" />
            </a>
          ) : (
            <button
              type="button"
              className="mx-auto flex size-8 cursor-default items-center justify-center rounded-md text-muted-foreground"
              aria-label={tooltipLabel}
            >
              <GitCommit className="size-4" />
            </button>
          )}
        </TooltipTrigger>
        <TooltipContent side="right">{tooltipLabel}</TooltipContent>
      </Tooltip>
    );
  }

  return (
    <div className="flex items-center justify-between gap-2 px-2 py-1.5 text-xs text-muted-foreground">
      {version.commit_url ? (
        <a
          href={version.commit_url}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1.5 rounded hover:text-foreground"
          title={`Commit ${version.git_sha_full}`}
        >
          <GitCommit className="size-3.5" />
          <span className="font-mono">{version.git_sha}</span>
        </a>
      ) : (
        <span
          className="inline-flex items-center gap-1.5"
          title={isDev ? "Local development build" : version.git_sha_full}
        >
          <GitCommit className="size-3.5" />
          <span className="font-mono">{version.git_sha}</span>
        </span>
      )}

      {version.run_url && (
        <a
          href={version.run_url}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1 rounded hover:text-foreground"
          title={`GitHub Actions run #${version.gh_run_id}`}
        >
          <span>build</span>
          <ExternalLink className="size-3" />
        </a>
      )}
    </div>
  );
}
