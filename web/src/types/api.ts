export interface User {
  id: string;
  github_id: number;
  username: string;
  avatar_url: string;
  role: "admin" | "user";
  created_at: string;
}

export interface AuthResponse {
  user: User;
}

export interface APIToken {
  id: string;
  user_id: string;
  name: string;
  last_used_at: string | null;
  expires_at: string | null;
  revoked_at: string | null;
  created_at: string;
}

export interface CreateAPITokenRequest {
  name: string;
}

export interface CreateAPITokenResponse {
  id: string;
  name: string;
  /** Raw token value — shown to the user once at creation, never stored. */
  token: string;
}

export interface Organization {
  id: string;
  name: string;
  slug: string;
  is_personal: boolean;
  personal_owner_user_id?: string;
  membership_role: "owner" | "member";
  project_count: number;
  created_at: string;
  updated_at: string;
}

export interface PlatformInfo {
  dns_target_ip: string;
}

/**
 * Resource tier identifiers — mirrors `internal/build/tiers.go`. Backend is the
 * source of truth; the table in `lib/deployment-helpers.ts` carries the
 * matching UI metadata (RAM / CPU / labels) and must stay in sync.
 */
export type ResourceTier = "nano" | "small" | "medium" | "large";

export interface Project {
  id: string;
  name: string;
  github_repo: string;
  github_owner: string;
  branch: string;
  user_id: string;
  organization_id: string;
  organization_name?: string;
  framework: string;
  package_manager: string;
  root_directory: string;
  output_directory: string;
  build_command: string;
  install_command: string;
  node_version: string;
  port: number;
  host_network_access: boolean;
  data_volume_enabled: boolean;
  data_mount_path: string;
  resource_tier: ResourceTier;
  start_command: string;
  health_path: string;
  status: "active" | "paused" | "deleted";
  latest_deployment_id: string | null;
  latest_deployment_status: string | null;
  latest_deployment_branch: string | null;
  latest_deployment_commit_sha: string | null;
  latest_deployment_commit_message: string | null;
  latest_deployment_created_at: string | null;
  latest_deployment_environment: string | null;
  latest_deployment_screenshot_path: string | null;
  latest_preview_deploy_at: string | null;
  latest_production_deploy_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface CreateProjectPayload {
  organization_id?: string;
  name: string;
  github_repo: string;
  github_owner: string;
  branch: string;
  framework: string;
  package_manager: string;
  root_directory: string;
  output_directory: string;
  build_command: string;
  install_command: string;
  node_version: string;
  port: number;
  start_command?: string;
  health_path?: string;
  host_network_access?: boolean;
  data_volume_enabled?: boolean;
  data_mount_path?: string;
  resource_tier?: ResourceTier;
  auto_build_enabled?: boolean;
  auto_production_enabled?: boolean;
}

export type DeploymentStatus =
  | "queued"
  | "building"
  | "deploying"
  | "live"
  | "failed"
  | "rolled_back"
  | "replaced";

export interface Deployment {
  id: string;
  project_id: string;
  environment: "preview" | "production";
  preview_instance_id?: string;
  commit_sha: string;
  commit_message: string;
  branch: string;
  status: DeploymentStatus;
  container_id: string;
  container_name: string;
  image_tag: string;
  build_duration: number;
  triggered_by: string;
  trigger_source: "manual" | "webhook" | "api";
  triggered_by_username: string;
  screenshot_path: string | null;
  error_message?: string;
  created_at: string;
  finished_at: string | null;
}

export interface Domain {
  id: string;
  project_id: string;
  preview_instance_id?: string;
  domain: string;
  environment: "preview" | "production";
  is_auto: boolean;
  is_primary: boolean;
  dns_verified: boolean;
  ssl_status: "pending" | "active" | "error";
  ssl_expires_at: string | null;
  created_at: string;
}

export interface PreviewInstance {
  id: string;
  project_id: string;
  branch: string;
  branch_slug: string;
  is_default: boolean;
  status: "active" | "deleted";
  domain: string;
  latest_deployment_id?: string | null;
  latest_deployment_status?: DeploymentStatus | null;
  latest_deployment_commit_sha?: string | null;
  latest_deployment_commit_message?: string | null;
  latest_deployment_created_at?: string | null;
  latest_deployment_screenshot_path?: string | null;
  created_at: string;
  updated_at: string;
}

export interface DomainLogEvent {
  deployment_id: string; // actually "domain:{id}" topic key
  line_number: number;
  content: string;
  stream: string; // "{step}:{status}" e.g. "dns:success", "ssl:running", "done:error"
}

export interface VerifyDomainResponse {
  status: "verifying";
  domain_id: string;
}

export type VariableScope = "shared" | "preview" | "production";

export type ProjectVariableKind = "env" | "secret";

export interface ProjectVariable {
  id: string;
  project_id: string;
  environment: VariableScope;
  kind: ProjectVariableKind;
  key: string;
  value: string; // masked in responses
  created_at: string;
  updated_at: string;
}

export type EnvVariable = ProjectVariable;
export type SecretVariable = ProjectVariable;

export interface BuildLog {
  id: number;
  deployment_id: string;
  line_number: number;
  content: string;
  stream: "stdout" | "stderr";
  timestamp: string;
}

export interface GitHubRepo {
  id: number;
  full_name: string;
  name: string;
  owner: { login: string; avatar_url: string };
  private: boolean;
  default_branch: string;
  language: string | null;
  updated_at: string;
}

export type AnalyticsEnvironmentFilter = "all" | "preview" | "production";
export type AnalyticsRangePreset = "1h" | "24h" | "7d" | "30d";

export interface AnalyticsDomainGroups {
  all: string[];
  preview: string[];
  production: string[];
}

export interface AnalyticsTimePoint {
  timestamp: string;
  value: number;
}

export interface AnalyticsBreakdownItem {
  name: string;
  value: number;
  pageviews?: number;
  visitors?: number;
  visits?: number;
  bounces?: number;
  total_visit_duration_ms?: number;
}

export interface AudienceInstallPayload {
  host_url: string;
  script_url: string;
  snippet: string;
  ai_prompt: string;
  domains: AnalyticsDomainGroups;
}

export interface AudienceSummary {
  visitors: number;
  pageviews: number;
  visits: number;
  bounces: number;
  bounce_rate: number;
  avg_visit_duration_ms: number;
  total_visit_duration_ms: number;
}

export interface RealtimeSummary {
  views: number;
  visitors: number;
  events: number;
}

export interface AudienceSeries {
  pageviews: AnalyticsTimePoint[];
  visits: AnalyticsTimePoint[];
}

export interface AudienceAnalyticsPayload {
  available: boolean;
  enabled: boolean;
  tracking_mode: string;
  status: string;
  website_id: string;
  website_name: string;
  open_url: string;
  verified_at?: string | null;
  last_event_at?: string | null;
  error?: string;
  install: AudienceInstallPayload;
  summary: AudienceSummary;
  realtime?: RealtimeSummary | null;
  series: AudienceSeries;
  top_pages: AnalyticsBreakdownItem[];
  top_referrers: AnalyticsBreakdownItem[];
  top_countries: AnalyticsBreakdownItem[];
}

export interface RuntimeSummary {
  requests: number;
  api_requests: number;
  bandwidth_bytes: number;
  error_rate: number;
  p95_latency_ms: number;
}

export interface RuntimeSeries {
  requests: AnalyticsTimePoint[];
  api_requests: AnalyticsTimePoint[];
  bandwidth: AnalyticsTimePoint[];
  p95_latency_ms: AnalyticsTimePoint[];
}

export interface RuntimeAnalyticsPayload {
  available: boolean;
  error?: string;
  summary: RuntimeSummary;
  series: RuntimeSeries;
  top_paths: AnalyticsBreakdownItem[];
  status_codes: AnalyticsBreakdownItem[];
}

export interface ProjectAnalyticsPayload {
  environment: AnalyticsEnvironmentFilter;
  range: AnalyticsRangePreset;
  timezone: string;
  domains: AnalyticsDomainGroups;
  audience: AudienceAnalyticsPayload;
  runtime: RuntimeAnalyticsPayload;
}

export type ProjectEmailStatus =
  | "not_configured"
  | "ready_to_install"
  | "smtp_tested"
  | "error";

export interface ProjectEmailSettings {
  project_id: string;
  provider: "webglobe" | "smtp";
  smtp_host: string;
  smtp_port: number;
  smtp_security: "starttls" | "tls" | "none";
  smtp_user: string;
  email_from: string;
  email_from_name: string;
  contact_email_to: string;
  recaptcha_site_key: string;
  recaptcha_mode: "v3";
  recaptcha_score_threshold: number;
  status: ProjectEmailStatus;
  last_tested_at?: string | null;
  last_test_error?: string;
}

export interface ProjectEmailRequiredStatus {
  env_missing: boolean;
  secrets_missing: boolean;
  missing_env: string[] | null;
  missing_secrets: string[] | null;
}

export interface ProjectEmailPayload {
  settings: ProjectEmailSettings;
  status: {
    configured: boolean;
    required: ProjectEmailRequiredStatus;
  };
  install: {
    ai_prompt: string;
    env_keys: string[];
  };
}

export interface ProjectEmailSaveRequest {
  provider: "webglobe" | "smtp";
  smtp_host: string;
  smtp_port: number;
  smtp_security: "starttls" | "tls" | "none";
  smtp_user: string;
  smtp_password?: string;
  email_from: string;
  email_from_name: string;
  contact_email_to: string;
  recaptcha_site_key: string;
  recaptcha_secret_key?: string;
  recaptcha_score_threshold: number;
}

// Auto-build configuration
export interface AutoBuildConfig {
  enabled: boolean;
  production_branch: string;
  preview_branches: string;
  auto_production_enabled: boolean;
  webhook_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface UpdateAutoBuildConfigPayload {
  enabled: boolean;
  production_branch: string;
  preview_branches: string;
  auto_production_enabled: boolean;
}

export interface ProtectionStatus {
  preview_enabled: boolean;
  production_enabled: boolean;
}

export interface ProtectionUpdateResponse {
  environment: string;
  enabled: boolean;
  password?: string; // only present when enabling or regenerating
}

// Deployment list query filters
export interface DeploymentListFilters {
  branch?: string;
  environment?: "preview" | "production";
  status?: string;
  triggered_by?: string;
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
}

// Paginated deployment list response
export interface DeploymentListResponse {
  deployments: Deployment[];
  total: number;
}

// Volume info
export interface VolumeInfo {
  environment: "preview" | "production";
  name: string;
  exists: boolean;
  size_bytes: number;
  created_at: string | null;
  mount_path: string;
  in_use: boolean;
}

export interface VersionInfo {
  git_sha: string;
  git_sha_full: string;
  build_time: string;
  gh_repo: string;
  gh_run_id: string;
  commit_url: string;
  run_url: string;
}

export interface HealthResponse {
  status: "ok";
  version?: VersionInfo;
}

export type MonorepoTooling = "turborepo" | "nx";
export type MonorepoFramework = "nextjs" | "vite" | "astro" | "static";
export type MonorepoPackageManager = "auto" | "bun" | "pnpm" | "npm" | "yarn";

export interface MonorepoApp {
  name: string;
  path: string;
  framework: MonorepoFramework;
  output_directory: string;
  suggested_build_command: string;
  buildable: boolean;
}

export interface RepoInspection {
  is_monorepo: boolean;
  package_manager: MonorepoPackageManager;
  tooling: MonorepoTooling[];
  apps: MonorepoApp[];
  truncated: boolean;
}
