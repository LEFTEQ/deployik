import { useMemo, useState } from "react";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowUpRight, Copy, RefreshCcw, Sparkles } from "lucide-react";
import { toast } from "sonner";

import { AUDIENCE_STATUS_META } from "@/components/projects/project-analytics-meta";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { CodePanel } from "@/components/ui/code-panel";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";

const STEP_VALUES = ["install", "verify", "events"] as const;
type StepValue = (typeof STEP_VALUES)[number];

export function ProjectIntegrationTab({ projectId }: { projectId: string }) {
  const queryClient = useQueryClient();
  const [step, setStep] = useState<StepValue>("install");
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
      <div className="grid gap-4">
        <Skeleton className="h-48 w-full" />
        <Skeleton className="h-72 w-full" />
      </div>
    );
  }

  if (error || !data) {
    return (
      <Card className="border-rose-400/25">
        <CardHeader>
          <CardTitle className="text-base text-rose-100">
            Integration failed to load
          </CardTitle>
        </CardHeader>
        <CardContent className="text-sm text-muted-foreground">
          {error instanceof Error ? error.message : "Unknown analytics error."}
        </CardContent>
      </Card>
    );
  }

  const meta = AUDIENCE_STATUS_META[data.audience.status] || {
    label: "Ready to install",
    badgeClass: "border-primary/25 bg-primary/12 text-primary",
    description:
      "The website exists. Add the tracker to start collecting audience data.",
  };

  return (
    <div className="space-y-4">
      <Card className="overflow-hidden border-white/10">
        <CardContent className="relative px-5 py-5 sm:px-6">
          <div className="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-primary/40 to-transparent" />
          <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
            <div className="space-y-3">
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant="outline" className={meta.badgeClass}>
                  {meta.label}
                </Badge>
                <Badge
                  variant="outline"
                  className="border-white/10 bg-white/5 font-mono text-slate-200"
                >
                  {data.audience.website_id || "website pending"}
                </Badge>
              </div>
              <div>
                <h2 className="text-lg font-semibold tracking-tight text-foreground">
                  Analytics Integration
                </h2>
                <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
                  Keep setup separate from the analytics dashboard. Install the
                  tracker, verify traffic, then add custom events only when you
                  need them.
                </p>
              </div>
            </div>

            <div className="flex flex-wrap gap-2">
              <Button
                size="sm"
                onClick={() =>
                  copyValue(data.audience.install.ai_prompt, "AI install prompt")
                }
              >
                <Sparkles className="mr-1.5 h-3.5 w-3.5" />
                Install with AI
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() =>
                  copyValue(data.audience.install.snippet, "Manual snippet")
                }
              >
                <Copy className="mr-1.5 h-3.5 w-3.5" />
                Copy snippet
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() => verifyMutation.mutate()}
                disabled={verifyMutation.isPending}
              >
                <RefreshCcw
                  className={cn(
                    "mr-1.5 h-3.5 w-3.5",
                    verifyMutation.isPending && "animate-spin",
                  )}
                />
                Verify
              </Button>
              {data.audience.open_url ? (
                <Button asChild size="sm" variant="ghost">
                  <a
                    href={data.audience.open_url}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    <ArrowUpRight className="mr-1.5 h-3.5 w-3.5" />
                    Open Umami
                  </a>
                </Button>
              ) : null}
            </div>
          </div>
        </CardContent>
      </Card>

      <Tabs value={step} onValueChange={(value) => setStep(value as StepValue)}>
        <TabsList className="h-auto flex-wrap justify-start gap-1 rounded-2xl border border-white/8 bg-black/10 p-1">
          <TabsTrigger value="install">Install</TabsTrigger>
          <TabsTrigger value="verify">Verify</TabsTrigger>
          <TabsTrigger value="events">Track Events</TabsTrigger>
        </TabsList>

        <TabsContent value="install" className="mt-4">
          <div className="grid gap-4 xl:grid-cols-[minmax(0,0.9fr)_minmax(320px,1.1fr)]">
            <Card className="border-white/10">
              <CardHeader>
                <CardTitle className="text-base">Install Surface</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <InfoTile
                  label="Collection host"
                  value={data.audience.install.host_url || "Unavailable"}
                />
                <InfoTile
                  label="Tracker script"
                  value={data.audience.install.script_url || "Unavailable"}
                />
                <InfoTile
                  label="Tracked domains"
                  value={data.audience.install.domains.all.length.toString()}
                />
                <div className="rounded-2xl border border-white/8 bg-black/10 p-4 text-sm text-muted-foreground">
                  Use the AI prompt for framework-aware installation. Keep the
                  snippet path small and first-party, and avoid installing the
                  tracker more than once.
                </div>
              </CardContent>
            </Card>

            <div className="space-y-4">
              <CodePanel
                title="AI Install Prompt"
                description="Paste this into Claude, Codex, or ChatGPT inside the app repository."
                value={data.audience.install.ai_prompt}
                onCopy={() =>
                  copyValue(data.audience.install.ai_prompt, "AI install prompt")
                }
              />
              <CodePanel
                title="Manual Snippet"
                description="Fallback snippet if you want to wire Umami manually."
                value={data.audience.install.snippet}
                heightClassName="h-36"
                onCopy={() =>
                  copyValue(data.audience.install.snippet, "Manual snippet")
                }
              />
            </div>
          </div>
        </TabsContent>

        <TabsContent value="verify" className="mt-4">
          <div className="grid gap-4 xl:grid-cols-[minmax(0,0.72fr)_minmax(340px,1fr)]">
            <Card className="border-white/10">
              <CardHeader>
                <CardTitle className="text-base">Verification Status</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <InfoTile
                  label="Status"
                  value={meta.label}
                  valueClassName="text-foreground"
                />
                <InfoTile
                  label="Last event"
                  value={data.audience.last_event_at || "No events yet"}
                />
                <InfoTile
                  label="Verified"
                  value={data.audience.verified_at || "Not verified yet"}
                />
                {data.audience.error ? (
                  <div className="rounded-2xl border border-rose-400/25 bg-rose-400/10 px-4 py-3 text-sm text-rose-100">
                    {data.audience.error}
                  </div>
                ) : null}
                <div className="rounded-2xl border border-primary/20 bg-primary/10 px-4 py-3 text-sm text-slate-100">
                  {meta.description}
                </div>
                <Button onClick={() => verifyMutation.mutate()} disabled={verifyMutation.isPending}>
                  <RefreshCcw
                    className={cn(
                      "mr-1.5 h-3.5 w-3.5",
                      verifyMutation.isPending && "animate-spin",
                    )}
                  />
                  Verify installation
                </Button>
              </CardContent>
            </Card>

            <Card className="border-white/10">
              <CardHeader>
                <CardTitle className="text-base">Recent host coverage</CardTitle>
              </CardHeader>
              <CardContent>
                <ScrollArea className="h-72">
                  <div className="space-y-2">
                    {data.audience.install.domains.all.length ? (
                      data.audience.install.domains.all.map((domain) => (
                        <div
                          key={domain}
                          className="rounded-2xl border border-white/8 bg-black/10 px-3 py-3 text-sm text-foreground"
                        >
                          {domain}
                        </div>
                      ))
                    ) : (
                      <div className="rounded-2xl border border-dashed border-white/10 px-4 py-12 text-sm text-muted-foreground">
                        No verified domains are attached to this project yet.
                      </div>
                    )}
                  </div>
                </ScrollArea>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="events" className="mt-4">
          <div className="grid gap-4 xl:grid-cols-[minmax(0,0.78fr)_minmax(320px,1fr)]">
            <Card className="border-white/10">
              <CardHeader>
                <CardTitle className="text-base">Track Events</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4 text-sm leading-6 text-muted-foreground">
                <p>
                  Once pageviews are live, add a tiny analytics wrapper in the
                  app instead of calling Umami all over the codebase. Track only
                  meaningful product events: conversion starts, completed
                  submissions, purchases, upgrades, and activation milestones.
                </p>
                <div className="rounded-2xl border border-white/8 bg-black/10 p-4">
                  Recommended event names:
                  <ul className="mt-3 space-y-1 font-mono text-xs text-slate-100">
                    <li>`signup_started`</li>
                    <li>`signup_completed`</li>
                    <li>`checkout_started`</li>
                    <li>`checkout_completed`</li>
                    <li>`contact_form_submitted`</li>
                  </ul>
                </div>
              </CardContent>
            </Card>

            <CodePanel
              title="Event Helper Example"
              description="A small wrapper keeps event naming consistent across the app."
              value={eventHelperSnippet}
              onCopy={() => copyValue(eventHelperSnippet, "Event helper")}
            />
          </div>
        </TabsContent>
      </Tabs>
    </div>
  );
}

function InfoTile({
  label,
  value,
  valueClassName,
}: {
  label: string;
  value: string;
  valueClassName?: string;
}) {
  return (
    <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-3">
      <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">
        {label}
      </p>
      <p
        className={cn(
          "mt-2 break-all text-sm font-medium text-foreground",
          valueClassName,
        )}
      >
        {value}
      </p>
    </div>
  );
}
