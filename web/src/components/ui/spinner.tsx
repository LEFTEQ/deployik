import { LoaderCircle } from "lucide-react";

import { cn } from "@/lib/utils";

export function Spinner({ className }: { className?: string }) {
  return (
    <LoaderCircle
      aria-hidden="true"
      className={cn("size-4 animate-spin text-primary", className)}
    />
  );
}

export function LoadingState({
  title = "Loading…",
  description,
  className,
}: {
  title?: string;
  description?: string;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "flex min-h-[220px] flex-col items-center justify-center gap-3 rounded-xl border border-dashed border-border/70 bg-card/40 px-6 text-center",
        className,
      )}
    >
      <Spinner className="size-5" />
      <div className="space-y-1">
        <p className="text-sm font-medium text-foreground">{title}</p>
        {description ? (
          <p className="max-w-md text-sm text-muted-foreground">
            {description}
          </p>
        ) : null}
      </div>
    </div>
  );
}
