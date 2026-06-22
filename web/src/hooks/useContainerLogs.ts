import { useCallback, useEffect, useRef, useState } from "react";

import { api } from "@/lib/api";

export interface ContainerLogLine {
  id: number;
  content: string;
}

export type ContainerLogStatus =
  | "idle"
  | "connecting"
  | "open"
  | "closed"
  | "error";

export interface ContainerLogTarget {
  projectId: string;
  environment: "preview" | "production";
  branch?: string;
}

/**
 * Streams a running container's runtime logs over the member-logs WebSocket
 * (`/ws/projects/{id}/logs`). Frames are plain text lines (one per message),
 * unlike the build-log hook which reads JSON. Reconnects whenever the target
 * (project / environment / branch) changes.
 */
export function useContainerLogs(target: ContainerLogTarget | null) {
  const [lines, setLines] = useState<ContainerLogLine[]>([]);
  const [status, setStatus] = useState<ContainerLogStatus>("idle");
  const seqRef = useRef(0);

  const key = target
    ? `${target.projectId}|${target.environment}|${target.branch ?? ""}`
    : null;

  useEffect(() => {
    if (!target) {
      setStatus("idle");
      return;
    }

    setLines([]);
    seqRef.current = 0;
    setStatus("connecting");

    const params = new URLSearchParams({ environment: target.environment });
    if (target.branch) params.set("branch", target.branch);
    const url = api.getWebSocketUrl(
      `/projects/${target.projectId}/logs?${params.toString()}`,
    );
    const ws = new WebSocket(url);

    ws.onopen = () => setStatus("open");
    ws.onclose = () => setStatus((s) => (s === "error" ? s : "closed"));
    ws.onerror = () => setStatus("error");
    ws.onmessage = (event) => {
      const content = typeof event.data === "string" ? event.data : "";
      setLines((prev) => {
        const next = [...prev, { id: seqRef.current++, content }];
        return next.length > 5000 ? next.slice(-5000) : next;
      });
    };

    return () => ws.close();
    // key encodes the target fields; reconnect only when they change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);

  const clear = useCallback(() => setLines([]), []);

  return { lines, status, clear };
}
