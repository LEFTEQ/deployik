// Interactive 4-step installer: URL → token → scope → confirm.
// Each step validates before moving on so the user gets immediate feedback
// (server reachable? token authenticates? scope chosen?) instead of a silent
// failure at the end.
//
// `--yes` skips prompts but still runs validations — bad token / unreachable
// URL still aborts, since installing a config that can't authenticate would
// be a footgun.

import {
  findExistingMcpConfig,
  installMcp,
  type InstallScope,
} from "./install-mcp.js";
import { installSkillResult, prompt, promptYesNo, promptChoice, closeReadline } from "./install-skill.js";
import { openInBrowser } from "./lib/browser.js";
import { box, dim, fail, info, ok, section, warn, bold, cyan } from "./lib/term.js";
import { VERSION } from "./version.js";

export interface InstallAllOpts {
  scope: InstallScope;
  yes: boolean;
  url?: string;
  token?: string;
  skipSkill?: boolean;
  skipMcp?: boolean;
  cwd?: string;
}

const DEFAULT_URL = "https://deployik.example.com";

export async function installAll(opts: InstallAllOpts): Promise<number> {
  const cwd = opts.cwd ?? process.cwd();
  const existing = opts.skipMcp
    ? undefined
    : findExistingMcpConfig({ scope: opts.scope, cwd });

  if (!opts.yes) {
    process.stdout.write("\n" + box(`@lovinka/deployik-mcp installer (${VERSION})`, [
      `${dim("Wires the Deployik MCP server into Claude's config and copies")}`,
      `${dim("the bundled how-to recipes into ~/.claude/skills/.")}`,
    ]) + "\n");
  }

  // ─── Step 1: URL ──────────────────────────────────────────────
  let url = opts.url ?? process.env.DEPLOYIK_URL ?? existing?.url ?? "";
  if (!opts.skipMcp) {
    if (!opts.yes && !url) {
      process.stdout.write(section(1, 4, "Deployik server URL"));
      process.stdout.write(`\n${dim("Public default works for the example.com instance. For VPN-only")}\n`);
      process.stdout.write(`${dim("installs, point at the internal hostname (e.g. http://10.x.x.x:8080).")}\n\n`);
      url = await prompt("Deployik URL", DEFAULT_URL);
    }
    if (!url) url = DEFAULT_URL;
    const reach = await probeHealth(url);
    if (reach.ok) {
      process.stdout.write("  " + ok(`reachable${reach.version ? ` (deployik ${reach.version})` : ""}\n`));
    } else {
      process.stdout.write("  " + warn(`unreachable: ${reach.error}\n`));
      if (!opts.yes) {
        const cont = await promptYesNo("  Continue anyway? [y/N] ");
        if (!cont) return abort("Install aborted at URL step.");
      }
    }
  }

  // ─── Step 2: Token ────────────────────────────────────────────
  let token = opts.token ?? process.env.DEPLOYIK_TOKEN ?? "";
  if (!token && existing?.token && (!opts.url || sameUrl(url, existing.url))) {
    token = existing.token;
  }
  let user: { username?: string; role?: string } | undefined;
  if (!opts.skipMcp) {
    process.stdout.write(section(2, 4, "Personal Access Token"));
    if (!token && !opts.yes) {
      const tokensUrl = `${url.replace(/\/+$/, "")}/account/tokens`;
      process.stdout.write(`\n${dim("Create a token at:")} ${cyan(tokensUrl)}\n`);
      const open = await promptYesNo(`Open the page in your browser? [Y/n] `);
      const yes = (open as unknown as boolean) === true || open;
      if (yes) {
        const opened = openInBrowser(tokensUrl);
        process.stdout.write(`  ${opened ? ok("opening browser…") : warn("couldn't auto-open — visit the URL manually")}\n\n`);
      }
      token = await prompt("Paste the token (starts with dpk_)");
    }
    if (!token) {
      process.stderr.write(
        "\n" + fail("DEPLOYIK_TOKEN missing.") +
        `\n  Pass ${bold("--token=<dpk_...>")} or set ${bold("DEPLOYIK_TOKEN")} env var.\n` +
        `  Get a token at ${cyan(`${url.replace(/\/+$/, "")}/account/tokens`)}\n`,
      );
      return 2;
    }
    if (!token.startsWith("dpk_")) {
      process.stdout.write("  " + warn(`token doesn't start with 'dpk_' — Deployik PATs always do. Continuing anyway.\n`));
    }
    const auth = await probeWhoami(url, token);
    if (auth.ok) {
      user = auth.user;
      process.stdout.write("  " + ok(`authenticated as ${bold(auth.user.username ?? "?")} (${auth.user.role ?? "?"})\n`));
    } else {
      process.stdout.write("  " + fail(`token rejected: ${auth.error}\n`));
      // 401 (token actually rejected) is a hard abort even with --yes — installing
      // a known-broken token gives the user a silently-failing MCP. Network errors
      // (timeout / DNS) only abort interactively, since VPN-only installs may
      // legitimately fail to reach the URL from the install machine.
      const isAuthFailure = (auth.error || "").includes("401");
      if (isAuthFailure) return abort("Install aborted — token did not authenticate.");
      if (!opts.yes) return abort("Install aborted — fix the token and re-run.");
      process.stdout.write("  " + warn("continuing anyway (--yes); make sure DEPLOYIK_TOKEN is valid before using.\n"));
    }
  }

  // ─── Step 3: Scope ────────────────────────────────────────────
  let scope = opts.scope;
  if (!opts.yes) {
    process.stdout.write(section(3, 4, "Install scope"));
    process.stdout.write(`\n  ${bold("g")} = global ${dim("(recommended) — every project + every Claude window")}`);
    process.stdout.write(`\n  ${bold("l")} = local  ${dim("— only when Claude is opened in this folder (writes ./.mcp.json)")}\n\n`);
    const def = scope === "local" ? "l" : "g";
    const picked = await promptChoice("Scope", [["g", "global"], ["l", "local"]], def);
    scope = picked === "l" ? "local" : "global";
    process.stdout.write("  " + ok(`scope: ${bold(scope)}\n`));
  }

  // ─── Step 4: Confirm ──────────────────────────────────────────
  if (!opts.yes) {
    process.stdout.write(section(4, 4, "Confirm"));
    process.stdout.write(`\n  ${info("About to:")}`);
    if (!opts.skipMcp) {
      process.stdout.write(`\n    • register MCP server '${bold("deployik")}' (${scope}) → ${url}`);
      if (user?.username) process.stdout.write(`\n      authenticated as ${user.username}`);
    }
    if (!opts.skipSkill) {
      process.stdout.write(`\n    • copy Deployik skill files (${scope})`);
    }
    if (scope === "global") process.stdout.write(`\n  ${dim("Backups of any existing config files will be made automatically.")}`);
    process.stdout.write(`\n\n`);
    // Treat empty input (just Enter) as Y since the prompt explicitly says [Y/n].
    const answer = (await prompt("Proceed?", "y")).toLowerCase();
    if (answer === "n" || answer === "no") return abort("Aborted.");
  }

  // ─── Execute ─────────────────────────────────────────────────
  process.stdout.write(`\n${dim("─".repeat(60))}\n`);

  if (!opts.skipMcp) {
    process.stdout.write(`${bold("Writing MCP config…")}\n`);
    const res = installMcp({ scope, url, token, cwd });
    for (const w of res.written) process.stdout.write("  " + ok(`${w.created ? "created" : "updated"} ${w.path}${w.backupPath ? `  ${dim("backup → " + w.backupPath)}` : ""}\n`));
    for (const s of res.skipped) process.stdout.write("  " + dim(`skipped ${s.path}  (${s.reason})\n`));
    if (res.written.length === 0) process.stderr.write("  " + fail("no MCP config targets were writable\n"));
  }

  if (!opts.skipSkill) {
    process.stdout.write(`${bold("Writing skill files…")}\n`);
    const r = await installSkillResult({ scope, yes: true, cwd });
    if (r) process.stdout.write("  " + ok(`wrote ${r.files.length} files to ${r.target}${r.alreadyExisted ? dim(" (overwrote existing)") : ""}\n`));
  }

  process.stdout.write(`\n${ok("Done.")} Restart Claude Code or Claude Desktop to load the MCP server.\n`);
  if (scope === "local") process.stdout.write(`\n${dim("Note: with --local, the MCP only fires when Claude is opened in this folder.")}\n`);
  closeReadline();
  return 0;
}

