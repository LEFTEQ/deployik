// One-shot installer: registers the @lovinka/deployik-mcp MCP server in
// Claude's config AND copies the bundled Deployik skill into Claude's skills
// directory. Default scope is GLOBAL (recommended) — once installed, every
// project + every Claude window can use the deployik tools.

import { installMcp, type InstallScope } from "./install-mcp.js";
import { installSkillResult, prompt, promptYesNo } from "./install-skill.js";

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
  const scope = opts.scope;
  process.stdout.write(`\n=== Installing @lovinka/deployik-mcp (${scope}) ===\n\n`);

  // Resolve URL + token
  let url = opts.url ?? process.env.DEPLOYIK_URL ?? "";
  let token = opts.token ?? process.env.DEPLOYIK_TOKEN ?? "";

  if (!opts.skipMcp) {
    if (!opts.yes) {
      if (!url) url = await prompt("Deployik URL", DEFAULT_URL);
      if (!token) token = await prompt("Personal Access Token (dpk_…)");
    } else {
      if (!url) url = DEFAULT_URL;
    }
    if (!token) {
      process.stderr.write(
        `\nERROR: DEPLOYIK_TOKEN missing.\nPass --token=<dpk_...> or set DEPLOYIK_TOKEN env var.\n` +
          `Get a token at: Account → Access tokens in your Deployik dashboard.\n`,
      );
      return 2;
    }
    if (!url) url = DEFAULT_URL;
  }

  // Confirmation
  if (!opts.yes) {
    process.stdout.write(`\nAbout to perform:\n`);
    if (!opts.skipMcp) process.stdout.write(`  • Register MCP server 'deployik' (${scope}) pointing at ${url}\n`);
    if (!opts.skipSkill) process.stdout.write(`  • Copy Deployik skill files (${scope})\n`);
    process.stdout.write(`\n`);
    const ok = await promptYesNo("Proceed? [y/N] ");
    if (!ok) {
      process.stdout.write("Aborted.\n");
      return 0;
    }
  }

  // 1. MCP registration
  if (!opts.skipMcp) {
    process.stdout.write(`\n--- MCP server registration ---\n`);
    const res = installMcp({ scope, url, token, cwd });
    for (const w of res.written) {
      process.stdout.write(
        `  ✓ ${w.created ? "created" : "updated"} ${w.path}${w.backupPath ? `  (backup → ${w.backupPath})` : ""}\n`,
      );
    }
    for (const s of res.skipped) {
      process.stdout.write(`  · skipped ${s.path}  (${s.reason})\n`);
    }
    if (res.written.length === 0) {
      process.stderr.write(`  ! no MCP config targets were writable — nothing changed.\n`);
    }
  }

  // 2. Skill files
  if (!opts.skipSkill) {
    process.stdout.write(`\n--- Skill files ---\n`);
    const r = await installSkillResult({ scope, yes: true, cwd });
    if (r) {
      process.stdout.write(
        `  ✓ wrote ${r.files.length} files to ${r.target}${r.alreadyExisted ? " (overwrote existing)" : ""}\n`,
      );
    } else {
      process.stdout.write(`  · skill install was skipped\n`);
    }
  }

  process.stdout.write(`\nDone. Restart Claude Code / Claude Desktop to pick up the new MCP server.\n`);
  if (scope === "local") {
    process.stdout.write(
      `\nNote: with --local scope, the .mcp.json and .claude/skills/deployik-howto/ are written into\n` +
        `the current directory. The MCP server only fires when Claude is opened in this folder.\n`,
    );
  }
  return 0;
}
