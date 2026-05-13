import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { registerTool, type ToolContext } from "./_helpers.js";
import { resolveProject } from "../resolve/project.js";
import { checkSafety } from "../lib/safety.js";
import { renderDryRun } from "../lib/format.js";
import { requireScope } from "../lib/normalize.js";
import type { EnvVariable, VariableScope } from "../client/types.js";

const SCOPE = z.string().describe("'shared' (both envs), 'preview' (test/staging/dev), or 'production' (live/prod). Aliases accepted.");

export function registerEnvTools(server: McpServer, ctx: ToolContext): void {
  registerTool(server, ctx, {
    name: "list_env_vars",
    description: "List environment variables for a project + scope. Values are masked (****last4).",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: SCOPE,
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const environment = requireScope(args.environment);
      const { project } = await resolveProject(ctx, args);
      const vars = await ctx.client.request<EnvVariable[]>(`/projects/${project.id}/env`, {
        query: { environment },
      });
      const text =
        vars.length === 0
          ? "(no env vars in this scope)"
          : vars.map((v) => `  ${v.key} = ${v.value}`).join("\n");
      return { text, data: vars };
    },
  });

  registerTool(server, ctx, {
    name: "upsert_env_var",
    description: "Set (create or update) a single environment variable. Use this for additive changes.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: SCOPE,
      key: z.string().describe("Environment variable name."),
      value: z.string(),
    },
    handler: async (args) => {
      const environment = requireScope(args.environment);
      const { project } = await resolveProject(ctx, args);
      await ctx.client.request(`/projects/${project.id}/env`, {
        method: "POST",
        body: { environment, key: args.key, value: args.value },
      });
      return { text: `Set ${args.key} in ${environment} scope of ${project.name}.` };
    },
  });

  registerTool(server, ctx, {
    name: "bulk_set_env_vars",
    description:
      "REPLACE all env vars in a scope with the given set. Destructive — variables not in the set are deleted. Requires `confirm: true`.",
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
          toolName: "bulk_set_env_vars",
          tier,
          expectedName: project.name,
          impact: {
            project: project.name,
            scope: environment,
            replacing_with: args.variables.length,
            note: "Existing keys not in this set will be DELETED.",
          },
        },
        { confirm: args.confirm, confirm_name: args.confirm_name },
      );
      if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      const res = await ctx.client.request<{ count: number }>(`/projects/${project.id}/env`, {
        method: "PUT",
        body: { environment, variables: args.variables },
      });
      return { text: `Replaced ${environment} env vars on ${project.name} — now ${res.count} keys.` };
    },
  });

  registerTool(server, ctx, {
    name: "delete_env_var",
    description: "Delete an environment variable from a scope. Requires `confirm: true` (and `confirm_name` for production).",
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
          toolName: "delete_env_var",
          tier,
          expectedName: project.name,
          impact: { project: project.name, scope: environment, key: args.key },
        },
        { confirm: args.confirm, confirm_name: args.confirm_name },
      );
      if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      await ctx.client.request(`/projects/${project.id}/env/${encodeURIComponent(args.key)}`, {
        method: "DELETE",
        query: { environment },
      });
      return { text: `Deleted ${args.key} from ${environment} scope of ${project.name}.` };
    },
  });
}

export const VAR_SCOPE_SCHEMA = SCOPE;
export type VarScope = VariableScope;
