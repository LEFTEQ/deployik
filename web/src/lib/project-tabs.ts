export const PROJECT_TABS = [
  "overview",
  "deployments",
  "analytics",
  "integration",
  "settings",
] as const;

export type ProjectTabValue = (typeof PROJECT_TABS)[number];

export function normalizeProjectTab(value: unknown): ProjectTabValue {
  if (typeof value !== "string") {
    return "overview";
  }

  return (PROJECT_TABS as readonly string[]).includes(value)
    ? (value as ProjectTabValue)
    : "overview";
}

export function normalizeDeploymentReturnTab(value: unknown): ProjectTabValue {
  if (typeof value !== "string") {
    return "deployments";
  }

  return (PROJECT_TABS as readonly string[]).includes(value)
    ? (value as ProjectTabValue)
    : "deployments";
}
