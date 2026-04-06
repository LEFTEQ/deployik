import { getReadyEnvironmentDomains } from "@/lib/deployment-helpers";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { Domain, Project } from "@/types/api";

export interface ReleasePanelContentProps {
  project: Project;
  domains: Domain[] | undefined;
  releaseTagName: string;
  onReleaseTagChange: (value: string) => void;
}

export function ReleasePanelContent({
  project,
  domains,
  releaseTagName,
  onReleaseTagChange,
}: ReleasePanelContentProps) {
  return (
    <div className="mt-6 grid gap-4 px-4 lg:grid-cols-[minmax(0,0.8fr)_minmax(280px,1fr)] lg:px-0">
      <div className="space-y-4">
        <div className="rounded-xl border border-white/8 bg-black/10 p-4">
          <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">
            Source branch
          </p>
          <p className="mt-2 text-sm font-medium text-foreground">
            {project.branch}
          </p>
          <p className="mt-2 text-sm text-muted-foreground">
            Repository: {project.github_owner}/{project.github_repo}
          </p>
        </div>

        <div className="rounded-xl border border-white/8 bg-black/10 p-4">
          <Label htmlFor="release-tag">Release tag</Label>
          <Input
            id="release-tag"
            value={releaseTagName}
            onChange={(event) => onReleaseTagChange(event.target.value)}
            className="mt-3"
            placeholder="release-20260402-1455"
          />
          <p className="mt-3 text-sm text-muted-foreground">
            This tag becomes the production deploy ref so the released build
            stays traceable.
          </p>
        </div>
      </div>

      <div className="space-y-4">
        <div className="rounded-xl border border-white/8 bg-black/10 p-4">
          <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">
            Production endpoints
          </p>
          <div className="mt-3 space-y-2">
            {getReadyEnvironmentDomains(domains, "production").length ? (
              getReadyEnvironmentDomains(domains, "production").map(
                (domain) => (
                  <div
                    key={domain.id}
                    className="rounded-xl border border-white/8 px-3 py-2 text-sm text-foreground"
                  >
                    {domain.domain}
                  </div>
                ),
              )
            ) : (
              <div className="rounded-xl border border-dashed border-white/10 px-3 py-6 text-sm text-muted-foreground">
                No verified production domain yet. The release will still build
                and deploy, but no public production URL is active.
              </div>
            )}
          </div>
        </div>

        <div className="rounded-xl border border-primary/15 bg-primary/10 p-4 text-sm text-slate-100">
          Release is the intentional production action. Preview remains the fast
          iteration path.
        </div>
      </div>
    </div>
  );
}
