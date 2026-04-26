import { Check, Copy } from "lucide-react";
import { ChevronRight } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { cn } from "@/lib/utils";
import type { Domain, PlatformInfo } from "@/types/api";

export interface DnsSetupGuideProps {
  domain: string;
  environment: Domain["environment"];
  platform: PlatformInfo | undefined;
}

interface DnsTarget {
  isApex: boolean;
  host: string;
  apex: string;
}

function describeDomain(domain: string): DnsTarget {
  const labels = domain.split(".").filter(Boolean);
  if (labels.length <= 2) {
    return { isApex: true, host: "@", apex: domain };
  }
  if (labels.length === 3) {
    return {
      isApex: false,
      host: labels[0]!,
      apex: labels.slice(1).join("."),
    };
  }
  return {
    isApex: false,
    host: labels.slice(0, -2).join("."),
    apex: labels.slice(-2).join("."),
  };
}

interface CopyChipProps {
  value: string;
  label?: string;
  size?: "sm" | "md";
  tone?: "default" | "muted";
  className?: string;
}

function CopyChip({
  value,
  label,
  size = "sm",
  tone = "default",
  className,
}: CopyChipProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async (event: React.MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    event.stopPropagation();
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      toast.success(`${label ?? "Value"} copied`);
      window.setTimeout(() => setCopied(false), 1400);
    } catch {
      toast.error(`Couldn't copy ${(label ?? "value").toLowerCase()}`);
    }
  };

  return (
    <button
      type="button"
      onClick={handleCopy}
      title={`Click to copy: ${value}`}
      className={cn(
        "inline-flex items-center gap-1.5 rounded-md border border-transparent font-mono transition-colors",
        "hover:border-border hover:bg-background hover:text-foreground",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        size === "sm" ? "px-1.5 py-0.5 text-xs" : "px-2 py-1 text-sm",
        tone === "muted"
          ? "bg-muted/40 text-muted-foreground"
          : "bg-muted text-foreground",
        className,
      )}
    >
      <span className="break-all text-left">{value}</span>
      {copied ? (
        <Check className="h-3 w-3 shrink-0 text-emerald-500" />
      ) : (
        <Copy className="h-3 w-3 shrink-0 opacity-50" />
      )}
    </button>
  );
}

