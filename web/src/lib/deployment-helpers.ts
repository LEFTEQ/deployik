import { formatDistanceToNow } from "date-fns";
import type {
  Deployment,
  DeploymentStatus,
  Domain,
  VariableScope,
} from "@/types/api";

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
): Domain[] {
  return (domains ?? []).filter((domain) => domain.environment === environment);
}

/** Filters domains that are both in the given environment and ready. */
export function getReadyEnvironmentDomains(
  domains: Domain[] | undefined,
  environment: Domain["environment"],
): Domain[] {
  return getEnvironmentDomains(domains, environment).filter(isDomainReady);
}

/**
 * Returns the primary HTTPS URL for an environment, preferring auto domains
 * for preview and custom domains for production.
 */
export function getPrimaryEnvironmentUrl(
  domains: Domain[] | undefined,
  environment: Domain["environment"],
): string | null {
  const readyDomains = getReadyEnvironmentDomains(domains, environment);
  if (!readyDomains.length) return null;

  const explicitPrimary = readyDomains.find((domain) => domain.is_primary);
  if (explicitPrimary) {
    return `https://${explicitPrimary.domain}`;
  }

  const fallback =
    readyDomains.find((domain) =>
      environment === "preview" ? domain.is_auto : !domain.is_auto,
    ) ?? readyDomains[0];
  if (!fallback) return null;

  return `https://${fallback.domain}`;
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
export function formatRelativeDate(value: string): string {
  return formatDistanceToNow(new Date(value), { addSuffix: true });
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
