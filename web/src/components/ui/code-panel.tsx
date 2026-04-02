import { Copy } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
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
    <Card className={cn("@container/card", className)}>
      <CardHeader>
        <div>
          <CardTitle className="text-base">{title}</CardTitle>
          {description ? (
            <CardDescription>{description}</CardDescription>
          ) : null}
        </div>
        <CardAction>
          <Button
            size="sm"
            variant="ghost"
            onClick={onCopy}
            className="shrink-0"
          >
            <Copy className="mr-1.5 h-3.5 w-3.5" />
            Copy
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent>
        <div className="overflow-hidden rounded-lg border bg-muted/30">
          <ScrollArea className={heightClassName}>
            <pre className="min-h-full whitespace-pre-wrap px-4 py-3 font-mono text-[12px] leading-6 text-foreground">
              {content || emptyLabel}
            </pre>
          </ScrollArea>
        </div>
      </CardContent>
    </Card>
  );
}
