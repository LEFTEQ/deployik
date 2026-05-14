// `deployik-mcp install-skill` — write the bundled Deployik knowledge into
// Claude Code's skills directory so the same recipes the MCP serves are also
// available as a regular `/skills/deployik-howto` skill that fires
// independently of any MCP tool call (e.g. when the user is reading a doc and
// types `/skills`).
//
// Idempotent. Asks for confirmation unless --yes is passed.

import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";
import { createInterface } from "node:readline";
import { RECIPE_FILES } from "./knowledge/recipes.generated.js";

interface InstallOpts {
  yes: boolean;
  destBase?: string;
}

export async function installSkill(opts: InstallOpts): Promise<number> {
  const base = opts.destBase ?? join(homedir(), ".claude", "skills");
  const target = join(base, "deployik-howto");

  if (!RECIPE_FILES.length) {
    process.stderr.write(`No bundled recipes found in this build.\n`);
    return 1;
  }

  process.stdout.write(`This will write ${RECIPE_FILES.length} files to:\n  ${target}\n`);
  for (const f of RECIPE_FILES) process.stdout.write(`  - ${f.file}\n`);

  if (existsSync(target)) {
    process.stdout.write(`\nNote: '${target}' already exists. Files will be overwritten.\n`);
  }

  if (!opts.yes) {
    const ok = await promptYesNo("\nProceed? [y/N] ");
    if (!ok) {
      process.stdout.write("Aborted. To skip the prompt next time, pass --yes.\n");
      return 0;
    }
  }

  mkdirSync(target, { recursive: true });
  for (const f of RECIPE_FILES) {
    writeFileSync(join(target, f.file), f.content, "utf8");
  }
  process.stdout.write(`\n✓ Installed Deployik skill to ${target}\n`);
  process.stdout.write(`  Test it: in Claude Code, type /skills and look for 'deployik-howto'.\n`);
  return 0;
}

function promptYesNo(prompt: string): Promise<boolean> {
  return new Promise((resolve) => {
    const rl = createInterface({ input: process.stdin, output: process.stdout });
    rl.question(prompt, (answer) => {
      rl.close();
      resolve(/^y(es)?$/i.test(answer.trim()));
    });
  });
}
