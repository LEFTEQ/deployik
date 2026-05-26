import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { CallToolResult, ToolAnnotations } from "@modelcontextprotocol/sdk/types.js";
import type { ZodRawShape } from "zod";
import { DeployikClient } from "../client/http.js";
import { ApiError, asString } from "../client/errors.js";
import { appendAudit, redact, type AuditEntry } from "../config/audit.js";
import type { ResolveContext } from "../resolve/project.js";

export type ServerMode = "stdio" | "http";

export interface ToolContext extends ResolveContext {
  client: DeployikClient;
  stateDir: string;
  cwd: string;
  /**
   * Transport mode. `stdio` runs inside the user's repo (filesystem-aware
   * tools like init_in_repo / show_binding work). `http` runs as a shared
   * daemon with no per-repo context (those tools are skipped at registration).
   */
  mode: ServerMode;
}

export type Annotations = ToolAnnotations;

export interface ToolResult {
  text: string;
  data?: unknown;
  isError?: boolean;
}

export interface ToolDef<S extends ZodRawShape | undefined> {
  name: string;
  description: string;
  inputSchema?: S;
  annotations?: Annotations & { title?: string };
  audit?: boolean;
  handler: (args: ShapeOutput<S>, ctx: ToolContext) => Promise<ToolResult>;
}

type ShapeOutput<S extends ZodRawShape | undefined> = S extends ZodRawShape
  ? { [K in keyof S]: S[K] extends { _output: infer T } ? T : unknown }
  : Record<string, never>;

export function registerTool<S extends ZodRawShape | undefined>(
  server: McpServer,
  ctx: ToolContext,
  def: ToolDef<S>,
): void {
  type Cb = (rawArgs: unknown) => Promise<CallToolResult>;
  const cb: Cb = async (rawArgs) => {
    const args = (rawArgs ?? {}) as ShapeOutput<S>;
    const start = Date.now();
    try {
      const result = await def.handler(args, ctx);
      if (def.audit) {
        const entry: AuditEntry = {
          ts: new Date().toISOString(),
          tool: def.name,
          args: redact(args as Record<string, unknown>),
          durationMs: Date.now() - start,
          outcome: "ok",
        };
        appendAudit(ctx.stateDir, entry);
      }
      const out: CallToolResult = {
        content: [{ type: "text", text: result.text }],
      };
      if (result.data !== undefined) {
        // MCP requires structuredContent to be a JSON object. Wrap arrays + primitives.
        out.structuredContent =
          result.data !== null && typeof result.data === "object" && !Array.isArray(result.data)
            ? (result.data as Record<string, unknown>)
            : { value: result.data };
      }
      if (result.isError) {
        out.isError = true;
      }
      return out;
    } catch (err) {
      const message = formatError(err);
      if (def.audit) {
        appendAudit(ctx.stateDir, {
          ts: new Date().toISOString(),
          tool: def.name,
          args: redact(args as Record<string, unknown>),
          httpStatus: err instanceof ApiError ? err.status : undefined,
          durationMs: Date.now() - start,
          outcome: "error",
          error: asString(err),
        });
      }
      return {
        content: [{ type: "text", text: message }],
        isError: true,
      };
    }
  };

  const config: Record<string, unknown> = {
    description: def.description,
  };
  if (def.annotations?.title) config.title = def.annotations.title;
  if (def.inputSchema) config.inputSchema = def.inputSchema;
  const annotations: ToolAnnotations = { openWorldHint: true, ...(def.annotations ?? {}) };
  delete (annotations as Record<string, unknown>).title;
  config.annotations = annotations;

  // SDK overload variance: cast through unknown.
  (server.registerTool as unknown as (n: string, c: unknown, cb: Cb) => unknown)(def.name, config, cb);
}

export function formatError(err: unknown): string {
  if (err instanceof ApiError) {
    const lines = [`Error ${err.status || ""} calling ${err.endpoint}: ${err.message}`.trim()];
    if (err.hint) lines.push(`Hint: ${err.hint}`);
    return lines.join("\n");
  }
  if (err instanceof Error) return err.message;
  return String(err);
}

export function jsonBlock(label: string, value: unknown): string {
  return `${label}:\n${JSON.stringify(value, null, 2)}`;
}
