#!/usr/bin/env node
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { buildServer } from "./server.js";
import { ConfigError } from "./config/env.js";
import { installSkill } from "./install-skill.js";
import { installMcp, uninstallMcp, type InstallScope } from "./install-mcp.js";
import { installAll } from "./install.js";
import { VERSION } from "./version.js";

const HELP = `deployik-mcp ${VERSION}

USAGE
  deployik-mcp                       Start the MCP server over stdio (default).
  deployik-mcp install               Register the MCP server in Claude config
                                     AND copy the bundled skill (recommended).
  deployik-mcp install-mcp           Only register the MCP server in Claude config.
  deployik-mcp install-skill         Only copy the Deployik skill files.
  deployik-mcp uninstall             Remove the 'deployik' MCP server entry from
                                     every Claude config it lives in.
  deployik-mcp --help, -h            Show this message.
  deployik-mcp --version, -v         Print version and exit.

SCOPES (default: --global)
  --global                           Install to user-level Claude configs:
                                     • ~/.claude.json                       (Claude Code)
                                     • ~/Library/Application Support/…      (Claude Desktop, mac)
                                     • %APPDATA%/Claude/…                   (Claude Desktop, win)
                                     • ~/.config/Claude/…                   (Claude Desktop, linux)
                                     Once installed, every project + every
                                     Claude window has the deployik tools.
  --local                            Install to the current project only:
                                     • <cwd>/.mcp.json
                                     • <cwd>/.claude/skills/deployik-howto/

FLAGS
  --yes, -y                          Skip confirmation prompts (use defaults).
  --url=<url>                        Deployik URL (default: https://deployik.example.com).
  --token=<dpk_...>                  Personal Access Token. Required for install
                                     unless DEPLOYIK_TOKEN env var is set.

ENVIRONMENT
  DEPLOYIK_URL                       Used as the URL default when not passed.
  DEPLOYIK_TOKEN                     Used as the token default when not passed.
  DEPLOYIK_TIMEOUT_MS                Per-request timeout in ms (default 30000).

EXAMPLES
  npx -y @lovinka/deployik-mcp install
      Interactive global install: prompts for URL + token, writes Claude
      Code + Claude Desktop configs, copies skill files.

  npx -y @lovinka/deployik-mcp install --yes --token=dpk_xxx
      Non-interactive global install with default URL.

  npx -y @lovinka/deployik-mcp install --local --token=dpk_xxx
      Project-scoped install: writes .mcp.json + .claude/skills/ in cwd.

  npx -y @lovinka/deployik-mcp uninstall
      Removes the 'deployik' MCP server entry from every global Claude config.
`;

interface ParsedFlags {
  scope: InstallScope;
  yes: boolean;
  url?: string;
  token?: string;
}

function parseFlags(argv: string[]): ParsedFlags {
  let scope: InstallScope = "global";
  let yes = false;
  let url: string | undefined;
  let token: string | undefined;
  for (const arg of argv) {
    if (arg === "--global") scope = "global";
    else if (arg === "--local") scope = "local";
    else if (arg === "--yes" || arg === "-y") yes = true;
    else if (arg.startsWith("--url=")) url = arg.slice("--url=".length);
    else if (arg.startsWith("--token=")) token = arg.slice("--token=".length);
  }
  return { scope, yes, url, token };
}

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
  const rest = argv.slice(1);

  if (sub === "install") {
    const flags = parseFlags(rest);
    const code = await installAll(flags);
    process.exit(code);
  }
  if (sub === "install-mcp") {
    const flags = parseFlags(rest);
    const code = await installAll({ ...flags, skipSkill: true });
    process.exit(code);
  }
  if (sub === "install-skill") {
    const flags = parseFlags(rest);
    const code = await installSkill({ scope: flags.scope, yes: flags.yes });
    process.exit(code);
  }
  if (sub === "uninstall") {
    const flags = parseFlags(rest);
    const res = uninstallMcp({ scope: flags.scope });
    for (const w of res.written) process.stdout.write(`  ✓ removed from ${w.path}${w.backupPath ? ` (backup → ${w.backupPath})` : ""}\n`);
    for (const s of res.skipped) process.stdout.write(`  · ${s.path} — ${s.reason}\n`);
    if (res.written.length === 0) process.stdout.write(`Nothing removed.\n`);
    process.exit(0);
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

// `installMcp` is exported from a sibling module for programmatic use; the
// re-export here keeps the published `dist/index.js` self-contained for
// scripts that prefer requiring the entry point.
export { installMcp };

main();
