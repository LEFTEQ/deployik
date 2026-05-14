import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { ensureGitignore } from "./binding.js";

export interface CachedProject {
  id: string;
  name: string;
  workspace?: string;
  github_owner: string;
  github_repo: string;
}

export interface CachedWorkspace {
  id: string;
  slug: string;
  name: string;
}

export interface CacheData {
  version: 1;
  fetchedAt: string;
  ttlSeconds: number;
  projects: CachedProject[];
  workspaces: CachedWorkspace[];
  platform?: { dnsTargetIp: string };
}

const CACHE_FILE = "cache.json";
const DEFAULT_TTL_SECONDS = 60 * 60; // 1h

export function cachePath(stateDir: string): string {
  return join(stateDir, CACHE_FILE);
}

export function readCache(stateDir: string): CacheData | undefined {
  const path = cachePath(stateDir);
  if (!existsSync(path)) return undefined;
  try {
    const raw = readFileSync(path, "utf8");
    const parsed = JSON.parse(raw) as CacheData;
    if (parsed.version !== 1) return undefined;
    return parsed;
  } catch {
    return undefined;
  }
}

export function writeCache(stateDir: string, data: Omit<CacheData, "version" | "fetchedAt" | "ttlSeconds"> & { ttlSeconds?: number }): CacheData {
  mkdirSync(stateDir, { recursive: true });
  // Self-heal .gitignore on every cache write — cheap, defensive.
  try {
    ensureGitignore(dirname(stateDir));
  } catch {
    // best-effort
  }
  const full: CacheData = {
    version: 1,
    fetchedAt: new Date().toISOString(),
    ttlSeconds: data.ttlSeconds ?? DEFAULT_TTL_SECONDS,
    projects: data.projects,
    workspaces: data.workspaces,
    platform: data.platform,
  };
  writeFileSync(cachePath(stateDir), `${JSON.stringify(full, null, 2)}\n`, "utf8");
  return full;
}

export function isCacheFresh(cache: CacheData | undefined): boolean {
  if (!cache) return false;
  const fetched = Date.parse(cache.fetchedAt);
  if (Number.isNaN(fetched)) return false;
  return Date.now() - fetched < cache.ttlSeconds * 1000;
}
