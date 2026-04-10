import { useEffect, useRef, useState, useCallback } from "react";
import { api } from "@/lib/api";
import type { DomainLogEvent } from "@/types/api";

export type VerificationState =
  | "idle"
  | "connecting"
  | "verifying"
  | "success"
  | "error";

interface DomainVerificationResult {
  logs: DomainLogEvent[];
  state: VerificationState;
  summary: string | null;
  durationMs: number | null;
  clearLogs: () => void;
}

export function parseStream(stream: string): { step: string; status: string } {
  const [step = "", status = ""] = stream.split(":");
  return { step, status };
}

export function useDomainVerification(
  domainId: string | null,
): DomainVerificationResult {
  const [logs, setLogs] = useState<DomainLogEvent[]>([]);
  const [state, setState] = useState<VerificationState>("idle");
  const [summary, setSummary] = useState<string | null>(null);
  const [durationMs, setDurationMs] = useState<number | null>(null);
  const wsRef = useRef<WebSocket | null>(null);

  const connect = useCallback(() => {
    if (!domainId) return;

    setState("connecting");
    const url = api.getWebSocketUrl(`/domains/${domainId}/logs`);
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => setState("verifying");
    ws.onclose = () => {};
    ws.onerror = () => setState("error");

    ws.onmessage = (event) => {
      const line: DomainLogEvent = JSON.parse(event.data);
      const { step, status } = parseStream(line.stream);

      if (step === "done") {
        // Extract duration from content like "Domain verified and live (took 14200ms)"
        const durationMatch = line.content.match(/took (\d+)ms/);
        const ms =
          durationMatch && durationMatch[1]
            ? parseInt(durationMatch[1], 10)
            : null;
        setDurationMs(ms);

        if (status === "success") {
          const seconds = ms ? (ms / 1000).toFixed(0) : "?";
          setState("success");
          setSummary(`Verified in ${seconds}s`);
        } else {
          setState("error");
          setSummary(line.content);
        }

        // Close WS after receiving done
        ws.close();
      }

      setLogs((prev) => {
        if (prev.some((l) => l.line_number === line.line_number)) return prev;
        const next = [...prev, line];
        return next.length > 200 ? next.slice(-200) : next;
      });
    };

    return ws;
  }, [domainId]);

  useEffect(() => {
    if (!domainId) {
      return;
    }

    const ws = connect();
    return () => {
      ws?.close();
    };
  }, [connect, domainId]);

  const clearLogs = useCallback(() => {
    setLogs([]);
    setState("idle");
    setSummary(null);
    setDurationMs(null);
  }, []);

  return { logs, state, summary, durationMs, clearLogs };
}