export function DnsSetupGuide({
  domain,
  environment,
  platform,
}: DnsSetupGuideProps) {
  const dnsTargetIp = platform?.dns_target_ip?.trim();
  const sampleDomain =
    domain ||
    (environment === "preview" ? "staging.example.com" : "example.com");
  const target = describeDomain(sampleDomain);
  const cnameTarget = `${sampleDomain}.`;

  const Ip = ({ size = "sm" }: { size?: "sm" | "md" }) =>
    dnsTargetIp ? (
      <CopyChip value={dnsTargetIp} label="Target IP" size={size} />
    ) : (
      <span className="font-mono text-xs text-muted-foreground">
        the target VPS IP
      </span>
    );

  return (
    <Collapsible>
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="flex w-full items-center gap-2 rounded-lg border bg-muted/30 px-4 py-3 text-sm font-medium text-foreground hover:bg-accent transition-colors group"
        >
          <ChevronRight className="h-4 w-4 text-muted-foreground transition-transform group-data-[state=open]:rotate-90" />
          DNS Setup Guide
          <span className="text-xs text-muted-foreground ml-auto">
            How to configure your domain
          </span>
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="mt-2 space-y-3 rounded-lg border p-4">
          <p className="text-sm text-muted-foreground">
            Point the domain at this VPS in your registrar's DNS settings, then
            click Verify. Works with any DNS provider — Webglobe, GoDaddy,
            Cloudflare, Namecheap, Vercel, Wedos, etc. Tip: every value below
            with a copy icon is one click away.
          </p>

          <div className="grid gap-3 md:grid-cols-2">
            <div className="rounded-xl border bg-muted/30 p-4">
              <p className="text-xs uppercase tracking-[0.22em] text-muted-foreground">
                Target IP
              </p>
              <div className="mt-2">
                {dnsTargetIp ? (
                  <CopyChip value={dnsTargetIp} label="Target IP" size="md" />
                ) : (
                  <p className="font-mono text-sm text-muted-foreground">
                    Unavailable
                  </p>
                )}
              </div>
            </div>

            <div className="rounded-xl border bg-muted/30 p-4">
              <p className="text-xs uppercase tracking-[0.22em] text-muted-foreground">
                Record to create
              </p>
              <div className="mt-2 space-y-1.5 text-sm">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-muted-foreground">Domain:</span>
                  <CopyChip value={sampleDomain} label="Domain" />
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-muted-foreground">Type:</span>
                  <span className="font-mono text-xs font-medium">A</span>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-muted-foreground">Host / Name:</span>
                  <CopyChip value={target.host} label="Host" />
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-muted-foreground">Value:</span>
                  <Ip />
                </div>
              </div>
            </div>
          </div>

          <div className="rounded-xl border bg-muted/30 p-4">
            <p className="text-sm font-medium">Quick reference: A vs CNAME</p>
            <div className="mt-3 grid gap-3 text-sm text-muted-foreground md:grid-cols-2">
              <div>
                <p className="font-medium text-foreground">
                  A record → use this
                </p>
                <p className="mt-1">
                  Points a hostname directly at an IP address (e.g. <Ip />).
                  Use this for the apex/root and for any subdomain you point at
                  the VPS.
                </p>
              </div>
              <div>
                <p className="font-medium text-foreground">
                  CNAME → only points to other hostnames
                </p>
                <p className="mt-1">
                  A CNAME can only point to another domain name, never to an
                  IP. If your provider rejects an IP value with "fully-qualified
                  domain name required", you picked CNAME by mistake — switch
                  the record type to A.
                </p>
              </div>
            </div>
          </div>

          {target.isApex ? (
            <div className="rounded-xl border bg-muted/30 p-4">
              <p className="text-sm font-medium">
                Root domain ({sampleDomain}) — two records
              </p>
              <div className="mt-3 space-y-3 text-sm text-muted-foreground">
                <div>
                  <p className="font-medium text-foreground">
                    1. The apex itself
                  </p>
                  <ul className="mt-1.5 space-y-1 pl-5 list-disc marker:text-muted-foreground/60">
                    <li className="flex flex-wrap items-center gap-2">
                      <span>Type:</span>
                      <span className="font-mono text-xs font-medium text-foreground">
                        A
                      </span>
                    </li>
                    <li className="flex flex-wrap items-center gap-2">
                      <span>Host / Name:</span>
                      <CopyChip value="@" label="Host" />
                      <span className="text-xs">
                        (some providers use a blank field or the bare domain)
                      </span>
                    </li>
                    <li className="flex flex-wrap items-center gap-2">
                      <span>Value:</span>
                      <Ip />
                    </li>
                  </ul>
                </div>
                <div>
                  <p className="font-medium text-foreground">
                    2. The www variant
                  </p>
                  <p className="mt-1">
                    Pick one — both work, Deployik will redirect{" "}
                    <span className="font-mono">www</span> to the apex
                    automatically:
                  </p>
                  <ul className="mt-1.5 space-y-2 pl-5 list-disc marker:text-muted-foreground/60">
                    <li>
                      <span className="font-medium text-foreground">
                        A record (simplest):
                      </span>
                      <div className="mt-1 flex flex-wrap items-center gap-2">
                        <span>Host</span>
                        <CopyChip value="www" label="Host" />
                        <span>Value</span>
                        <Ip />
                      </div>
                    </li>
                    <li>
                      <span className="font-medium text-foreground">
                        CNAME (lower maintenance):
                      </span>
                      <div className="mt-1 flex flex-wrap items-center gap-2">
                        <span>Host</span>
                        <CopyChip value="www" label="Host" />
                        <span>Value</span>
                        <CopyChip value={cnameTarget} label="CNAME target" />
                      </div>
                      <p className="mt-1 text-xs">
                        Note the trailing dot — many providers (Webglobe,
                        Wedos, BIND-style panels) require it. If the IP ever
                        changes, you only update the apex A record.
                      </p>
                    </li>
                  </ul>
                </div>
              </div>
            </div>
          ) : (
            <div className="rounded-xl border bg-muted/30 p-4">
              <p className="text-sm font-medium">
                Subdomain ({sampleDomain}) — one record
              </p>
              <ul className="mt-3 space-y-1 pl-5 list-disc marker:text-muted-foreground/60 text-sm text-muted-foreground">
                <li className="flex flex-wrap items-center gap-2">
                  <span>Type:</span>
                  <span className="font-mono text-xs font-medium text-foreground">
                    A
                  </span>
                </li>
                <li className="flex flex-wrap items-center gap-2">
                  <span>Host / Name:</span>
                  <CopyChip value={target.host} label="Host" />
                  <span className="text-xs">
                    (just the subdomain part — don't include{" "}
                    <span className="font-mono">.{target.apex}</span>)
                  </span>
                </li>
                <li className="flex flex-wrap items-center gap-2">
                  <span>Value:</span>
                  <Ip />
                </li>
              </ul>
            </div>
          )}

          <div className="rounded-xl border bg-muted/30 p-4">
            <p className="text-sm font-medium">Common pitfalls</p>
            <ul className="mt-3 list-disc space-y-1.5 pl-5 marker:text-muted-foreground/60 text-sm text-muted-foreground">
              <li>
                <span className="font-medium text-foreground">
                  IP in a CNAME field.
                </span>{" "}
                CNAMEs only accept hostnames. Switch to A, or change the value
                to a hostname (e.g. the apex domain with a trailing dot).
              </li>
              <li>
                <span className="font-medium text-foreground">
                  Trailing dot.
                </span>{" "}
                Some providers require CNAME values to end with a dot —{" "}
                <CopyChip value={cnameTarget} label="CNAME target" /> not{" "}
                <span className="font-mono">{sampleDomain}</span>.
              </li>
              <li>
                <span className="font-medium text-foreground">
                  Conflicting records.
                </span>{" "}
                Remove old A, AAAA, ALIAS, or web-forwarding records pointing
                at a previous host before saving the new one.
              </li>
              <li>
                <span className="font-medium text-foreground">TTL.</span>{" "}
                Default is fine (3600s). Lower it temporarily if you're testing
                changes — shorter TTL means faster propagation.
              </li>
              <li>
                <span className="font-medium text-foreground">
                  Cloudflare proxy.
                </span>{" "}
                If you use Cloudflare, set the record to{" "}
                <span className="font-medium">DNS-only</span> (grey cloud)
                until Deployik issues SSL — orange-cloud proxying breaks Let's
                Encrypt HTTP-01 verification.
              </li>
            </ul>
          </div>

          <div className="rounded-xl border border-dashed border-border/70 px-4 py-3 text-sm text-muted-foreground">
            <p>
              DNS changes can take a few minutes (sometimes longer) to
              propagate. Once the record resolves to <Ip />, click Verify —
              Deployik checks the A record, issues an SSL certificate, and
              activates the domain. For root domains, SSL is also issued for
              the <span className="font-mono">www</span> variant and redirected
              to the apex.
            </p>
          </div>
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
