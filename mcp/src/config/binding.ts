import { existsSync, mkdirSync, readFileSync, writeFileSync, appendFileSync } from "node:fs";
import { dirname, join } from "node:path";

export interface RepoBinding {
  project: string;
  workspace?: string;
  defaultEnvironment?: "preview" | "production";
}

const BINDING_FILE = "binding.json";
const GITIGNORE_LINE = ".deployik/";

export function bindingPath(stateDir: string): string {
  return join(stateDir, BINDING_FILE);
}

export function readBinding(stateDir: string): RepoBinding | undefined {
  const path = bindingPath(stateDir);
  if (!existsSync(path)) return undefined;
  try {
    const raw = readFileSync(path, "utf8");
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed.project !== "string") return undefined;
    return parsed as RepoBinding;
  } catch {
    return undefined;
  }
}

export function writeBinding(stateDir: string, binding: RepoBinding): void {
  mkdirSync(stateDir, { recursive: true });
  writeFileSync(bindingPath(stateDir), `${JSON.stringify(binding, null, 2)}\n`, "utf8");
}

/** Append `.deployik/` to the consuming repo's .gitignore if absent. Returns true if it added the line. */
export function ensureGitignore(cwd: string): boolean {
  const gitignorePath = join(cwd, ".gitignore");
  if (!existsSync(dirname(gitignorePath))) return false;
  let existing = "";
  if (existsSync(gitignorePath)) {
    existing = readFileSync(gitignorePath, "utf8");
    const lines = existing.split(/\r?\n/);
    if (lines.some((l) => l.trim() === GITIGNORE_LINE || l.trim() === ".deployik")) {
      return false;
    }
  }
  const prefix = existing.length === 0 || existing.endsWith("\n") ? "" : "\n";
  appendFileSync(gitignorePath, `${prefix}${GITIGNORE_LINE}\n`, "utf8");
  return true;
}
