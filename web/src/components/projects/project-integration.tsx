import { useMemo, useState } from "react";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, ChevronDown, ChevronRight, RefreshCcw } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { CodePanel } from "@/components/ui/code-panel";
import { LoadingState, Spinner } from "@/components/ui/spinner";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";

export function ProjectIntegrationTab({ projectId }: { projectId: string }) {
  const queryClient = useQueryClient();
  const timezone =
    Intl.DateTimeFormat().resolvedOptions().timeZone?.trim() || "UTC";

  const { data, isLoading, error } = useQuery({
    queryKey: ["project-analytics-integration", projectId, timezone],
    queryFn: () =>
      api.getProjectAnalytics(projectId, {
        environment: "all",
        range: "24h",
        timezone,
      }),
  });

  const verifyMutation = useMutation({
    mutationFn: () =>
      api.verifyProjectAnalytics(projectId, {
        environment: "all",
        range: "24h",
        timezone,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["project-analytics", projectId],
      });
      queryClient.invalidateQueries({
        queryKey: ["project-analytics-integration", projectId],
      });
      toast.success("Analytics verification refreshed");
    },
    onError: (err) => toast.error(err.message),
  });

  const copyValue = async (value: string, label: string) => {
    if (!value.trim()) {
      toast.error(`${label} is not available yet`);
      return;
    }
    try {
      await navigator.clipboard.writeText(value);
      toast.success(`${label} copied`);
    } catch {
      toast.error(`Couldn't copy ${label.toLowerCase()}`);
    }
  };

  const eventHelperSnippet = useMemo(
    () =>
      [
        "window.umami = window.umami || {};",
        "",
        "export function trackAnalyticsEvent(name, data) {",
        "  if (typeof window !== 'undefined' && window.umami?.track) {",
        "    window.umami.track(name, data);",
        "  }",
        "}",
        "",
        "export function identifyAnalyticsUser(id, traits) {",
        "  if (typeof window !== 'undefined' && window.umami?.identify) {",
        "    window.umami.identify(id, traits);",
        "  }",
        "}",
      ].join("\n"),
    [],
  );

  if (isLoading) {
    return (
      <LoadingState
        title="Loading integration…"
        description="Preparing the Umami install prompt, snippet, and verification status."
        className="min-h-[340px]"
      />
    );
  }

  if (error || !data) {
    return (
      <div className="rounded-xl border border-rose-400/25 bg-rose-400/10 px-5 py-4 text-sm text-rose-100">
        {error instanceof Error ? error.message : "Unknown analytics error."}
      </div>
    );
  }

  // Unavailable state — Umami not configured on this instance
  if (data.audience.status === "unavailable") {
    return (
      <div className="rounded-xl border border-dashed border-border/70 px-5 py-12 text-center text-sm text-muted-foreground">
        Analytics not configured for this project. Contact your administrator.
      </div>
    );
  }

  const step1Complete =
    data.audience.status === "receiving_data" ||
    data.audience.status === "waiting_for_data";
  const step2Verified = Boolean(data.audience.verified_at);

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-lg font-semibold tracking-tight text-foreground">
          Analytics Integration
        </h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Set up audience tracking for your project.
        </p>
      </div>

      <div className="space-y-0">
        {/* Step 1: Install Tracker */}
        <WizardStep
          number={1}
          title="Install Tracker"
          statusLabel={step1Complete ? "Complete" : "Pending"}
          isComplete={step1Complete}
          defaultOpen={!step1Complete}
          isLast={false}
        >
          <p className="mb-4 text-sm text-muted-foreground">
            Add the tracking snippet to your site. Paste this in your{" "}
            <code className="rounded bg-muted/60 px-1 py-0.5 text-xs text-foreground">
              {"<head>"}
            </code>
            :
          </p>
          <div className="space-y-4">
            <CodePanel
              title="Manual Snippet"
              description="Paste this script tag into your HTML head."
              value={data.audience.install.snippet}
              heightClassName="h-28"
              onCopy={() =>
                copyValue(data.audience.install.snippet, "Manual snippet")
              }
            />
            <CodePanel
              title="AI Install Prompt"
              description="Paste this into Claude, Codex, or ChatGPT inside the app repository."
              value={data.audience.install.ai_prompt}
              onCopy={() =>
                copyValue(data.audience.install.ai_prompt, "AI install prompt")
              }
            />
          </div>
        </WizardStep>

        {/* Step 2: Verify Installation */}
        <WizardStep
          number={2}
          title="Verify Installation"
          statusLabel={step2Verified ? "Verified" : "Pending"}
          isComplete={step2Verified}
          defaultOpen={step1Complete && !step2Verified}
          isLast={false}
        >
          <p className="mb-4 text-sm text-muted-foreground">
            Domains being tracked:
          </p>
          {data.audience.install.domains.all.length > 0 ? (
            <div className="mb-4 flex flex-wrap gap-2">
              {data.audience.install.domains.all.map((domain) => (
                <Badge
                  key={domain}
                  variant="outline"
                  className="gap-1.5 border-white/10 bg-white/5 font-mono text-xs text-slate-200"
                >
                  <Check className="h-3 w-3 text-emerald-400" />
                  {domain}
                </Badge>
              ))}
            </div>
          ) : (
            <div className="mb-4 rounded-xl border border-dashed border-border/70 px-4 py-4 text-sm text-muted-foreground">
              No verified domains are attached to this project yet.
            </div>
          )}
          {data.audience.error ? (
            <div className="mb-4 rounded-xl border border-rose-400/25 bg-rose-400/10 px-4 py-3 text-sm text-rose-100">
              {data.audience.error}
            </div>
          ) : null}
          <Button
            size="sm"
            onClick={() => verifyMutation.mutate()}
            disabled={verifyMutation.isPending}
          >
            {verifyMutation.isPending ? (
              <Spinner className="mr-1.5 size-3.5" />
            ) : (
              <RefreshCcw className="mr-1.5 h-3.5 w-3.5" />
            )}
            Verify Now
          </Button>
        </WizardStep>

        {/* Step 3: Track Custom Events */}
        <WizardStep
          number={3}
          title="Track Custom Events"
          statusLabel="Optional"
          isComplete={false}
          isOptional
          defaultOpen={false}
          isLast={true}
        >
          <p className="mb-4 text-sm text-muted-foreground">
            Once pageviews are live, add a tiny analytics wrapper instead of
            calling Umami directly across the codebase. Track only meaningful
            product events: conversion starts, completed submissions, purchases,
            upgrades, and activation milestones.
          </p>
          <CodePanel
            title="Event Helper Example"
            description="A small wrapper keeps event naming consistent across the app."
            value={eventHelperSnippet}
            onCopy={() => copyValue(eventHelperSnippet, "Event helper")}
          />
        </WizardStep>
      </div>
    </div>
  );
}

