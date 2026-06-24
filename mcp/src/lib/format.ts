import type { Project, Deployment, Domain, VolumeInfo, ProtectionStatus } from "../client/types.js";

export function renderDryRun(dryRun: { tool: string; tier: string; willDo: Record<string, unknown>; nextCall: Record<string, unknown> }): string {
  const lines = [
    `DRY RUN — \`${dryRun.tool}\` (${dryRun.tier})`,
    "",
    "Impact:",
    ...Object.entries(dryRun.willDo).map(([k, v]) => `  • ${k}: ${formatValue(v)}`),
    "",
    "To proceed, call this tool again with:",
    `  ${JSON.stringify(dryRun.nextCall)}`,
  ];
  return lines.join("\n");
}

export function renderProjectSummary(project: Project): string {
  const lines = [
    `Project: ${project.name} (id: ${project.id})`,
    `  Group:        ${project.organization_name ?? project.organization_id}`,
    `  Repo:         ${project.github_owner}/${project.github_repo} @ ${project.branch}`,
    `  Framework:    ${project.framework} (${project.package_manager}, node ${project.node_version})`,
    `  Status:       ${project.status}`,
    `  Resource:     ${project.resource_tier}`,
    `  Created:      ${project.created_at}`,
  ];
  if (project.latest_deployment_id) {
    lines.push(
      `  Latest:       ${project.latest_deployment_environment ?? "?"} · ${project.latest_deployment_status ?? "?"} (${project.latest_deployment_id})`,
    );
  }
  return lines.join("\n");
}

export function renderDeploymentSummary(deployment: Deployment): string {
  return [
    `Deployment ${deployment.id}`,
    `  Project:     ${deployment.project_id}`,
    `  Environment: ${deployment.environment}`,
    `  Branch:      ${deployment.branch}`,
    `  Status:      ${deployment.status}`,
    `  Commit:      ${deployment.commit_sha.slice(0, 8)} — ${truncate(deployment.commit_message, 80)}`,
    `  Triggered:   ${deployment.trigger_source} · ${deployment.triggered_by_username || deployment.triggered_by}`,
    `  Created:     ${deployment.created_at}${deployment.finished_at ? ` → finished ${deployment.finished_at}` : ""}`,
    deployment.error_message ? `  Error:       ${deployment.error_message}` : "",
  ]
    .filter(Boolean)
    .join("\n");
}

export function renderDomainsList(domains: Domain[]): string {
  if (domains.length === 0) return "(no domains)";
  return domains
    .map((d) => {
      const flags = [
        d.is_primary ? "primary" : null,
        d.is_auto ? "auto" : null,
        d.dns_verified ? "dns-ok" : "dns-pending",
        `ssl:${d.ssl_status}`,
      ]
        .filter(Boolean)
        .join(" ");
      return `  • ${d.domain}  [${d.environment}]  ${flags}`;
    })
    .join("\n");
}

export function renderVolumesList(volumes: VolumeInfo[]): string {
  if (volumes.length === 0) return "(no volumes)";
  return volumes
    .map((v) => {
      const size = v.exists ? formatBytes(v.size_bytes) : "(missing)";
      const inUse = v.in_use ? "in-use" : "idle";
      return `  • ${v.name}  [${v.environment}]  ${size}  ${inUse}  mount=${v.mount_path}`;
    })
    .join("\n");
}

export function renderProtection(status: ProtectionStatus): string {
  const lines = [
    `  Preview:    ${status.preview_enabled ? "enabled" : "disabled"}`,
    `  Production: ${status.production_enabled ? "enabled" : "disabled"}`,
  ];
  if (status.preview_bypass_url) lines.push(`  Preview bypass:    ${status.preview_bypass_url}`);
  if (status.production_bypass_url) lines.push(`  Production bypass: ${status.production_bypass_url}`);
  return lines.join("\n");
}

function truncate(s: string, n: number): string {
  if (s.length <= n) return s;
  return `${s.slice(0, n - 1)}…`;
}

function formatValue(v: unknown): string {
  if (typeof v === "string") return v;
  return JSON.stringify(v);
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KiB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MiB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GiB`;
}
