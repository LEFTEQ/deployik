import { existsSync, mkdirSync, readFileSync, writeFileSync, appendFileSync } from "node:fs";
import { dirname, join } from "node:path";

/**
 * Repo ↔ Deployik project binding.
 *
 * Stored in **two places** with different visibility intent:
 *
 *   .deployik.json          (at repo root, COMMITTED to git)
 *                            Public binding info — project slug + workspace +
 *                            default environment. Safe to share with teammates;
 *                            no secrets here, no per-user cache. The whole team
 *                            sees the same binding without having to re-discover.
 *
 *   .deployik/binding.json  (GITIGNORED, legacy)
 *                            Older versions of this MCP wrote the binding here.
 *                            We still read it as a fallback so existing repos
 *                            keep working. New writes always go to .deployik.json.
 *
 *   .deployik/              (GITIGNORED, per-developer)
 *                            cache.json, token, audit.log — anything sensitive
 *                            or machine-local. .gitignore is checked + enforced
 *                            on every write so a teammate clobbering it doesn't
 *                            silently leak a token next commit.
 */
export interface RepoBinding {
  project: string;
  workspace?: string;
  defaultEnvironment?: "preview" | "production";
}

const PUBLIC_FILE = ".deployik.json";
const PUBLIC_SCHEMA = "https://example.com/schemas/deployik.json";
const GITIGNORE_LINE = ".deployik/";

export function publicConfigPath(cwd: string): string {
  return join(cwd, PUBLIC_FILE);
}

export function legacyBindingPath(stateDir: string): string {
  return join(stateDir, "binding.json");
}

export function readBinding(stateDir: string, cwd?: string): RepoBinding | undefined {
  // Prefer the public, committed config at repo root.
  if (cwd) {
    const publicPath = publicConfigPath(cwd);
    if (existsSync(publicPath)) {
      try {
        const raw = readFileSync(publicPath, "utf8");
        const parsed = JSON.parse(raw);
        if (parsed && typeof parsed.project === "string") {
          return {
            project: parsed.project,
            workspace: parsed.workspace,
            defaultEnvironment: parsed.defaultEnvironment,
          };
        }
      } catch {
        // fall through to legacy
      }
    }
  }
  // Fallback: legacy private binding for repos that bound under the old layout.
  const legacy = legacyBindingPath(stateDir);
  if (existsSync(legacy)) {
    try {
      const raw = readFileSync(legacy, "utf8");
      const parsed = JSON.parse(raw);
      if (!parsed || typeof parsed.project !== "string") return undefined;
      return parsed as RepoBinding;
    } catch {
      return undefined;
    }
  }
  return undefined;
}

export function writeBinding(stateDir: string, cwd: string, binding: RepoBinding): void {
  // Always write to the public, committed file.
  const payload = {
    $schema: PUBLIC_SCHEMA,
    project: binding.project,
    ...(binding.workspace ? { workspace: binding.workspace } : {}),
    ...(binding.defaultEnvironment ? { defaultEnvironment: binding.defaultEnvironment } : {}),
  };
  writeFileSync(publicConfigPath(cwd), `${JSON.stringify(payload, null, 2)}\n`, "utf8");

  // Always (re-)ensure the gitignore protects the private dir, even if the
  // .deployik.json itself is committed. Cheap, defensive — if a teammate
  // dropped the line, the next MCP call patches it back.
  ensureGitignore(cwd);

  // Make sure the private state dir exists so cache.json / token / audit.log
  // have a home, but don't touch the legacy binding file — that stays for
  // backwards compat only.
  mkdirSync(stateDir, { recursive: true });
}

/**
 * Append `.deployik/` to .gitignore if absent. Idempotent. Returns true if a
 * line was added. Called on every binding write so the protection is
 * self-healing (teammate edits, merge conflicts, manual deletes — all fixed
 * silently next time anything touches `.deployik/`).
 */
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

/**
 * Verify the public config is sane. Used by `show_binding`-style tools to
 * surface schema issues that would otherwise look like "binding missing".
 */
export function validatePublicConfig(cwd: string): { ok: boolean; reason?: string } {
  const path = publicConfigPath(cwd);
  if (!existsSync(path)) return { ok: false, reason: "missing" };
  try {
    const raw = readFileSync(path, "utf8");
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object") return { ok: false, reason: "not an object" };
    if (typeof parsed.project !== "string" || parsed.project.length === 0) {
      return { ok: false, reason: "missing 'project' string" };
    }
    return { ok: true };
  } catch (err) {
    return { ok: false, reason: `parse error: ${(err as Error).message}` };
  }
}
