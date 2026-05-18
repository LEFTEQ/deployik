import { formatDistance } from "date-fns";
import type {
  Deployment,
  DeploymentStatus,
  Domain,
  ResourceTier,
  VariableScope,
} from "@/types/api";

/**
 * Resource tier metadata for the project Resources picker. Numeric values
 * MUST match `internal/build/tiers.go`. A frontend unit test cross-checks
 * the keys against the backend's known set so a drift in either direction
 * fails CI before it reaches production.
 */
export const RESOURCE_TIER_META = {
  nano: {
    label: "Nano",
    description: "Static sites and very light pages.",
    memoryMB: 256,
    cpuCores: 0.5,
    buildMemoryMB: 1536,
    buildCpuCores: 1.0,
    badgeClass: "border-white/10 bg-white/5 text-slate-200",
  },
  small: {
    label: "Small",
    description: "Default. Most marketing sites and small apps.",
    memoryMB: 512,
    cpuCores: 1.0,
    buildMemoryMB: 2048,
    buildCpuCores: 2.0,
    badgeClass: "border-sky-400/25 bg-sky-400/12 text-sky-100",
  },
  medium: {
    label: "Medium",
    description: "Real Next.js apps with moderate traffic.",
    memoryMB: 1024,
    cpuCores: 2.0,
    buildMemoryMB: 3072,
    buildCpuCores: 2.0,
    badgeClass: "border-violet-400/25 bg-violet-400/12 text-violet-100",
  },
  large: {
    label: "Large",
    description: "Heavy workloads with bigger Node heaps.",
    memoryMB: 2048,
    cpuCores: 2.0,
    buildMemoryMB: 4096,
    buildCpuCores: 2.0,
    badgeClass: "border-amber-400/25 bg-amber-400/12 text-amber-100",
  },
} satisfies Record<
  ResourceTier,
  {
    label: string;
    description: string;
    memoryMB: number;
    cpuCores: number;
    buildMemoryMB: number;
    buildCpuCores: number;
    badgeClass: string;
  }
>;

export const RESOURCE_TIER_ORDER: ResourceTier[] = [
  "nano",
  "small",
  "medium",
  "large",
];

/**
 * Pretty memory render: 1024 → "1 GB", 512 → "512 MB".
 */
export function formatTierMemory(memoryMB: number): string {
  if (memoryMB >= 1024 && memoryMB % 1024 === 0) {
    return `${memoryMB / 1024} GB`;
  }
  return `${memoryMB} MB`;
}

/**
 * Deployment statuses that indicate the deployment is still in progress.
 */
export const ACTIVE_DEPLOYMENT_STATUSES = new Set<DeploymentStatus>([
  "queued",
  "building",
  "deploying",
]);

/**
 * Visual metadata for each deployment status (label, badge class, dot class).
 */
export const DEPLOYMENT_STATUS_META = {
  queued: {
    label: "Queued",
    badgeClass: "border-white/10 bg-white/5 text-slate-200",
    dotClass: "bg-slate-400",
  },
  building: {
    label: "Building",
    badgeClass: "border-amber-400/25 bg-amber-400/12 text-amber-100",
    dotClass: "bg-amber-400",
  },
  deploying: {
    label: "Deploying",
    badgeClass: "border-sky-400/25 bg-sky-400/12 text-sky-100",
    dotClass: "bg-sky-400",
  },
  live: {
    label: "Live",
    badgeClass: "border-emerald-400/25 bg-emerald-400/12 text-emerald-100",
    dotClass: "bg-emerald-400",
  },
  failed: {
    label: "Failed",
    badgeClass: "border-rose-400/25 bg-rose-400/12 text-rose-100",
    dotClass: "bg-rose-400",
  },
  rolled_back: {
    label: "Rolled back",
    badgeClass: "border-orange-400/25 bg-orange-400/12 text-orange-100",
    dotClass: "bg-orange-400",
  },
  replaced: {
    label: "Replaced",
    badgeClass: "border-white/10 bg-white/5 text-slate-200",
    dotClass: "bg-slate-500",
  },
} satisfies Record<
  DeploymentStatus,
  { label: string; badgeClass: string; dotClass: string }
>;

/**
 * Visual metadata for deployment environments (preview / production).
 */
export const ENVIRONMENT_META = {
  preview: {
    label: "Preview",
    description: "Auto preview URLs and branded staging domains.",
    badgeClass:
      "border-sky-400/25 bg-sky-400/12 text-sky-100 shadow-[inset_0_0_0_1px_rgba(56,189,248,0.12)]",
  },
  production: {
    label: "Production",
    description: "The real customer-facing domain you bought.",
    badgeClass:
      "border-emerald-400/25 bg-emerald-400/12 text-emerald-100 shadow-[inset_0_0_0_1px_rgba(16,185,129,0.12)]",
  },
} satisfies Record<
  Domain["environment"],
  { label: string; description: string; badgeClass: string }
>;

/**
 * Visual metadata for variable scopes (shared / preview / production).
 */
