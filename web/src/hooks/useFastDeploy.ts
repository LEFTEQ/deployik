import { useCallback, useEffect, useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { toast } from "sonner";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import type { Deployment } from "@/types/api";

export type FastDeployEnvironment = "preview" | "production";
export type ProductionConfirmState = "idle" | "confirming";

const PRODUCTION_CONFIRM_WINDOW_MS = 3000;

/**
 * Shared deploy trigger for the global header button and the rebuild banner CTA.
 * Preview deploys fire on the first click; production requires a second click
 * within {@link PRODUCTION_CONFIRM_WINDOW_MS}, surfaced via {@link productionState}.
 */
export function useFastDeploy(projectId: string) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [productionState, setProductionState] =
    useState<ProductionConfirmState>("idle");
  const confirmTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const clearConfirmTimer = useCallback(() => {
    if (confirmTimerRef.current) {
      clearTimeout(confirmTimerRef.current);
      confirmTimerRef.current = null;
    }
  }, []);

  useEffect(() => () => clearConfirmTimer(), [clearConfirmTimer]);

  const mutation = useMutation({
    mutationFn: (env: FastDeployEnvironment) =>
      api.triggerDeployment(projectId, { environment: env }),
    onSuccess: (deployment: Deployment) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.deployments(projectId),
      });
      queryClient.invalidateQueries({
        queryKey: queryKeys.project(projectId),
      });
      toast.success(
        `${deployment.environment === "production" ? "Production" : "Preview"} deploy triggered`,
      );
      navigate({
        to: "/projects/$id/deployments/$did",
        params: { id: projectId, did: deployment.id },
      });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const triggerPreview = useCallback(() => {
    if (mutation.isPending) return;
    mutation.mutate("preview");
  }, [mutation]);

  const triggerProduction = useCallback(() => {
    if (mutation.isPending) return;
    if (productionState === "idle") {
      setProductionState("confirming");
      clearConfirmTimer();
      confirmTimerRef.current = setTimeout(() => {
        setProductionState("idle");
        confirmTimerRef.current = null;
      }, PRODUCTION_CONFIRM_WINDOW_MS);
      return;
    }
    clearConfirmTimer();
    setProductionState("idle");
    mutation.mutate("production");
  }, [clearConfirmTimer, mutation, productionState]);

  // Skips the 3-second confirm window for callers that gate the action behind
  // their own confirmation UI (e.g. a dropdown + AlertDialog flow).
  const triggerProductionConfirmed = useCallback(() => {
    if (mutation.isPending) return;
    clearConfirmTimer();
    setProductionState("idle");
    mutation.mutate("production");
  }, [clearConfirmTimer, mutation]);

  return {
    triggerPreview,
    triggerProduction,
    triggerProductionConfirmed,
    productionState,
    isPending: mutation.isPending,
  };
}
