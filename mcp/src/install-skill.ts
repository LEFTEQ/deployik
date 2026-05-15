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
import { createInterface, type Interface as ReadlineInterface } from "node:readline";
import { RECIPE_FILES } from "./knowledge/recipes.generated.js";
import type { InstallScope } from "./install-mcp.js";

// Two read modes, chosen at first prompt by checking whether stdin is a TTY:
//
//   TTY   → singleton readline; user is typing interactively.
//   pipe  → drain stdin once into a line buffer (piped input EOFs as soon as
//           it drains, which auto-closes readline mid-wizard with ERR_USE_AFTER_CLOSE).
//
// Same async API for both — callers don't care which mode is active.

let sharedRl: ReadlineInterface | null = null;
let pipedLines: string[] | null = null;
let pipeReadPromise: Promise<void> | null = null;

function isPipedStdin(): boolean {
  return process.stdin.isTTY !== true;
}

function ensurePipedLines(): Promise<void> {
  if (pipeReadPromise) return pipeReadPromise;
  pipeReadPromise = new Promise((resolveFn) => {
    let data = "";
    process.stdin.setEncoding("utf8");
    process.stdin.on("data", (chunk) => { data += chunk; });
    process.stdin.on("end", () => {
      pipedLines = data.split(/\r?\n/);
      resolveFn();
    });
    process.stdin.on("error", () => { pipedLines = []; resolveFn(); });
  });
  return pipeReadPromise;
}

async function readLine(promptText: string): Promise<string> {
  if (isPipedStdin()) {
    process.stdout.write(promptText);
    await ensurePipedLines();
    const line = pipedLines!.shift() ?? "";
    process.stdout.write(line + "\n");
    return line;
  }
  if (!sharedRl) sharedRl = createInterface({ input: process.stdin, output: process.stdout });
  return new Promise((resolveFn) => {
    sharedRl!.question(promptText, (answer) => resolveFn(answer));
  });
}

export function closeReadline(): void {
  if (sharedRl) { sharedRl.close(); sharedRl = null; }
}

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

export async function promptYesNo(promptText: string): Promise<boolean> {
  const answer = await readLine(promptText);
  return /^y(es)?$/i.test(answer.trim());
}

export async function prompt(message: string, defaultValue?: string): Promise<string> {
  const suffix = defaultValue ? ` [${defaultValue}]` : "";
  const answer = await readLine(`${message}${suffix}: `);
  const trimmed = answer.trim();
  return trimmed || defaultValue || "";
}

/**
 * Single-letter choice prompt, e.g. promptChoice("Scope", [["g","global"],["l","local"]], "g").
 * Returns the chosen letter (lowercased).
 */
export async function promptChoice(
  message: string,
  choices: [letter: string, label: string][],
  defaultLetter: string,
): Promise<string> {
  const display = choices
    .map(([l, lab]) => (l === defaultLetter ? l.toUpperCase() : l) + "=" + lab)
    .join(" / ");
  const answer = await readLine(`${message} [${display}]: `);
  const a = answer.trim().toLowerCase();
  if (!a) return defaultLetter.toLowerCase();
  const match = choices.find(([l]) => l.toLowerCase() === a[0]);
  return match ? match[0].toLowerCase() : defaultLetter.toLowerCase();
}