function sameUrl(left: string | undefined, right: string | undefined): boolean {
  if (!left || !right) return false;
  return left.replace(/\/+$/, "") === right.replace(/\/+$/, "");
}

function abort(msg: string): number {
  process.stdout.write(`\n${fail(msg)}\n`);
  closeReadline();
  return 1;
}

interface HealthResult { ok: boolean; version?: string; error?: string; }
async function probeHealth(url: string): Promise<HealthResult> {
  const target = url.replace(/\/+$/, "") + "/api/health";
  try {
    const ctrl = new AbortController();
    const t = setTimeout(() => ctrl.abort(), 5000);
    const res = await fetch(target, { signal: ctrl.signal });
    clearTimeout(t);
    if (!res.ok) return { ok: false, error: `HTTP ${res.status}` };
    const body = (await res.json().catch(() => null)) as { version?: { git_sha?: string } } | null;
    return { ok: true, version: body?.version?.git_sha?.slice(0, 7) };
  } catch (err) {
    return { ok: false, error: (err as Error).name === "AbortError" ? "timeout (5s)" : (err as Error).message };
  }
}

interface AuthResult { ok: boolean; user: { username?: string; role?: string }; error?: string; }
async function probeWhoami(url: string, token: string): Promise<AuthResult> {
  const target = url.replace(/\/+$/, "") + "/api/auth/me";
  try {
    const ctrl = new AbortController();
    const t = setTimeout(() => ctrl.abort(), 8000);
    const res = await fetch(target, {
      headers: { Authorization: `Bearer ${token}`, Accept: "application/json" },
      signal: ctrl.signal,
    });
    clearTimeout(t);
    if (res.status === 401) return { ok: false, user: {}, error: "401 unauthorized — token revoked, expired, or wrong" };
    if (!res.ok) return { ok: false, user: {}, error: `HTTP ${res.status}` };
    const body = (await res.json().catch(() => null)) as { username?: string; role?: string } | null;
    return { ok: true, user: { username: body?.username, role: body?.role } };
  } catch (err) {
    return { ok: false, user: {}, error: (err as Error).name === "AbortError" ? "timeout (8s)" : (err as Error).message };
  }
}
