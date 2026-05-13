import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { registerTool, type ToolContext } from "./_helpers.js";
import type { GitHubRepo, GitHubBranch, RepoInspection } from "../client/types.js";

export function registerGithubTools(server: McpServer, ctx: ToolContext): void {
  registerTool(server, ctx, {
    name: "list_github_repos",
    description: "List GitHub repos accessible to the token owner (via their stored OAuth token).",
    annotations: { readOnlyHint: true },
    handler: async () => {
      const repos = await ctx.client.request<GitHubRepo[]>(`/github/repos`);
      const text =
        repos.length === 0
          ? "(no repos visible — check that the GitHub OAuth token has `repo` scope)"
          : repos.map((r) => `  • ${r.full_name}  (default: ${r.default_branch})${r.private ? " [private]" : ""}`).join("\n");
      return { text, data: repos };
    },
  });

  registerTool(server, ctx, {
    name: "list_github_branches",
    description: "List branches in a GitHub repo.",
    inputSchema: {
      owner: z.string(),
      repo: z.string(),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const branches = await ctx.client.request<GitHubBranch[]>(`/github/branches`, {
        query: { owner: args.owner, repo: args.repo },
      });
      const text = branches.map((b) => `  • ${b.name}  ${b.commit.sha.slice(0, 7)}`).join("\n");
      return { text, data: branches };
    },
  });

  registerTool(server, ctx, {
    name: "inspect_github_repo",
    description:
      "Inspect a GitHub repo for monorepo structure and per-app framework/package-manager detection. Used by `setup_project_from_repo`.",
    inputSchema: {
      owner: z.string(),
      repo: z.string(),
      branch: z.string().default("main"),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const report = await ctx.client.request<RepoInspection>(
        `/github/repos/${encodeURIComponent(args.owner)}/${encodeURIComponent(args.repo)}/inspect`,
        { query: { branch: args.branch } },
      );
      const lines = [
        `Repo: ${args.owner}/${args.repo}@${args.branch}`,
        `  Monorepo:        ${report.is_monorepo ? "yes" : "no"}`,
        `  Package manager: ${report.package_manager}`,
        `  Tooling:         ${report.tooling.join(", ") || "(none)"}`,
        `  Apps:`,
      ];
      for (const app of report.apps) {
        lines.push(
          `    - ${app.name}  [${app.framework}]  path=${app.path || "/"}  out=${app.output_directory}  buildable=${app.buildable}`,
        );
      }
      if (report.truncated) lines.push("  (truncated — repo tree exceeded GitHub API limits)");
      return { text: lines.join("\n"), data: report };
    },
  });
}
