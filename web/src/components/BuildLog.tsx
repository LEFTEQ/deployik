import { useEffect, useRef } from "react";
import { cn } from "@/lib/utils";

interface LogLine {
  line_number: number;
  content: string;
  stream: "stdout" | "stderr";
}

interface BuildLogProps {
  logs: LogLine[];
  isStreaming?: boolean;
  className?: string;
}

export function BuildLog({ logs, isStreaming, className }: BuildLogProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const autoScrollRef = useRef(true);

  // Auto-scroll to bottom when new logs arrive
  useEffect(() => {
    if (autoScrollRef.current && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [logs]);

  const handleScroll = () => {
    if (!containerRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = containerRef.current;
    autoScrollRef.current = scrollHeight - scrollTop - clientHeight < 50;
  };

  return (
    <div
      ref={containerRef}
      onScroll={handleScroll}
      className={cn(
        "rounded-md bg-zinc-950 p-3 font-mono text-[11px] leading-5 overflow-y-auto md:p-4 md:text-xs",
        className,
      )}
    >
      {logs.length === 0 ? (
        <span className="text-zinc-500">
          {isStreaming ? "Waiting for logs..." : "No logs available"}
        </span>
      ) : (
        logs.map((line) => (
          <div
            key={line.line_number}
            className={cn(
              "whitespace-pre-wrap break-all",
              line.stream === "stderr" ? "text-red-400" : "text-zinc-300",
            )}
          >
            {line.content}
          </div>
        ))
      )}
      {isStreaming && (
        <span className="inline-block h-4 w-1.5 animate-pulse bg-zinc-400" />
      )}
    </div>
  );
}
