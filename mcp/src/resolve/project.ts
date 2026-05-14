import { execSync } from "node:child_process";
import { DeployikClient } from "../client/http.js";
import { readBinding, writeBinding } from "../config/binding.js";
import { readCache, writeCache, isCacheFresh, type CachedProject } from "../config/cache.js";
import type { Project, Organization } from "../client/types.js";

export interface ResolveInput {
  /** Explicit ULID. Highest precedence. */
  project_id?: string;
  /** Project slug — second precedence. */
  project?: string;
  /** Optional workspace slug or id to disambiguate. */
  workspace?: string;
}

export interface ResolveContext {
  client: DeployikClient;
  stateDir: string;
  cwd: string;
}

export interface ResolvedProject {
  project: Project;
  source: "id" | "slug" | "binding" | "git_remote" | "single_workspace";
}

export class ResolveError extends Error {
  constructor(message: string, public readonly hint: string) {
    super(message);
    this.name = "ResolveError";
  }
}

export async function resolveProject(ctx: ResolveContext, input: ResolveInput = {}): Promise<ResolvedProject> {
  // 1. explicit id
  if (input.project_id) {
    const project = await ctx.client.request<Project>(`/projects/${encodeURIComponent(input.project_id)}`);
    return { project, source: "id" };
  }

  // gather projects once
  const projects = await fetchProjects(ctx);

  // 2. explicit slug
  if (input.project) {
    const match = matchProjectByName(projects, input.project, input.workspace);
    if (match) return { project: match, source: "slug" };
    throw new ResolveError(
      `No project named '${input.project}' is visible to this token${input.workspace ? ` in workspace '${input.workspace}'` : ""}.`,
      "Call `list_projects` to see what this token can reach.",
    );
  }

  // 3. binding file (public .deployik.json or legacy .deployik/binding.json)
  const binding = readBinding(ctx.stateDir, ctx.cwd);
  if (binding?.project) {
    const match = matchProjectByName(projects, binding.project, binding.workspace);
    if (match) return { project: match, source: "binding" };
  }

  // 4. git remote → cached project lookup, AUTO-BIND if a single unambiguous match
  const remote = readGitRemote(ctx.cwd);
  if (remote) {
    const matches = projects.filter(
      (p) => p.github_owner.toLowerCase() === remote.owner.toLowerCase() && p.github_repo.toLowerCase() === remote.repo.toLowerCase(),
    );
    if (matches.length === 1) {
      const match = matches[0]!;
      // Self-bind: write .deployik.json so the next call resolves via the
      // binding (cheaper) and so teammates pulling this repo inherit the
      // mapping. Best-effort — never block resolution.
      try {
        writeBinding(ctx.stateDir, ctx.cwd, {
          project: match.name,
          workspace: match.organization_name ?? match.organization_id,
        });
      } catch {
        // ignore — fall through to returning the match anyway
      }
      return { project: match, source: "git_remote" };
    }
    // Multiple repos share the same github_owner/github_repo (e.g. several
    // projects deploying different apps from one monorepo) — too ambiguous to
    // auto-bind, but still surface the candidates to help the AI/user.
    if (matches.length > 1) {
      throw new ResolveError(
        `This repo (${remote.owner}/${remote.repo}) maps to ${matches.length} Deployik projects: ${matches.map((m) => m.name).join(", ")}.`,
        "Pass `project: <slug>` to disambiguate, or run `init_in_repo` with the slug you want bound.",
      );
    }
  }

  // 5. single project visible
  if (projects.length === 1) {
    return { project: projects[0]!, source: "single_workspace" };
  }

  throw new ResolveError(
    "Could not figure out which Deployik project you mean.",
    "Pass `project: <slug>` or `project_id: <id>`, or run `init_in_repo` to bind this folder to a project.",
  );
}

export async function fetchProjects(ctx: ResolveContext): Promise<Project[]> {
  const projects = await ctx.client.request<Project[]>(`/projects`);
  await refreshCache(ctx, projects);
  return projects;
}

export async function refreshCache(ctx: ResolveContext, projects: Project[]): Promise<void> {
  // workspaces fetched lazily — only when cache is stale or missing
  let workspaces: Organization[] = [];
  const existing = readCache(ctx.stateDir);
  if (!existing || !isCacheFresh(existing)) {
    try {
      workspaces = await ctx.client.request<Organization[]>(`/organizations`);
    } catch {
      workspaces = existing?.workspaces.map((w) => ({
        id: w.id,
        slug: w.slug,
        name: w.name,
        is_personal: false,
        membership_role: "member",
        project_count: 0,
        created_at: "",
        updated_at: "",
      })) ?? [];
    }
  } else {
    workspaces = existing.workspaces.map((w) => ({
      id: w.id,
      slug: w.slug,
      name: w.name,
      is_personal: false,
      membership_role: "member",
      project_count: 0,
      created_at: "",
      updated_at: "",
    }));
  }

  const cached: CachedProject[] = projects.map((p) => ({
    id: p.id,
    name: p.name,
    workspace: p.organization_name ?? p.organization_id,
    github_owner: p.github_owner,
    github_repo: p.github_repo,
  }));

  writeCache(ctx.stateDir, {
    projects: cached,
    workspaces: workspaces.map((w) => ({ id: w.id, slug: w.slug, name: w.name })),
    platform: existing?.platform,
  });
}

function matchProjectByName(projects: Project[], slug: string, workspace?: string): Project | undefined {
  const slugLower = slug.toLowerCase();
  const matches = projects.filter((p) => p.name.toLowerCase() === slugLower);
  if (matches.length === 0) return undefined;
  if (matches.length === 1) return matches[0]!;
  if (!workspace) return matches[0]!;
  const ws = workspace.toLowerCase();
  const refined = matches.find((p) => (p.organization_name ?? "").toLowerCase() === ws || p.organization_id === workspace);
  return refined ?? matches[0]!;
}

interface GitRemote {
  owner: string;
  repo: string;
}

function readGitRemote(cwd: string): GitRemote | undefined {
  try {
    const url = execSync("git remote get-url origin", { cwd, encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] }).trim();
    return parseGithubUrl(url);
  } catch {
    return undefined;
  }
}

function parseGithubUrl(url: string): GitRemote | undefined {
  // git@github.com:owner/repo.git | https://github.com/owner/repo(.git)?
  const ssh = url.match(/^git@github\.com:([^\/]+)\/([^\/]+?)(?:\.git)?$/);
  if (ssh) return { owner: ssh[1]!, repo: ssh[2]! };
  const https = url.match(/^https?:\/\/github\.com\/([^\/]+)\/([^\/]+?)(?:\.git)?$/);
  if (https) return { owner: https[1]!, repo: https[2]! };
  return undefined;
}
