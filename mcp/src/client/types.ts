// Types mirrored from web/src/types/api.ts. Kept manual to avoid coupling the
// published package to the frontend build. Keep in sync when the backend models
// move.

export interface User {
  id: string;
  github_id: number;
  username: string;
  avatar_url: string;
  role: "admin" | "user";
  created_at: string;
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

export interface Group {
  id: string;
  name: string;
  slug: string;
  is_default: boolean;
  personal_owner_user_id?: string;
  membership_role: "owner" | "member";
  project_count: number;
  display_order: number;
  created_at: string;
  updated_at: string;
}

export interface GroupMember {
  group_id: string;
  user_id: string;
  username: string;
  avatar_url: string;
  role: "owner" | "member";
  created_at: string;
}

export interface GroupInvite {
  id: string;
  group_id: string;
  group_name?: string;
  github_username: string;
  role: "owner" | "member";
  invited_by_user_id: string;
  invited_by_username?: string;
  status: "pending" | "accepted" | "declined" | "canceled";
  responded_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface PlatformInfo {
  dns_target_ip: string;
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
  host_network_access?: boolean;
  data_volume_enabled?: boolean;
  data_mount_path?: string;
  start_command?: string;
  health_path?: string;
  resource_tier?: ResourceTier;
  build_filter_enabled?: boolean;
  watch_paths?: string[];
  auto_build_enabled?: boolean;
  auto_production_enabled?: boolean;
}

export type Environment = "preview" | "production";
export type VariableScope = "shared" | "preview" | "production";
export type ProjectVariableKind = "env" | "secret";

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
  environment: Environment;
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

export interface DeploymentListResponse {
  deployments: Deployment[];
  total: number;
}

export interface DeploymentListFilters {
  branch?: string;
  environment?: Environment;
  status?: string;
  triggered_by?: string;
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
}

export interface BuildLog {
  id: number;
  deployment_id: string;
  line_number: number;
  content: string;
  stream: "stdout" | "stderr";
  timestamp: string;
}

export interface Domain {
  id: string;
  project_id: string;
  preview_instance_id?: string;
  domain: string;
  environment: Environment;
  is_auto: boolean;
  is_primary: boolean;
  dns_verified: boolean;
  ssl_status: "pending" | "active" | "error";
  ssl_expires_at: string | null;
  created_at: string;
}

export interface VerifyDomainResponse {
  status: "verifying";
  domain_id: string;
}

export interface ProjectVariable {
  id: string;
  project_id: string;
  environment: VariableScope;
  kind: ProjectVariableKind;
  key: string;
  value: string;
  created_at: string;
  updated_at: string;
}

export type EnvVariable = ProjectVariable;
export type SecretVariable = ProjectVariable;

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
  password?: string;
}

export interface VolumeInfo {
  environment: Environment;
  name: string;
  exists: boolean;
  size_bytes: number;
  created_at: string | null;
  mount_path: string;
  in_use: boolean;
}

// ----- Services (postgres sidecar in v1) -----

export type ServiceType = "postgres";
export type ServiceStatus = "pending" | "running" | "stopped" | "failed";

export interface ProjectService {
  id: string;
  project_id: string;
  environment: Environment;
  type: ServiceType;
  image: string;
  db_name: string;
  db_user: string;
  host_port: number;
  status: ServiceStatus;
  last_started_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface ServiceCredentials {
  db_name: string;
  db_user: string;
  password: string;
  internal_host: string;
  internal_port: number;
  vps_loopback_port: number;
  ssh_tunnel_cmd: string;
}

export interface AttachServiceRequest {
  environment: Environment;
  type: ServiceType;
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

export interface GitHubBranch {
  name: string;
  commit: { sha: string };
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

export interface APIToken {
  id: string;
  user_id: string;
  name: string;
  last_used_at: string | null;
  expires_at: string | null;
  revoked_at: string | null;
  created_at: string;
}

export interface CreateAPITokenResponse {
  id: string;
  name: string;
  token: string;
}

export type AnalyticsEnvironmentFilter = "all" | "preview" | "production";
export type AnalyticsRangePreset = "1h" | "24h" | "7d" | "30d";

export interface ProjectAnalyticsPayload {
  environment: AnalyticsEnvironmentFilter;
  range: AnalyticsRangePreset;
  timezone: string;
  // remainder of fields are passed through as `unknown` to the AI — the analytics
  // payload is large and shape-y enough that mirroring every nested field adds
  // maintenance without value. The MCP returns it verbatim.
  [extra: string]: unknown;
}

export interface ProjectEmailPayload {
  settings: Record<string, unknown>;
  status: { configured: boolean; required: Record<string, unknown> };
  install: { ai_prompt: string; env_keys: string[] };
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
