import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { registerTool, type ToolContext } from "./_helpers.js";
import { resolveProject } from "../resolve/project.js";
import type { ProjectAnalyticsPayload } from "../client/types.js";

const ENV = z.enum(["all", "preview", "production"]);
const RANGE = z.enum(["1h", "24h", "7d", "30d"]);

export function registerAnalyticsTools(server: McpServer, ctx: ToolContext): void {
  registerTool(server, ctx, {
    name: "get_project_analytics",
    description: "Get the combined Umami audience + Loki runtime analytics payload for a project.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: ENV.default("all"),
      range: RANGE.default("24h"),
      timezone: z.string().default("UTC"),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const payload = await ctx.client.request<ProjectAnalyticsPayload>(`/projects/${project.id}/analytics`, {
        query: { environment: args.environment, range: args.range, timezone: args.timezone },
      });
      return { text: JSON.stringify(payload, null, 2), data: payload };
    },
  });

  registerTool(server, ctx, {
    name: "verify_project_analytics",
    description: "Force a refresh of analytics verification status (re-checks Umami install signal).",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: ENV.default("all"),
      range: RANGE.default("24h"),
      timezone: z.string().default("UTC"),
    },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const payload = await ctx.client.request<ProjectAnalyticsPayload>(`/projects/${project.id}/analytics/verify`, {
        method: "POST",
        query: { environment: args.environment, range: args.range, timezone: args.timezone },
      });
      return { text: JSON.stringify(payload, null, 2), data: payload };
    },
  });
}
