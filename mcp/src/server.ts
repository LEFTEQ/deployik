import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { DeployikClient } from "./client/http.js";
import { loadEnv } from "./config/env.js";
import { registerKnowledgePrompts } from "./knowledge/prompts.js";
import { registerAllTools } from "./tools/index.js";
import type { ToolContext } from "./tools/_helpers.js";
import { VERSION } from "./version.js";

export interface BuildServerResult {
  server: McpServer;
  ctx: ToolContext;
}

export function buildServer(env = process.env, cwd: string = process.cwd()): BuildServerResult {
  const cfg = loadEnv(env, cwd);
  const client = new DeployikClient({
    baseUrl: cfg.baseUrl,
    token: cfg.token,
    timeoutMs: cfg.timeoutMs,
    userAgent: `deployik-mcp/${VERSION}`,
  });

  const server = new McpServer(
    {
      name: "deployik",
      version: VERSION,
    },
    {
      capabilities: {
        tools: {},
        prompts: {},
      },
    },
  );

  const ctx: ToolContext = {
    client,
    stateDir: cfg.stateDir,
    cwd: cfg.cwd,
  };

  registerKnowledgePrompts(server);
  registerAllTools(server, ctx);

  return { server, ctx };
}
