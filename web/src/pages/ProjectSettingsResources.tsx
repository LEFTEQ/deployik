import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import { AlertTriangle, Check, Cpu } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import {
  RESOURCE_TIER_META,
  RESOURCE_TIER_ORDER,
  formatTierMemory,
} from "@/lib/deployment-helpers";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { LoadingState } from "@/components/ui/spinner";
import type { ResourceTier } from "@/types/api";

export function ProjectSettingsResources() {
  const { id } = useParams({ strict: false }) as { id: string };
  const queryClient = useQueryClient();

  const { data: project, isLoading } = useQuery({
    queryKey: queryKeys.project(id),
    queryFn: () => api.getProject(id),
  });

  const [selectedTier, setSelectedTier] = useState<ResourceTier | null>(null);

  // Sync the local choice to the loaded project — and re-sync if the user
  // navigates away and back, or another tab updates the project.
  useEffect(() => {
    if (project?.resource_tier) {
      setSelectedTier(project.resource_tier);
    }
  }, [project?.resource_tier]);

  const saveMutation = useMutation({
    mutationFn: (tier: ResourceTier) =>
      api.updateProject(id, { resource_tier: tier }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.project(id) });
      toast.success("Resources updated. Changes apply on the next deploy.");
    },
    onError: (err) => toast.error(err.message),
  });

  if (isLoading || !project) {
    return (
      <LoadingState
        title="Loading resources..."
        description="Fetching the project's current tier."
        className="min-h-[200px]"
      />
    );
  }

  const currentTier = project.resource_tier ?? "small";
  const tier = selectedTier ?? currentTier;
  const meta = RESOURCE_TIER_META[tier];
  const hasChanges = tier !== currentTier;

  return (
    <div className="space-y-8" data-testid="project-resources-page">
      <div className="flex items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <Cpu className="h-5 w-5 text-muted-foreground" />
            <h2 className="text-lg font-semibold">Resources</h2>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            Memory and CPU limits applied to your deployed containers. Bigger
            tiers also get a larger build allowance, so you can host heavier
            Next.js apps without OOM-killing the host.
          </p>
        </div>
      </div>

      <Card>
        <CardContent className="space-y-2 p-2">
          {RESOURCE_TIER_ORDER.map((name) => {
            const tierMeta = RESOURCE_TIER_META[name];
            const isSelected = tier === name;
            const isCurrent = currentTier === name;
            return (
              <button
                key={name}
                type="button"
                onClick={() => setSelectedTier(name)}
                data-testid={`resource-tier-option-${name}`}
                aria-pressed={isSelected}
                className={cn(
                  "group flex w-full items-center gap-4 rounded-lg border border-transparent px-4 py-3 text-left transition-colors",
                  "hover:bg-accent",
                  isSelected
                    ? "border-primary/40 bg-primary/8 ring-1 ring-primary/30"
                    : "bg-transparent",
                )}
              >
                <span
                  className={cn(
                    "flex h-5 w-5 shrink-0 items-center justify-center rounded-full border transition-colors",
                    isSelected
                      ? "border-primary bg-primary text-primary-foreground"
                      : "border-white/15 bg-transparent",
                  )}
                  aria-hidden
                >
                  {isSelected && <Check className="h-3 w-3" />}
                </span>
                <div className="flex-1">
                  <div className="flex items-center gap-2">
                    <span className="font-medium">{tierMeta.label}</span>
                    {isCurrent && (
                      <span className="rounded-full border border-emerald-400/30 bg-emerald-400/10 px-2 py-0.5 text-[10px] uppercase tracking-wider text-emerald-100">
                        Current
                      </span>
                    )}
                  </div>
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    {tierMeta.description}
                  </p>
                </div>
                <div className="hidden text-right text-sm sm:block">
                  <div className="font-mono text-foreground">
                    {formatTierMemory(tierMeta.memoryMB)} ·{" "}
                    {tierMeta.cpuCores} CPU
                  </div>
                  <div className="font-mono text-[11px] text-muted-foreground">
                    build {formatTierMemory(tierMeta.buildMemoryMB)} /{" "}
                    {tierMeta.buildCpuCores} CPU
                  </div>
                </div>
              </button>
            );
          })}
        </CardContent>
      </Card>

      {/* Mobile fallback for build allowance (the desktop column is hidden under sm:) */}
      <div className="rounded-lg border bg-muted/30 px-4 py-3 text-sm sm:hidden">
        <div className="font-mono text-foreground">
          Runtime {formatTierMemory(meta.memoryMB)} · {meta.cpuCores} CPU
        </div>
        <div className="font-mono text-xs text-muted-foreground">
          Build {formatTierMemory(meta.buildMemoryMB)} / {meta.buildCpuCores}{" "}
          CPU
        </div>
      </div>

      <div className="flex items-center gap-3 rounded-lg border border-amber-400/20 bg-amber-400/8 px-4 py-3 text-sm text-amber-100">
        <AlertTriangle className="h-4 w-4 shrink-0" />
        <span>
          Tier changes apply on the <strong>next deploy</strong>. Existing
          containers keep their current limits until they're replaced.
        </span>
      </div>

      <div className="flex items-center justify-end gap-2">
        <Button
          variant="ghost"
          onClick={() => setSelectedTier(currentTier)}
          disabled={!hasChanges || saveMutation.isPending}
          data-testid="resource-tier-reset"
        >
          Reset
        </Button>
        <Button
          onClick={() => saveMutation.mutate(tier)}
          disabled={!hasChanges || saveMutation.isPending}
          data-testid="resource-tier-save"
        >
          {saveMutation.isPending ? "Saving..." : "Save changes"}
        </Button>
      </div>
    </div>
  );
}
