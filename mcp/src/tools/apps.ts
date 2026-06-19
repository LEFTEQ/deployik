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
      organization_id: z.string().optional().describe("Workspace/group id. Defaults to your personal workspace when omitted."),
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

  registerTool(server, ctx, {
    name: "deploy_app",
    description: "Coordinated deploy of every member of an app bundle for one environment: ordered (by deploy_order when the app is deploy-ordered), health-gated, with best-effort rollback to the last good release on failure. Returns immediately; the rollout runs in the background.",
    inputSchema: {
      app_id: z.string(),
      environment: z.enum(["preview", "production"]).default("production"),
    },
    annotations: { title: "Deploy app" },
    handler: async (args) => {
      const res = await ctx.client.request<{ member_count: number; deploy_ordered: boolean }>(`/apps/${args.app_id}/deploy`, {
        method: "POST",
        body: { environment: args.environment },
      });
      return {
        text: `Started ${args.environment} deploy of app ${args.app_id} (${res.member_count} member(s)${res.deploy_ordered ? ", ordered" : ""}). Watch member deployments or list_app_releases for the outcome.`,
        data: res,
      };
    },
  });

  registerTool(server, ctx, {
    name: "list_app_releases",
    description: "List an app's coordinated-deploy release history for an environment (newest first). Use a release id with rollback_app.",
    inputSchema: {
      app_id: z.string(),
      environment: z.enum(["preview", "production"]).default("production"),
    },
    annotations: { readOnlyHint: true, title: "List app releases" },
    handler: async (args) => {
      const releases = await ctx.client.request<Array<{ id: string; status: string; created_at: string }>>(`/apps/${args.app_id}/releases?environment=${args.environment}`);
      const text = releases.length
        ? releases.map((r) => `${r.id} [${r.status}] ${r.created_at}`).join("\n")
        : "(no releases yet)";
      return { text, data: releases };
    },
  });

  registerTool(server, ctx, {
    name: "rollback_app",
    description: "Roll an app's members back to a prior release: redeploys each member to the exact deployment it ran in that release. Returns immediately; the rollback runs in the background.",
    inputSchema: {
      app_id: z.string(),
      release_id: z.string(),
      environment: z.enum(["preview", "production"]).default("production"),
    },
    annotations: { title: "Rollback app" },
    handler: async (args) => {
      await ctx.client.request(`/apps/${args.app_id}/rollback`, {
        method: "POST",
        body: { environment: args.environment, release_id: args.release_id },
      });
      return {
        text: `Started rollback of app ${args.app_id} (${args.environment}) to release ${args.release_id}.`,
        data: { app_id: args.app_id, release_id: args.release_id, environment: args.environment },
      };
    },
  });

  // App-level env vars / secrets. These are layered UNDERNEATH each member
  // project's own variables at deploy time (app shared → app env → project
  // shared → project env). Set once on the app, inherited by every member.
  registerAppVariableTools(server, ctx, "env");
  registerAppVariableTools(server, ctx, "secret");
}

interface AppVariable {
  key: string;
  environment: string;
  value: string;
}

// registerAppVariableTools registers list/set/delete for one app variable store.
function registerAppVariableTools(server: McpServer, ctx: ToolContext, kind: "env" | "secret"): void {
  const path = kind === "secret" ? "secrets" : "env";
  const label = kind === "secret" ? "secret" : "env var";
  const suffix = kind === "secret" ? "secret" : "env_var";

  registerTool(server, ctx, {
    name: `list_app_${suffix}s`,
    description: `List an app's ${label}s for one environment scope (values masked). These are inherited by every member project.`,
    inputSchema: {
      app_id: z.string(),
      environment: z.enum(["shared", "preview", "production"]).default("shared"),
    },
    annotations: { readOnlyHint: true, title: `List app ${label}s` },
    handler: async (args) => {
      const vars = await ctx.client.request<AppVariable[]>(`/apps/${args.app_id}/${path}?environment=${args.environment}`);
      const text = vars.length ? vars.map((v) => `${v.key}=${v.value} (${v.environment})`).join("\n") : `(no app ${label}s)`;
      return { text, data: vars };
    },
  });

  registerTool(server, ctx, {
    name: `set_app_${suffix}`,
    description: `Set an app-level ${label} (inherited by all member projects). Scope defaults to shared.`,
    inputSchema: {
      app_id: z.string(),
      key: z.string(),
      value: z.string(),
      environment: z.enum(["shared", "preview", "production"]).default("shared"),
    },
    annotations: { title: `Set app ${label}` },
    handler: async (args) => {
      await ctx.client.request(`/apps/${args.app_id}/${path}`, {
        method: "POST",
        body: { key: args.key, value: args.value, environment: args.environment },
      });
      return { text: `Set app ${label} ${args.key} (${args.environment}) on app ${args.app_id}.`, data: { key: args.key, environment: args.environment } };
    },
  });

  registerTool(server, ctx, {
    name: `delete_app_${suffix}`,
    description: `Delete an app-level ${label} from one scope.`,
    inputSchema: {
      app_id: z.string(),
      key: z.string(),
      environment: z.enum(["shared", "preview", "production"]).default("shared"),
    },
    annotations: { title: `Delete app ${label}` },
    handler: async (args) => {
      await ctx.client.request<void>(`/apps/${args.app_id}/${path}/${encodeURIComponent(args.key)}?environment=${args.environment}`, { method: "DELETE" });
      return { text: `Deleted app ${label} ${args.key} (${args.environment}).`, data: { key: args.key, environment: args.environment, deleted: true } };
    },
  });
}
