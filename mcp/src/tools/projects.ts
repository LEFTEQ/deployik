import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { registerTool, type ToolContext } from "./_helpers.js";
import { resolveProject, fetchProjects } from "../resolve/project.js";
import { checkSafety } from "../lib/safety.js";
import { renderDryRun, renderProjectSummary } from "../lib/format.js";
import type { Project, CreateProjectPayload } from "../client/types.js";
import { resolveGroupId } from "../lib/groups.js";

export interface CreateProjectToolArgs {
  name: string;
  github_owner: string;
  github_repo: string;
  branch: string;
  framework: "nextjs" | "vite" | "astro" | "static" | "node-api";
  package_manager: "auto" | "bun" | "pnpm" | "npm" | "yarn";
  root_directory: string;
  output_directory: string;
  build_command: string;
  install_command: string;
  node_version: string;
  port: number;
  host_network_access?: boolean;
  data_volume_enabled?: boolean;
  data_mount_path?: string;
  group_id?: string;
  group?: string;
  organization_id?: string;
  auto_build_enabled: boolean;
  auto_production_enabled: boolean;
  resource_tier?: "nano" | "small" | "medium" | "large";
  start_command?: string;
  health_path?: string;
}

export function buildCreateProjectPayload(
  args: CreateProjectToolArgs,
  resolvedGroupId?: string,
): CreateProjectPayload {
  const payload: CreateProjectPayload = {
    name: args.name,
    github_owner: args.github_owner,
    github_repo: args.github_repo,
    branch: args.branch,
    framework: args.framework,
    package_manager: args.package_manager,
    root_directory: args.root_directory,
    output_directory: args.output_directory,
    build_command: args.build_command,
    install_command: args.install_command,
    node_version: args.node_version,
    port: args.port,
    auto_build_enabled: args.auto_build_enabled,
    auto_production_enabled: args.auto_production_enabled,
  };
  const groupId = resolvedGroupId ?? args.group_id ?? args.organization_id;
  if (groupId) payload.organization_id = groupId;
  if (args.host_network_access !== undefined) payload.host_network_access = args.host_network_access;
  if (args.data_volume_enabled !== undefined) payload.data_volume_enabled = args.data_volume_enabled;
  if (args.data_mount_path) payload.data_mount_path = args.data_mount_path;
  if (args.resource_tier) payload.resource_tier = args.resource_tier;
  if (args.start_command) payload.start_command = args.start_command;
  if (args.health_path) payload.health_path = args.health_path;
  return payload;
}