export const VARIABLE_SCOPE_META = {
  shared: {
    label: "Shared",
    description:
      "Applied to both preview and production unless a scoped value overrides it.",
    badgeClass:
      "border-fuchsia-400/25 bg-fuchsia-400/12 text-fuchsia-100 shadow-[inset_0_0_0_1px_rgba(217,70,239,0.12)]",
  },
  preview: {
    label: "Preview",
    description: "Only used for preview deployments.",
    badgeClass: ENVIRONMENT_META.preview.badgeClass,
  },
  production: {
    label: "Production",
    description: "Only used for production deployments.",
    badgeClass: ENVIRONMENT_META.production.badgeClass,
  },
} satisfies Record<
  VariableScope,
  { label: string; description: string; badgeClass: string }
>;

// ---------------------------------------------------------------------------
// Domain helpers
// ---------------------------------------------------------------------------

/** Returns true when a domain has verified DNS and active SSL. */
export function isDomainReady(domain: Domain): boolean {
  return domain.dns_verified && domain.ssl_status === "active";
}

/** Filters domains by environment. */
export function getEnvironmentDomains(
  domains: Domain[] | undefined,
  environment: Domain["environment"],
  previewInstanceId?: string,
): Domain[] {
  return (domains ?? []).filter((domain) => {
    if (domain.environment !== environment) return false;
    if (environment === "preview" && previewInstanceId) {
      return domain.preview_instance_id === previewInstanceId;
    }
    return true;
  });
}

/** Filters domains that are both in the given environment and ready. */
export function getReadyEnvironmentDomains(
  domains: Domain[] | undefined,
  environment: Domain["environment"],
  previewInstanceId?: string,
): Domain[] {
  return getEnvironmentDomains(domains, environment, previewInstanceId).filter(
    isDomainReady,
  );
}

/**
 * Returns the preferred domain for an environment, even if DNS/SSL is not
 * ready yet. Preview prefers auto domains; production prefers custom domains.
 */
export function getPreferredEnvironmentDomain(
  domains: Domain[] | undefined,
  environment: Domain["environment"],
  previewInstanceId?: string,
): Domain | null {
  const environmentDomains = getEnvironmentDomains(
    domains,
    environment,
    previewInstanceId,
  );
  if (!environmentDomains.length) return null;

  const explicitPrimary = environmentDomains.find((domain) => domain.is_primary);
  if (explicitPrimary) {
    return explicitPrimary;
  }

  const fallback =
    environmentDomains.find((domain) =>
      environment === "preview" ? domain.is_auto : !domain.is_auto,
    ) ?? environmentDomains[0];

  return fallback ?? null;
}

/**
 * Returns the primary HTTPS URL for an environment, using only domains with
 * verified DNS and active SSL.
 */
export function getPrimaryEnvironmentUrl(
  domains: Domain[] | undefined,
  environment: Domain["environment"],
  previewInstanceId?: string,
): string | null {
  const domain = getPreferredEnvironmentDomain(
    getReadyEnvironmentDomains(domains, environment, previewInstanceId),
    environment,
    previewInstanceId,
  );

  return domain ? `https://${domain.domain}` : null;
}

// ---------------------------------------------------------------------------
// Deployment helpers
// ---------------------------------------------------------------------------

/** Returns the latest deployment for a given environment (first match). */
export function getLatestEnvironmentDeployment(
  deployments: Deployment[] | undefined,
  environment: Deployment["environment"],
): Deployment | undefined {
  return (deployments ?? []).find(
    (deployment) => deployment.environment === environment,
  );
}

/** Returns the latest live deployment for a given environment. */
export function getLatestLiveEnvironmentDeployment(
  deployments: Deployment[] | undefined,
  environment: Deployment["environment"],
): Deployment | undefined {
  return (deployments ?? []).find(
    (deployment) =>
      deployment.environment === environment && deployment.status === "live",
  );
}

// ---------------------------------------------------------------------------
// Formatting helpers
// ---------------------------------------------------------------------------

/** Formats a date string as a human-readable relative time (e.g. "5 minutes ago"). */
export function formatRelativeDateFrom(
  value: string,
  baseDate: Date = new Date(),
): string {
  return formatDistance(new Date(value), baseDate, { addSuffix: true });
}

/** Formats a date string relative to the current time. */
export function formatRelativeDate(value: string): string {
  return formatRelativeDateFrom(value);
}

/** Formats a date string as a local absolute timestamp for titles/tooltips. */
export function formatDateTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;

  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

/** Builds a release tag name like `release-20260405-1430`. */
export function buildReleaseTagName(): string {
  const now = new Date();
  const pad = (value: number) => value.toString().padStart(2, "0");

  return [
    "release",
    `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}`,
    `${pad(now.getHours())}${pad(now.getMinutes())}`,
  ].join("-");
}

/** Formats a number in compact notation (e.g. 1200 -> "1.2K"). */
export function formatCompactNumber(value: number): string {
  return new Intl.NumberFormat("en", {
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(value);
}
