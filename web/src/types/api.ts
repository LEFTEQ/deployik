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
  status: "active" | "paused" | "deleted";
  latest_deployment_id: string | null;
  latest_deployment_status: string | null;
  latest_deployment_branch: string | null;
  latest_deployment_commit_sha: string | null;
  latest_deployment_commit_message: string | null;
  latest_deployment_created_at: string | null;
  created_at: string;
  updated_at: string;
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
  domain: string;
  environment: "preview" | "production";
  is_auto: boolean;
  dns_verified: boolean;
  ssl_status: "pending" | "active" | "error";
  ssl_expires_at: string | null;
  created_at: string;
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

// Auto-build configuration
export interface AutoBuildConfig {
  enabled: boolean;
  production_branch: string;
  preview_branches: string;
  webhook_active: boolean;
  created_at: string;
  updated_at: string;
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
