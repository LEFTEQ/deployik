import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { registerTool, type ToolContext } from "./_helpers.js";
import { resolveProject } from "../resolve/project.js";
import { checkSafety } from "../lib/safety.js";
import { renderDryRun, renderVolumesList } from "../lib/format.js";
import type { VolumeInfo } from "../client/types.js";

const ENV = z.enum(["preview", "production"]);

export function registerVolumeTools(server: McpServer, ctx: ToolContext): void {
  registerTool(server, ctx, {
    name: "list_volumes",
    description: "List preview + production volumes for a project with on-disk size and in_use flag.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const volumes = await ctx.client.request<VolumeInfo[]>(`/projects/${project.id}/volumes`);
      return { text: renderVolumesList(volumes), data: volumes };
    },
  });

  registerTool(server, ctx, {
    name: "delete_volume",
    description: "Delete a named volume for one environment. The volume must not be in use (returns 409 otherwise). Requires `confirm: true`.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: ENV,
      confirm: z.boolean().optional(),
      confirm_name: z.string().optional(),
    },
    annotations: { destructiveHint: true },
    audit: true,
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const tier = args.environment === "production" ? "destructive_production" : "destructive";
      const safety = checkSafety(
        {
          toolName: "delete_volume",
          tier,
          expectedName: project.name,
          impact: { project: project.name, environment: args.environment, note: "All data on this volume is lost." },
        },
        { confirm: args.confirm, confirm_name: args.confirm_name },
      );
      if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      await ctx.client.request(`/projects/${project.id}/volumes/${args.environment}`, { method: "DELETE" });
      return { text: `Deleted ${args.environment} volume for ${project.name}.` };
    },
  });

  registerTool(server, ctx, {
    name: "recreate_volume",
    description: "Drop and recreate a volume. Container must be stopped (returns 409 otherwise). Requires `confirm: true`.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: ENV,
      confirm: z.boolean().optional(),
      confirm_name: z.string().optional(),
    },
    annotations: { destructiveHint: true },
    audit: true,
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const tier = args.environment === "production" ? "destructive_production" : "destructive";
      const safety = checkSafety(
        {
          toolName: "recreate_volume",
          tier,
          expectedName: project.name,
          impact: { project: project.name, environment: args.environment, note: "Drops all data on the volume and creates a fresh empty one." },
        },
        { confirm: args.confirm, confirm_name: args.confirm_name },
      );
      if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      await ctx.client.request(`/projects/${project.id}/volumes/${args.environment}/recreate`, { method: "POST" });
      return { text: `Recreated ${args.environment} volume for ${project.name}.` };
    },
  });
}
