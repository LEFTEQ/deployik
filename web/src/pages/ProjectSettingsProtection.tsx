import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import { Check, Copy, Eye, EyeOff, RefreshCw } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { LoadingState } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";

type Environment = "preview" | "production";

// generateSuggestion produces a strong 16-char base64url password, matching the
// shape the backend generates by default (12 random bytes -> 16 chars, no padding).
function generateSuggestion(): string {
  const bytes = new Uint8Array(12);
  crypto.getRandomValues(bytes);
  let binary = "";
  for (const b of bytes) binary += String.fromCharCode(b);
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export function ProjectSettingsProtection() {
  const { id } = useParams({ strict: false }) as { id: string };

  const { data: protection, isLoading } = useQuery({
    queryKey: queryKeys.protection(id),
    queryFn: () => api.getProtectionStatus(id),
  });

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
            must enter a password to access the site. You can set your own
            password or use a generated one.
          </p>
        </div>
      </div>

      <div className="divide-y rounded-lg border">
        <ProtectionRow
          projectId={id}
          environment="preview"
          enabled={protection?.preview_enabled ?? false}
        />
        <ProtectionRow
          projectId={id}
          environment="production"
          enabled={protection?.production_enabled ?? false}
        />
      </div>
    </div>
  );
}

function ProtectionRow({
  projectId,
  environment,
  enabled,
}: {
  projectId: string;
  environment: Environment;
  enabled: boolean;
}) {
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState("");
  // savedValue holds the last known plaintext for this session (from enable / save
  // / reveal). null means we don't currently know it (e.g. after a page reload).
  const [savedValue, setSavedValue] = useState<string | null>(null);
  const [show, setShow] = useState(true);
  const [copied, setCopied] = useState(false);

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: queryKeys.protection(projectId) });

  const dirty = savedValue !== null && value !== savedValue;

  const toggleMutation = useMutation({
    mutationFn: (next: boolean) =>
      api.updateProtection(projectId, { environment, enabled: next }),
    onSuccess: (res, next) => {
      invalidate();
      if (next) {
        // Enabled immediately with a generated password — reveal it inline so the
        // user can keep it or change it.
        setSavedValue(res.password ?? null);
        setValue(res.password ?? "");
        setShow(true);
        setEditing(true);
        toast.success("Protection enabled");
      } else {
        setEditing(false);
        setSavedValue(null);
        setValue("");
        toast.success("Protection disabled");
      }
    },
    onError: (err) => toast.error((err as Error).message),
  });

  const revealMutation = useMutation({
    mutationFn: () => api.revealProtectionPassword(projectId, environment),
    onSuccess: (res) => {
      setSavedValue(res.password);
      setValue(res.password);
      setShow(true);
    },
    onError: (err) => toast.error((err as Error).message),
  });

  const saveMutation = useMutation({
    mutationFn: () =>
      api.regeneratePassword(projectId, { environment, password: value }),
    onSuccess: (res) => {
      invalidate();
      setSavedValue(res.password ?? value);
      setEditing(false);
      toast.success("Password updated");
    },
    onError: (err) => toast.error((err as Error).message),
  });

  const busy =
    toggleMutation.isPending ||
    revealMutation.isPending ||
    saveMutation.isPending;

  function openEditor() {
    setEditing(true);
    if (savedValue === null) {
      // Fetch + fill the current password (this is the audited "reveal").
      revealMutation.mutate();
    } else {
      setValue(savedValue);
      setShow(true);
    }
  }

  function cancelEditor() {
    setEditing(false);
    if (savedValue !== null) setValue(savedValue);
  }

  function copyValue() {
    if (!value) return;
    navigator.clipboard.writeText(value);
    setCopied(true);
    toast.success("Password copied");
    setTimeout(() => setCopied(false), 1500);
  }

  return (
    <div className="px-4 py-3">
      <div className="flex items-center justify-between">
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
          {enabled && !editing && (
            <Button
              variant="ghost"
              size="sm"
              onClick={openEditor}
              disabled={busy}
            >
              Show / change password
            </Button>
          )}
          <Switch
            checked={enabled}
            onCheckedChange={(next) => toggleMutation.mutate(next)}
            disabled={busy}
          />
        </div>
      </div>

      {enabled && editing && (
        <div className="mt-3 space-y-2 rounded-md border bg-muted/20 p-3">
          <div className="flex items-center gap-2">
            <Input
              type={show ? "text" : "password"}
              value={value}
              onChange={(e) => setValue(e.target.value)}
              placeholder={
                revealMutation.isPending ? "Loading…" : "Enter a password"
              }
              disabled={revealMutation.isPending}
              className="font-mono"
              autoComplete="off"
              spellCheck={false}
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              onClick={() => setShow((s) => !s)}
              title={show ? "Hide" : "Show"}
            >
              {show ? (
                <EyeOff className="h-4 w-4" />
              ) : (
                <Eye className="h-4 w-4" />
              )}
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              onClick={copyValue}
              title="Copy"
            >
              {copied ? (
                <Check className="h-4 w-4 text-emerald-400" />
              ) : (
                <Copy className="h-4 w-4" />
              )}
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              onClick={() => {
                setValue(generateSuggestion());
                setShow(true);
              }}
              title="Generate a new password"
            >
              <RefreshCw className="h-4 w-4" />
            </Button>
          </div>
          <div className="flex items-center justify-end gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={cancelEditor}
              disabled={busy}
            >
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={() => saveMutation.mutate()}
              disabled={busy || !value || !dirty}
            >
              {saveMutation.isPending ? "Saving…" : "Save"}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
