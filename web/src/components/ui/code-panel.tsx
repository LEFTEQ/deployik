import { Copy } from "lucide-react";

import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";

interface CodePanelProps {
  title: string;
  value: string;
  onCopy: () => void;
  description?: string;
  emptyLabel?: string;
  className?: string;
  heightClassName?: string;
}

export function CodePanel({
  title,
  value,
  onCopy,
  description,
  emptyLabel = "Nothing to show yet.",
  className,
  heightClassName = "h-52",
}: CodePanelProps) {
  const content = value.trim();

  return (
    <div className={cn("space-y-3", className)}>
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="text-sm font-medium text-foreground">{title}</p>
          {description ? (
            <p className="mt-1 text-xs leading-5 text-muted-foreground">
              {description}
            </p>
          ) : null}
        </div>
        <Button
          size="sm"
          variant="ghost"
          onClick={onCopy}
          className="shrink-0 text-muted-foreground hover:text-foreground"
        >
          <Copy className="mr-1.5 h-3.5 w-3.5" />
          Copy
        </Button>
      </div>

      <div className="overflow-hidden rounded-2xl border border-white/10 bg-black/30 shadow-[inset_0_1px_0_rgba(255,255,255,0.04)]">
        <ScrollArea className={heightClassName}>
          <pre className="min-h-full whitespace-pre-wrap px-4 py-3 font-mono text-[12px] leading-6 text-slate-100">
            {content || emptyLabel}
          </pre>
        </ScrollArea>
      </div>
    </div>
  );
}
