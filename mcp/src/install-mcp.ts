// Register the @lovinka/deployik-mcp server in Claude's MCP config so users
// don't have to hand-edit JSON. Supports two scopes:
//
//   global — Claude Code's user-level `~/.claude.json` (always) and, if
//            present, Claude Desktop's platform-specific config file. This is
//            the recommended path because once it's set the MCP server is
//            available in every project + every Claude window.
//
//   local  — Project-level `.mcp.json` in the current directory. Newer
//            Claude Code versions auto-pick this up. Useful for a repo where
//            you want to commit a per-project MCP config (e.g. pointed at a
//            preview instance of Deployik).

import { existsSync, mkdirSync, readFileSync, writeFileSync, copyFileSync } from "node:fs";
import { homedir, platform } from "node:os";
import { dirname, join, resolve } from "node:path";

export type InstallScope = "global" | "local";

export interface McpInstallOpts {
  scope: InstallScope;
  url: string;
  token: string;
  cwd?: string;
  name?: string; // defaults to "deployik"
  packageSpec?: string; // defaults to "@lovinka/deployik-mcp"
}

export interface McpInstallResult {
  written: { path: string; created: boolean; backupPath?: string }[];
  skipped: { path: string; reason: string }[];
}

export interface ExistingMcpConfig {
  path: string;
  url?: string;
  token?: string;
}

interface ClaudeConfig {
  mcpServers?: Record<string, McpServerEntry>;
  [k: string]: unknown;
}

interface McpServerEntry {
  type?: "stdio";
  command: string;
  args: string[];
  env?: Record<string, string>;
}

function makeEntry(spec: string, url: string, token: string): McpServerEntry {
  return {
    type: "stdio",
    command: "npx",
    args: ["-y", spec],
    env: {
      DEPLOYIK_URL: url,
      DEPLOYIK_TOKEN: token,
    },
  };
}

/**
 * The set of config files we know how to write for a given scope. For
 * `global`, we try every install target and include the ones whose parent
 * directory exists (so we don't materialise a Claude Desktop directory on a
 * machine that doesn't have it installed).
 */
function targetsFor(scope: InstallScope, cwd: string): { path: string; mustExistDir: boolean; label: string }[] {
  if (scope === "local") {
    return [{ path: resolve(cwd, ".mcp.json"), mustExistDir: false, label: "project .mcp.json" }];
  }
  const home = homedir();
  const out: { path: string; mustExistDir: boolean; label: string }[] = [
    { path: join(home, ".claude.json"), mustExistDir: false, label: "Claude Code (~/.claude.json)" },
  ];
  if (platform() === "darwin") {
    out.push({
      path: join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json"),
      mustExistDir: true,
      label: "Claude Desktop (macOS)",
    });
  } else if (platform() === "win32") {
    const appdata = process.env.APPDATA || join(home, "AppData", "Roaming");
    out.push({
      path: join(appdata, "Claude", "claude_desktop_config.json"),
      mustExistDir: true,
      label: "Claude Desktop (Windows)",
    });
  } else {
    out.push({
      path: join(home, ".config", "Claude", "claude_desktop_config.json"),
      mustExistDir: true,
      label: "Claude Desktop (Linux)",
    });
  }
  return out;
}

function readConfig(path: string): ClaudeConfig | undefined {
  if (!existsSync(path)) return undefined;
  try {
    const raw = readFileSync(path, "utf8").trim();
    if (!raw) return {};
    return JSON.parse(raw);
  } catch (err) {
    throw new Error(`Failed to parse ${path} — refusing to overwrite. (${(err as Error).message})`);
  }
}

export function findExistingMcpConfig(opts: {
  scope: InstallScope;
  cwd?: string;
  name?: string;
}): ExistingMcpConfig | undefined {
  const cwd = opts.cwd ?? process.cwd();
  const name = opts.name ?? "deployik";
  const scopes: InstallScope[] =
    opts.scope === "local" ? ["local", "global"] : ["global", "local"];

  for (const scope of scopes) {
    for (const target of targetsFor(scope, cwd)) {
      if (!existsSync(target.path)) continue;
      const config = readConfig(target.path);
      const entry = config?.mcpServers?.[name];
      const url = entry?.env?.DEPLOYIK_URL?.trim();
      const token = entry?.env?.DEPLOYIK_TOKEN?.trim();
      if (url || token) {
        return {
          path: target.path,
          ...(url ? { url } : {}),
          ...(token ? { token } : {}),
        };
      }
    }
  }
  return undefined;
}

function backup(path: string): string {
  const ts = new Date().toISOString().replace(/[:.]/g, "-");
  const bak = `${path}.bak.${ts}`;
  copyFileSync(path, bak);
  return bak;
}

export function installMcp(opts: McpInstallOpts): McpInstallResult {
  const cwd = opts.cwd ?? process.cwd();
  const name = opts.name ?? "deployik";
  const spec = opts.packageSpec ?? "@lovinka/deployik-mcp";
  const entry = makeEntry(spec, opts.url, opts.token);

  const result: McpInstallResult = { written: [], skipped: [] };

  for (const target of targetsFor(opts.scope, cwd)) {
    if (target.mustExistDir && !existsSync(dirname(target.path))) {
      result.skipped.push({ path: target.path, reason: `parent dir not found (${target.label} not installed?)` });
      continue;
    }
    const existing = readConfig(target.path);
    const created = !existsSync(target.path);
    let backupPath: string | undefined;
    if (existing && existsSync(target.path)) {
      backupPath = backup(target.path);
    }
    const next: ClaudeConfig = existing ?? {};
    next.mcpServers = { ...(next.mcpServers ?? {}), [name]: entry };

    mkdirSync(dirname(target.path), { recursive: true });
    writeFileSync(target.path, JSON.stringify(next, null, 2) + "\n", "utf8");
    result.written.push({ path: target.path, created, backupPath });
  }

  return result;
}

export interface RemoveOpts {
  scope: InstallScope;
  name?: string;
  cwd?: string;
}

/** Symmetric uninstall — removes the named mcpServers entry from every target. */
export function uninstallMcp(opts: RemoveOpts): McpInstallResult {
  const cwd = opts.cwd ?? process.cwd();
  const name = opts.name ?? "deployik";
  const result: McpInstallResult = { written: [], skipped: [] };

  for (const target of targetsFor(opts.scope, cwd)) {
    if (!existsSync(target.path)) {
      result.skipped.push({ path: target.path, reason: "not present" });
      continue;
    }
    const config = readConfig(target.path);
    if (!config?.mcpServers || !(name in config.mcpServers)) {
      result.skipped.push({ path: target.path, reason: `no '${name}' entry` });
      continue;
    }
    const backupPath = backup(target.path);
    const remaining = { ...config.mcpServers };
    delete remaining[name];
    config.mcpServers = remaining;
    writeFileSync(target.path, JSON.stringify(config, null, 2) + "\n", "utf8");
    result.written.push({ path: target.path, created: false, backupPath });
  }
  return result;
}
