import { useMemo, useState } from "react";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { format, formatDistanceToNow, parseISO } from "date-fns";
import {
  Activity,
  ArrowUpRight,
  BarChart3,
  Copy,
  Gauge,
  Globe2,
  Radar,
  RefreshCcw,
  ShieldCheck,
  Sparkles,
} from "lucide-react";
import { toast } from "sonner";

import {
  AnalyticsMetricChart,
  type AnalyticsChartDatum,
} from "@/components/analytics/metric-chart";
import { AnalyticsStatCard } from "@/components/analytics/stat-card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";
import type {
  AnalyticsBreakdownItem,
  AnalyticsEnvironmentFilter,
  AnalyticsRangePreset,
  AnalyticsTimePoint,
  ProjectAnalyticsPayload,
} from "@/types/api";

const ENVIRONMENT_OPTIONS: AnalyticsEnvironmentFilter[] = [
  "all",
  "preview",
  "production",
];

const RANGE_OPTIONS: AnalyticsRangePreset[] = ["1h", "24h", "7d", "30d"];

const AUDIENCE_STATUS_META: Record<
  string,
  { label: string; badgeClass: string; description: string }
> = {
  provisioning: {
    label: "Provisioning",
    badgeClass: "border-sky-400/25 bg-sky-400/12 text-sky-100",
    description: "Deployik is creating or syncing the linked Umami website.",
  },
  ready_to_install: {
    label: "Ready to install",
    badgeClass: "border-primary/25 bg-primary/12 text-primary",
    description:
      "The website exists. Add the tracker to start collecting audience data.",
  },
  waiting_for_data: {
    label: "Waiting for data",
    badgeClass: "border-amber-400/25 bg-amber-400/12 text-amber-100",
    description:
      "Tracking is configured, but Umami has not seen recent traffic yet.",
  },
  receiving_data: {
    label: "Receiving data",
    badgeClass: "border-emerald-400/25 bg-emerald-400/12 text-emerald-100",
    description: "Audience analytics is live and receiving traffic.",
  },
  stale: {
    label: "No recent data",
    badgeClass: "border-orange-400/25 bg-orange-400/12 text-orange-100",
    description:
      "This project has historical traffic, but nothing recent in the selected window.",
  },
  unavailable: {
    label: "Unavailable",
    badgeClass: "border-white/10 bg-white/5 text-slate-200",
    description: "Umami is not configured on this Deployik instance.",
  },
  error: {
    label: "Error",
    badgeClass: "border-rose-400/25 bg-rose-400/12 text-rose-100",
    description:
      "Deployik could not provision or query the linked analytics website.",
  },
};

