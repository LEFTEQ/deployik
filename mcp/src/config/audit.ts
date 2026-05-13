import { appendFileSync, mkdirSync } from "node:fs";
import { join } from "node:path";

export interface AuditEntry {
  ts: string;
  tool: string;
  args: Record<string, unknown>;
  httpStatus?: number;
  durationMs?: number;
  outcome: "ok" | "error" | "dry-run";
  error?: string;
}

const AUDIT_FILE = "audit.log";

const SECRET_KEYS = new Set([
  "value",
  "secret",
  "password",
  "token",
  "recaptcha_secret_key",
  "smtp_password",
  "DEPLOYIK_TOKEN",
]);

export function redact(args: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(args)) {
    if (SECRET_KEYS.has(k)) {
      if (typeof v === "string" && v.length > 0) {
        out[k] = `***${v.slice(-4)}`;
      } else if (v !== undefined) {
        out[k] = "***";
      }
      continue;
    }
    if (v && typeof v === "object" && !Array.isArray(v)) {
      out[k] = redact(v as Record<string, unknown>);
    } else {
      out[k] = v;
    }
  }
  return out;
}

export function appendAudit(stateDir: string, entry: AuditEntry): void {
  try {
    mkdirSync(stateDir, { recursive: true });
    appendFileSync(join(stateDir, AUDIT_FILE), `${JSON.stringify(entry)}\n`, "utf8");
  } catch {
    // audit is best-effort; never throw out of a tool call
  }
}
