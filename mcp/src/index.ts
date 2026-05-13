#!/usr/bin/env node
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { buildServer } from "./server.js";
import { ConfigError } from "./config/env.js";

async function main(): Promise<void> {
  try {
    const { server } = buildServer();
    const transport = new StdioServerTransport();
    await server.connect(transport);
    process.stderr.write("deployik-mcp ready (stdio)\n");
  } catch (err) {
    if (err instanceof ConfigError) {
      process.stderr.write(`deployik-mcp: ${err.message}\n`);
      process.exit(2);
    }
    process.stderr.write(`deployik-mcp: ${(err as Error).message}\n`);
    process.exit(1);
  }
}

main();
