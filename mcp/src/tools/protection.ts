import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { registerTool, type ToolContext } from "./_helpers.js";
import { resolveProject } from "../resolve/project.js";
import { checkSafety } from "../lib/safety.js";
import { renderDryRun, renderProtection } from "../lib/format.js";
import type { ProtectionStatus, ProtectionUpdateResponse, BypassLinkResponse } from "../client/types.js";

const ENV = z.enum(["preview", "production"]);

export function registerProtectionTools(server: McpServer, ctx: ToolContext): void {
  registerTool(server, ctx, {
    name: "get_protection",
    description: "Check whether password protection is enabled on preview and/or production.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const status = await ctx.client.request<ProtectionStatus>(`/projects/${project.id}/protection`);
      return { text: `Protection on ${project.name}:\n${renderProtection(status)}`, data: status };
    },
  });

  registerTool(server, ctx, {
    name: "set_protection",
    description:
      "Enable or disable password protection for one environment. When enabling, returns the generated password (shown ONCE — store it).",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: ENV,
      enabled: z.boolean(),
      confirm: z.boolean().optional(),
      confirm_name: z.string().optional(),
    },
    annotations: { destructiveHint: true },
    audit: true,
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      // Disabling protection is destructive (exposes site); production gets the higher tier.
      const tier = args.environment === "production" ? "destructive_production" : args.enabled ? "mutating" : "destructive";
      if (tier !== "mutating") {
        const safety = checkSafety(
          {
            toolName: "set_protection",
            tier,
            expectedName: project.name,
            impact: { project: project.name, environment: args.environment, enabled: args.enabled },
          },
          { confirm: args.confirm, confirm_name: args.confirm_name },
        );
        if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      }
      const res = await ctx.client.request<ProtectionUpdateResponse>(`/projects/${project.id}/protection`, {
        method: "PUT",
        body: { environment: args.environment, enabled: args.enabled },
      });
      const passwordLine = res.password ? `\nPassword (shown once): ${res.password}` : "";
      const bypassLine = res.bypass_url ? `\nBypass link: ${res.bypass_url}` : "";
      return {
        text: `Protection ${res.enabled ? "enabled" : "disabled"} on ${args.environment} of ${project.name}.${passwordLine}${bypassLine}`,
        data: res,
      };
    },
  });

  registerTool(server, ctx, {
    name: "regenerate_protection_password",
    description: "Generate a fresh password for an already-protected environment. Old password stays valid only for existing cookies (24h TTL).",
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
          toolName: "regenerate_protection_password",
          tier,
          expectedName: project.name,
          impact: { project: project.name, environment: args.environment, note: "Old password keeps working only for already-issued cookies." },
        },
        { confirm: args.confirm, confirm_name: args.confirm_name },
      );
      if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      const res = await ctx.client.request<ProtectionUpdateResponse>(`/projects/${project.id}/protection/regenerate`, {
        method: "POST",
        body: { environment: args.environment },
      });
      return { text: `New password for ${args.environment} of ${project.name} (shown once):\n${res.password ?? "(none returned)"}`, data: res };
    },
  });

  registerTool(server, ctx, {
    name: "get_bypass_link",
    description:
      "Get a STABLE password-bypass link for a protected environment. Append it to the site URL " +
      "(it already includes ?_dpkbypass=...) to skip the password gate — for screenshots or a quick " +
      "preview. Treat it as a credential; rotate_bypass_link revokes it.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: ENV,
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const res = await ctx.client.request<BypassLinkResponse>(
        `/projects/${project.id}/protection/bypass-link?environment=${args.environment}`,
      );
      const line = res.url
        ? `Bypass link for ${args.environment} of ${project.name}:\n${res.url}`
        : `Bypass token for ${args.environment} of ${project.name} (no SSL-active domain yet):\n?${res.param}=${res.token}`;
      return { text: line, data: res };
    },
  });

  registerTool(server, ctx, {
    name: "rotate_bypass_link",
    description:
      "Rotate (revoke) the bypass link for an environment — every previously-shared link stops working.",
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
      const safety = checkSafety(
        {
          toolName: "rotate_bypass_link",
          tier: "destructive",
          expectedName: project.name,
          impact: { project: project.name, environment: args.environment, note: "All existing bypass links stop working." },
        },
        { confirm: args.confirm, confirm_name: args.confirm_name },
      );
      if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      const res = await ctx.client.request<BypassLinkResponse>(
        `/projects/${project.id}/protection/bypass/rotate?environment=${args.environment}`,
        { method: "POST" },
      );
      return {
        text: `Rotated bypass link for ${args.environment} of ${project.name}:\n${res.url || `?${res.param}=${res.token}`}`,
        data: res,
      };
    },
  });
}
