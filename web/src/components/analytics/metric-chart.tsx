import {
  Area,
  AreaChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

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
  return (
    <Card className="border-white/10">
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
        {description ? <CardDescription>{description}</CardDescription> : null}
      </CardHeader>
      <CardContent>
        {data.length ? (
          <div className="h-72">
            <ResponsiveContainer width="100%" height="100%">
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
                      <stop offset="5%" stopColor={item.color} stopOpacity={0.28} />
                      <stop offset="95%" stopColor={item.color} stopOpacity={0} />
                    </linearGradient>
                  ))}
                </defs>
                <CartesianGrid vertical={false} stroke="rgba(255,255,255,0.08)" />
                <XAxis
                  dataKey="label"
                  tickLine={false}
                  axisLine={false}
                  tickMargin={10}
                  minTickGap={24}
                  stroke="rgba(255,255,255,0.5)"
                />
                <YAxis
                  tickLine={false}
                  axisLine={false}
                  tickMargin={10}
                  stroke="rgba(255,255,255,0.5)"
                  tickFormatter={(value) =>
                    valueFormatter ? valueFormatter(Number(value)) : String(value)
                  }
                />
                <Tooltip
                  cursor={{ stroke: "rgba(255,255,255,0.16)", strokeWidth: 1 }}
                  contentStyle={{
                    borderRadius: 16,
                    border: "1px solid rgba(255,255,255,0.12)",
                    background: "rgba(10, 15, 27, 0.95)",
                    boxShadow: "0 24px 72px rgba(0,0,0,0.35)",
                  }}
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
                <Legend />
                {series.map((item) => (
                  <Area
                    key={item.key}
                    type="monotone"
                    dataKey={item.key}
                    name={item.label}
                    stroke={item.color}
                    fill={`url(#analytics-gradient-${item.key})`}
                    strokeWidth={2.2}
                    isAnimationActive={false}
                  />
                ))}
              </AreaChart>
            </ResponsiveContainer>
          </div>
        ) : (
          <div className="rounded-2xl border border-dashed border-white/10 px-4 py-12 text-sm text-muted-foreground">
            No chart data for the selected window yet.
          </div>
        )}
      </CardContent>
    </Card>
  );
}
