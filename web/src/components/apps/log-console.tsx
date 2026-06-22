import { useEffect, useRef } from "react";

import { cn } from "@/lib/utils";
import type {
  ContainerLogLine,
  ContainerLogStatus,
} from "@/hooks/useContainerLogs";

/**
 * Best-effort severity tint for a runtime log line. `docker logs` merges
 * stdout+stderr with no per-line stream marker, so this keys off content.
 */
function lineClass(content: string): string {
  if (/\b(error|fatal|panic|exception|fail(ed|ure)?)\b/i.test(content)) {
    return "text-red-400";
  }
  if (/\bwarn(ing)?\b/i.test(content)) {
    return "text-amber-300";
  }
  return "text-zinc-300";
}

export function LogConsole({
  lines,
  status,
  paused,
  wrap,
  className,
}: {
  lines: ContainerLogLine[];
  status: ContainerLogStatus;
  paused: boolean;
  wrap: boolean;
  className?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!paused && ref.current) {
      ref.current.scrollTop = ref.current.scrollHeight;
    }
  }, [lines, paused]);

  return (
    <div
      ref={ref}
      className={cn(
        "overflow-auto bg-zinc-950 p-3 font-mono text-[11px] leading-5 md:text-xs",
        className,
      )}
    >
      {lines.length === 0 ? (
        <span className="text-zinc-500">
          {status === "connecting" || status === "open"
            ? "Waiting for logs…"
            : status === "error"
              ? "Could not connect to the container log stream."
              : "No logs."}
        </span>
      ) : (
        lines.map((line) => (
          <div
            key={line.id}
            className={cn(
              wrap ? "whitespace-pre-wrap break-all" : "whitespace-pre",
              lineClass(line.content),
            )}
          >
            {line.content || " "}
          </div>
        ))
      )}
      {status === "open" && !paused ? (
        <span className="inline-block h-4 w-1.5 animate-pulse bg-zinc-400" />
      ) : null}
    </div>
  );
}
