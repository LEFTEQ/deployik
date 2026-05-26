import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { mkdirSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";
import { DeployikClient } from "./client/http.js";
import { loadEnv } from "./config/env.js";
import { registerKnowledgePrompts } from "./knowledge/prompts.js";
import { registerAllTools } from "./tools/index.js";
import type { ServerMode, ToolContext } from "./tools/_helpers.js";
import { VERSION } from "./version.js";

export interface BuildServerOptions {
  mode?: ServerMode;
}

export interface BuildServerResult {
  server: McpServer;
  ctx: ToolContext;
}

const DAEMON_HOME = join(homedir(), ".deployik-mcp");

export function buildServer(
  env: NodeJS.ProcessEnv = process.env,
  cwd: string = process.cwd(),
  options: BuildServerOptions = {},
): BuildServerResult {
  const mode: ServerMode = options.mode ?? "stdio";
  // In http (daemon) mode the server runs detached from any repo. Pointing
  // cwd at a daemon-owned dir keeps the binding/git-remote resolvers from
  // silently misreading whatever directory launchd happened to start us in.
  const effectiveCwd = mode === "http" ? ensureDaemonHome() : cwd;
  const cfg = loadEnv(env, effectiveCwd);

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
    mode,
  };

  registerKnowledgePrompts(server);
  registerAllTools(server, ctx);

  return { server, ctx };
}

function ensureDaemonHome(): string {
  mkdirSync(DAEMON_HOME, { recursive: true });
  return DAEMON_HOME;
}
