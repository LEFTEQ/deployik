import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { registerTool, type ToolContext } from "./_helpers.js";

// Minimal shapes for the API responses we render.
interface App {
  id: string;
  organization_id: string;
  name: string;
  slug: string;
  project_count: number;
}

interface AppHealth {
  app: App;
  members: Array<{
    project: { id: string; name: string; status: string };
    latest_preview_deploy_at?: string | null;
    latest_production_deploy_at?: string | null;
  }>;
}

export function registerAppTools(server: McpServer, ctx: ToolContext): void {
  registerTool(server, ctx, {
    name: "list_apps",
    description: "List app bundles (groups of projects deployed as one unit) the caller can access.",
    inputSchema: {},
    annotations: { readOnlyHint: true, title: "List apps" },
    handler: async () => {
      const apps = await ctx.client.request<App[]>("/apps");
      const text = apps.length
        ? apps.map((a) => `${a.name} (id: ${a.id}, ${a.project_count} project(s))`).join("\n")
        : "(no apps)";
      return { text, data: apps };
    },
  });

  registerTool(server, ctx, {
    name: "create_app",
    description: "Create an app bundle inside a workspace. Optionally move existing projects into it by id.",
    inputSchema: {
      name: z.string(),
      organization_id: z.string().describe("Workspace/group id the app belongs to."),
      project_ids: z.array(z.string()).default([]),
    },
    annotations: { title: "Create app bundle" },
    handler: async (args) => {
      const app = await ctx.client.request<App>("/apps", {
        method: "POST",
        body: { name: args.name, organization_id: args.organization_id, project_ids: args.project_ids ?? [] },
      });
      return {
        text: `Created app '${app.name}' (id: ${app.id}).${
          (args.project_ids ?? []).length ? ` Added ${(args.project_ids ?? []).length} project(s).` : ""
        }`,
        data: app,
      };
    },
  });

  registerTool(server, ctx, {
    name: "get_app_health",
    description: "Composite snapshot of an app: the app + its member projects with each one's latest preview/production deploy timestamps.",
    inputSchema: { app_id: z.string() },
    annotations: { readOnlyHint: true, title: "App health" },
    handler: async (args) => {
      const health = await ctx.client.request<AppHealth>(`/apps/${args.app_id}/health`);
      const lines = [
        `App: ${health.app.name} (id: ${health.app.id})`,
        ``,
        `Members:`,
        ...(health.members.length
          ? health.members.map(
              (m) =>
                `  - ${m.project.name} [${m.project.status}] preview=${m.latest_preview_deploy_at ?? "(none)"} prod=${m.latest_production_deploy_at ?? "(none)"}`,
            )
          : ["  (none)"]),
      ];
      return { text: lines.join("\n"), data: health };
    },
  });

  registerTool(server, ctx, {
    name: "update_app",
    description: "Rename an app bundle.",
    inputSchema: { app_id: z.string(), name: z.string() },
    annotations: { title: "Update app" },
    handler: async (args) => {
      const app = await ctx.client.request<App>(`/apps/${args.app_id}`, {
        method: "PATCH",
        body: { name: args.name },
      });
      return { text: `Renamed app to '${app.name}' (id: ${app.id}).`, data: app };
    },
  });

  registerTool(server, ctx, {
    name: "delete_app",
    description: "Delete an app bundle. Member projects survive (they become standalone).",
    inputSchema: { app_id: z.string() },
    annotations: { title: "Delete app" },
    handler: async (args) => {
      await ctx.client.request<void>(`/apps/${args.app_id}`, { method: "DELETE" });
      return { text: `Deleted app ${args.app_id}.`, data: { id: args.app_id, deleted: true } };
    },
  });

  registerTool(server, ctx, {
    name: "add_project_to_app",
    description: "Move one or more projects into an app bundle.",
    inputSchema: { app_id: z.string(), project_ids: z.array(z.string()) },
    annotations: { title: "Add projects to app" },
    handler: async (args) => {
      const app = await ctx.client.request<App>(`/apps/${args.app_id}/projects`, {
        method: "POST",
        body: { project_ids: args.project_ids },
      });
      return { text: `Added ${args.project_ids.length} project(s) to '${app.name}'.`, data: app };
    },
  });

  registerTool(server, ctx, {
    name: "remove_project_from_app",
    description: "Detach a project from its app bundle (it becomes standalone).",
    inputSchema: { app_id: z.string(), project_id: z.string() },
    annotations: { title: "Remove project from app" },
    handler: async (args) => {
      await ctx.client.request<void>(`/apps/${args.app_id}/projects/${args.project_id}`, { method: "DELETE" });
      return {
        text: `Removed project ${args.project_id} from app ${args.app_id}.`,
        data: { app_id: args.app_id, project_id: args.project_id, removed: true },
      };
    },
  });
}
