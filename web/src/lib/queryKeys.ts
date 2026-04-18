// Centralized TanStack Query cache keys.
//
// All components should import from here instead of inlining string arrays,
// so that refactors (adding filters, renaming scopes, etc.) update every
// producer and consumer in lockstep. Also reduces typo-driven cache misses.

export const queryKeys = {
  // Auth & identity
  organizations: () => ["organizations"] as const,
  platform: () => ["platform"] as const,

  // Projects
  projects: (organizationId: string | null | undefined) =>
    ["projects", organizationId ?? "all"] as const,
  commandProjects: (organizationId: string | null | undefined) =>
    ["command-projects", organizationId ?? "all"] as const,
  project: (projectId: string) => ["project", projectId] as const,

  // Deployments
  deployments: (projectId: string) => ["deployments", projectId] as const,
  deployment: (deploymentId: string) => ["deployment", deploymentId] as const,
  buildLogs: (deploymentId: string) => ["build-logs", deploymentId] as const,

  // Domains
  domains: (projectId: string) => ["domains", projectId] as const,

  // Auto-build
  autoBuild: (projectId: string) => ["auto-build", projectId] as const,

  // Password protection
  protection: (projectId: string) => ["protection", projectId] as const,

  // Variables (env + secrets). Kind is "env" | "secret"; scope is
  // "shared" | "preview" | "production" | undefined.
  projectVariables: (
    kind: "env" | "secret",
    projectId: string,
    scope?: string,
  ) =>
    scope === undefined
      ? (["project-variables", kind, projectId] as const)
      : (["project-variables", kind, projectId, scope] as const),

  // Analytics
  projectAnalytics: (
    projectId: string,
    environment?: string,
    range?: string,
    timezone?: string,
  ) =>
    ["project-analytics", projectId, environment, range, timezone] as const,
  projectAnalyticsIntegration: (projectId: string, timezone?: string) =>
    ["project-analytics-integration", projectId, timezone] as const,

  // GitHub
  githubRepos: () => ["github-repos"] as const,
  githubBranches: (owner: string, repo: string) =>
    ["github-branches", owner, repo] as const,
} as const;

// Stale-time presets. The default 30s in QueryClient remains for anything
// unspecified; these override for data whose freshness cadence we understand.
export const staleTimes = {
  /** Deployment lists that poll every 3s for active builds. Must be shorter than refetchInterval. */
  activeDeployments: 2_000,
  /** Project lists refresh on navigation; 30s matches global default. */
  projectList: 30_000,
  /** Analytics history rarely changes mid-session. */
  analytics: 5 * 60_000,
  /** Platform info (VPS IP) never changes for a session. */
  platform: 60 * 60_000,
  /** GitHub repos/branches: short TTL to pick up new repos but avoid hammering GitHub. */
  github: 60_000,
} as const;
