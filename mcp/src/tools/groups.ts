import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { registerTool, type ToolContext } from "./_helpers.js";
import { checkSafety } from "../lib/safety.js";
import { renderDryRun } from "../lib/format.js";
import {
  fetchGroups,
  renderGroupsList,
  resolveGroup,
  resolveGroupId,
} from "../lib/groups.js";
import { fetchProjects } from "../resolve/project.js";
import type { Group, Project } from "../client/types.js";

export function registerGroupTools(server: McpServer, ctx: ToolContext): void {
  registerTool(server, ctx, {
    name: "list_groups",
    description:
      "List dashboard groups this token can access. Use these ids with create_project.group_id or move_projects_to_group.",
    annotations: { readOnlyHint: true, title: "List groups" },
    handler: async () => {
      const groups = await fetchGroups(ctx.client);
      return { text: renderGroupsList(groups), data: groups };
    },
  });

  registerTool(server, ctx, {
    name: "create_group",
    description:
      "Create a dashboard group. Optionally move existing projects into it by id or project slug.",
    inputSchema: {
      name: z.string(),
      project_ids: z.array(z.string()).default([]),
      projects: z
        .array(z.string())
        .default([])
        .describe("Project slugs to move into the new group."),
    },
    annotations: { title: "Create dashboard group" },
    handler: async (args) => {
      const projectIds = await resolveProjectIds(
        ctx,
        args.project_ids ?? [],
        args.projects ?? [],
      );
      const group = await ctx.client.request<Group>("/groups", {
        method: "POST",
        body: { name: args.name, project_ids: projectIds },
      });
      return {
        text: `Created dashboard group '${group.name}' (id: ${group.id}).${
          projectIds.length ? ` Moved ${projectIds.length} project(s).` : ""
        }`,
        data: group,
      };
    },
  });

  registerTool(server, ctx, {
    name: "update_group",
    description: "Rename a dashboard group.",
    inputSchema: {
      group_id: z.string().optional(),
      group: z.string().optional().describe("Dashboard group name, slug, or id."),
      name: z.string(),
    },
    annotations: { title: "Rename dashboard group" },
    handler: async (args) => {
      const groupId = await resolveGroupId(ctx.client, args);
      if (!groupId) throw new Error("Pass group_id or group.");
      const group = await ctx.client.request<Group>(
        `/groups/${encodeURIComponent(groupId)}`,
        {
          method: "PATCH",
          body: { name: args.name },
        },
      );
      return { text: `Renamed dashboard group to '${group.name}'.`, data: group };
    },
  });

  registerTool(server, ctx, {
    name: "move_projects_to_group",
    description:
      "Move existing projects into a dashboard group. Pass project_ids or project slugs.",
    inputSchema: {
      group_id: z.string().optional(),
      group: z
        .string()
        .optional()
        .describe("Destination dashboard group name, slug, or id."),
      project_ids: z.array(z.string()).default([]),
      projects: z.array(z.string()).default([]).describe("Project slugs to move."),
    },
    annotations: { title: "Move projects to group" },
    handler: async (args) => {
      const groupId = await resolveGroupId(ctx.client, args);
      if (!groupId) throw new Error("Pass group_id or group.");
      const projectIds = await resolveProjectIds(
        ctx,
        args.project_ids ?? [],
        args.projects ?? [],
      );
      const group = await ctx.client.request<Group>(
        `/groups/${encodeURIComponent(groupId)}/projects`,
        {
          method: "PUT",
          body: { project_ids: projectIds },
        },
      );
      return {
        text: `Moved ${projectIds.length} project(s) into '${group.name}'.`,
        data: group,
      };
    },
  });

  registerTool(server, ctx, {
    name: "delete_group",
    description:
      "Delete a dashboard group. Projects are moved to the owner's default personal group. Requires confirm:true and confirm_name matching the group name.",
    inputSchema: {
      group_id: z.string().optional(),
      group: z.string().optional().describe("Dashboard group name, slug, or id."),
      confirm: z.boolean().optional(),
      confirm_name: z.string().optional(),
    },
    annotations: { destructiveHint: true, title: "Delete dashboard group" },
    audit: true,
    handler: async (args) => {
      const group = await resolveGroup(ctx.client, args);
      if (!group) throw new Error("Pass group_id or group.");
      const safety = checkSafety(
        {
          toolName: "delete_group",
          tier: "destructive",
          expectedName: group.name,
          impact: {
            group: group.name,
            id: group.id,
            note: "Deletes the dashboard group and moves its projects to the owner's default personal group.",
          },
        },
        { confirm: args.confirm, confirm_name: args.confirm_name },
      );
      if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      await ctx.client.request(`/groups/${encodeURIComponent(group.id)}`, {
        method: "DELETE",
      });
      return { text: `Deleted dashboard group '${group.name}'.` };
    },
  });
}

async function resolveProjectIds(
  ctx: ToolContext,
  explicitIds: string[],
  slugs: string[],
): Promise<string[]> {
  const ids = new Set(explicitIds.filter(Boolean));
  if (slugs.length === 0) return [...ids];

  const projects = await fetchProjects(ctx);
  for (const slug of slugs) {
    const project = findProjectBySlug(projects, slug);
    if (!project) {
      throw new Error(
        `No project named '${slug}' is visible to this token. Call list_projects to see available projects.`,
      );
    }
    ids.add(project.id);
  }
  return [...ids];
}

function findProjectBySlug(projects: Project[], slug: string): Project | undefined {
  const normalized = slug.trim().toLowerCase();
  return projects.find(
    (project) => project.id === slug || project.name.toLowerCase() === normalized,
  );
}
