// Install a long-lived deployik-mcp daemon via macOS launchd.
//
// The daemon (src/daemon.ts) is one HTTP process on 127.0.0.1, shared by
// every Claude Code window. This module owns the install/uninstall side:
//
//   1. Write a launchd plist (~/Library/LaunchAgents/com.lovinka.deployik-mcp.plist)
//      that runs `node <absolute path>/dist/daemon.js` with KeepAlive=true,
//      RunAtLoad=true, and DEPLOYIK_* in EnvironmentVariables.
//   2. Bootstrap it via `launchctl bootout` (clear any old version) +
//      `launchctl bootstrap gui/$UID <plist>`.
//   3. Rewrite Claude configs: replace the old stdio entry
//      ({ command: "npx", args: [...], env: {...} }) with
//      ({ type: "http", url: "http://127.0.0.1:8788/mcp" }).
//   4. Uninstall reverses all three.
//
// Token lives only in the plist's <EnvironmentVariables>. Plist file is
// chmod 0600 — readable by the current user only. Other users on the box
// would need root to read it.

import { execFileSync } from "node:child_process";
import {
  chmodSync,
  copyFileSync,
  cpSync,
  existsSync,
  mkdirSync,
  readFileSync,
  rmSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { homedir, platform } from "node:os";
import { dirname, join, resolve } from "node:path";

export const DAEMON_LABEL = "com.lovinka.deployik-mcp";
export const DEFAULT_DAEMON_PORT = 8788;
export const DEFAULT_URL = "https://deployik.example.com";

const MCP_NAME = "deployik";

export interface DaemonInstallOpts {
  /**
   * Source directory containing the `mcp` package layout: `dist/`,
   * `node_modules/`, `package.json`. The installer stages a copy of these
   * three into `~/.deployik-mcp/runtime/` so the launchd-spawned process
   * doesn't have to read from a TCC-protected location (~/Documents, etc.).
   */
  sourceDir: string;
  /** Absolute path to node. Defaults to the node that's running this code. */
  nodePath?: string;
  url: string;
  token: string;
  port?: number;
}

/** Where the staged daemon runtime lives — outside any TCC-protected folder. */
function runtimeDir(): string {
  return join(homedir(), ".deployik-mcp", "runtime");
}

/**
 * Locate the `node_modules` directory that contains our dependencies.
 *
 * Two real layouts to support:
 *
 *   Local dev:  sourceDir = .../mcp                       → node_modules in sourceDir
 *   npx cache:  sourceDir = ~/.npm/_npx/<hash>/node_modules/@lovinka/deployik-mcp
 *               → node_modules is the grandparent (two levels up)
 *
 * We probe for `@modelcontextprotocol/sdk` as the existence sentinel — it's a
 * required dep, so wherever it lives is where Node will resolve our imports.
 */
function findNodeModulesRoot(sourceDir: string): string {
  const sentinel = join("node_modules", "@modelcontextprotocol", "sdk");
  const candidates = [
    sourceDir,                            // local dev: mcp/
    resolve(sourceDir, "..", ".."),       // npx: node_modules/@lovinka/deployik-mcp → up to npx root
    resolve(sourceDir, "..", "..", ".."), // scoped install under another package's node_modules
  ];
  for (const dir of candidates) {
    if (existsSync(join(dir, sentinel))) return dir;
  }
  throw new Error(`Cannot locate node_modules containing @modelcontextprotocol/sdk relative to ${sourceDir}. Tried: ${candidates.join(", ")}`);
}

/**
 * Stage the runtime into ~/.deployik-mcp/runtime/ — outside any TCC-protected
 * folder so launchd-spawned processes can read it on every macOS. Idempotent:
 * each call wipes the previous runtime and writes fresh. Returns the absolute
 * path to the staged daemon entry script.
 *
 * Layout written: runtime/{dist, node_modules, package.json}. Both layouts
 * (local dev and published-via-npx) end up identical after staging, so the
 * plist always points at runtime/dist/daemon.js.
 */
function stageRuntime(sourceDir: string): string {
  const requiredInSource = ["dist/daemon.js", "package.json"];
  for (const r of requiredInSource) {
    if (!existsSync(join(sourceDir, r))) {
      throw new Error(`source missing ${r} (run 'npm run build' in ${sourceDir} first)`);
    }
  }
  const depsRoot = findNodeModulesRoot(sourceDir);
  const target = runtimeDir();
  for (const sub of ["dist", "node_modules", "package.json"]) {
    rmSync(join(target, sub), { recursive: true, force: true });
  }
  mkdirSync(target, { recursive: true });
  cpSync(join(sourceDir, "dist"), join(target, "dist"), { recursive: true, dereference: true });
  cpSync(join(depsRoot, "node_modules"), join(target, "node_modules"), { recursive: true, dereference: true });
  copyFileSync(join(sourceDir, "package.json"), join(target, "package.json"));
  return join(target, "dist", "daemon.js");
}

export interface DaemonInstallResult {
  plistPath: string;
  port: number;
  url: string;
  loaded: boolean;
  loadError?: string;
  configsWritten: { path: string; created: boolean; backupPath?: string }[];
  configsSkipped: { path: string; reason: string }[];
}

export interface DaemonUninstallResult {
  plistRemoved: boolean;
  bootoutOk: boolean;
  bootoutError?: string;
  configsCleaned: { path: string; removed: boolean }[];
}

/** Returns the plist path even when the file doesn't exist yet. */
export function daemonPlistPath(): string {
  return join(homedir(), "Library", "LaunchAgents", `${DAEMON_LABEL}.plist`);
}

/** Where launchd's stdout/stderr land for the daemon. */
export function daemonLogPaths(): { out: string; err: string } {
  const dir = join(homedir(), "Library", "Logs");
  return {
    out: join(dir, "deployik-mcp.out.log"),
    err: join(dir, "deployik-mcp.err.log"),
  };
}

export function installDaemon(opts: DaemonInstallOpts): DaemonInstallResult {
  if (platform() !== "darwin") {
    throw new Error("--daemon install currently supports macOS only (launchd). For Linux, write a systemd --user unit pointing at `node dist/daemon.js`.");
  }
  const port = opts.port ?? DEFAULT_DAEMON_PORT;
  const nodePath = opts.nodePath ?? resolveNodePath();
  // Stage the runtime into ~/.deployik-mcp/runtime/ — launchd-spawned
  // processes hit macOS TCC denials when reading from ~/Documents/ etc.,
  // and the user's source repo lives there. Staging outside TCC-protected
  // folders is the only reliable fix without asking the user to grant
  // Full Disk Access in System Settings.
  const daemonScript = stageRuntime(resolve(opts.sourceDir));
  const logs = daemonLogPaths();
  mkdirSync(dirname(logs.out), { recursive: true });

  const plistPath = daemonPlistPath();
  mkdirSync(dirname(plistPath), { recursive: true });
  const plist = renderPlist({
    label: DAEMON_LABEL,
    nodePath,
    daemonScript,
    env: {
      DEPLOYIK_URL: opts.url,
      DEPLOYIK_TOKEN: opts.token,
      DEPLOYIK_DAEMON_PORT: String(port),
    },
    stdoutPath: logs.out,
    stderrPath: logs.err,
  });
  writeFileSync(plistPath, plist, "utf8");
  // 0600: token lives plaintext inside this file — only the current user reads it.
  chmodSync(plistPath, 0o600);

  // Re-bootstrap so a re-run picks up env/script changes. Two races to
  // navigate:
  //   1. `bootout` returns before launchd has fully released the label —
  //      `print` reports the service as unloaded (exit 113) almost
  //      immediately, but the underlying process can still be in cleanup.
  //   2. A bootstrap fired before the old process is fully reaped fails
  //      with the famously vague "Bootstrap failed: 5: Input/output error".
  //
  // So we bootout (best-effort), then retry the bootstrap with backoff
  // until it sticks or we run out of attempts. Six tries × ~700ms gives
  // launchd ~4s of room — plenty in practice; the manual fix during dev
  // worked after ~2s.
  const uid = process.getuid?.() ?? 501;
  const domain = `gui/${uid}`;
  if (isServiceLoaded(domain, DAEMON_LABEL)) {
    try { execFileSync("launchctl", ["bootout", `${domain}/${DAEMON_LABEL}`], { stdio: "ignore" }); } catch { /* ignore */ }
  }

  let loaded = false;
  let loadError: string | undefined;
  const attempts = 6;
  const backoffMs = 700;
  for (let i = 0; i < attempts; i++) {
    try {
      execFileSync("launchctl", ["bootstrap", domain, plistPath], { stdio: ["ignore", "pipe", "pipe"] });
      loaded = true;
      loadError = undefined;
      break;
    } catch (err) {
      loadError = (err as { stderr?: Buffer }).stderr?.toString("utf8").trim() || (err as Error).message;
      // Only retry on the specific race error; anything else (bad plist,
      // permission denied) will keep failing — return fast for those.
      if (!loadError.toLowerCase().includes("input/output error")) break;
      if (i < attempts - 1) sleepSync(backoffMs);
    }
  }

  const configsResult = rewriteClaudeConfigsForDaemon(port);
  return {
    plistPath,
    port,
    url: `http://127.0.0.1:${port}/mcp`,
    loaded,
    ...(loadError !== undefined ? { loadError } : {}),
    configsWritten: configsResult.written,
    configsSkipped: configsResult.skipped,
  };
}

export interface UninstallDaemonOpts {
  /** Restore the stdio entry instead of removing it entirely. Defaults true. */
  restoreStdio?: boolean;
  /** Used only when restoreStdio is true. */
  url?: string;
  /** Used only when restoreStdio is true. */
  token?: string;
}

export function uninstallDaemon(opts: UninstallDaemonOpts = {}): DaemonUninstallResult {
  const result: DaemonUninstallResult = {
    plistRemoved: false,
    bootoutOk: false,
    configsCleaned: [],
  };

  const uid = process.getuid?.() ?? 501;
  const domain = `gui/${uid}`;
  try {
    execFileSync("launchctl", ["bootout", `${domain}/${DAEMON_LABEL}`], { stdio: ["ignore", "pipe", "pipe"] });
    result.bootoutOk = true;
  } catch (err) {
    result.bootoutError = (err as { stderr?: Buffer }).stderr?.toString("utf8").trim() || (err as Error).message;
  }

  const plistPath = daemonPlistPath();
  if (existsSync(plistPath)) {
    try {
      unlinkSync(plistPath);
      result.plistRemoved = true;
    } catch {
      result.plistRemoved = false;
    }
  }

  // Clear the daemon entries from Claude configs. If restoreStdio is set with
  // url+token, swap back to the original stdio shape so the user doesn't lose
  // the integration entirely; otherwise just remove.
  const targets = claudeConfigTargets();
  for (const target of targets) {
    if (!existsSync(target.path)) {
      result.configsCleaned.push({ path: target.path, removed: false });
      continue;
    }
    const cfg = readConfig(target.path);
    if (!cfg?.mcpServers || !(MCP_NAME in cfg.mcpServers)) {
      result.configsCleaned.push({ path: target.path, removed: false });
      continue;
    }
    backupFile(target.path);
    const next = { ...cfg.mcpServers };
    if (opts.restoreStdio && opts.url && opts.token) {
      next[MCP_NAME] = makeStdioEntry(opts.url, opts.token);
    } else {
      delete next[MCP_NAME];
    }
    cfg.mcpServers = next;
    writeFileSync(target.path, JSON.stringify(cfg, null, 2) + "\n", "utf8");
    result.configsCleaned.push({ path: target.path, removed: true });
  }

  // Wipe the staged runtime — but leave .deployik/ state dir alone so audit
  // history survives uninstall/reinstall cycles.
  try { rmSync(runtimeDir(), { recursive: true, force: true }); } catch { /* ignore */ }

  return result;
}

// ─── plist rendering ──────────────────────────────────────────────────────

interface PlistOpts {
  label: string;
  nodePath: string;
  daemonScript: string;
  env: Record<string, string>;
  stdoutPath: string;
  stderrPath: string;
}

function renderPlist(opts: PlistOpts): string {
  const envEntries = Object.entries(opts.env)
    .map(([k, v]) => `    <key>${xmlEscape(k)}</key>\n    <string>${xmlEscape(v)}</string>`)
    .join("\n");
  return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>${xmlEscape(opts.label)}</string>
  <key>ProgramArguments</key>
  <array>
    <string>${xmlEscape(opts.nodePath)}</string>
    <string>${xmlEscape(opts.daemonScript)}</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict>
${envEntries}
  </dict>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ProcessType</key>
  <string>Background</string>
  <key>StandardOutPath</key>
  <string>${xmlEscape(opts.stdoutPath)}</string>
  <key>StandardErrorPath</key>
  <string>${xmlEscape(opts.stderrPath)}</string>
</dict>
</plist>
`;
}

function xmlEscape(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

function resolveNodePath(): string {
  // process.execPath is the absolute path to the currently-running node binary.
  // launchd has a sparse PATH so we can't rely on "node" alone.
  return process.execPath;
}

function isServiceLoaded(domain: string, label: string): boolean {
  // `launchctl print` exits non-zero when the service isn't in the domain.
  try {
    execFileSync("launchctl", ["print", `${domain}/${label}`], { stdio: "ignore" });
    return true;
  } catch {
    return false;
  }
}

function sleepSync(ms: number): void {
  // Block the event loop without burning CPU. Cleaner than an empty `while`
  // spin loop, lighter than spawning `sleep`. SharedArrayBuffer + Atomics.wait
  // is the canonical Node way to do this synchronously.
  const i32 = new Int32Array(new SharedArrayBuffer(4));
  Atomics.wait(i32, 0, 0, ms);
}

// ─── Claude config rewrite ────────────────────────────────────────────────

interface ClaudeConfig {
  mcpServers?: Record<string, McpEntry>;
  [k: string]: unknown;
}

interface McpStdioEntry {
  type?: "stdio";
  command: string;
  args: string[];
  env?: Record<string, string>;
}

interface McpHttpEntry {
  type: "http";
  url: string;
}

type McpEntry = McpStdioEntry | McpHttpEntry;

function makeStdioEntry(url: string, token: string): McpStdioEntry {
  return {
    type: "stdio",
    command: "npx",
    args: ["-y", "@lovinka/deployik-mcp"],
    env: { DEPLOYIK_URL: url, DEPLOYIK_TOKEN: token },
  };
}

function makeHttpEntry(port: number): McpHttpEntry {
  return {
    type: "http",
    url: `http://127.0.0.1:${port}/mcp`,
  };
}

