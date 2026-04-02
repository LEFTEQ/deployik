import type { ReactNode } from "react";

import {
  Card,
  CardAction,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { cn } from "@/lib/utils";

export function AnalyticsStatCard({
  label,
  value,
  hint,
  icon,
  className,
}: {
  label: string;
  value: string;
  hint?: string;
  icon?: ReactNode;
  className?: string;
}) {
  return (
    <Card
      className={cn(
        "@container/card bg-gradient-to-t from-primary/5 to-card shadow-xs",
        className,
      )}
    >
      <CardHeader>
        <CardDescription>{label}</CardDescription>
        <CardTitle className="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl">
          {value}
        </CardTitle>
        {icon ? (
          <CardAction className="text-muted-foreground">{icon}</CardAction>
        ) : null}
      </CardHeader>
      <CardFooter className="flex-col items-start gap-1.5 text-sm">
        {hint ? (
          <p className="line-clamp-2 text-muted-foreground">{hint}</p>
        ) : null}
      </CardFooter>
    </Card>
  );
}
