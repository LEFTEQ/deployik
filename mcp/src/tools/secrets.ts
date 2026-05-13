import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { registerTool, type ToolContext } from "./_helpers.js";
import { resolveProject } from "../resolve/project.js";
import { checkSafety } from "../lib/safety.js";
import { renderDryRun } from "../lib/format.js";
import { requireScope } from "../lib/normalize.js";
import type { SecretVariable } from "../client/types.js";

const SCOPE = z.string().describe("'shared' (both envs), 'preview' (test/staging/dev), or 'production' (live/prod). Aliases accepted.");

export function registerSecretTools(server: McpServer, ctx: ToolContext): void {
  registerTool(server, ctx, {
    name: "list_secrets",
    description: "List secrets for a project + scope. Values are masked.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: SCOPE,
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const environment = requireScope(args.environment);
      const { project } = await resolveProject(ctx, args);
      const vars = await ctx.client.request<SecretVariable[]>(`/projects/${project.id}/secrets`, {
        query: { environment },
      });
      const text =
        vars.length === 0
          ? "(no secrets in this scope)"
          : vars.map((v) => `  ${v.key} = ${v.value}`).join("\n");
      return { text, data: vars };
    },
  });

  registerTool(server, ctx, {
    name: "upsert_secret",
    description: "Set (create or update) a single secret. Use this for additive changes. `NEXT_PUBLIC_*` keys are rejected by the backend (use env vars instead).",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: SCOPE,
      key: z.string(),
      value: z.string(),
    },
    handler: async (args) => {
      const environment = requireScope(args.environment);
      const { project } = await resolveProject(ctx, args);
      await ctx.client.request(`/projects/${project.id}/secrets`, {
        method: "POST",
        body: { environment, key: args.key, value: args.value },
      });
      return { text: `Set secret ${args.key} in ${environment} scope of ${project.name}.` };
    },
  });

  registerTool(server, ctx, {
    name: "bulk_set_secrets",
    description: "REPLACE all secrets in a scope. Destructive — requires `confirm: true` (and `confirm_name` for production).",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: SCOPE,
      variables: z.array(z.object({ key: z.string(), value: z.string() })).min(0),
      confirm: z.boolean().optional(),
      confirm_name: z.string().optional(),
    },
    annotations: { destructiveHint: true },
    audit: true,
    handler: async (args) => {
      const environment = requireScope(args.environment);
      const { project } = await resolveProject(ctx, args);
      const tier = environment === "production" ? "destructive_production" : "destructive";
      const safety = checkSafety(
        {
          toolName: "bulk_set_secrets",
          tier,
          expectedName: project.name,
          impact: {
            project: project.name,
            scope: environment,
            replacing_with: args.variables.length,
            note: "Existing secrets not in this set will be DELETED.",
          },
        },
        { confirm: args.confirm, confirm_name: args.confirm_name },
      );
      if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      const res = await ctx.client.request<{ count: number }>(`/projects/${project.id}/secrets`, {
        method: "PUT",
        body: { environment, variables: args.variables },
      });
      return { text: `Replaced ${environment} secrets on ${project.name} — now ${res.count} keys.` };
    },
  });

  registerTool(server, ctx, {
    name: "delete_secret",
    description: "Delete a secret. Requires `confirm: true` (and `confirm_name` for production).",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: SCOPE,
      key: z.string(),
      confirm: z.boolean().optional(),
      confirm_name: z.string().optional(),
    },
    annotations: { destructiveHint: true },
    audit: true,
    handler: async (args) => {
      const environment = requireScope(args.environment);
      const { project } = await resolveProject(ctx, args);
      const tier = environment === "production" ? "destructive_production" : "destructive";
      const safety = checkSafety(
        {
          toolName: "delete_secret",
          tier,
          expectedName: project.name,
          impact: { project: project.name, scope: environment, key: args.key },
        },
        { confirm: args.confirm, confirm_name: args.confirm_name },
      );
      if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      await ctx.client.request(`/projects/${project.id}/secrets/${encodeURIComponent(args.key)}`, {
        method: "DELETE",
        query: { environment },
      });
      return { text: `Deleted secret ${args.key} from ${environment} scope of ${project.name}.` };
    },
  });
}
