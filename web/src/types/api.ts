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
