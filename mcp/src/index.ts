#!/usr/bin/env node
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { buildServer } from "./server.js";
import { ConfigError } from "./config/env.js";
import { installSkill } from "./install-skill.js";
import { VERSION } from "./version.js";

const HELP = `deployik-mcp ${VERSION}

USAGE
  deployik-mcp                  Start the MCP server over stdio (default).
  deployik-mcp install-skill    Copy the bundled Deployik how-to recipes to
                                ~/.claude/skills/deployik-howto/ so Claude
                                Code surfaces them as a regular skill.
                                Pass --yes / -y to skip the confirmation.
  deployik-mcp --help, -h       Show this message.
  deployik-mcp --version, -v    Print version and exit.

ENVIRONMENT
  DEPLOYIK_URL                  Base URL of your Deployik instance
                                (e.g. https://deployik.example.com).
  DEPLOYIK_TOKEN                Personal Access Token (dpk_...). If unset,
                                falls back to ./.deployik/token if present.
  DEPLOYIK_TIMEOUT_MS           Per-request timeout in ms (default 30000).
`;

async function main(): Promise<void> {
  const argv = process.argv.slice(2);

  if (argv.includes("--help") || argv.includes("-h")) {
    process.stdout.write(HELP);
    process.exit(0);
  }
  if (argv.includes("--version") || argv.includes("-v")) {
    process.stdout.write(`${VERSION}\n`);
    process.exit(0);
  }

  const sub = argv[0];
  if (sub === "install-skill") {
    const yes = argv.includes("--yes") || argv.includes("-y");
    const code = await installSkill({ yes });
    process.exit(code);
  }
  if (sub && !sub.startsWith("-")) {
    process.stderr.write(`Unknown subcommand: ${sub}\nRun \`deployik-mcp --help\` for usage.\n`);
    process.exit(2);
  }

  try {
    const { server } = buildServer();
    const transport = new StdioServerTransport();
    await server.connect(transport);
    process.stderr.write(`deployik-mcp ${VERSION} ready (stdio)\n`);
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
