import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Copy, KeyRound } from "lucide-react";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

import { api } from "@/lib/api";

type Props = {
  projectId: string;
  environment: string;
  onClose: () => void;
};

export function CredentialsDialog({ projectId, environment, onClose }: Props) {
  const queryClient = useQueryClient();
  const [showPassword, setShowPassword] = useState(false);

  const credsKey = ["projects", projectId, "services", environment, "credentials"] as const;
  const creds = useQuery({
    queryKey: credsKey,
    queryFn: () => api.getServiceCredentials(projectId, environment),
  });

  const regenerate = useMutation({
    mutationFn: () => api.regenerateServicePassword(projectId, environment),
    onSuccess: (newCreds) => {
      queryClient.setQueryData(credsKey, newCreds);
      toast.success(
        "Password regenerated. Redeploy to apply the new password to the running container.",
      );
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const copy = (label: string, value: string) => {
    navigator.clipboard.writeText(value);
    toast.success(`${label} copied`);
  };

  const c = creds.data;

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Connect to {environment} Postgres</DialogTitle>
          <DialogDescription>
            The database is reachable only from inside the deploy network and
            via SSH tunnel from your laptop. Credentials are decrypted
            on-demand for this dialog.
          </DialogDescription>
        </DialogHeader>

        {creds.isLoading ? (
          <div className="text-sm text-muted-foreground">Loading…</div>
        ) : !c ? (
          <div className="text-sm text-red-300">Failed to load credentials.</div>
        ) : (
          <div className="space-y-4">
            <Field label="Database" value={c.db_name} onCopy={copy} />
            <Field label="User" value={c.db_user} onCopy={copy} />
            <div className="space-y-1">
              <Label className="text-xs uppercase tracking-wide text-muted-foreground">
                Password
              </Label>
              <div className="flex gap-2">
                <Input
                  readOnly
                  type={showPassword ? "text" : "password"}
                  value={c.password}
                />
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => setShowPassword((v) => !v)}
                >
                  {showPassword ? "Hide" : "Show"}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => copy("Password", c.password)}
                >
                  <Copy className="size-3.5" />
                </Button>
              </div>
            </div>
            <Field
              label="Internal host (inside deploy network)"
              value={`${c.internal_host}:${c.internal_port}`}
              onCopy={copy}
              mono
            />
            {c.vps_loopback_port > 0 ? (
              <Field
                label="SSH tunnel from your laptop"
                value={c.ssh_tunnel_cmd}
                onCopy={copy}
                mono
                hint="Run this in a separate terminal, then `psql postgresql://...@127.0.0.1:15432/...`"
              />
            ) : (
              <div className="text-xs text-muted-foreground">
                Container hasn't started yet — restart it to get a tunnel port.
              </div>
            )}
          </div>
        )}

        <DialogFooter className="gap-2">
          <Button
            variant="outline"
            onClick={() => regenerate.mutate()}
            disabled={regenerate.isPending}
          >
            <KeyRound className="mr-1.5 size-3.5" />
            Regenerate password
          </Button>
          <Button onClick={onClose}>Close</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Field({
  label,
  value,
  onCopy,
  mono,
  hint,
}: {
  label: string;
  value: string;
  onCopy: (label: string, value: string) => void;
  mono?: boolean;
  hint?: string;
}) {
  return (
    <div className="space-y-1">
      <Label className="text-xs uppercase tracking-wide text-muted-foreground">
        {label}
      </Label>
      <div className="flex gap-2">
        <Input readOnly value={value} className={mono ? "font-mono text-xs" : ""} />
        <Button type="button" variant="outline" size="sm" onClick={() => onCopy(label, value)}>
          <Copy className="size-3.5" />
        </Button>
      </div>
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}
