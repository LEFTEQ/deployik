import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, KeyRound, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import type { APIToken } from "@/types/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";

function statusOf(token: APIToken): {
  label: string;
  tone: "active" | "revoked" | "expired";
} {
  if (token.revoked_at) return { label: "Revoked", tone: "revoked" };
  if (token.expires_at && new Date(token.expires_at) < new Date()) {
    return { label: "Expired", tone: "expired" };
  }
  return { label: "Active", tone: "active" };
}

export function UserTokens() {
  const queryClient = useQueryClient();
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [createdToken, setCreatedToken] = useState<string | null>(null);

  const tokensQuery = useQuery({
    queryKey: queryKeys.myTokens(),
    queryFn: () => api.listMyTokens(),
  });

  const createMutation = useMutation({
    mutationFn: (n: string) => api.createMyToken({ name: n }),
    onSuccess: (resp) => {
      setCreatedToken(resp.token);
      setName("");
      queryClient.invalidateQueries({ queryKey: queryKeys.myTokens() });
    },
    onError: (err) => toast.error(err.message),
  });

  const revokeMutation = useMutation({
    mutationFn: (id: string) => api.revokeMyToken(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.myTokens() });
      toast.success("Token revoked");
    },
    onError: (err) => toast.error(err.message),
  });

  const tokens = tokensQuery.data ?? [];

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Personal access tokens
          </h1>
          <p className="text-sm text-muted-foreground">
            Long-lived bearer tokens for tools and skills that call the Deployik
            API. Each token has the same permissions as your account. Treat them
            like passwords.
          </p>
        </div>
        <Button
          className="h-11 w-full md:h-9 md:w-auto"
          onClick={() => setCreating(true)}
        >
          <KeyRound className="mr-2 h-4 w-4" />
          Create token
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Your tokens</CardTitle>
        </CardHeader>
        <CardContent>
          {tokensQuery.isLoading ? (
            <p className="text-sm text-muted-foreground">Loading...</p>
          ) : tokens.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No tokens yet. Create one to use the Deployik API from outside the
              dashboard.
            </p>
          ) : (
            <ul className="divide-y divide-border">
              {tokens.map((token) => {
                const status = statusOf(token);
                const lastUsed = token.last_used_at
                  ? new Date(token.last_used_at).toLocaleString()
                  : "Never used";
                return (
                  <li
                    key={token.id}
                    className="flex items-center justify-between gap-3 py-3"
                  >
                    <div className="min-w-0 flex-1 space-y-0.5">
                      <div className="flex items-center gap-2">
                        <span className="min-w-0 truncate font-medium">
                          {token.name}
                        </span>
                        <Badge
                          variant={
                            status.tone === "active" ? "outline" : "secondary"
                          }
                          className={
                            status.tone === "revoked"
                              ? "border-red-500/40 text-red-300"
                              : status.tone === "expired"
                                ? "border-amber-500/40 text-amber-200"
                                : "border-emerald-500/40 text-emerald-200"
                          }
                        >
                          {status.label}
                        </Badge>
                      </div>
                      <p className="text-xs text-muted-foreground">
                        Created{" "}
                        {new Date(token.created_at).toLocaleDateString()} ·{" "}
                        {lastUsed}
                      </p>
                    </div>
                    {status.tone === "active" ? (
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => revokeMutation.mutate(token.id)}
                        disabled={revokeMutation.isPending}
                      >
                        <Trash2 className="mr-1.5 h-3.5 w-3.5" />
                        Revoke
                      </Button>
                    ) : null}
                  </li>
                );
              })}
            </ul>
          )}
        </CardContent>
      </Card>

      {/* Create dialog (name input → create) */}
      <Dialog
        open={creating && createdToken === null}
        onOpenChange={(open) => {
          if (!open) {
            setCreating(false);
            setName("");
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create token</DialogTitle>
            <DialogDescription>
              Give the token a name so you can recognize it later. The token
              value is shown once and never again.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="token-name">Name</Label>
            <Input
              id="token-name"
              placeholder="e.g. deployik-howto skill"
              value={name}
              onChange={(e) => setName(e.target.value)}
              autoFocus
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreating(false)}>
              Cancel
            </Button>
            <Button
              onClick={() => createMutation.mutate(name.trim())}
              disabled={!name.trim() || createMutation.isPending}
            >
              {createMutation.isPending ? "Creating..." : "Create"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Reveal dialog (shown once, contains the raw token) */}
      <Dialog
        open={createdToken !== null}
        onOpenChange={(open) => {
          if (!open) {
            setCreatedToken(null);
            setCreating(false);
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Token created</DialogTitle>
            <DialogDescription>
              Copy this token now — it will never be shown again. Store it
              somewhere safe (e.g. <code>~/.config/deployik/config</code> for
              the deployik-howto skill).
            </DialogDescription>
          </DialogHeader>
          <div className="rounded-md border bg-muted/40 p-3 font-mono text-sm break-all">
            {createdToken}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                if (createdToken) {
                  void navigator.clipboard.writeText(createdToken);
                  toast.success("Copied to clipboard");
                }
              }}
            >
              <Copy className="mr-2 h-4 w-4" />
              Copy
            </Button>
            <Button
              onClick={() => {
                setCreatedToken(null);
                setCreating(false);
              }}
            >
              Done
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
