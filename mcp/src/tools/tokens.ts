import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { registerTool, type ToolContext } from "./_helpers.js";
import { checkSafety } from "../lib/safety.js";
import { renderDryRun } from "../lib/format.js";
import type { APIToken, CreateAPITokenResponse } from "../client/types.js";

export function registerTokenTools(server: McpServer, ctx: ToolContext): void {
  registerTool(server, ctx, {
    name: "list_my_tokens",
    description: "List Personal Access Tokens belonging to the token owner. Raw values are never returned.",
    annotations: { readOnlyHint: true },
    handler: async () => {
      const tokens = await ctx.client.request<APIToken[]>(`/me/tokens`);
      const text = tokens
        .map((t) => {
          const status = t.revoked_at ? "revoked" : t.expires_at && Date.parse(t.expires_at) < Date.now() ? "expired" : "active";
          return `  • ${t.name.padEnd(32)} ${status.padEnd(8)} last_used=${t.last_used_at ?? "(never)"}  id=${t.id}`;
        })
        .join("\n");
      return { text: text || "(no tokens)", data: tokens };
    },
  });

  registerTool(server, ctx, {
    name: "create_my_token",
    description:
      "Create a new Personal Access Token. The raw token value is returned ONCE — store it immediately, the server only keeps a hash.",
    inputSchema: {
      name: z.string().max(100),
    },
    handler: async (args) => {
      const res = await ctx.client.request<CreateAPITokenResponse>(`/me/tokens`, {
        method: "POST",
        body: { name: args.name },
      });
      return {
        text: `Created token '${res.name}' (id: ${res.id}).\n\nRaw token (copy now, will not be shown again):\n${res.token}`,
        data: res,
      };
    },
  });

  registerTool(server, ctx, {
    name: "revoke_my_token",
    description: "Revoke a Personal Access Token. Requires `confirm: true`.",
    inputSchema: {
      id: z.string(),
      confirm: z.boolean().optional(),
    },
    annotations: { destructiveHint: true },
    audit: true,
    handler: async (args) => {
      const safety = checkSafety(
        {
          toolName: "revoke_my_token",
          tier: "destructive",
          impact: { token_id: args.id, note: "Token can no longer authenticate." },
        },
        { confirm: args.confirm },
      );
      if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      await ctx.client.request(`/me/tokens/${args.id}`, { method: "DELETE" });
      return { text: `Revoked token ${args.id}.` };
    },
  });
}