function claudeConfigTargets(): { path: string; mustExistDir: boolean }[] {
  const home = homedir();
  const out: { path: string; mustExistDir: boolean }[] = [
    { path: join(home, ".claude.json"), mustExistDir: false },
  ];
  if (platform() === "darwin") {
    out.push({
      path: join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json"),
      mustExistDir: true,
    });
  }
  return out;
}

function readConfig(path: string): ClaudeConfig | undefined {
  if (!existsSync(path)) return undefined;
  try {
    const raw = readFileSync(path, "utf8").trim();
    if (!raw) return {};
    return JSON.parse(raw) as ClaudeConfig;
  } catch (err) {
    throw new Error(`Failed to parse ${path}: ${(err as Error).message}`);
  }
}

function backupFile(path: string): string {
  const ts = new Date().toISOString().replace(/[:.]/g, "-");
  const bak = `${path}.bak.${ts}`;
  copyFileSync(path, bak);
  return bak;
}

function rewriteClaudeConfigsForDaemon(port: number): {
  written: { path: string; created: boolean; backupPath?: string }[];
  skipped: { path: string; reason: string }[];
} {
  const written: { path: string; created: boolean; backupPath?: string }[] = [];
  const skipped: { path: string; reason: string }[] = [];

  for (const target of claudeConfigTargets()) {
    if (target.mustExistDir && !existsSync(dirname(target.path))) {
      skipped.push({ path: target.path, reason: "parent dir not found (Claude Desktop not installed?)" });
      continue;
    }
    const existing = readConfig(target.path);
    const created = !existsSync(target.path);
    let backupPath: string | undefined;
    if (existing && existsSync(target.path)) {
      backupPath = backupFile(target.path);
    }
    const cfg: ClaudeConfig = existing ?? {};
    cfg.mcpServers = { ...(cfg.mcpServers ?? {}), [MCP_NAME]: makeHttpEntry(port) };
    mkdirSync(dirname(target.path), { recursive: true });
    writeFileSync(target.path, JSON.stringify(cfg, null, 2) + "\n", "utf8");
    written.push({ path: target.path, created, ...(backupPath !== undefined ? { backupPath } : {}) });
  }

  return { written, skipped };
}
