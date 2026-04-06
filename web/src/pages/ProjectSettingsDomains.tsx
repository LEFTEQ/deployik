import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import {
  CheckCircle2,
  ExternalLink,
  GlobeLock,
  Link2,
  LoaderCircle,
} from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import { ENVIRONMENT_META, isDomainReady } from "@/lib/deployment-helpers";
import { DnsSetupGuide } from "@/components/projects/dns-setup-guide";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { LoadingState } from "@/components/ui/spinner";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import type { Domain } from "@/types/api";

export function ProjectSettingsDomains() {
  const { id } = useParams({ strict: false }) as { id: string };
  const queryClient = useQueryClient();
  const [newDomain, setNewDomain] = useState("");
  const [newDomainEnvironment, setNewDomainEnvironment] =
    useState<Domain["environment"]>("production");

  const { data: domains, isLoading } = useQuery({
    queryKey: ["domains", id],
    queryFn: () => api.listDomains(id),
  });

  const { data: platform } = useQuery({
    queryKey: ["platform"],
    queryFn: () => api.getPlatformInfo(),
  });

  const addMutation = useMutation({
    mutationFn: () =>
      api.addDomain(id, {
        domain: newDomain.trim().toLowerCase(),
        environment: newDomainEnvironment,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["domains", id] });
      setNewDomain("");
      setNewDomainEnvironment("production");
      toast.success("Domain added");
    },
    onError: (err) => toast.error(err.message),
  });

  const verifyMutation = useMutation({
    mutationFn: (domainId: string) => api.verifyDomain(id, domainId),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ["domains", id] });
      toast.success(result.message);
    },
    onError: (err) => toast.error(err.message),
  });

  return (
    <div className="space-y-8">
      {/* Add domain form */}
      <div className="space-y-3">
        <div>
          <h2 className="text-base font-semibold">Add Custom Domain</h2>
          <p className="text-sm text-muted-foreground">
            Choose whether the domain should front preview or production.
            Production is usually the bought, customer-facing domain.
          </p>
        </div>
        <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_180px_auto]">
          <Input
            placeholder="example.com"
            value={newDomain}
            onChange={(e) => setNewDomain(e.target.value)}
          />
          <Select
            value={newDomainEnvironment}
            onValueChange={(value) =>
              setNewDomainEnvironment(value as Domain["environment"])
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="preview">Preview</SelectItem>
              <SelectItem value="production">Production</SelectItem>
            </SelectContent>
          </Select>
          <Button
            onClick={() => addMutation.mutate()}
            disabled={!newDomain.trim() || addMutation.isPending}
          >
            {addMutation.isPending ? "Adding..." : "Add domain"}
          </Button>
        </div>
      </div>

      {/* DNS setup guide */}
      <DnsSetupGuide
        domain={newDomain.trim().toLowerCase()}
        environment={newDomainEnvironment}
        platform={platform}
      />

      {/* Domain inventory */}
      <div className="space-y-3">
        <div>
          <h2 className="text-base font-semibold">Domain Inventory</h2>
          <p className="text-sm text-muted-foreground">
            Verified domains become quick links automatically.
          </p>
        </div>
        {isLoading ? (
          <LoadingState
            title="Loading domains..."
            description="Fetching custom domains, verification, and SSL state."
            className="min-h-[220px]"
          />
        ) : !domains?.length ? (
          <div className="rounded-lg border border-dashed border-border/70 px-4 py-6 text-sm text-muted-foreground">
            No domains yet.
          </div>
        ) : (
          <div className="divide-y rounded-lg border">
            {domains.map((domain) => {
              const ready = isDomainReady(domain);
              const verifying =
                verifyMutation.isPending &&
                verifyMutation.variables === domain.id;

              return (
                <div
                  key={domain.id}
                  className="flex flex-col gap-4 px-4 py-3 md:flex-row md:items-center md:justify-between"
                >
                  <div className="space-y-2">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="text-sm font-medium">{domain.domain}</p>
                      <Badge
                        variant="outline"
                        className={
                          ENVIRONMENT_META[domain.environment].badgeClass
                        }
                      >
                        {ENVIRONMENT_META[domain.environment].label}
                      </Badge>
                      <Badge variant={domain.is_auto ? "secondary" : "outline"}>
                        {domain.is_auto ? "Auto" : "Custom"}
                      </Badge>
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

                  <div className="flex flex-wrap gap-2">
                    {ready ? (
                      <Button asChild size="sm" variant="outline">
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
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => verifyMutation.mutate(domain.id)}
                        disabled={verifyMutation.isPending}
                      >
                        {verifying ? (
                          <LoaderCircle className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                        ) : (
                          <GlobeLock className="mr-1.5 h-3.5 w-3.5" />
                        )}
                        Verify
                      </Button>
                    ) : null}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
