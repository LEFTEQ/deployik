import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts";

import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";

export type AnalyticsChartSeries = {
  key: string;
  label: string;
  color: string;
};

export type AnalyticsChartDatum = {
  label: string;
  [key: string]: string | number;
};

export function AnalyticsMetricChart({
  title,
  description,
  data,
  series,
  valueFormatter,
}: {
  title: string;
  description?: string;
  data: AnalyticsChartDatum[];
  series: AnalyticsChartSeries[];
  valueFormatter?: (value: number) => string;
}) {
  const config = series.reduce<ChartConfig>((acc, item) => {
    acc[item.key] = {
      label: item.label,
      color: item.color,
    };
    return acc;
  }, {});

  return (
    <Card className="@container/card min-w-0">
      <CardHeader>
        <div>
          <CardTitle className="text-base">{title}</CardTitle>
          {description ? (
            <CardDescription>{description}</CardDescription>
          ) : null}
        </div>
        <CardAction className="hidden @[720px]/card:flex">
          <ChartLegend
            content={<ChartLegendContent />}
            verticalAlign="top"
            align="right"
          />
        </CardAction>
      </CardHeader>
      <CardContent className="px-2 pb-2 pt-4 sm:px-6">
        {data.length ? (
          <ChartContainer
            config={config}
            className="aspect-auto h-[280px] w-full"
          >
            <AreaChart data={data}>
              <defs>
                {series.map((item) => (
                  <linearGradient
                    key={item.key}
                    id={`analytics-gradient-${item.key}`}
                    x1="0"
                    y1="0"
                    x2="0"
                    y2="1"
                  >
                    <stop
                      offset="5%"
                      stopColor={`var(--color-${item.key})`}
                      stopOpacity={0.28}
                    />
                    <stop
                      offset="95%"
                      stopColor={`var(--color-${item.key})`}
                      stopOpacity={0}
                    />
                  </linearGradient>
                ))}
              </defs>
              <CartesianGrid vertical={false} />
              <XAxis
                dataKey="label"
                tickLine={false}
                axisLine={false}
                tickMargin={8}
                minTickGap={28}
              />
              <YAxis
                tickLine={false}
                axisLine={false}
                tickMargin={8}
                tickFormatter={(value) =>
                  valueFormatter ? valueFormatter(Number(value)) : String(value)
                }
              />
              <ChartTooltip
                cursor={false}
                content={
                  <ChartTooltipContent
                    indicator="dot"
                    formatter={(value, name) => {
                      const numericValue = Number(value ?? 0);
                      return [
                        valueFormatter
                          ? valueFormatter(numericValue)
                          : numericValue.toLocaleString(),
                        String(name),
                      ];
                    }}
                  />
                }
              />
              <ChartLegend
                className="@[720px]/card:hidden"
                content={<ChartLegendContent />}
                verticalAlign="bottom"
              />
              {series.map((item) => (
                <Area
                  key={item.key}
                  dataKey={item.key}
                  name={item.label}
                  type="natural"
                  fill={`url(#analytics-gradient-${item.key})`}
                  stroke={`var(--color-${item.key})`}
                  strokeWidth={2}
                  isAnimationActive={false}
                />
              ))}
            </AreaChart>
          </ChartContainer>
        ) : (
          <div className="rounded-xl border border-dashed border-border/70 px-4 py-12 text-sm text-muted-foreground">
            No chart data for the selected window yet.
          </div>
        )}
      </CardContent>
    </Card>
  );
}
