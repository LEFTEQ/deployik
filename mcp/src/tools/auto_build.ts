import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { registerTool, type ToolContext } from "./_helpers.js";
import { resolveProject } from "../resolve/project.js";
import { checkSafety } from "../lib/safety.js";
import { renderDryRun } from "../lib/format.js";
import type { AutoBuildConfig } from "../client/types.js";

export function registerAutoBuildTools(server: McpServer, ctx: ToolContext): void {
  registerTool(server, ctx, {
    name: "get_auto_build",
    description: "Get the auto-build configuration for a project (GitHub webhook + branch matchers).",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      try {
        const cfg = await ctx.client.request<AutoBuildConfig>(`/projects/${project.id}/auto-build`);
        return {
          text: [
            `Auto-build: ${cfg.enabled ? "enabled" : "disabled"}`,
            `  Production branch:       ${cfg.production_branch}`,
            `  Preview branches:        ${cfg.preview_branches}`,
            `  Auto-deploy production:  ${cfg.auto_production_enabled ? "yes" : "no"}`,
            `  Webhook active:          ${cfg.webhook_active}`,
          ].join("\n"),
          data: cfg,
        };
      } catch (err) {
        return { text: `(no auto-build config — call configure_auto_build to create one)` };
      }
    },
  });

  registerTool(server, ctx, {
    name: "configure_auto_build",
    description:
      "Create or update auto-build config. Creates a GitHub webhook on first call. Set `enabled:false` to pause without deleting. Set `auto_production_enabled:true` to fan out pushes to the production branch into production deploys.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      enabled: z.boolean().default(true),
      production_branch: z.string().describe("Branch name to treat as production (usually 'main')."),
      preview_branches: z.string().default("*").describe("Comma-separated list of preview branches, or '*' for all."),
      auto_production_enabled: z.boolean().default(false),
    },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const cfg = await ctx.client.request<AutoBuildConfig>(`/projects/${project.id}/auto-build`, {
        method: "PUT",
        body: {
          enabled: args.enabled,
          production_branch: args.production_branch,
          preview_branches: args.preview_branches,
          auto_production_enabled: args.auto_production_enabled,
        },
      });
      return { text: `Auto-build configured for ${project.name}. Webhook active: ${cfg.webhook_active}.`, data: cfg };
    },
  });

  registerTool(server, ctx, {
    name: "delete_auto_build",
    description: "Delete the auto-build config and remove the GitHub webhook. Requires `confirm: true`.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      confirm: z.boolean().optional(),
    },
    annotations: { destructiveHint: true },
    audit: true,
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const safety = checkSafety(
        {
          toolName: "delete_auto_build",
          tier: "destructive",
          impact: { project: project.name, note: "Removes GitHub webhook and clears auto-deploy config." },
        },
        { confirm: args.confirm },
      );
      if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      await ctx.client.request(`/projects/${project.id}/auto-build`, { method: "DELETE" });
      return { text: `Auto-build deleted for ${project.name}.` };
    },
  });
}