interface WizardStepProps {
  number: number;
  title: string;
  statusLabel: string;
  isComplete: boolean;
  isOptional?: boolean;
  defaultOpen: boolean;
  isLast: boolean;
  children: React.ReactNode;
}

function WizardStep({
  number,
  title,
  statusLabel,
  isComplete,
  isOptional = false,
  defaultOpen,
  isLast,
  children,
}: WizardStepProps) {
  const [open, setOpen] = useState(defaultOpen);

  return (
    <div className="flex gap-4">
      {/* Left column: number + vertical line */}
      <div className="flex flex-col items-center">
        <div
          className={cn(
            "flex h-8 w-8 shrink-0 items-center justify-center rounded-full border text-sm font-semibold",
            isComplete
              ? "border-emerald-400/50 bg-emerald-400/15 text-emerald-300"
              : isOptional
                ? "border-white/15 bg-white/5 text-muted-foreground"
                : "border-primary/40 bg-primary/10 text-primary",
          )}
        >
          {isComplete ? <Check className="h-4 w-4" /> : number}
        </div>
        {!isLast && (
          <div
            className={cn(
              "mt-1 w-px flex-1",
              isComplete ? "bg-emerald-400/25" : "bg-border/50",
            )}
          />
        )}
      </div>

      {/* Right column: header + collapsible content */}
      <div className={cn("min-w-0 flex-1 pb-8", isLast && "pb-0")}>
        <Collapsible open={open} onOpenChange={setOpen}>
          <CollapsibleTrigger className="group flex w-full items-center justify-between gap-3 rounded-lg py-1 text-left hover:opacity-80">
            <div className="flex items-center gap-3">
              <span className="font-medium text-foreground">{title}</span>
              <span
                className={cn(
                  "rounded-full px-2 py-0.5 text-xs font-medium",
                  isComplete
                    ? "bg-emerald-400/15 text-emerald-300"
                    : isOptional
                      ? "bg-white/5 text-muted-foreground"
                      : "bg-amber-400/15 text-amber-300",
                )}
              >
                {isComplete && !isOptional ? (
                  <span className="flex items-center gap-1">
                    <Check className="h-3 w-3" />
                    {statusLabel}
                  </span>
                ) : (
                  statusLabel
                )}
              </span>
            </div>
            <span className="text-muted-foreground">
              {open ? (
                <ChevronDown className="h-4 w-4" />
              ) : (
                <ChevronRight className="h-4 w-4" />
              )}
            </span>
          </CollapsibleTrigger>

          <CollapsibleContent className="mt-4">{children}</CollapsibleContent>
        </Collapsible>
      </div>
    </div>
  );
}
