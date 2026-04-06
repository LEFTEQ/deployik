import { useMemo, useState } from "react";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { format, parseISO } from "date-fns";
import { ChevronDown } from "lucide-react";
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
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { LoadingState, Spinner } from "@/components/ui/spinner";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { cn } from "@/lib/utils";
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

const AUDIENCE_BREAKDOWN_TABS = ["Pages", "Referrers", "Countries"] as const;
type AudienceBreakdownTab = (typeof AUDIENCE_BREAKDOWN_TABS)[number];

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
  const [audienceOpen, setAudienceOpen] = useState(true);
  const [runtimeOpen, setRuntimeOpen] = useState(true);
  const [audienceTab, setAudienceTab] = useState<AudienceBreakdownTab>("Pages");
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

  const audienceBreakdownItems = useMemo(() => {
    if (!data) return [];
    switch (audienceTab) {
      case "Pages":
        return data.audience.top_pages;
      case "Referrers":
        return data.audience.top_referrers;
      case "Countries":
        return data.audience.top_countries;
    }
  }, [data, audienceTab]);

  const audienceBreakdownFormatter = (item: AnalyticsBreakdownItem) => {
    if (audienceTab === "Pages") return formatNumber(item.pageviews ?? item.value);
    return formatNumber(item.visitors ?? item.value);
  };

  return (
    <div className="space-y-6">
      {/* Top controls */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <ToggleGroup
          type="single"
          value={environment}
          onValueChange={(value) =>
            value ? setEnvironment(value as AnalyticsEnvironmentFilter) : null
          }
          variant="outline"
        >
          {ENVIRONMENT_OPTIONS.map((value) => (
            <ToggleGroupItem key={value} value={value}>
              {formatEnvironmentLabel(value)}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>

        <div className="flex items-center gap-2">
          <ToggleGroup
            type="single"
            value={range}
            onValueChange={(value) =>
              value ? setRange(value as AnalyticsRangePreset) : null
            }
            variant="outline"
          >
            {RANGE_OPTIONS.map((value) => (
              <ToggleGroupItem key={value} value={value}>
                {value}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>

          {data && data.audience.status !== "ready_to_install" ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => verifyMutation.mutate()}
              disabled={verifyMutation.isPending}
            >
              {verifyMutation.isPending ? (
                <Spinner className="size-3.5" />
              ) : null}
              Verify
            </Button>
          ) : null}
        </div>
      </div>

      {isLoading ? (
        <LoadingState
          title="Loading analytics..."
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
            <Card className="border-primary/20">
              <CardHeader>
                <div className="space-y-2">
                  <Badge
                    variant="outline"
                    className={readyToInstallMeta.badgeClass}
                  >
                    {readyToInstallMeta.label}
                  </Badge>
                  <CardTitle className="text-base">
                    Set up audience analytics to start tracking visitors and
                    pageviews.
                  </CardTitle>
                  <CardDescription>
                    Go to the Integration tab to install the tracking snippet.
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
              {/* Status badge row */}
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
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={onSetupAnalytics}
                  className="ml-auto"
                >
                  Integration
                </Button>
              </div>

              {/* Four key stat cards */}
              <div className="grid gap-4 grid-cols-2 md:grid-cols-4">
                <AnalyticsStatCard
                  label="Pageviews"
                  value={formatNumber(data.audience.summary.pageviews)}
                  hint="Total pageviews tracked by Umami."
                />
                <AnalyticsStatCard
                  label="Visitors"
                  value={formatNumber(data.audience.summary.visitors)}
                  hint="Unique visitors for the selected window."
                />
                <AnalyticsStatCard
                  label="Requests"
                  value={
                    data.runtime.available
                      ? formatNumber(data.runtime.summary.requests)
                      : "---"
                  }
                  hint="Total edge requests for the selected range."
                />
                <AnalyticsStatCard
                  label="Error Rate"
                  value={
                    data.runtime.available
                      ? formatPercent(data.runtime.summary.error_rate)
                      : "---"
                  }
                  hint="4xx and 5xx requests divided by total requests."
                />
              </div>

              {/* Audience section */}
              <Collapsible open={audienceOpen} onOpenChange={setAudienceOpen}>
                <CollapsibleTrigger asChild>
                  <button
                    type="button"
                    className="flex w-full items-center justify-between rounded-lg border bg-muted/30 px-4 py-3 text-sm font-medium text-foreground hover:bg-muted/50 transition-colors"
                  >
                    <span>Audience</span>
                    <ChevronDown
                      className={cn(
                        "h-4 w-4 text-muted-foreground transition-transform",
                        audienceOpen && "rotate-180",
                      )}
                    />
                  </button>
                </CollapsibleTrigger>
                <CollapsibleContent>
                  <div className="mt-4 space-y-4">
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

                    {/* Breakdown tabs */}
                    <Card>
                      <CardHeader>
                        <div className="flex items-center gap-1">
                          {AUDIENCE_BREAKDOWN_TABS.map((tab) => (
                            <Button
                              key={tab}
                              variant={audienceTab === tab ? "secondary" : "ghost"}
                              size="sm"
                              onClick={() => setAudienceTab(tab)}
                            >
                              {tab}
                            </Button>
                          ))}
                        </div>
                      </CardHeader>
                      <CardContent>
                        <BreakdownList
                          items={audienceBreakdownItems}
                          formatter={audienceBreakdownFormatter}
                        />
                      </CardContent>
                    </Card>
                  </div>
                </CollapsibleContent>
              </Collapsible>

              {/* Runtime section */}
              <Collapsible open={runtimeOpen} onOpenChange={setRuntimeOpen}>
                <CollapsibleTrigger asChild>
                  <button
                    type="button"
                    className="flex w-full items-center justify-between rounded-lg border bg-muted/30 px-4 py-3 text-sm font-medium text-foreground hover:bg-muted/50 transition-colors"
                  >
                    <span>
                      Runtime
                      {data.runtime.available ? (
                        <span className="ml-3 text-xs font-normal text-muted-foreground">
                          Bandwidth: {formatBytes(data.runtime.summary.bandwidth_bytes)}
                          {" \u00B7 "}
                          P95: {formatDuration(data.runtime.summary.p95_latency_ms)}
                        </span>
                      ) : null}
                    </span>
                    <ChevronDown
                      className={cn(
                        "h-4 w-4 text-muted-foreground transition-transform",
                        runtimeOpen && "rotate-180",
                      )}
                    />
                  </button>
                </CollapsibleTrigger>
                <CollapsibleContent>
                  <div className="mt-4 space-y-4">
                    {data.runtime.available ? (
                      <>
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

                        <Card>
                          <CardHeader>
                            <CardTitle className="text-base">
                              Status Codes
                            </CardTitle>
                          </CardHeader>
                          <CardContent>
                            <BreakdownList
                              items={data.runtime.status_codes}
                              formatter={(item) => formatNumber(item.value)}
                            />
                          </CardContent>
                        </Card>
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
                  </div>
                </CollapsibleContent>
              </Collapsible>
            </>
          )}
        </>
      ) : null}
    </div>
  );
}

function BreakdownList({
  items,
  formatter,
}: {
  items: AnalyticsBreakdownItem[];
  formatter: (item: AnalyticsBreakdownItem) => string;
}) {
  if (!items.length) {
    return (
      <div className="rounded-xl border border-dashed border-border/70 px-4 py-10 text-sm text-muted-foreground">
        No data for the selected window yet.
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {items.map((item) => (
        <div
          key={item.name}
          className="flex items-center justify-between gap-3 rounded-lg border bg-muted/30 px-3 py-2.5"
        >
          <p className="truncate text-sm font-medium text-foreground">
            {item.name || "Unknown"}
          </p>
          <p className="shrink-0 text-sm tabular-nums text-muted-foreground">
            {formatter(item)}
          </p>
        </div>
      ))}
    </div>
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