export function registerProjectTools(server: McpServer, ctx: ToolContext): void {
  registerTool(server, ctx, {
    name: "list_projects",
    description: "List Deployik projects this token can access. Optionally filter by dashboard group.",
    inputSchema: {
      group_id: z.string().optional().describe("Dashboard group id to filter by."),
      group: z.string().optional().describe("Dashboard group name, slug, or id to filter by."),
      workspace: z.string().optional().describe("Deprecated alias for group name, slug, or id."),
    },
    annotations: { readOnlyHint: true, title: "List projects" },
    handler: async (args) => {
      const groupId = await resolveGroupId(ctx.client, {
        group_id: args.group_id,
        group: args.group,
        workspace: args.workspace,
      });
      const projects = groupId
        ? await ctx.client.request<Project[]>("/projects", {
            query: { organization_id: groupId },
          })
        : await fetchProjects(ctx);
      const filtered = args.workspace && !groupId
        ? projects.filter(
            (p) =>
              (p.organization_name ?? "").toLowerCase() === args.workspace!.toLowerCase() ||
              p.organization_id === args.workspace,
          )
        : projects;
      const text =
        filtered.length === 0
          ? "(no projects visible to this token)"
          : filtered
              .map(
                (p) =>
                  `• ${p.name}  [${p.organization_name ?? p.organization_id}]  ${p.github_owner}/${p.github_repo}@${p.branch}  status=${p.status}`,
              )
              .join("\n");
      return {
        text,
        data: filtered.map((p) => ({
          id: p.id,
          name: p.name,
          group: p.organization_name ?? p.organization_id,
          workspace: p.organization_name ?? p.organization_id,
          repo: `${p.github_owner}/${p.github_repo}`,
          branch: p.branch,
          status: p.status,
          latest_deployment_status: p.latest_deployment_status,
          latest_deployment_environment: p.latest_deployment_environment,
          latest_deployment_created_at: p.latest_deployment_created_at,
        })),
      };
    },
  });

  registerTool(server, ctx, {
    name: "get_project",
    description: "Get a single project by id, slug, or the .deployik binding for this repo.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional().describe("Project slug, e.g. 'my-app'."),
      group_id: z.string().optional(),
      group: z.string().optional().describe("Dashboard group name, slug, or id."),
      workspace: z.string().optional(),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      return { text: renderProjectSummary(project), data: project };
    },
  });

  registerTool(server, ctx, {
    name: "create_project",
    description:
      "Create a new Deployik project from a GitHub repo. Supports generated Next.js/Vite/Astro/static/node-api Dockerfiles and user-provided Dockerfiles. For Dockerfile/Go/custom apps, use framework='static', set root_directory to the folder containing Dockerfile, and set port to the container listen port. Use `setup_project_from_repo` if you want JS app auto-inspection of monorepos + an initial deploy.",
    inputSchema: {
      name: z.string().describe("DNS-safe slug. Must match ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$"),
      github_owner: z.string(),
      github_repo: z.string(),
      branch: z.string().default("main"),
      framework: z
        .enum(["nextjs", "vite", "astro", "static", "node-api"])
        .default("nextjs")
        .describe("Build preset for generated Dockerfiles. For a user Dockerfile, choose 'static'; Dockerfile presence makes Deployik build it as-is."),
      package_manager: z.enum(["auto", "bun", "pnpm", "npm", "yarn"]).default("auto"),
      root_directory: z.string().default("").describe("App subdirectory. For Dockerfile apps, this must be the directory containing Dockerfile."),
      output_directory: z.string().default("").describe("Generated-Dockerfile output folder. Ignored by user-provided Dockerfiles."),
      build_command: z.string().default("").describe("Generated-Dockerfile build command. Ignored by user-provided Dockerfiles."),
      install_command: z.string().default("").describe("Generated-Dockerfile install command. Ignored by user-provided Dockerfiles."),
      node_version: z.string().default("22"),
      port: z.number().int().min(1).max(65535).default(3000).describe("Container HTTP listen port. Required for Dockerfile apps if they do not listen on 3000."),
      host_network_access: z.boolean().optional().describe("Allow runtime container to reach host services via host.docker.internal."),
      data_volume_enabled: z.boolean().optional().describe("Enable a persistent Docker volume for runtime file storage."),
      data_mount_path: z.string().optional().describe("Container mount path for the persistent volume, e.g. /data or /app/data."),
      group_id: z.string().optional().describe("Dashboard group id. Preferred over organization_id."),
      group: z.string().optional().describe("Dashboard group name, slug, or id."),
      organization_id: z.string().optional().describe("Backward-compatible alias for group_id."),
      auto_build_enabled: z.boolean().default(true),
      auto_production_enabled: z.boolean().default(false),
      resource_tier: z.enum(["nano", "small", "medium", "large"]).optional(),
      start_command: z.string().optional(),
      health_path: z.string().optional(),
    },
    annotations: { title: "Create project" },
    handler: async (args) => {
      const groupId = await resolveGroupId(ctx.client, {
        group_id: args.group_id,
        group: args.group,
        organization_id: args.organization_id,
      });
      const payload = buildCreateProjectPayload(args, groupId);
      const project = await ctx.client.request<Project>(`/projects`, { method: "POST", body: payload });
      return { text: `Created project '${project.name}' (id: ${project.id}).\n\n${renderProjectSummary(project)}`, data: project };
    },
  });

  registerTool(server, ctx, {
    name: "update_project",
    description: "Patch fields on a project. Pass any subset of mutable fields.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      patch: z
        .object({
          name: z.string().optional(),
          branch: z.string().optional(),
          framework: z.string().optional(),
          package_manager: z.string().optional(),
          root_directory: z.string().optional(),
          output_directory: z.string().optional(),
          build_command: z.string().optional(),
          install_command: z.string().optional(),
          node_version: z.string().optional(),
          port: z.number().optional(),
          host_network_access: z.boolean().optional(),
          data_volume_enabled: z.boolean().optional(),
          data_mount_path: z.string().optional(),
          start_command: z.string().optional(),
          health_path: z.string().optional(),
          resource_tier: z.enum(["nano", "small", "medium", "large"]).optional(),
        })
        .describe("Fields to update. Server validates."),
    },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const updated = await ctx.client.request<Project>(`/projects/${project.id}`, {
        method: "PATCH",
        body: args.patch,
      });
      return { text: renderProjectSummary(updated), data: updated };
    },
  });

  registerTool(server, ctx, {
    name: "delete_project",
    description:
      "Soft-delete a project (stops containers, removes nginx configs, cleans domains). Destructive — requires `confirm: true` AND `confirm_name: <project slug>`.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      confirm: z.boolean().optional(),
      confirm_name: z.string().optional(),
    },
    annotations: { destructiveHint: true },
    audit: true,
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const safety = checkSafety(
        {
          toolName: "delete_project",
          tier: "destructive_production",
          expectedName: project.name,
          impact: {
            project: project.name,
            id: project.id,
            workspace: project.organization_name ?? project.organization_id,
            group: project.organization_name ?? project.organization_id,
            note: "Will stop all containers, remove nginx configs, drop domains. Soft-deleted in DB (status='deleted').",
          },
        },
        { confirm: args.confirm, confirm_name: args.confirm_name },
      );
      if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      await ctx.client.request(`/projects/${project.id}`, { method: "DELETE" });
      return { text: `Deleted project '${project.name}' (id: ${project.id}).` };
    },
  });
}
