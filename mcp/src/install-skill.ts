// Copy the bundled Deployik knowledge into Claude's skills directory so the
// same recipes the MCP serves are also available as a regular skill (fires
// from `/skills` independently of any MCP tool call).
//
// Scope:
//   global → ~/.claude/skills/deployik-howto/        (default)
//   local  → <cwd>/.claude/skills/deployik-howto/    (per-project)

import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { join, resolve } from "node:path";
import { createInterface } from "node:readline";
import { RECIPE_FILES } from "./knowledge/recipes.generated.js";
import type { InstallScope } from "./install-mcp.js";

export interface SkillInstallOpts {
  scope: InstallScope;
  yes: boolean;
  cwd?: string;
  destBase?: string;
}

export interface SkillInstallResult {
  target: string;
  files: string[];
  alreadyExisted: boolean;
}

export function skillTargetDir(scope: InstallScope, cwd: string, destBase?: string): string {
  if (destBase) return resolve(destBase, "deployik-howto");
  if (scope === "local") return resolve(cwd, ".claude", "skills", "deployik-howto");
  return join(homedir(), ".claude", "skills", "deployik-howto");
}

export async function installSkill(opts: SkillInstallOpts): Promise<number> {
  const result = await installSkillResult(opts);
  if (result === null) return 0;
  process.stdout.write(`\n✓ Installed Deployik skill to ${result.target}\n`);
  process.stdout.write(`  Test it: in Claude Code, type /skills and look for 'deployik-howto'.\n`);
  return 0;
}

export async function installSkillResult(opts: SkillInstallOpts): Promise<SkillInstallResult | null> {
  const cwd = opts.cwd ?? process.cwd();
  const target = skillTargetDir(opts.scope, cwd, opts.destBase);

  if (!RECIPE_FILES.length) {
    process.stderr.write(`No bundled recipes found in this build.\n`);
    return null;
  }

  process.stdout.write(`Skill files (${opts.scope}) → ${target}\n`);
  for (const f of RECIPE_FILES) process.stdout.write(`  - ${f.file}\n`);

  const alreadyExisted = existsSync(target);
  if (alreadyExisted) {
    process.stdout.write(`Note: '${target}' already exists. Files will be overwritten.\n`);
  }

  if (!opts.yes) {
    const ok = await promptYesNo("Proceed with skill install? [y/N] ");
    if (!ok) {
      process.stdout.write("Skipped skill install.\n");
      return null;
    }
  }

  mkdirSync(target, { recursive: true });
  for (const f of RECIPE_FILES) writeFileSync(join(target, f.file), f.content, "utf8");

  return { target, files: RECIPE_FILES.map((f) => f.file), alreadyExisted };
}

export function promptYesNo(prompt: string): Promise<boolean> {
  return new Promise((resolveFn) => {
    const rl = createInterface({ input: process.stdin, output: process.stdout });
    rl.question(prompt, (answer) => {
      rl.close();
      resolveFn(/^y(es)?$/i.test(answer.trim()));
    });
  });
}

export function prompt(message: string, defaultValue?: string): Promise<string> {
  return new Promise((resolveFn) => {
    const rl = createInterface({ input: process.stdin, output: process.stdout });
    const suffix = defaultValue ? ` [${defaultValue}]` : "";
    rl.question(`${message}${suffix}: `, (answer) => {
      rl.close();
      const trimmed = answer.trim();
      resolveFn(trimmed || defaultValue || "");
    });
  });
}
