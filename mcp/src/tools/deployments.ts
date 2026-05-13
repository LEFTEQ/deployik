import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { registerTool, type ToolContext } from "./_helpers.js";
import { resolveProject } from "../resolve/project.js";
import { renderDeploymentSummary } from "../lib/format.js";
import { formatLogs } from "../lib/logs.js";
import type { Deployment, DeploymentListResponse, BuildLog } from "../client/types.js";

export function registerDeploymentTools(server: McpServer, ctx: ToolContext): void {
  registerTool(server, ctx, {
    name: "list_deployments",
    description:
      "List deployments for a project with optional filters (branch, environment, status, triggered_by, date range, pagination).",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      branch: z.string().optional(),
      environment: z.enum(["preview", "production"]).optional(),
      status: z.string().optional(),
      triggered_by: z.string().optional(),
      from: z.string().optional().describe("ISO-8601 lower bound (inclusive)."),
      to: z.string().optional().describe("ISO-8601 upper bound (inclusive)."),
      limit: z.number().int().positive().max(200).default(50),
      offset: z.number().int().min(0).default(0),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const res = await ctx.client.request<DeploymentListResponse>(`/projects/${project.id}/deployments`, {
        query: {
          branch: args.branch,
          environment: args.environment,
          status: args.status,
          triggered_by: args.triggered_by,
          from: args.from,
          to: args.to,
          limit: args.limit,
          offset: args.offset,
        },
      });
      const text =
        (res.deployments ?? []).length === 0
          ? `(no deployments matching filters; total: ${res.total ?? 0})`
          : (res.deployments ?? [])
              .map(
                (d) =>
                  `• ${d.id}  ${d.environment.padEnd(10)}  ${d.status.padEnd(10)}  ${d.branch}  ${d.commit_sha.slice(0, 7)}  ${d.created_at}`,
              )
              .join("\n");
      return { text: `Total: ${res.total ?? 0}\n${text}`, data: res };
    },
  });

  registerTool(server, ctx, {
    name: "get_deployment",
    description: "Get a deployment by id.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      deployment_id: z.string(),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const deployment = await ctx.client.request<Deployment>(
        `/projects/${project.id}/deployments/${args.deployment_id}`,
      );
      return { text: renderDeploymentSummary(deployment), data: deployment };
    },
  });

  registerTool(server, ctx, {
    name: "trigger_deployment",
    description: "Trigger a new deployment for a project. Returns the new deployment record.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: z.enum(["preview", "production"]),
      branch: z.string().optional(),
      create_tag: z.boolean().default(false),
      tag_name: z.string().optional(),
    },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const body: Record<string, unknown> = {
        environment: args.environment,
      };
      if (args.branch) body.branch = args.branch;
      if (args.create_tag) body.create_tag = true;
      if (args.tag_name) body.tag_name = args.tag_name;
      const deployment = await ctx.client.request<Deployment>(`/projects/${project.id}/deployments`, {
        method: "POST",
        body,
      });
      return { text: `Triggered ${args.environment} deploy.\n${renderDeploymentSummary(deployment)}`, data: deployment };
    },
  });

  registerTool(server, ctx, {
    name: "get_deploy_logs",
    description:
      "Get build logs for a deployment. Returns formatted lines with stderr highlighted and likely error lines anchored.",
    inputSchema: {
      deployment_id: z.string(),
      since_line: z.number().int().min(0).default(0).describe("Skip lines below this line_number."),
      max_lines: z.number().int().positive().max(2000).default(200),
      tail: z.boolean().default(true).describe("Return the last N lines (true) or the first N (false)."),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const logs = await ctx.client.request<BuildLog[]>(`/deployments/${args.deployment_id}/logs`);
      const filtered = args.since_line > 0 ? logs.filter((l) => l.line_number > args.since_line) : logs;
      const formatted = formatLogs(filtered, { maxLines: args.max_lines, tail: args.tail });
      const text = `${formatted.text}\n\n${formatted.hint ?? `(${formatted.returnedLines} lines)`}`;
      return { text, data: formatted };
    },
  });

  registerTool(server, ctx, {
    name: "get_deploy_screenshot",
    description: "Get the URL of a deployment's post-deploy screenshot PNG. Returns the URL; AI/UI can fetch it.",
    inputSchema: {
      deployment_id: z.string(),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const url = ctx.client.screenshotUrl(args.deployment_id);
      return { text: `Screenshot URL: ${url}\n(Authenticate with the same DEPLOYIK_TOKEN to fetch.)`, data: { url } };
    },
  });
}
