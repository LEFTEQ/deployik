import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { registerTool, type ToolContext } from "./_helpers.js";
import { resolveProject } from "../resolve/project.js";
import { checkSafety } from "../lib/safety.js";
import { renderDomainsList, renderDryRun } from "../lib/format.js";
import type { Domain, VerifyDomainResponse } from "../client/types.js";

export function registerDomainTools(server: McpServer, ctx: ToolContext): void {
  registerTool(server, ctx, {
    name: "list_domains",
    description: "List domains attached to a project with DNS + SSL status.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const domains = await ctx.client.request<Domain[]>(`/projects/${project.id}/domains`);
      return { text: renderDomainsList(domains), data: domains };
    },
  });

  registerTool(server, ctx, {
    name: "add_domain",
    description: "Attach a custom domain to a project environment. After adding, call `verify_domain` to verify DNS and provision SSL.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      domain: z.string(),
      environment: z.enum(["preview", "production"]),
    },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const domain = await ctx.client.request<Domain>(`/projects/${project.id}/domains`, {
        method: "POST",
        body: { domain: args.domain, environment: args.environment },
      });
      return { text: `Added ${domain.domain} to ${domain.environment} of ${project.name}.\nNext: call verify_domain to provision SSL.`, data: domain };
    },
  });

  registerTool(server, ctx, {
    name: "verify_domain",
    description: "Verify DNS for a domain and provision SSL. The call returns immediately; provisioning runs server-side and streams logs over WS.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      domain_id: z.string(),
    },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const res = await ctx.client.request<VerifyDomainResponse>(`/projects/${project.id}/domains/${args.domain_id}/verify`, {
        method: "POST",
      });
      return { text: `Verification started for domain ${args.domain_id}. Status: ${res.status}.`, data: res };
    },
  });

  registerTool(server, ctx, {
    name: "update_domain",
    description: "Update a domain: change environment (move) and/or set is_primary. Cannot move auto-generated domains.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      domain_id: z.string(),
      environment: z.enum(["preview", "production"]).optional(),
      is_primary: z.boolean().optional(),
    },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const patch: Record<string, unknown> = {};
      if (args.environment) patch.environment = args.environment;
      if (typeof args.is_primary === "boolean") patch.is_primary = args.is_primary;
      const updated = await ctx.client.request<Domain>(`/projects/${project.id}/domains/${args.domain_id}`, {
        method: "PATCH",
        body: patch,
      });
      return { text: `Updated ${updated.domain} → env=${updated.environment} primary=${updated.is_primary}`, data: updated };
    },
  });

  registerTool(server, ctx, {
    name: "delete_domain",
    description: "Delete a custom domain. Auto-generated domains cannot be deleted. Requires `confirm: true`.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      domain_id: z.string(),
      confirm: z.boolean().optional(),
    },
    annotations: { destructiveHint: true },
    audit: true,
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const safety = checkSafety(
        {
          toolName: "delete_domain",
          tier: "destructive",
          impact: { project: project.name, domain_id: args.domain_id },
        },
        { confirm: args.confirm },
      );
      if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      await ctx.client.request(`/projects/${project.id}/domains/${args.domain_id}`, { method: "DELETE" });
      return { text: `Deleted domain ${args.domain_id} from ${project.name}.` };
    },
  });
}
