import { useAuthStore } from "@/store/auth";
import type {
  AuthResponse,
  User,
  Organization,
  Project,
  Deployment,
  Domain,
  EnvVariable,
  SecretVariable,
  VariableScope,
  BuildLog,
  GitHubRepo,
  PlatformInfo,
  AnalyticsEnvironmentFilter,
  AnalyticsRangePreset,
  ProjectAnalyticsPayload,
  AutoBuildConfig,
  DeploymentListFilters,
  DeploymentListResponse,
  ProtectionStatus,
  ProtectionUpdateResponse,
  VerifyDomainResponse,
  VolumeInfo,
  HealthResponse,
  RepoInspection,
} from "@/types/api";

const API_URL = import.meta.env.VITE_API_URL || "/api";

class ApiClient {
  private refreshPromise: Promise<void> | null = null;

  private getHeaders(hasBody = false): HeadersInit {
    const headers: HeadersInit = {};
    if (hasBody) {
      headers["Content-Type"] = "application/json";
    }
    return headers;
  }

  private async request<T>(
    endpoint: string,
    options: RequestInit = {},
    allowRefresh = true,
  ): Promise<T> {
    const hasBody = !!options.body;
    const response = await fetch(`${API_URL}${endpoint}`, {
      ...options,
      credentials: "include",
      headers: {
        ...this.getHeaders(hasBody),
        ...options.headers,
      },
    });

    if (
      response.status === 401 &&
      allowRefresh &&
      this.shouldRefresh(endpoint)
    ) {
      try {
        await this.refreshSession();
      } catch {
        useAuthStore.getState().clearAuth();
        throw new Error("Session expired");
      }
      return this.request<T>(endpoint, options, false);
    }

    if (response.status === 401) {
      useAuthStore.getState().clearAuth();
      throw new Error("Session expired");
    }

    if (!response.ok) {
      const error = await response.json().catch(() => ({}));
      throw new Error(
        error.message || error.error || `Request failed (${response.status})`,
      );
    }

    const text = await response.text();
    if (!text) return {} as T;
    return JSON.parse(text);
  }

  private shouldRefresh(endpoint: string): boolean {
    return !["/auth/refresh", "/auth/logout", "/auth/github/callback"].some(
      (path) => endpoint.startsWith(path),
    );
  }

  private async refreshSession(): Promise<void> {
    if (!this.refreshPromise) {
      this.refreshPromise = this.request<AuthResponse>(
        "/auth/refresh",
        { method: "POST" },
        false,
      )
        .then((data) => {
          useAuthStore.getState().setAuthenticated(data.user);
        })
        .finally(() => {
          this.refreshPromise = null;
        });
    }
    await this.refreshPromise;
  }

  // Auth
  async completeGithubAuth(code: string, state: string): Promise<AuthResponse> {
    return this.request(
      `/auth/github/callback?code=${encodeURIComponent(code)}&state=${encodeURIComponent(state)}`,
    );
  }

  async getMe(): Promise<User> {
    return this.request("/auth/me");
  }

  async getHealth(): Promise<HealthResponse> {
    return this.request("/health", { method: "GET" }, false);
  }

  async listOrganizations(): Promise<Organization[]> {
    return this.request("/organizations");
  }

  async getPlatformInfo(): Promise<PlatformInfo> {
    return this.request("/platform");
  }

  async logout(): Promise<void> {
    return this.request("/auth/logout", { method: "POST" }, false);
  }

  // GitHub
  async listGithubRepos(): Promise<GitHubRepo[]> {
    return this.request("/github/repos");
  }

  async listGithubBranches(
    owner: string,
    repo: string,
  ): Promise<{ name: string; commit: { sha: string } }[]> {
    return this.request(
      `/github/branches?owner=${encodeURIComponent(owner)}&repo=${encodeURIComponent(repo)}`,
    );
  }

  async inspectRepo(
    owner: string,
    repo: string,
    branch: string,
  ): Promise<RepoInspection> {
    return this.request(
      `/github/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/inspect?branch=${encodeURIComponent(branch)}`,
    );
  }

