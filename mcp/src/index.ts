#!/usr/bin/env node
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { buildServer } from "./server.js";
import { ConfigError } from "./config/env.js";
import { installSkill } from "./install-skill.js";
import { installMcp, uninstallMcp, type InstallScope } from "./install-mcp.js";
import { installAll } from "./install.js";
import { installDaemon, uninstallDaemon, DEFAULT_DAEMON_PORT } from "./install-daemon.js";
import { startDaemon } from "./daemon.js";
import { VERSION } from "./version.js";

const HELP = `deployik-mcp ${VERSION}

USAGE
  deployik-mcp                       Start the MCP server over stdio (default).
  deployik-mcp daemon                Run the HTTP daemon in the foreground
                                     (binds 127.0.0.1:${DEFAULT_DAEMON_PORT} by default; for testing).
  deployik-mcp install               Register the MCP server in Claude config
                                     AND copy the bundled skill (recommended).
  deployik-mcp install --daemon      Install as a long-lived launchd daemon
                                     (one HTTP process, shared by every Claude
                                     window). Rewrites Claude configs to use
                                     http://127.0.0.1:${DEFAULT_DAEMON_PORT}/mcp. macOS only.
  deployik-mcp install-mcp           Only register the MCP server in Claude config.
  deployik-mcp install-skill         Only copy the Deployik skill files.
  deployik-mcp uninstall             Remove the 'deployik' MCP server entry from
                                     every Claude config it lives in.
  deployik-mcp uninstall --daemon    Stop the launchd daemon, remove the plist,
                                     and clear the HTTP entry from Claude configs.
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
  daemon: boolean;
  port?: number;
}

function parseFlags(argv: string[]): ParsedFlags {
  let scope: InstallScope = "global";
  let yes = false;
  let url: string | undefined;
  let token: string | undefined;
  let daemon = false;
  let port: number | undefined;
  for (const arg of argv) {
    if (arg === "--global") scope = "global";
    else if (arg === "--local") scope = "local";
    else if (arg === "--yes" || arg === "-y") yes = true;
    else if (arg === "--daemon") daemon = true;
    else if (arg.startsWith("--url=")) url = arg.slice("--url=".length);
    else if (arg.startsWith("--token=")) token = arg.slice("--token=".length);
    else if (arg.startsWith("--port=")) {
      const n = parseInt(arg.slice("--port=".length), 10);
      if (Number.isFinite(n) && n > 0) port = n;
    }
  }
  return { scope, yes, url, token, daemon, ...(port !== undefined ? { port } : {}) };
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

  if (sub === "daemon") {
    await startDaemon();
    return;
  }
  if (sub === "install") {
    const flags = parseFlags(rest);
    if (flags.daemon) {
      const code = installDaemonCommand(flags);
      process.exit(code);
    }
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
    if (flags.daemon) {
      uninstallDaemonCommand();
      process.exit(0);
    }
    const res = uninstallMcp({ scope: flags.scope });
    for (const w of res.written) process.stdout.write(`  ✓ removed from ${w.path}${w.backupPath ? ` (backup → ${w.backupPath})` : ""}\n`);
    for (const s of res.skipped) process.stdout.write(`  · ${s.path} — ${s.reason}\n`);
    if (res.written.length === 0) process.stdout.write(`Nothing removed.\n`);
    process.exit(0);
  }
  if (sub && !sub.startsWith("-")) {
    process.stderr.write(
      `Unknown subcommand: ${sub} (this build is @lovinka/deployik-mcp ${VERSION}).\n` +
        `Run \`deployik-mcp --help\` for usage.\n` +
        `\nIf you expected this subcommand to exist, your npx cache may be serving\n` +
        `an old version. Force a refresh with one of:\n` +
        `  npx -y @lovinka/deployik-mcp@latest ${sub}\n` +
        `  rm -rf ~/.npm/_npx && npx -y @lovinka/deployik-mcp ${sub}\n`,
    );
    process.exit(2);
  }

  try {
    const { server } = buildServer();
    const transport = new StdioServerTransport();

    // The SDK's StdioServerTransport only listens for stdin 'data' + 'error'
    // (see node_modules/@modelcontextprotocol/sdk/.../server/stdio.js). When
    // the MCP client (Claude Code, Claude Desktop) closes the stdio pipe on
    // shutdown, stdin emits 'end' but nothing exits the process — the data
    // listener keeps the event loop alive forever, and the npm exec wrapper
    // stays parked waiting on its child. Result: zombie process per closed
    // session. Wire our own exit triggers so the daemon dies cleanly.
    let shuttingDown = false;
    const shutdown = (code = 0): void => {
      if (shuttingDown) return;
      shuttingDown = true;
      void transport.close().catch(() => { /* best-effort */ });
      process.exit(code);
    };
    transport.onclose = () => shutdown(0);
    process.stdin.once("end", () => shutdown(0));
    process.stdin.once("close", () => shutdown(0));
    process.once("SIGINT", () => shutdown(0));
    process.once("SIGTERM", () => shutdown(0));
    process.once("SIGHUP", () => shutdown(0));

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

function installDaemonCommand(flags: ParsedFlags): number {
  const token = flags.token ?? process.env.DEPLOYIK_TOKEN ?? "";
  if (!token) {
    process.stderr.write(
      `deployik-mcp install --daemon: missing token.\n` +
      `  Pass --token=<dpk_...> or set DEPLOYIK_TOKEN in your environment.\n`,
    );
    return 2;
  }
  const url = flags.url ?? process.env.DEPLOYIK_URL ?? "https://deployik.example.com";
  const sourceDir = resolveMcpSourceDir();
  try {
    const result = installDaemon({
      sourceDir,
      url,
      token,
      ...(flags.port !== undefined ? { port: flags.port } : {}),
    });
    process.stdout.write(`\n  ✓ wrote plist     ${result.plistPath}\n`);
    process.stdout.write(`  ${result.loaded ? "✓" : "✗"} launchctl bootstrap${result.loaded ? "" : ` failed: ${result.loadError}`}\n`);
    process.stdout.write(`  ✓ daemon URL      ${result.url}\n`);
    for (const w of result.configsWritten) {
      process.stdout.write(`  ✓ rewrote ${w.path}${w.backupPath ? ` (backup → ${w.backupPath})` : ""}\n`);
    }
    for (const s of result.configsSkipped) {
      process.stdout.write(`  · skipped ${s.path} — ${s.reason}\n`);
    }
    process.stdout.write(`\n  Restart Claude Code to pick up the HTTP MCP entry.\n`);
    process.stdout.write(`  Logs: ~/Library/Logs/deployik-mcp.{out,err}.log\n`);
    return result.loaded ? 0 : 1;
  } catch (err) {
    process.stderr.write(`deployik-mcp install --daemon: ${(err as Error).message}\n`);
    return 1;
  }
}

function uninstallDaemonCommand(): void {
  const result = uninstallDaemon();
  process.stdout.write(`  ${result.bootoutOk ? "✓" : "·"} launchctl bootout${result.bootoutError ? ` (${result.bootoutError})` : ""}\n`);
  process.stdout.write(`  ${result.plistRemoved ? "✓ removed plist" : "· plist already absent"}\n`);
  for (const c of result.configsCleaned) {
    process.stdout.write(`  ${c.removed ? "✓" : "·"} ${c.path}${c.removed ? "" : " (no deployik entry)"}\n`);
  }
}

function resolveMcpSourceDir(): string {
  // We're running from dist/index.js — the mcp package root is one level up.
  // Works for both local builds and a published npm install
  // (node_modules/@lovinka/deployik-mcp/dist/index.js).
  const here = dirname(fileURLToPath(import.meta.url));
  return resolve(here, "..");
}

// `installMcp` is exported from a sibling module for programmatic use; the
// re-export here keeps the published `dist/index.js` self-contained for
// scripts that prefer requiring the entry point.
export { installMcp };

main();
