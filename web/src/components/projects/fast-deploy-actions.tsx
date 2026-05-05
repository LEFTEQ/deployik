import { GlobeLock, Loader2, Rocket } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useFastDeploy } from "@/hooks/useFastDeploy";
import { cn } from "@/lib/utils";

export interface FastDeployActionsProps {
  projectId: string;
}

const previewButtonClass =
  "border-sky-400/25 bg-sky-400/[0.06] text-sky-100 hover:border-sky-400/40 hover:bg-sky-400/[0.12] hover:text-sky-50";

const productionButtonClass =
  "border-emerald-400/25 bg-emerald-400/[0.06] text-emerald-100 hover:border-emerald-400/40 hover:bg-emerald-400/[0.12] hover:text-emerald-50";

export function FastDeployActions({ projectId }: FastDeployActionsProps) {
  const { triggerPreview, triggerProduction, productionState, isPending } =
    useFastDeploy(projectId);

  const confirming = productionState === "confirming";

  return (
    <div className="flex items-center gap-1.5">
      <Button
        size="sm"
        variant="outline"
        onClick={triggerPreview}
        disabled={isPending}
        title="Deploy the latest commit to the preview environment"
        className={cn(previewButtonClass)}
      >
        {isPending ? (
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
        ) : (
          <Rocket className="h-3.5 w-3.5" />
        )}
        Deploy preview
      </Button>
      <Button
        size="sm"
        variant={confirming ? "destructive" : "outline"}
        onClick={triggerProduction}
        disabled={isPending}
        title={
          confirming
            ? "Click again within 3 seconds to confirm production deploy"
            : "Deploy the latest commit to production"
        }
        className={cn(!confirming && productionButtonClass)}
      >
        {isPending ? (
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
        ) : (
          <GlobeLock className="h-3.5 w-3.5" />
        )}
        {confirming ? "Click to confirm" : "Deploy production"}
      </Button>
    </div>
  );
}
