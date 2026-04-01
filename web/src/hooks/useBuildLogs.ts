import { useEffect, useRef, useState, useCallback } from 'react';
import { api } from '@/lib/api';

interface LogLine {
  deployment_id: string;
  line_number: number;
  content: string;
  stream: 'stdout' | 'stderr';
}

export function useBuildLogs(deploymentId: string | null) {
  const [logs, setLogs] = useState<LogLine[]>([]);
  const [isConnected, setIsConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);

  const connect = useCallback(() => {
    if (!deploymentId) return;

    const url = api.getWebSocketUrl(`/deployments/${deploymentId}/logs`);
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => setIsConnected(true);
    ws.onclose = () => setIsConnected(false);
    ws.onerror = () => setIsConnected(false);

    ws.onmessage = (event) => {
      const line: LogLine = JSON.parse(event.data);
      setLogs((prev) => {
        // Prevent duplicates
        if (prev.some((l) => l.line_number === line.line_number)) return prev;
        // Cap at 5000 lines
        const next = [...prev, line];
        return next.length > 5000 ? next.slice(-5000) : next;
      });
    };

    return ws;
  }, [deploymentId]);

  useEffect(() => {
    const ws = connect();
    return () => {
      ws?.close();
    };
  }, [connect]);

  const clearLogs = useCallback(() => setLogs([]), []);

  return { logs, isConnected, clearLogs };
}
