import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { registerTool, type ToolContext } from "./_helpers.js";
import { resolveProject } from "../resolve/project.js";
import { checkSafety } from "../lib/safety.js";
import { renderDryRun } from "../lib/format.js";
import type {
  AttachServiceRequest,
  ProjectService,
  ServiceCredentials,
  ServiceType,
} from "../client/types.js";

const ENV = z.enum(["preview", "production"]);
const TYPE: z.ZodType<ServiceType> = z.enum(["postgres"]);

function renderServicesList(services: ProjectService[]): string {
  if (services.length === 0) return "(no services attached)";
  return services
    .map((s) => {
      const port = s.host_port > 0 ? `:${s.host_port}` : "(no host port)";
      return `  • [${s.environment}] ${s.type}  ${s.status}  db=${s.db_name} user=${s.db_user}  loopback=127.0.0.1${port}`;
    })
    .join("\n");
}

function renderCredentials(c: ServiceCredentials): string {
  const lines = [
    `db_name:           ${c.db_name}`,
    `db_user:           ${c.db_user}`,
    `password:          ${c.password}`,
    `internal_host:     ${c.internal_host}`,
    `internal_port:     ${c.internal_port}`,
    `vps_loopback_port: ${c.vps_loopback_port || "(not bound)"}`,
  ];
  if (c.ssh_tunnel_cmd) {
    lines.push("", `SSH tunnel:`, `  ${c.ssh_tunnel_cmd}`);
  }
  return lines.join("\n");
}

export function registerServiceTools(server: McpServer, ctx: ToolContext): void {
  registerTool(server, ctx, {
    name: "list_services",
    description:
      "List all services (sidecars) attached to a project across preview + production. v1 only supports postgres.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const services = await ctx.client.request<ProjectService[]>(`/projects/${project.id}/services`);
      return { text: renderServicesList(services), data: services };
    },
  });

  registerTool(server, ctx, {
    name: "attach_service",
    description:
      "Attach a service (v1: postgres) to one environment of a project. The container is not started immediately — the next deployment brings it up via the build pipeline's EnsureServices hook. Returns 409 if a service of the same type is already attached for the environment.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: ENV,
      type: TYPE.default("postgres"),
    },
    audit: true,
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const body: AttachServiceRequest = { environment: args.environment, type: args.type };
      const service = await ctx.client.request<ProjectService>(`/projects/${project.id}/services`, {
        method: "POST",
        body,
      });
      return {
        text: `Attached ${service.type} to ${service.environment} (id: ${service.id}). It will start on the next deployment.`,
        data: service,
      };
    },
  });

  registerTool(server, ctx, {
    name: "detach_service",
    description:
      "Detach a service: stops the container, drops the named volume, and removes the project_services row. Permanently destroys the database. Requires `confirm: true` (and `confirm_name: <project>` for production).",
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
          toolName: "detach_service",
          tier,
          expectedName: project.name,
          impact: {
            project: project.name,
            environment: args.environment,
            note: "Stops the postgres container and DELETES the named volume. All data is lost.",
          },
        },
        { confirm: args.confirm, confirm_name: args.confirm_name },
      );
      if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      await ctx.client.request(`/projects/${project.id}/services/${args.environment}`, {
        method: "DELETE",
      });
      return { text: `Detached ${args.environment} service for ${project.name}. Volume deleted.` };
    },
  });

  registerTool(server, ctx, {
    name: "get_service_credentials",
    description:
      "Reveal the postgres credentials (db_name, db_user, plaintext password, internal host/port, VPS loopback port, SSH tunnel command) for one environment. Privileged read — the server records an audit entry.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: ENV,
    },
    audit: true,
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const creds = await ctx.client.request<ServiceCredentials>(
        `/projects/${project.id}/services/${args.environment}/credentials`,
      );
      return { text: renderCredentials(creds), data: creds };
    },
  });

  registerTool(server, ctx, {
    name: "regenerate_service_password",
    description:
      "Rotate the stored postgres password and return the new credentials. The running container keeps the OLD password until the next deployment restarts it — schedule a deploy (or call restart_service) right after to apply. Requires `confirm: true` (and `confirm_name: <project>` for production).",
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
          toolName: "regenerate_service_password",
          tier,
          expectedName: project.name,
          impact: {
            project: project.name,
            environment: args.environment,
            note: "Rotates the stored password. The container keeps the old password until the next deploy/restart — apps using the secret will break until then.",
          },
        },
        { confirm: args.confirm, confirm_name: args.confirm_name },
      );
      if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      const creds = await ctx.client.request<ServiceCredentials>(
        `/projects/${project.id}/services/${args.environment}/regenerate-password`,
        { method: "POST" },
      );
      return { text: renderCredentials(creds), data: creds };
    },
  });

  registerTool(server, ctx, {
    name: "restart_service",
    description:
      "Stop and re-create the postgres container, preserving the volume. Brief downtime for any app using this database. Requires `confirm: true` (and `confirm_name: <project>` for production).",
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
          toolName: "restart_service",
          tier,
          expectedName: project.name,
          impact: {
            project: project.name,
            environment: args.environment,
            note: "Stops + recreates the postgres container. Volume (data) is preserved. Apps will see a short connection drop.",
          },
        },
        { confirm: args.confirm, confirm_name: args.confirm_name },
      );
      if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      const result = await ctx.client.request<{ status: string; host_port: number }>(
        `/projects/${project.id}/services/${args.environment}/restart`,
        { method: "POST" },
      );
      return {
        text: `Restarted ${args.environment} postgres for ${project.name} (status: ${result.status}, host_port: ${result.host_port}).`,
        data: result,
      };
    },
  });

  registerTool(server, ctx, {
    name: "reset_service",
    description:
      "WIPE the postgres volume and start a fresh empty database. ALL DATA IS LOST. The server requires a typed-confirm body matching '<project>-<environment>'. Requires `confirm: true` (and `confirm_name: <project>` for production).",
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
      // Reset is uniformly the harshest tier — it always wipes data, even on preview.
      const safety = checkSafety(
        {
          toolName: "reset_service",
          tier: "destructive_production",
          expectedName: project.name,
          impact: {
            project: project.name,
            environment: args.environment,
            note: `Drops the volume and recreates an empty postgres. ALL DATA IS LOST. Server typed-confirm string is "${project.name}-${args.environment}".`,
          },
        },
        { confirm: args.confirm, confirm_name: args.confirm_name },
      );
      if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      const result = await ctx.client.request<{ status: string; host_port: number }>(
        `/projects/${project.id}/services/${args.environment}/reset`,
        { method: "POST", body: { confirm: `${project.name}-${args.environment}` } },
      );
      return {
        text: `Reset ${args.environment} postgres for ${project.name}. Empty database is running (status: ${result.status}, host_port: ${result.host_port}).`,
        data: result,
      };
    },
  });
}
