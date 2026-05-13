import { readFileSync, existsSync } from "node:fs";
import { join } from "node:path";

export interface ResolvedEnv {
  baseUrl: string;
  token: string;
  /** Absolute path to the consuming repo root (cwd by default). */
  cwd: string;
  /** Absolute path to the `.deployik/` directory. */
  stateDir: string;
  timeoutMs: number;
}

export class ConfigError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ConfigError";
  }
}

function readTokenFile(stateDir: string): string | undefined {
  const path = join(stateDir, "token");
  if (!existsSync(path)) return undefined;
  try {
    const raw = readFileSync(path, "utf8").trim();
    return raw || undefined;
  } catch {
    return undefined;
  }
}

export function loadEnv(env: NodeJS.ProcessEnv = process.env, cwd: string = process.cwd()): ResolvedEnv {
  const baseUrl = env.DEPLOYIK_URL?.trim();
  if (!baseUrl) {
    throw new ConfigError(
      "DEPLOYIK_URL is not set. Add it to your MCP server config, e.g. \"DEPLOYIK_URL\": \"https://deployik.example.com\".",
    );
  }

  const stateDir = join(cwd, ".deployik");
  const token = env.DEPLOYIK_TOKEN?.trim() || readTokenFile(stateDir);
  if (!token) {
    throw new ConfigError(
      "DEPLOYIK_TOKEN is not set and no .deployik/token file was found. Create a token at Account → Access tokens and pass it as DEPLOYIK_TOKEN in the MCP server env.",
    );
  }

  const timeoutMs = parseInt(env.DEPLOYIK_TIMEOUT_MS ?? "30000", 10);

  return {
    baseUrl,
    token,
    cwd,
    stateDir,
    timeoutMs: Number.isFinite(timeoutMs) && timeoutMs > 0 ? timeoutMs : 30_000,
  };
}
