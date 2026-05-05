import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ChevronDown, GlobeLock, Loader2, Rocket } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { getEnvironmentDomains } from "@/lib/deployment-helpers";
import { useFastDeploy } from "@/hooks/useFastDeploy";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

export interface DeployMenuProps {
  projectId: string;
  /** Source branch shown in the production confirm dialog. Optional. */
  productionBranch?: string;
}

/**
 * Shared "Deploy" dropdown used by the project header and the Environments
 * panel. Hides "Deploy production" until the project has at least one
 * production domain, and gates production behind an AlertDialog confirm so it
 * works inside menus that close on click.
 */
export function DeployMenu({ projectId, productionBranch }: DeployMenuProps) {
  const [confirmOpen, setConfirmOpen] = useState(false);
  const {
    triggerPreview,
    triggerProductionConfirmed,
    isPending: isDeployPending,
  } = useFastDeploy(projectId);

  const { data: domains } = useQuery({
    queryKey: queryKeys.domains(projectId),
    queryFn: () => api.listDomains(projectId),
  });

  const hasProductionDomain =
    getEnvironmentDomains(domains ?? [], "production").length > 0;

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button size="sm" disabled={isDeployPending}>
            {isDeployPending ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <Rocket className="h-3.5 w-3.5" />
            )}
            Deploy
            <ChevronDown className="ml-0.5 h-3.5 w-3.5 opacity-70" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-56">
          <DropdownMenuItem onSelect={() => triggerPreview()}>
            <Rocket className="h-3.5 w-3.5 text-sky-300" />
            <span>Deploy preview</span>
          </DropdownMenuItem>
          {hasProductionDomain ? (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                onSelect={(event) => {
                  event.preventDefault();
                  setConfirmOpen(true);
                }}
              >
                <GlobeLock className="h-3.5 w-3.5 text-emerald-300" />
                <span>Deploy production</span>
              </DropdownMenuItem>
            </>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>

      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Deploy to production?</AlertDialogTitle>
            <AlertDialogDescription>
              This will deploy the latest commit
              {productionBranch ? (
                <>
                  {" on "}
                  <span className="font-mono font-medium text-foreground">
                    {productionBranch}
                  </span>
                </>
              ) : null}{" "}
              to your production domain. Live traffic will be cut over once the
              build finishes.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isDeployPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={isDeployPending}
              onClick={(event) => {
                event.preventDefault();
                triggerProductionConfirmed();
                setConfirmOpen(false);
              }}
            >
              <GlobeLock className="h-3.5 w-3.5" />
              Deploy production
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
