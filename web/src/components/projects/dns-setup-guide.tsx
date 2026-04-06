import { Copy } from "lucide-react";
import { ChevronRight } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import type { Domain, PlatformInfo } from "@/types/api";

export interface DnsSetupGuideProps {
  domain: string;
  environment: Domain["environment"];
  platform: PlatformInfo | undefined;
}

function getDnsHostHint(domain: string): string {
  const labels = domain.split(".").filter(Boolean);
  if (labels.length <= 2) {
    return "@";
  }
  if (labels.length === 3) {
    return labels[0]!;
  }
  return `${labels.slice(0, -2).join(".")} (subdomain part)`;
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
  const sampleHost = getDnsHostHint(sampleDomain);

  const copyValue = async (value: string, label: string) => {
    try {
      await navigator.clipboard.writeText(value);
      toast.success(`${label} copied`);
    } catch {
      toast.error(`Couldn't copy ${label.toLowerCase()}`);
    }
  };

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
            Point the domain at this VPS, then click Verify. Preview domains
            should usually be subdomains; production is usually the real bought
            domain.
          </p>

          <div className="grid gap-3 md:grid-cols-2">
            <div className="rounded-xl border bg-muted/30 p-4">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <p className="text-xs uppercase tracking-[0.22em] text-muted-foreground">
                    Target IP
                  </p>
                  <p className="mt-2 font-mono text-sm">
                    {dnsTargetIp || "Unavailable"}
                  </p>
                </div>
                {dnsTargetIp ? (
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => copyValue(dnsTargetIp, "Target IP")}
                  >
                    <Copy className="mr-1.5 h-3.5 w-3.5" />
                    Copy
                  </Button>
                ) : null}
              </div>
            </div>

            <div className="rounded-xl border bg-muted/30 p-4">
              <p className="text-xs uppercase tracking-[0.22em] text-muted-foreground">
                Record to create
              </p>
              <div className="mt-2 space-y-1 text-sm">
                <p>
                  <span className="text-muted-foreground">Domain:</span>{" "}
                  <span className="font-medium">{sampleDomain}</span>
                </p>
                <p>
                  <span className="text-muted-foreground">Type:</span>{" "}
                  <span className="font-medium">A</span>
                </p>
                <p>
                  <span className="text-muted-foreground">Host / Name:</span>{" "}
                  <span className="font-medium">{sampleHost}</span>
                </p>
              </div>
            </div>
          </div>

          <div className="grid gap-3 xl:grid-cols-2">
            <div className="rounded-xl border bg-muted/30 p-4">
              <p className="text-sm font-medium">GoDaddy</p>
              <div className="mt-3 space-y-2 text-sm text-muted-foreground">
                <p>1. Open your domain in GoDaddy and go to DNS.</p>
                <p>2. Add an A record for the host shown above.</p>
                <p>3. Set Points to to {dnsTargetIp || "the target VPS IP"}.</p>
                <p>
                  4. For root domains, also add `www` and point it to the same
                  place.
                </p>
                <p>5. Leave TTL on the default value.</p>
                <p>
                  6. Remove conflicting A, AAAA, or forwarded records for the
                  same host.
                </p>
              </div>
            </div>

            <div className="rounded-xl border bg-muted/30 p-4">
              <p className="text-sm font-medium">
                Vercel DNS / Domain bought on Vercel
              </p>
              <div className="mt-3 space-y-2 text-sm text-muted-foreground">
                <p>1. Open the domain in Vercel and go to DNS Records.</p>
                <p>2. Add an A record for the host shown above.</p>
                <p>3. Set the value to {dnsTargetIp || "the target VPS IP"}.</p>
                <p>
                  4. For root domains, also create `www` and point it at the
                  same target or CNAME it to the root.
                </p>
                <p>
                  5. Remove old Vercel-specific records for the same host if
                  they conflict.
                </p>
              </div>
            </div>
          </div>

          <div className="rounded-xl border border-dashed border-border/70 px-4 py-3 text-sm text-muted-foreground">
            <p>
              Deployik verifies A-record resolution to the VPS IP. If you use a
              subdomain, prefer an A record directly to the server. After DNS
              propagates, click Verify to issue SSL and activate the domain. Root
              domains are served without `www`; Deployik also issues SSL for the
              `www` variant and redirects it to the apex host.
            </p>
          </div>
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
