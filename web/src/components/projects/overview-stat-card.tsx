import type { ReactNode } from "react";
import {
  Card,
  CardAction,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export interface OverviewStatCardProps {
  label: string;
  value: string;
  icon: ReactNode;
  hint: string;
}

export function OverviewStatCard({
  label,
  value,
  icon,
  hint,
}: OverviewStatCardProps) {
  return (
    <Card className="@container/card bg-gradient-to-t from-primary/5 to-card shadow-xs">
      <CardHeader>
        <CardDescription>{label}</CardDescription>
        <CardTitle className="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl">
          {value}
        </CardTitle>
        <CardAction className="text-muted-foreground">{icon}</CardAction>
      </CardHeader>
      <CardFooter className="flex-col items-start gap-1.5 text-sm">
        <p className="line-clamp-2 text-muted-foreground">{hint}</p>
      </CardFooter>
    </Card>
  );
}