  // Projects
  async listProjects(organizationId?: string): Promise<Project[]> {
    const params = new URLSearchParams();
    if (organizationId) {
      params.set("organization_id", organizationId);
    }
    const suffix = params.toString();
    return this.request(`/projects${suffix ? `?${suffix}` : ""}`);
  }

  async getProject(id: string): Promise<Project> {
    return this.request(`/projects/${id}`);
  }

  async createProject(
    data: Partial<Project> & {
      name: string;
      github_repo: string;
      github_owner: string;
    },
  ): Promise<Project> {
    return this.request("/projects", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateProject(id: string, data: Partial<Project>): Promise<Project> {
    return this.request(`/projects/${id}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
  }

  async deleteProject(id: string): Promise<void> {
    return this.request(`/projects/${id}`, { method: "DELETE" });
  }

  // Deployments
  async listDeployments(projectId: string): Promise<Deployment[]> {
    const res = await this.request<DeploymentListResponse>(
      `/projects/${projectId}/deployments`,
    );
    return res.deployments ?? [];
  }

  async getDeployment(
    projectId: string,
    deploymentId: string,
  ): Promise<Deployment> {
    return this.request(`/projects/${projectId}/deployments/${deploymentId}`);
  }

  async triggerDeployment(
    projectId: string,
    data: {
      environment: string;
      branch?: string;
      create_tag?: boolean;
      tag_name?: string;
    },
  ): Promise<Deployment> {
    return this.request(`/projects/${projectId}/deployments`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  // Domains
  async listDomains(projectId: string): Promise<Domain[]> {
    return this.request(`/projects/${projectId}/domains`);
  }

  async addDomain(
    projectId: string,
    data: { domain: string; environment: string },
  ): Promise<Domain> {
    return this.request(`/projects/${projectId}/domains`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async deleteDomain(projectId: string, domainId: string): Promise<void> {
    return this.request(`/projects/${projectId}/domains/${domainId}`, {
      method: "DELETE",
    });
  }

  async verifyDomain(
    projectId: string,
    domainId: string,
  ): Promise<VerifyDomainResponse | { error: string }> {
    return this.request(`/projects/${projectId}/domains/${domainId}/verify`, {
      method: "POST",
    });
  }

  // Env vars
  async listEnvVars(
    projectId: string,
    environment: VariableScope,
  ): Promise<EnvVariable[]> {
    return this.request(
      `/projects/${projectId}/env?environment=${environment}`,
    );
  }

  async bulkSetEnvVars(
    projectId: string,
    data: {
      environment: VariableScope;
      variables: { key: string; value: string }[];
    },
  ): Promise<{ count: number }> {
    return this.request(`/projects/${projectId}/env`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async upsertEnvVar(
    projectId: string,
    data: { key: string; value: string; environment: string },
  ): Promise<void> {
    return this.request(`/projects/${projectId}/env`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async deleteEnvVar(
    projectId: string,
    key: string,
    environment: VariableScope,
  ): Promise<void> {
    return this.request(
      `/projects/${projectId}/env/${key}?environment=${environment}`,
      { method: "DELETE" },
    );
  }

  async listSecrets(
    projectId: string,
    environment: VariableScope,
  ): Promise<SecretVariable[]> {
    return this.request(
      `/projects/${projectId}/secrets?environment=${environment}`,
    );
  }

  async bulkSetSecrets(
    projectId: string,
    data: {
      environment: VariableScope;
      variables: { key: string; value: string }[];
    },
  ): Promise<{ count: number }> {
    return this.request(`/projects/${projectId}/secrets`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async upsertSecret(
    projectId: string,
    data: { key: string; value: string; environment: string },
  ): Promise<void> {
    return this.request(`/projects/${projectId}/secrets`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async deleteSecret(
    projectId: string,
    key: string,
    environment: VariableScope,
  ): Promise<void> {
    return this.request(
      `/projects/${projectId}/secrets/${key}?environment=${environment}`,
      { method: "DELETE" },
    );
  }

  async getProjectAnalytics(
    projectId: string,
    params: {
      environment: AnalyticsEnvironmentFilter;
      range: AnalyticsRangePreset;
      timezone: string;
    },
  ): Promise<ProjectAnalyticsPayload> {
    const search = new URLSearchParams({
      environment: params.environment,
      range: params.range,
      timezone: params.timezone,
    });
    return this.request(`/projects/${projectId}/analytics?${search.toString()}`);
  }

  async verifyProjectAnalytics(
    projectId: string,
    params: {
      environment: AnalyticsEnvironmentFilter;
      range: AnalyticsRangePreset;
      timezone: string;
    },
  ): Promise<ProjectAnalyticsPayload> {
    const search = new URLSearchParams({
      environment: params.environment,
      range: params.range,
      timezone: params.timezone,
    });
    return this.request(`/projects/${projectId}/analytics/verify?${search.toString()}`, {
      method: "POST",
    });
  }

  // Build logs (Phase 9)
  async getBuildLogs(deploymentId: string): Promise<BuildLog[]> {
    return this.request(`/deployments/${deploymentId}/logs`);
  }

  // Filtered deployment listing
  async listDeploymentsFiltered(
    projectId: string,
    filters: DeploymentListFilters,
  ): Promise<DeploymentListResponse> {
    const params = new URLSearchParams();
    if (filters.branch) params.set("branch", filters.branch);
    if (filters.environment) params.set("environment", filters.environment);
    if (filters.status) params.set("status", filters.status);
    if (filters.triggered_by) params.set("triggered_by", filters.triggered_by);
    if (filters.from) params.set("from", filters.from);
    if (filters.to) params.set("to", filters.to);
    if (filters.limit) params.set("limit", String(filters.limit));
    if (filters.offset) params.set("offset", String(filters.offset));
    const query = params.toString();
    return this.request<DeploymentListResponse>(
      `/projects/${projectId}/deployments${query ? "?" + query : ""}`,
    );
  }

  // Auto-build configuration
  async getAutoBuildConfig(projectId: string): Promise<AutoBuildConfig> {
    return this.request<AutoBuildConfig>(
      `/projects/${projectId}/auto-build`,
    );
  }

  async updateAutoBuildConfig(
    projectId: string,
    data: {
      enabled: boolean;
      production_branch: string;
      preview_branches: string;
    },
  ): Promise<AutoBuildConfig> {
    return this.request<AutoBuildConfig>(
      `/projects/${projectId}/auto-build`,
      { method: "PUT", body: JSON.stringify(data) },
    );
  }

  async deleteAutoBuildConfig(projectId: string): Promise<void> {
    return this.request(`/projects/${projectId}/auto-build`, {
      method: "DELETE",
    });
  }

  // Deployment screenshots
  getDeploymentScreenshotUrl(deploymentId: string): string {
    return `${API_URL}/deployments/${deploymentId}/screenshot`;
  }

  // Password protection
  async getProtectionStatus(projectId: string): Promise<ProtectionStatus> {
    return this.request(`/projects/${projectId}/protection`);
  }

  async updateProtection(
    projectId: string,
    data: { environment: string; enabled: boolean },
  ): Promise<ProtectionUpdateResponse> {
    return this.request(`/projects/${projectId}/protection`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async regeneratePassword(
    projectId: string,
    data: { environment: string },
  ): Promise<ProtectionUpdateResponse> {
    return this.request(`/projects/${projectId}/protection/regenerate`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  // Volumes
  async listVolumes(projectId: string): Promise<VolumeInfo[]> {
    return this.request(`/projects/${projectId}/volumes`);
  }

  async deleteVolume(
    projectId: string,
    env: "preview" | "production",
  ): Promise<void> {
    return this.request(`/projects/${projectId}/volumes/${env}`, {
      method: "DELETE",
    });
  }

  async recreateVolume(
    projectId: string,
    env: "preview" | "production",
  ): Promise<void> {
    return this.request(`/projects/${projectId}/volumes/${env}/recreate`, {
      method: "POST",
    });
  }

  // WebSocket URL builder
  getWebSocketUrl(path: string): string {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const wsBase = `${protocol}//${window.location.host}`;
    return `${wsBase}/ws${path}`;
  }
}

export const api = new ApiClient();
