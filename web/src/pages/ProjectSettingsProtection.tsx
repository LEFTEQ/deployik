import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import { toast } from "sonner";

import { api } from "@/lib/api";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { LoadingState } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";

export function ProjectSettingsProtection() {
  const { id } = useParams({ strict: false }) as { id: string };
  const queryClient = useQueryClient();
  const [passwordToShow, setPasswordToShow] = useState<string | null>(null);

  const { data: protection, isLoading } = useQuery({
    queryKey: ["protection", id],
    queryFn: () => api.getProtectionStatus(id),
  });

  const updateMutation = useMutation({
    mutationFn: (data: { environment: string; enabled: boolean }) =>
      api.updateProtection(id, data),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ["protection", id] });
      if (result.password) {
        setPasswordToShow(result.password);
      }
      toast.success(
        result.enabled ? "Protection enabled" : "Protection disabled",
      );
    },
    onError: (err) => toast.error(err.message),
  });

  const regenerateMutation = useMutation({
    mutationFn: (data: { environment: string }) =>
      api.regeneratePassword(id, data),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ["protection", id] });
      if (result.password) {
        setPasswordToShow(result.password);
      }
      toast.success("Password regenerated");
    },
    onError: (err) => toast.error(err.message),
  });

  const isPending = updateMutation.isPending || regenerateMutation.isPending;

  const anyEnabled =
    protection?.preview_enabled || protection?.production_enabled;

  if (isLoading) {
    return (
      <LoadingState
        title="Loading protection..."
        description="Checking password protection status."
        className="min-h-[200px]"
      />
    );
  }

  return (
    <div className="space-y-8">
      <div className="flex items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <h2 className="text-lg font-semibold">Password Protection</h2>
            {anyEnabled && (
              <Badge
                variant="outline"
                className="border-emerald-400/25 bg-emerald-400/12 text-emerald-100"
              >
                Enabled
              </Badge>
            )}
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            Control access to your deployed environments. When enabled, visitors
            see an "unavailable" page and must enter a password to access the
            site.
          </p>
        </div>
      </div>

      <div className="divide-y rounded-lg border">
        <ProtectionRow
          environment="preview"
          enabled={protection?.preview_enabled ?? false}
          onToggle={(enabled) =>
            updateMutation.mutate({ environment: "preview", enabled })
          }
          onRegenerate={() =>
            regenerateMutation.mutate({ environment: "preview" })
          }
          isPending={isPending}
        />
        <ProtectionRow
          environment="production"
          enabled={protection?.production_enabled ?? false}
          onToggle={(enabled) =>
            updateMutation.mutate({ environment: "production", enabled })
          }
          onRegenerate={() =>
            regenerateMutation.mutate({ environment: "production" })
          }
          isPending={isPending}
        />
      </div>

      {/* Password reveal dialog */}
      <AlertDialog
        open={!!passwordToShow}
        onOpenChange={() => setPasswordToShow(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Password Generated</AlertDialogTitle>
            <AlertDialogDescription>
              Save this password — it won't be shown again.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="select-all rounded-lg border bg-muted/30 p-4 text-center font-mono text-lg tracking-wider">
            {passwordToShow}
          </div>
          <AlertDialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                navigator.clipboard.writeText(passwordToShow ?? "");
                toast.success("Password copied");
              }}
            >
              Copy
            </Button>
            <AlertDialogAction onClick={() => setPasswordToShow(null)}>
              Done
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function ProtectionRow({
  environment,
  enabled,
  onToggle,
  onRegenerate,
  isPending,
}: {
  environment: string;
  enabled: boolean;
  onToggle: (enabled: boolean) => void;
  onRegenerate: () => void;
  isPending: boolean;
}) {
  return (
    <div className="flex items-center justify-between px-4 py-3">
      <div className="flex items-center gap-3">
        <Badge
          variant="outline"
          className={
            environment === "production"
              ? "border-amber-500/25 text-amber-200"
              : ""
          }
        >
          {environment === "preview" ? "Preview" : "Production"}
        </Badge>
        <span className="text-sm text-muted-foreground">
          {enabled ? "Password required" : "Public access"}
        </span>
      </div>
      <div className="flex items-center gap-2">
        {enabled && (
          <Button
            variant="ghost"
            size="sm"
            onClick={onRegenerate}
            disabled={isPending}
          >
            Regenerate
          </Button>
        )}
        <Switch
          checked={enabled}
          onCheckedChange={onToggle}
          disabled={isPending}
        />
      </div>
    </div>
  );
}
