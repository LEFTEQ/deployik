import { useMemo, useState } from "react";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { format, parseISO } from "date-fns";
import {
  Activity,
  BarChart3,
  Gauge,
  Globe2,
  Radar,
  ShieldCheck,
} from "lucide-react";
import { toast } from "sonner";

import { AUDIENCE_STATUS_META } from "@/components/projects/project-analytics-meta";
import {
  AnalyticsMetricChart,
  type AnalyticsChartDatum,
} from "@/components/analytics/metric-chart";
import { AnalyticsStatCard } from "@/components/analytics/stat-card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { LoadingState, Spinner } from "@/components/ui/spinner";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { api } from "@/lib/api";
import type {
  AnalyticsBreakdownItem,
  AnalyticsEnvironmentFilter,
  AnalyticsRangePreset,
  AnalyticsTimePoint,
} from "@/types/api";

const ENVIRONMENT_OPTIONS: AnalyticsEnvironmentFilter[] = [
  "all",
  "preview",
  "production",
];

const RANGE_OPTIONS: AnalyticsRangePreset[] = ["1h", "24h", "7d", "30d"];

export function ProjectAnalyticsTab({
  projectId,
  onSetupAnalytics,
}: {
  projectId: string;
  onSetupAnalytics: () => void;
}) {
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
  const selectedDomains =
    environment === "preview"
      ? (data?.domains.preview ?? [])
      : environment === "production"
        ? (data?.domains.production ?? [])
        : (data?.domains.all ?? []);
  const readyToInstallMeta = AUDIENCE_STATUS_META.ready_to_install ?? {
    label: "Ready to install",
    badgeClass: "border-primary/25 bg-primary/12 text-primary",
    description:
      "The website exists. Add the tracker to start collecting audience data.",
  };
  const audienceStatusMeta = AUDIENCE_STATUS_META[
    data?.audience.status ?? ""
  ] ??
    AUDIENCE_STATUS_META.receiving_data ?? {
      label: "Receiving data",
      badgeClass: "border-emerald-400/25 bg-emerald-400/12 text-emerald-100",
      description: "Audience analytics is live and receiving traffic.",
    };

  return (
    <div className="space-y-4">
      <Card className="@container/card">
        <CardHeader>
          <div>
            <CardTitle className="text-base">Analytics</CardTitle>
            <CardDescription>
              Audience analytics comes from Umami. Runtime analytics comes from
              the edge proxy and Loki.
            </CardDescription>
          </div>
          <CardAction className="flex flex-col gap-2">
            <ToggleGroup
              type="single"
              value={environment}
              onValueChange={(value) =>
                value
                  ? setEnvironment(value as AnalyticsEnvironmentFilter)
                  : null
              }
              variant="outline"
              className="hidden @[720px]/card:flex"
            >
              {ENVIRONMENT_OPTIONS.map((value) => (
                <ToggleGroupItem key={value} value={value}>
                  {formatEnvironmentLabel(value)}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
            <ToggleGroup
              type="single"
              value={range}
              onValueChange={(value) =>
                value ? setRange(value as AnalyticsRangePreset) : null
              }
              variant="outline"
              className="hidden @[720px]/card:flex"
            >
              {RANGE_OPTIONS.map((value) => (
                <ToggleGroupItem key={value} value={value}>
                  {value}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
            <div className="flex flex-wrap gap-2 @[720px]/card:hidden">
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
          </CardAction>
        </CardHeader>
      </Card>

      {isLoading ? (
        <LoadingState
          title="Loading analytics…"
          description="Pulling audience metrics from Umami and runtime metrics from Loki."
          className="min-h-[360px]"
        />
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
          {environment !== "all" && selectedDomains.length === 0 ? (
            <Card className="border-white/10">
              <CardHeader>
                <CardTitle className="text-base">
                  No {formatEnvironmentLabel(environment).toLowerCase()} domains
                  yet
                </CardTitle>
                <CardDescription>
                  This filter only shows analytics for verified{" "}
                  {formatEnvironmentLabel(environment).toLowerCase()} domains.
                  Add a domain for this environment and deploy it to start
                  seeing data here.
                </CardDescription>
              </CardHeader>
            </Card>
          ) : null}

          {data.audience.status === "ready_to_install" ? (
            <Card className="@container/card overflow-hidden">
              <CardHeader>
                <div className="space-y-2">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge
                      variant="outline"
                      className={readyToInstallMeta.badgeClass}
                    >
                      {readyToInstallMeta.label}
                    </Badge>
                    <Badge
                      variant="outline"
                      className="border-white/10 bg-white/5 text-slate-200"
                    >
                      Audience analytics
                    </Badge>
                  </div>
                  <CardTitle className="text-base">
                    Set up audience analytics to start tracking visitors,
                    pageviews, and events.
                  </CardTitle>
                  <CardDescription>
                    Keep setup in the Integration tab. The analytics surface
                    stays focused on metrics once the tracker is installed.
                  </CardDescription>
                </div>
                <CardAction className="flex shrink-0 gap-2">
                  <Button onClick={onSetupAnalytics}>Setup Analytics</Button>
                  <Button
                    variant="outline"
                    onClick={() => verifyMutation.mutate()}
                    disabled={verifyMutation.isPending}
                  >
                    {verifyMutation.isPending ? (
                      <Spinner className="size-3.5" />
                    ) : null}
                    Verify
                  </Button>
                </CardAction>
              </CardHeader>
            </Card>
          ) : (
            <>
              <Card className="@container/card">
                <CardHeader>
                  <div className="space-y-2">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge
                        variant="outline"
                        className={audienceStatusMeta.badgeClass}
                      >
                        {audienceStatusMeta.label}
                      </Badge>
                      <Badge
                        variant="outline"
                        className="border-white/10 bg-white/5 font-mono text-slate-200"
                      >
                        {data.audience.website_id || "website pending"}
                      </Badge>
                    </div>
                    <CardTitle className="text-base">
                      Audience + Runtime Analytics
                    </CardTitle>
                    <CardDescription>
                      Audience analytics comes from Umami. Runtime analytics
                      comes from the edge proxy and Loki.
                    </CardDescription>
                  </div>
                  <CardAction className="flex gap-2">
                    <Button
                      variant="outline"
                      onClick={() => verifyMutation.mutate()}
                      disabled={verifyMutation.isPending}
                    >
                      {verifyMutation.isPending ? (
                        <Spinner className="size-3.5" />
                      ) : null}
                      Verify
                    </Button>
                    <Button variant="ghost" onClick={onSetupAnalytics}>
                      Integration
                    </Button>
                  </CardAction>
                </CardHeader>
              </Card>

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
                    formatter={(item) =>
                      formatNumber(item.pageviews ?? item.value)
                    }
                  />
                  <BreakdownCard
                    title="Referrers"
                    description="Where traffic is coming from."
                    items={data.audience.top_referrers}
                    formatter={(item) =>
                      formatNumber(item.visitors ?? item.value)
                    }
                  />
                  <BreakdownCard
                    title="Countries"
                    description="Geographic split of audience traffic."
                    items={data.audience.top_countries}
                    formatter={(item) =>
                      formatNumber(item.visitors ?? item.value)
                    }
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
                        value={formatBytes(
                          data.runtime.summary.bandwidth_bytes,
                        )}
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
                        value={formatDuration(
                          data.runtime.summary.p95_latency_ms,
                        )}
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
                  <Card>
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
          )}
        </>
      ) : null}
    </div>
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
    <Card>
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
                className="flex items-center justify-between gap-3 rounded-lg border bg-muted/30 px-3 py-3"
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
          <div className="rounded-xl border border-dashed border-border/70 px-4 py-10 text-sm text-muted-foreground">
            No analytics rows for this section yet.
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function mergeSeries(
  series: { key: string; points: AnalyticsTimePoint[] | null | undefined }[],
  range: AnalyticsRangePreset,
): AnalyticsChartDatum[] {
  const rows = new Map<string, AnalyticsChartDatum>();

  for (const entry of series) {
    const points = Array.isArray(entry.points) ? entry.points : [];
    for (const point of points) {
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
