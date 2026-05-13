import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { registerTool, type ToolContext } from "./_helpers.js";
import type { Organization, PlatformInfo, User, HealthResponse } from "../client/types.js";

export function registerWorkspaceTools(server: McpServer, ctx: ToolContext): void {
  registerTool(server, ctx, {
    name: "list_workspaces",
    description: "List workspaces (organizations) the token owner can see.",
    annotations: { readOnlyHint: true },
    handler: async () => {
      const orgs = await ctx.client.request<Organization[]>(`/organizations`);
      const text = orgs
        .map(
          (o) =>
            `  • ${o.slug.padEnd(24)}  ${o.is_personal ? "personal" : "shared"}  role=${o.membership_role}  projects=${o.project_count}`,
        )
        .join("\n");
      return { text, data: orgs };
    },
  });

  registerTool(server, ctx, {
    name: "get_platform_info",
    description: "Get platform info — currently the VPS IP for DNS A-record setup of custom domains.",
    annotations: { readOnlyHint: true },
    handler: async () => {
      const info = await ctx.client.request<PlatformInfo>(`/platform`);
      return { text: `DNS target IP (point custom domains here): ${info.dns_target_ip}`, data: info };
    },
  });

  registerTool(server, ctx, {
    name: "whoami",
    description: "Return the user the current token belongs to. Use this to validate connectivity + auth on startup.",
    annotations: { readOnlyHint: true },
    handler: async () => {
      const user = await ctx.client.request<User>(`/auth/me`);
      return {
        text: `Authenticated as ${user.username} (${user.role}) — github_id=${user.github_id}, user_id=${user.id}`,
        data: user,
      };
    },
  });

  registerTool(server, ctx, {
    name: "get_health",
    description: "Public health probe — no auth required. Returns the server version block.",
    annotations: { readOnlyHint: true },
    handler: async () => {
      const health = await ctx.client.request<HealthResponse>(`/health`);
      const v = health.version;
      const versionLine = v ? `version ${v.git_sha} (built ${v.build_time})` : "(no version info)";
      return { text: `Status: ${health.status} — ${versionLine}`, data: health };
    },
  });
}