export function ProjectAnalyticsTab({ projectId }: { projectId: string }) {
  const queryClient = useQueryClient();
  const [environment, setEnvironment] =
    useState<AnalyticsEnvironmentFilter>("all");
  const [range, setRange] = useState<AnalyticsRangePreset>("24h");
  const timezone =
    Intl.DateTimeFormat().resolvedOptions().timeZone?.trim() || "UTC";

  const { data, isLoading, error } = useQuery({
    queryKey: ["project-analytics", projectId, environment, range, timezone],
    queryFn: () =>
      api.getProjectAnalytics(projectId, {
        environment,
        range,
        timezone,
      }),
  });

  const verifyMutation = useMutation({
    mutationFn: () =>
      api.verifyProjectAnalytics(projectId, {
        environment,
        range,
        timezone,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["project-analytics", projectId],
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

  const audienceChartData = useMemo(
    () =>
      data
        ? mergeSeries(
            [
              { key: "pageviews", points: data.audience.series.pageviews },
              { key: "visits", points: data.audience.series.visits },
            ],
            range,
          )
        : [],
    [data, range],
  );

  const runtimeTrafficData = useMemo(
    () =>
      data
        ? mergeSeries(
            [
              { key: "requests", points: data.runtime.series.requests },
              {
                key: "apiRequests",
                points: data.runtime.series.api_requests,
              },
            ],
            range,
          )
        : [],
    [data, range],
  );

  const runtimeDeliveryData = useMemo(
    () =>
      data
        ? mergeSeries(
            [
              { key: "bandwidth", points: data.runtime.series.bandwidth },
              {
                key: "latency",
                points: data.runtime.series.p95_latency_ms,
              },
            ],
            range,
          )
        : [],
    [data, range],
  );

  return (
    <div className="space-y-4">
      <Card className="border-white/10">
        <CardHeader className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <CardTitle className="text-base">Analytics</CardTitle>
            <CardDescription>
              Audience analytics comes from Umami. Runtime analytics comes from
              the edge proxy and Loki.
            </CardDescription>
          </div>
          <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center sm:justify-end">
            <div className="flex flex-wrap gap-2">
              {ENVIRONMENT_OPTIONS.map((value) => (
                <Button
                  key={value}
                  size="sm"
                  variant={environment === value ? "default" : "outline"}
                  onClick={() => setEnvironment(value)}
                >
                  {formatEnvironmentLabel(value)}
                </Button>
              ))}
            </div>
            <div className="flex flex-wrap gap-2">
              {RANGE_OPTIONS.map((value) => (
                <Button
                  key={value}
                  size="sm"
                  variant={range === value ? "default" : "outline"}
                  onClick={() => setRange(value)}
                >
                  {value}
                </Button>
              ))}
            </div>
          </div>
        </CardHeader>
      </Card>

      {isLoading ? (
        <div className="grid gap-4">
          <Skeleton className="h-56 w-full" />
          <Skeleton className="h-64 w-full" />
          <Skeleton className="h-64 w-full" />
        </div>
      ) : error ? (
        <Card className="border-rose-400/25">
          <CardHeader>
            <CardTitle className="text-base text-rose-100">
              Analytics failed to load
            </CardTitle>
            <CardDescription>{error.message}</CardDescription>
          </CardHeader>
        </Card>
      ) : data ? (
        <>
          <AudienceInstallCard
            data={data}
            isVerifying={verifyMutation.isPending}
            onCopyPrompt={() =>
              copyValue(data.audience.install.ai_prompt, "AI install prompt")
            }
            onCopySnippet={() =>
              copyValue(data.audience.install.snippet, "Manual snippet")
            }
            onVerify={() => verifyMutation.mutate()}
          />

          <section className="space-y-4">
            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
              <AnalyticsStatCard
                label="Visitors"
                value={formatNumber(data.audience.summary.visitors)}
                hint="Unique visitors for the selected window."
                icon={<Globe2 className="h-4 w-4" />}
              />
              <AnalyticsStatCard
                label="Pageviews"
                value={formatNumber(data.audience.summary.pageviews)}
                hint="Total pageviews tracked by Umami."
                icon={<BarChart3 className="h-4 w-4" />}
              />
              <AnalyticsStatCard
                label="Visits"
                value={formatNumber(data.audience.summary.visits)}
                hint="Unique visits in the selected range."
                icon={<Activity className="h-4 w-4" />}
              />
              <AnalyticsStatCard
                label="Bounce Rate"
                value={formatPercent(data.audience.summary.bounce_rate)}
                hint="Single-page visits divided by total visits."
                icon={<ShieldCheck className="h-4 w-4" />}
              />
              <AnalyticsStatCard
                label="Avg. Visit"
                value={formatDuration(
                  data.audience.summary.avg_visit_duration_ms,
                )}
                hint="Average visit duration from Umami session totals."
                icon={<Gauge className="h-4 w-4" />}
              />
              <AnalyticsStatCard
                label="Realtime"
                value={
                  data.audience.realtime
                    ? formatNumber(data.audience.realtime.visitors)
                    : "—"
                }
                hint={
                  environment === "all"
                    ? "Active visitors during the last few minutes."
                    : "Realtime is only shown for the full project view."
                }
                icon={<Radar className="h-4 w-4" />}
              />
            </div>

            <AnalyticsMetricChart
              title="Audience Traffic"
              description="Pageviews and visits from the linked Umami website."
              data={audienceChartData}
              series={[
                {
                  key: "pageviews",
                  label: "Pageviews",
                  color: "var(--color-chart-1)",
                },
                {
                  key: "visits",
                  label: "Visits",
                  color: "var(--color-chart-2)",
                },
              ]}
              valueFormatter={(value) => formatCompactNumber(value)}
            />

            <div className="grid gap-4 xl:grid-cols-3">
              <BreakdownCard
                title="Top Pages"
                description="Most-viewed paths in the selected window."
                items={data.audience.top_pages}
                formatter={(item) => formatNumber(item.pageviews ?? item.value)}
              />
              <BreakdownCard
                title="Referrers"
                description="Where traffic is coming from."
                items={data.audience.top_referrers}
                formatter={(item) => formatNumber(item.visitors ?? item.value)}
              />
              <BreakdownCard
                title="Countries"
                description="Geographic split of audience traffic."
                items={data.audience.top_countries}
                formatter={(item) => formatNumber(item.visitors ?? item.value)}
              />
            </div>
          </section>

          <section className="space-y-4">
            {data.runtime.available ? (
              <>
                <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-5">
                  <AnalyticsStatCard
                    label="Requests"
                    value={formatNumber(data.runtime.summary.requests)}
                    hint="Total edge requests for the selected range."
                    icon={<Activity className="h-4 w-4" />}
                  />
                  <AnalyticsStatCard
                    label="API Requests"
                    value={formatNumber(data.runtime.summary.api_requests)}
                    hint="Requests matching `/api/*` at the edge."
                    icon={<BarChart3 className="h-4 w-4" />}
                  />
                  <AnalyticsStatCard
                    label="Bandwidth"
                    value={formatBytes(data.runtime.summary.bandwidth_bytes)}
                    hint="Response bytes sent by nginx."
                    icon={<Globe2 className="h-4 w-4" />}
                  />
                  <AnalyticsStatCard
                    label="Error Rate"
                    value={formatPercent(data.runtime.summary.error_rate)}
                    hint="4xx and 5xx requests divided by total requests."
                    icon={<ShieldCheck className="h-4 w-4" />}
                  />
                  <AnalyticsStatCard
                    label="P95 Latency"
                    value={formatDuration(data.runtime.summary.p95_latency_ms)}
                    hint="95th percentile request latency from edge logs."
                    icon={<Gauge className="h-4 w-4" />}
                  />
                </div>

                <div className="grid gap-4 xl:grid-cols-2">
                  <AnalyticsMetricChart
                    title="Runtime Traffic"
                    description="Edge request volume split by all requests vs API requests."
                    data={runtimeTrafficData}
                    series={[
                      {
                        key: "requests",
                        label: "Requests",
                        color: "var(--color-chart-3)",
                      },
                      {
                        key: "apiRequests",
                        label: "API Requests",
                        color: "var(--color-chart-1)",
                      },
                    ]}
                    valueFormatter={(value) => formatCompactNumber(value)}
                  />
                  <AnalyticsMetricChart
                    title="Delivery"
                    description="Bandwidth and p95 latency from the edge proxy."
                    data={runtimeDeliveryData}
                    series={[
                      {
                        key: "bandwidth",
                        label: "Bandwidth",
                        color: "var(--color-chart-4)",
                      },
                      {
                        key: "latency",
                        label: "P95 Latency (ms)",
                        color: "var(--color-chart-5)",
                      },
                    ]}
                    valueFormatter={(value) => formatCompactNumber(value)}
                  />
                </div>

                <div className="grid gap-4 xl:grid-cols-2">
                  <BreakdownCard
                    title="Top Paths"
                    description="Most requested paths at the edge."
                    items={data.runtime.top_paths}
                    formatter={(item) => formatNumber(item.value)}
                  />
                  <BreakdownCard
                    title="Status Codes"
                    description="Request distribution by HTTP status."
                    items={data.runtime.status_codes}
                    formatter={(item) => formatNumber(item.value)}
                  />
                </div>
              </>
            ) : (
              <Card className="border-white/10">
                <CardHeader>
                  <CardTitle className="text-base">
                    Runtime analytics unavailable
                  </CardTitle>
                  <CardDescription>
                    {data.runtime.error ||
                      "This Deployik instance is not connected to Loki yet."}
                  </CardDescription>
                </CardHeader>
              </Card>
            )}
          </section>
        </>
      ) : null}
    </div>
  );
}

function AudienceInstallCard({
  data,
  isVerifying,
  onCopyPrompt,
  onCopySnippet,
  onVerify,
}: {
  data: ProjectAnalyticsPayload;
  isVerifying: boolean;
  onCopyPrompt: () => void;
  onCopySnippet: () => void;
  onVerify: () => void;
}) {
  const meta = AUDIENCE_STATUS_META[data.audience.status] || {
    label: "Ready to install",
    badgeClass: "border-primary/25 bg-primary/12 text-primary",
    description:
      "The website exists. Add the tracker to start collecting audience data.",
  };
  const statusHint =
    data.audience.status === "ready_to_install"
      ? "This project existed before audience analytics was wired up. Visitor and pageview stats will appear after you install the Umami tracker in the app. Runtime traffic below is already automatic."
      : data.audience.status === "waiting_for_data"
        ? "The tracker is configured, but Umami has not seen recent traffic yet. Runtime traffic below is already automatic."
        : null;

  return (
    <Card className="border-white/10 overflow-hidden">
      <CardContent className="relative px-6 py-6">
        <div className="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-primary/40 to-transparent" />
        <div className="grid gap-6 xl:grid-cols-[minmax(0,0.92fr)_minmax(320px,1.08fr)]">
          <div className="space-y-5">
            <div className="space-y-3">
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant="outline" className={meta.badgeClass}>
                  {meta.label}
                </Badge>
                <Badge
                  variant="outline"
                  className="border-white/10 bg-white/5 text-slate-200"
                >
                  {data.audience.tracking_mode || "ai_install"}
                </Badge>
              </div>
              <div>
                <h3 className="text-lg font-semibold tracking-tight text-foreground">
                  Audience Analytics
                </h3>
                <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
                  {meta.description}
                </p>
              </div>
            </div>

            <div className="grid gap-3 sm:grid-cols-2">
              <InfoTile
                label="Website ID"
                value={data.audience.website_id || "Not provisioned yet"}
              />
              <InfoTile
                label="Collection Host"
                value={data.audience.install.host_url || "Unavailable"}
              />
              <InfoTile
                label="Tracker Script"
                value={data.audience.install.script_url || "Unavailable"}
              />
              <InfoTile
                label="Last Event"
                value={
                  data.audience.last_event_at
                    ? formatRelativeDate(data.audience.last_event_at)
                    : "No events yet"
                }
              />
              <InfoTile
                label="Verified"
                value={
                  data.audience.verified_at
                    ? formatRelativeDate(data.audience.verified_at)
                    : "Not verified yet"
                }
              />
            </div>

            {data.audience.error ? (
              <div className="rounded-2xl border border-rose-400/25 bg-rose-400/10 px-4 py-3 text-sm text-rose-100">
                {data.audience.error}
              </div>
            ) : null}

            {statusHint ? (
              <div className="rounded-2xl border border-primary/20 bg-primary/10 px-4 py-3 text-sm text-slate-200">
                {statusHint}
              </div>
            ) : null}

            <div className="flex flex-wrap gap-2">
              <Button size="sm" onClick={onCopyPrompt}>
                <Sparkles className="mr-1.5 h-3.5 w-3.5" />
                Install with AI
              </Button>
              <Button size="sm" variant="outline" onClick={onCopySnippet}>
                <Copy className="mr-1.5 h-3.5 w-3.5" />
                Copy snippet
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={onVerify}
                disabled={isVerifying}
              >
                <RefreshCcw
                  className={cn(
                    "mr-1.5 h-3.5 w-3.5",
                    isVerifying && "animate-spin",
                  )}
                />
                Verify installation
              </Button>
              {data.audience.open_url ? (
                <Button asChild size="sm" variant="ghost">
                  <a
                    href={data.audience.open_url}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    <ArrowUpRight className="mr-1.5 h-3.5 w-3.5" />
                    Open in Umami
                  </a>
                </Button>
              ) : null}
            </div>
          </div>

          <div className="grid gap-4">
            <div className="space-y-2">
              <div className="flex items-center justify-between gap-3">
                <p className="text-sm font-medium text-foreground">
                  AI Install Prompt
                </p>
                <Button size="sm" variant="ghost" onClick={onCopyPrompt}>
                  <Copy className="mr-1.5 h-3.5 w-3.5" />
                  Copy
                </Button>
              </div>
              <Textarea
                readOnly
                value={data.audience.install.ai_prompt || ""}
                className="min-h-52 resize-y font-mono text-xs leading-6"
              />
            </div>

            <div className="space-y-2">
              <div className="flex items-center justify-between gap-3">
                <p className="text-sm font-medium text-foreground">
                  Manual Snippet
                </p>
                <Button size="sm" variant="ghost" onClick={onCopySnippet}>
                  <Copy className="mr-1.5 h-3.5 w-3.5" />
                  Copy
                </Button>
              </div>
              <Textarea
                readOnly
                value={data.audience.install.snippet || ""}
                className="min-h-28 resize-y font-mono text-xs leading-6"
              />
            </div>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function BreakdownCard({
  title,
  description,
  items,
  formatter,
}: {
  title: string;
  description: string;
  items: AnalyticsBreakdownItem[];
  formatter: (item: AnalyticsBreakdownItem) => string;
}) {
  return (
    <Card className="border-white/10">
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        {items.length ? (
          <div className="space-y-2">
            {items.map((item) => (
              <div
                key={item.name}
                className="flex items-center justify-between gap-3 rounded-2xl border border-white/8 bg-black/10 px-3 py-3"
              >
                <p className="truncate text-sm font-medium text-foreground">
                  {item.name || "Unknown"}
                </p>
                <p className="text-sm text-muted-foreground">
                  {formatter(item)}
                </p>
              </div>
            ))}
          </div>
        ) : (
          <div className="rounded-2xl border border-dashed border-white/10 px-4 py-10 text-sm text-muted-foreground">
            No analytics rows for this section yet.
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function InfoTile({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-3">
      <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">
        {label}
      </p>
      <p className="mt-2 truncate text-sm font-medium text-foreground">
        {value}
      </p>
    </div>
  );
}

function mergeSeries(
  series: { key: string; points: AnalyticsTimePoint[] }[],
  range: AnalyticsRangePreset,
): AnalyticsChartDatum[] {
  const rows = new Map<string, AnalyticsChartDatum>();

  for (const entry of series) {
    for (const point of entry.points) {
      const timestamp = point.timestamp;
      const existing = rows.get(timestamp) ?? {
        label: formatPointLabel(timestamp, range),
      };
      existing[entry.key] = point.value;
      rows.set(timestamp, existing);
    }
  }

  return Array.from(rows.entries())
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([, value]) => value);
}

function formatPointLabel(timestamp: string, range: AnalyticsRangePreset) {
  const date = parseISO(timestamp);
  if (range === "1h") return format(date, "HH:mm");
  if (range === "24h") return format(date, "HH:mm");
  return format(date, "MMM d");
}

function formatEnvironmentLabel(value: AnalyticsEnvironmentFilter) {
  if (value === "preview") return "Preview";
  if (value === "production") return "Production";
  return "All";
}

function formatNumber(value: number) {
  return Math.round(value).toLocaleString();
}

function formatCompactNumber(value: number) {
  return new Intl.NumberFormat("en", {
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(value);
}

function formatPercent(value: number) {
  return `${(value * 100).toFixed(value < 0.1 ? 1 : 0)}%`;
}

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let next = value;
  let unitIndex = 0;
  while (next >= 1024 && unitIndex < units.length - 1) {
    next /= 1024;
    unitIndex += 1;
  }
  return `${next.toFixed(next >= 10 || unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
}

function formatDuration(value: number) {
  if (!Number.isFinite(value) || value <= 0) return "0s";
  const totalSeconds = Math.round(value / 1000);
  if (totalSeconds < 60) return `${totalSeconds}s`;
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  if (minutes < 60) return `${minutes}m ${seconds}s`;
  const hours = Math.floor(minutes / 60);
  return `${hours}h ${minutes % 60}m`;
}

function formatRelativeDate(value: string) {
  return formatDistanceToNow(new Date(value), { addSuffix: true });
}
