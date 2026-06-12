import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import {
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  ExternalLink,
  GlobeLock,
  Link2,
  LoaderCircle,
  MoreHorizontal,
  Plus,
  RefreshCcw,
  Star,
  Trash2,
  X,
} from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import { queryKeys, staleTimes } from "@/lib/queryKeys";
import { ENVIRONMENT_META, isDomainReady } from "@/lib/deployment-helpers";
import {
  useDomainVerification,
  parseStream,
} from "@/hooks/useDomainVerification";
import type { VerificationState } from "@/hooks/useDomainVerification";
import { DnsSetupGuide } from "@/components/projects/dns-setup-guide";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { LoadingState } from "@/components/ui/spinner";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
import { cn } from "@/lib/utils";
import type { Domain, DomainLogEvent } from "@/types/api";

function VerificationLogPanel({
  logs,
  state,
  summary,
  minimized,
  onToggleMinimize,
}: {
  logs: DomainLogEvent[];
  state: VerificationState;
  summary: string | null;
  minimized: boolean;
  onToggleMinimize: () => void;
}) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (containerRef.current && !minimized) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [logs, minimized]);

  const isComplete = state === "success" || state === "error";

  if (minimized && isComplete && summary) {
    return (
      <button
        type="button"
        onClick={onToggleMinimize}
        className={cn(
          "flex w-full items-center justify-between px-4 py-2 text-xs transition-colors",
          state === "success"
            ? "bg-emerald-500/5 text-emerald-400 hover:bg-emerald-500/10"
            : "bg-red-500/5 text-red-400 hover:bg-red-500/10",
        )}
      >
        <span className="flex min-w-0 items-center gap-1.5">
          {state === "success" ? (
            <CheckCircle2 className="h-3 w-3 shrink-0" />
          ) : (
            <X className="h-3 w-3 shrink-0" />
          )}
          <span className="truncate text-left">{summary}</span>
        </span>
        <ChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" />
      </button>
    );
  }

  return (
    <div>
      {isComplete && (
        <button
          type="button"
          onClick={onToggleMinimize}
          className={cn(
            "flex w-full items-center justify-between px-4 py-1.5 text-xs transition-colors",
            state === "success"
              ? "bg-emerald-500/5 text-emerald-400 hover:bg-emerald-500/10"
              : "bg-red-500/5 text-red-400 hover:bg-red-500/10",
          )}
        >
          <span className="flex min-w-0 items-center gap-1.5">
            {state === "success" ? (
              <CheckCircle2 className="h-3 w-3 shrink-0" />
            ) : (
              <X className="h-3 w-3 shrink-0" />
            )}
            <span className="truncate text-left">{summary}</span>
          </span>
          <ChevronUp className="h-3 w-3 shrink-0 text-muted-foreground" />
        </button>
      )}
      <div
        ref={containerRef}
        className="max-h-[200px] overflow-y-auto bg-zinc-950 px-4 py-3 font-mono text-xs leading-6"
      >
        {logs.length === 0 ? (
          <span className="text-zinc-500">Connecting...</span>
        ) : (
          logs.map((line) => {
            const { status } = parseStream(line.stream);
            return (
              <div
                key={line.line_number}
                className={cn(
                  "whitespace-pre-wrap break-all",
                  status === "success" && "text-emerald-400",
                  status === "error" && "text-red-400",
                  status === "running" && "text-zinc-400",
                )}
              >
                <span className="mr-2 select-none text-zinc-700">
                  {line.line_number}
                </span>
                {line.content}
              </div>
            );
          })
        )}
        {(state === "verifying" || state === "connecting") && (
          <span className="inline-block h-3 w-1.5 animate-pulse bg-zinc-400" />
        )}
      </div>
    </div>
  );
}

