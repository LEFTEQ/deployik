import type { BuildLog } from "../client/types.js";

export interface FormatLogsOptions {
  maxLines?: number;
  maxBytes?: number;
  /** When true, keep the last N lines instead of the first. */
  tail?: boolean;
  /** When true, anchor likely error lines with a leading marker. */
  anchorErrors?: boolean;
}

export interface FormattedLogs {
  text: string;
  totalLines: number;
  returnedLines: number;
  truncated: boolean;
  errorAnchors: number[];
  hint?: string;
}

const ERROR_PATTERNS = [
  /\berror\b/i,
  /\bfailed\b/i,
  /\bpanic\b/i,
  /\bcannot\b/i,
  /\bENOENT\b/,
  /\bcommand not found\b/i,
  /\bexit (status |code )?[1-9]/i,
];

const DEFAULTS = { maxLines: 200, maxBytes: 8 * 1024 };

export function formatLogs(logs: BuildLog[], opts: FormatLogsOptions = {}): FormattedLogs {
  const maxLines = opts.maxLines ?? DEFAULTS.maxLines;
  const maxBytes = opts.maxBytes ?? DEFAULTS.maxBytes;
  const anchor = opts.anchorErrors !== false;
  const total = logs.length;
  if (total === 0) {
    return { text: "(no log lines yet)", totalLines: 0, returnedLines: 0, truncated: false, errorAnchors: [] };
  }

  const slice = opts.tail !== false ? logs.slice(-maxLines) : logs.slice(0, maxLines);
  const truncatedLines = slice.length < total;
  const errorAnchors: number[] = [];

  const rendered: string[] = [];
  let byteCount = 0;
  for (const log of slice) {
    const prefix = log.stream === "stderr" ? "! " : "  ";
    const isError = anchor && ERROR_PATTERNS.some((rx) => rx.test(log.content));
    const marker = isError ? "↑ " : "  ";
    const line = `${prefix}${marker}${log.line_number.toString().padStart(4, " ")}  ${log.content}`;
    const lineBytes = Buffer.byteLength(line, "utf8") + 1;
    if (byteCount + lineBytes > maxBytes) break;
    rendered.push(line);
    byteCount += lineBytes;
    if (isError) errorAnchors.push(log.line_number);
  }

  const returnedLines = rendered.length;
  const truncated = truncatedLines || returnedLines < slice.length;
  const text = rendered.join("\n");
  const lastSeen = slice[slice.length - 1]?.line_number ?? 0;
  const hint = truncated
    ? `Showing ${returnedLines} of ${total} lines. Call get_deploy_logs with { since_line: ${lastSeen} } for more.`
    : undefined;

  return { text, totalLines: total, returnedLines, truncated, errorAnchors, hint };
}
