import type { MouseEvent, MouseEventHandler, ReactNode } from "react";

import {
  buildGithubBranchUrl,
  buildGithubCommitUrl,
} from "@/lib/deployment-helpers";
import { cn } from "@/lib/utils";

const linkClasses =
  "text-inherit underline-offset-4 transition-colors hover:text-primary hover:underline";

type AnchorHandler = MouseEventHandler<HTMLAnchorElement>;

function handleClick(
  event: MouseEvent<HTMLAnchorElement>,
  onClick?: AnchorHandler,
) {
  // Prevent ancestor click handlers (e.g. clickable table rows with
  // role="link") from firing when the user clicks a nested GitHub link.
  event.stopPropagation();
  onClick?.(event);
}

export type CommitLinkProps = {
  owner: string;
  repo: string;
  sha: string;
  short?: boolean;
  className?: string;
  children?: ReactNode;
  onClick?: AnchorHandler;
  title?: string;
};

export function CommitLink({
  owner,
  repo,
  sha,
  short = true,
  className,
  children,
  onClick,
  title,
}: CommitLinkProps) {
  const display = children ?? (short ? sha.slice(0, 7) : sha);
  const tooltip = title ?? `View ${sha.slice(0, 7)} on GitHub`;

  return (
    <a
      href={buildGithubCommitUrl(owner, repo, sha)}
      target="_blank"
      rel="noopener noreferrer"
      className={cn(linkClasses, className)}
      onClick={(event) => handleClick(event, onClick)}
      title={tooltip}
    >
      {display}
    </a>
  );
}

export type BranchLinkProps = {
  owner: string;
  repo: string;
  branch: string;
  className?: string;
  children?: ReactNode;
  onClick?: AnchorHandler;
  title?: string;
};

export function BranchLink({
  owner,
  repo,
  branch,
  className,
  children,
  onClick,
  title,
}: BranchLinkProps) {
  return (
    <a
      href={buildGithubBranchUrl(owner, repo, branch)}
      target="_blank"
      rel="noopener noreferrer"
      className={cn(linkClasses, className)}
      onClick={(event) => handleClick(event, onClick)}
      title={title ?? `View ${branch} on GitHub`}
    >
      {children ?? branch}
    </a>
  );
}