export function ProjectSettingsDomains() {
  const { id } = useParams({ strict: false }) as { id: string };
  const queryClient = useQueryClient();
  const [showAddForm, setShowAddForm] = useState(false);
  const [newDomain, setNewDomain] = useState("");
  const [newDomainEnvironment, setNewDomainEnvironment] =
    useState<Domain["environment"]>("production");

  const [verifyingDomainId, setVerifyingDomainId] = useState<string | null>(
    null,
  );
  const [expandedLogDomainId, setExpandedLogDomainId] = useState<string | null>(
    null,
  );
  const [minimized, setMinimized] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Domain | null>(null);
  const {
    logs,
    state: verifyState,
    summary,
    clearLogs,
  } = useDomainVerification(verifyingDomainId);
  const attemptedRef = useRef<Set<string>>(new Set());

  const { data: domains, isLoading } = useQuery({
    queryKey: queryKeys.domains(id),
    queryFn: () => api.listDomains(id),
  });

  const { data: platform } = useQuery({
    queryKey: queryKeys.platform(),
    queryFn: () => api.getPlatformInfo(),
    staleTime: staleTimes.platform,
  });

  const addMutation = useMutation({
    mutationFn: () =>
      api.addDomain(id, {
        domain: newDomain.trim().toLowerCase(),
        environment: newDomainEnvironment,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.domains(id) });
      setNewDomain("");
      setNewDomainEnvironment("production");
      setShowAddForm(false);
      toast.success("Domain added");
    },
    onError: (err) => toast.error(err.message),
  });

  const verifyMutation = useMutation({
    mutationFn: (domainId: string) => api.verifyDomain(id, domainId),
    onMutate: (domainId) => {
      attemptedRef.current.add(domainId);
    },
    onSuccess: (_result, domainId) => {
      setVerifyingDomainId(domainId);
      setExpandedLogDomainId(domainId);
      setMinimized(false);
      clearLogs();
    },
    onError: (err) => toast.error(err.message),
  });

  const moveMutation = useMutation({
    mutationFn: ({
      domainId,
      environment,
    }: {
      domainId: string;
      environment: Domain["environment"];
    }) => api.updateDomain(id, domainId, { environment }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.domains(id) });
      toast.success("Environment updated");
    },
    onError: (err) => toast.error(err.message),
  });

  const setPrimaryMutation = useMutation({
    mutationFn: (domainId: string) =>
      api.updateDomain(id, domainId, { is_primary: true }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.domains(id) });
      toast.success("Primary updated");
    },
    onError: (err) => toast.error(err.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (domainId: string) => api.deleteDomain(id, domainId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.domains(id) });
      setDeleteTarget(null);
      toast.success("Domain deleted");
    },
    onError: (err) => {
      setDeleteTarget(null);
      toast.error(err.message);
    },
  });

  useEffect(() => {
    if (verifyState === "success" || verifyState === "error") {
      queryClient.invalidateQueries({ queryKey: queryKeys.domains(id) });
      const timer = setTimeout(() => {
        setMinimized(true);
        setVerifyingDomainId(null);
      }, 2000);
      return () => clearTimeout(timer);
    }
  }, [verifyState, queryClient, id]);

  // Auto-fire verify for any pending custom domain on page load (once per session per domain).
  useEffect(() => {
    if (!domains || verifyingDomainId !== null || verifyMutation.isPending)
      return;
    const pending = domains.find(
      (d) => !d.is_auto && !isDomainReady(d) && !attemptedRef.current.has(d.id),
    );
    if (pending) {
      verifyMutation.mutate(pending.id);
    }
  }, [domains, verifyingDomainId, verifyMutation]);

  const productionDomains =
    domains?.filter((d) => d.environment === "production") ?? [];
  const previewDomains =
    domains?.filter((d) => d.environment === "preview") ?? [];

  function handleCancel() {
    setShowAddForm(false);
    setNewDomain("");
    setNewDomainEnvironment("production");
  }

  function renderDomainRow(domain: Domain) {
    const ready = isDomainReady(domain);
    const isVerifying =
      verifyingDomainId === domain.id &&
      (verifyState === "verifying" || verifyState === "connecting");
    const showLogPanel =
      expandedLogDomainId === domain.id && verifyState !== "idle";
    const allVerifyDisabled =
      verifyMutation.isPending || verifyingDomainId !== null;
    const canMove = !moveMutation.isPending && verifyingDomainId === null;

    return (
      <div key={domain.id}>
        <div className="flex flex-col gap-4 px-4 py-3 md:flex-row md:items-center md:justify-between">
          <div className="min-w-0 space-y-2">
            <div className="flex flex-wrap items-center gap-2">
              <p className="break-all text-sm font-medium">{domain.domain}</p>
              <Badge
                variant="outline"
                className={ENVIRONMENT_META[domain.environment].badgeClass}
              >
                {ENVIRONMENT_META[domain.environment].label}
              </Badge>
              <Badge variant={domain.is_auto ? "secondary" : "outline"}>
                {domain.is_auto ? "Auto" : "Custom"}
              </Badge>
              {domain.is_primary ? (
                <Badge
                  variant="outline"
                  className="border-amber-400/40 text-amber-200"
                >
                  <Star className="mr-1 h-3 w-3" />
                  Primary
                </Badge>
              ) : null}
            </div>
            <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <span className="inline-flex items-center gap-1 rounded-full border border-white/8 px-2 py-1">
                <Link2 className="h-3 w-3" />
                DNS {domain.dns_verified ? "verified" : "pending"}
              </span>
              <span
                className={cn(
                  "inline-flex items-center gap-1 rounded-full border px-2 py-1",
                  domain.ssl_status === "active" &&
                    "border-emerald-400/25 text-emerald-200",
                  domain.ssl_status === "pending" &&
                    "border-amber-400/25 text-amber-100",
                  domain.ssl_status === "error" &&
                    "border-rose-400/25 text-rose-200",
                )}
              >
                <CheckCircle2 className="h-3 w-3" />
                SSL {domain.ssl_status}
              </span>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            {ready ? (
              <Button
                asChild
                size="sm"
                variant="outline"
                className="h-9 md:h-8"
              >
                <a
                  href={`https://${domain.domain}`}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  <ExternalLink className="mr-1.5 h-3.5 w-3.5" />
                  Open
                </a>
              </Button>
            ) : null}
            {!domain.is_auto ? (
              <>
                <Button
                  size="sm"
                  variant="outline"
                  className="h-9 md:h-8"
                  onClick={() => verifyMutation.mutate(domain.id)}
                  disabled={allVerifyDisabled}
                >
                  {isVerifying ? (
                    <LoaderCircle className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <GlobeLock className="mr-1.5 h-3.5 w-3.5" />
                  )}
                  {isVerifying ? "Verifying..." : "Verify"}
                </Button>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button
                      size="sm"
                      variant="outline"
                      className="h-9 w-9 px-0 md:h-8 md:w-8"
                      aria-label={`Open actions for ${domain.domain}`}
                    >
                      <MoreHorizontal className="h-4 w-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem
                      onSelect={() =>
                        moveMutation.mutate({
                          domainId: domain.id,
                          environment:
                            domain.environment === "preview"
                              ? "production"
                              : "preview",
                        })
                      }
                      disabled={!canMove}
                    >
                      <RefreshCcw className="mr-2 h-4 w-4" />
                      Move to{" "}
                      {domain.environment === "preview"
                        ? "Production"
                        : "Preview"}
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      onSelect={() => verifyMutation.mutate(domain.id)}
                      disabled={allVerifyDisabled}
                    >
                      <GlobeLock className="mr-2 h-4 w-4" />
                      Re-verify
                    </DropdownMenuItem>
                    {!domain.is_primary ? (
                      <DropdownMenuItem
                        onSelect={() => setPrimaryMutation.mutate(domain.id)}
                        disabled={setPrimaryMutation.isPending}
                      >
                        <Star className="mr-2 h-4 w-4" />
                        Set as primary
                      </DropdownMenuItem>
                    ) : null}
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                      className="text-red-400 focus:text-red-400"
                      onSelect={() => setDeleteTarget(domain)}
                      disabled={deleteMutation.isPending}
                    >
                      <Trash2 className="mr-2 h-4 w-4" />
                      Delete
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </>
            ) : null}
          </div>
        </div>

        {showLogPanel && (
          <VerificationLogPanel
            logs={logs}
            state={verifyState}
            summary={summary}
            minimized={minimized}
            onToggleMinimize={() => setMinimized((prev) => !prev)}
          />
        )}
      </div>
    );
  }

  const hasDomains = (domains?.length ?? 0) > 0;

  return (
    <div className="space-y-8">
      {/* Domain Inventory */}
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-base font-semibold">Domain Inventory</h2>
            <p className="text-sm text-muted-foreground">
              Verified domains become quick links automatically.
            </p>
          </div>
          {!showAddForm && (
            <Button
              size="sm"
              variant="outline"
              onClick={() => setShowAddForm(true)}
            >
              <Plus className="mr-1.5 h-3.5 w-3.5" />
              Add Domain
            </Button>
          )}
        </div>

        {isLoading ? (
          <LoadingState
            title="Loading domains..."
            description="Fetching custom domains, verification, and SSL state."
            className="min-h-[220px]"
          />
        ) : !hasDomains && !showAddForm ? (
          <div className="rounded-lg border border-dashed border-border/70 px-4 py-8 text-center">
            <p className="text-sm text-muted-foreground">
              No domains configured. Add a custom domain to get started.
            </p>
            <Button
              size="sm"
              variant="outline"
              className="mt-3"
              onClick={() => setShowAddForm(true)}
            >
              <Plus className="mr-1.5 h-3.5 w-3.5" />
              Add Domain
            </Button>
          </div>
        ) : (
          <div className="divide-y overflow-hidden rounded-lg border">
            {/* Inline add form */}
            {showAddForm && (
              <div className="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center">
                <Input
                  placeholder="example.com"
                  value={newDomain}
                  onChange={(e) => setNewDomain(e.target.value)}
                  className="flex-1"
                  autoFocus
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && newDomain.trim())
                      addMutation.mutate();
                    if (e.key === "Escape") handleCancel();
                  }}
                />
                <Select
                  value={newDomainEnvironment}
                  onValueChange={(value) =>
                    setNewDomainEnvironment(value as Domain["environment"])
                  }
                >
                  <SelectTrigger className="w-full sm:w-[160px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="preview">Preview</SelectItem>
                    <SelectItem value="production">Production</SelectItem>
                  </SelectContent>
                </Select>
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    onClick={() => addMutation.mutate()}
                    disabled={!newDomain.trim() || addMutation.isPending}
                  >
                    {addMutation.isPending ? "Saving..." : "Save"}
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={handleCancel}
                    disabled={addMutation.isPending}
                  >
                    <X className="h-4 w-4" />
                    Cancel
                  </Button>
                </div>
              </div>
            )}

            {/* Production group */}
            {productionDomains.length > 0 && (
              <>
                <div className="px-4 py-2">
                  <span className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                    Production
                  </span>
                </div>
                {productionDomains.map(renderDomainRow)}
              </>
            )}

            {/* Preview group */}
            {previewDomains.length > 0 && (
              <>
                <div className="px-4 py-2">
                  <span className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                    Preview
                  </span>
                </div>
                {previewDomains.map(renderDomainRow)}
              </>
            )}
          </div>
        )}
      </div>

      {/* DNS setup guide */}
      <DnsSetupGuide
        domain={newDomain.trim().toLowerCase()}
        environment={newDomainEnvironment}
        platform={platform}
      />

      <AlertDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open && !deleteMutation.isPending) {
            setDeleteTarget(null);
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle className="break-all">
              Delete {deleteTarget?.domain}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              This removes the proxy config for the domain, so the live URL will
              stop responding. Existing certificate files are left in place and
              expire naturally.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              className="bg-red-500 text-white hover:bg-red-500/90"
              disabled={deleteMutation.isPending}
              onClick={(e) => {
                e.preventDefault();
                if (deleteTarget) {
                  deleteMutation.mutate(deleteTarget.id);
                }
              }}
            >
              {deleteMutation.isPending ? "Deleting..." : "Delete"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
