import { getReadyEnvironmentDomains } from "@/lib/deployment-helpers";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import type { Domain, Project } from "@/types/api";

export interface ReleasePanelContentProps {
  project: Project;
  domains: Domain[] | undefined;
  createTag: boolean;
  onCreateTagChange: (value: boolean) => void;
  releaseTagName: string;
  onReleaseTagChange: (value: string) => void;
}

export function ReleasePanelContent({
  project,
  domains,
  createTag,
  onCreateTagChange,
  releaseTagName,
  onReleaseTagChange,
}: ReleasePanelContentProps) {
  const prodDomains = getReadyEnvironmentDomains(domains, "production");

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <p className="text-sm font-medium text-muted-foreground">
          Source branch
        </p>
        <p className="font-mono text-sm text-foreground">{project.branch}</p>
        <p className="font-mono text-xs text-muted-foreground">
          {project.github_owner}/{project.github_repo}
        </p>
      </div>

      <div className="flex items-center justify-between rounded-md border border-white/5 px-3 py-2.5">
        <Label htmlFor="create-tag" className="cursor-pointer text-sm">
          Create git tag
        </Label>
        <Switch
          id="create-tag"
          size="sm"
          checked={createTag}
          onCheckedChange={onCreateTagChange}
        />
      </div>

      {createTag && (
        <div className="space-y-2">
          <Label htmlFor="release-tag">Tag name</Label>
          <Input
            id="release-tag"
            value={releaseTagName}
            onChange={(event) => onReleaseTagChange(event.target.value)}
            placeholder="release-20260402-1455"
            className="font-mono"
          />
        </div>
      )}

      {prodDomains.length > 0 && (
        <div className="space-y-2">
          <p className="text-sm font-medium text-muted-foreground">
            Production endpoints
          </p>
          <div className="space-y-1.5">
            {prodDomains.map((domain) => (
              <div
                key={domain.id}
                className="rounded-md border border-white/5 px-3 py-2 font-mono text-sm text-foreground"
              >
                {domain.domain}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
