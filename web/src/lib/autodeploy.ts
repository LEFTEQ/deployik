export function resolveAutoDeploySourceBranch(
  selectedBranch: string | undefined,
  defaultBranch: string | undefined,
): string {
  const branch = selectedBranch?.trim();
  if (branch) return branch;

  const fallback = defaultBranch?.trim();
  if (fallback) return fallback;

  return "main";
}

export function shouldEnableProductionAutoDeploy(
  autoBuildEnabled: boolean,
  autoProductionEnabled: boolean,
): boolean {
  return autoBuildEnabled && autoProductionEnabled;
}
