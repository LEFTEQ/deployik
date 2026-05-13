// Normalize user-friendly English inputs into the strict values the Deployik
// backend expects. Non-technical users say "prod", "live", "main site" — the
// AI relays those literally. Match them charitably here so a single tool call
// works without a clarifying round-trip.

export type Environment = "preview" | "production";
export type VariableScope = "shared" | "preview" | "production";

const ENV_ALIASES: Record<string, Environment> = {
  production: "production",
  prod: "production",
  live: "production",
  "live site": "production",
  "main site": "production",
  main: "production",
  release: "production",
  public: "production",
  preview: "preview",
  prev: "preview",
  staging: "preview",
  stage: "preview",
  dev: "preview",
  development: "preview",
  test: "preview",
  testing: "preview",
  draft: "preview",
};

const SCOPE_ALIASES: Record<string, VariableScope> = {
  ...ENV_ALIASES,
  shared: "shared",
  both: "shared",
  all: "shared",
  everywhere: "shared",
  common: "shared",
  global: "shared",
};

export function normalizeEnvironment(input: string | undefined): Environment | undefined {
  if (!input) return undefined;
  const key = input.toLowerCase().trim();
  return ENV_ALIASES[key];
}

export function normalizeScope(input: string | undefined): VariableScope | undefined {
  if (!input) return undefined;
  const key = input.toLowerCase().trim();
  return SCOPE_ALIASES[key];
}

/** When the AI passes a fuzzy environment we want to honor it; throw if no match. */
export function requireEnvironment(input: string): Environment {
  const normalized = normalizeEnvironment(input);
  if (!normalized) {
    throw new Error(
      `Unknown environment '${input}'. Use 'preview' (test / staging / dev) or 'production' (live / public).`,
    );
  }
  return normalized;
}

export function requireScope(input: string): VariableScope {
  const normalized = normalizeScope(input);
  if (!normalized) {
    throw new Error(
      `Unknown scope '${input}'. Use 'shared' (both environments), 'preview' (test only), or 'production' (live only).`,
    );
  }
  return normalized;
}
