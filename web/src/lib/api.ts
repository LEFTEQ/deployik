import { useAuthStore } from "@/store/auth";
import { reportNetworkError, reportNetworkSuccess } from "@/lib/network-status";
import type {
  AuthResponse,
  User,
  Organization,
  Group,
  GroupInvite,
  GroupMember,
  App,
  AppHealth,
  AppDeployment,
  AppTopology,
  AppRelease,
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
  ProjectEmailPayload,
  ProjectEmailSaveRequest,
  AutoBuildConfig,
  DeploymentListFilters,
  DeploymentListResponse,
  ProtectionStatus,
  ProtectionUpdateResponse,
  RevealPasswordResponse,
  VerifyDomainResponse,
  VolumeInfo,
  HealthResponse,
  PreviewInstance,
  RepoInspection,
  APIToken,
  CreateAPITokenRequest,
  CreateAPITokenResponse,
  CreateProjectPayload,
  UpdateAutoBuildConfigPayload,
  ProjectService,
  ServiceCredentials,
  AttachServiceRequest,
  PushSubscriptionInfo,
  PushPreferencesUpdate,
  PushSubscribePayload,
} from "@/types/api";

const API_URL = import.meta.env.VITE_API_URL || "/api";

type AutoBuildConfigResponse = Omit<
  AutoBuildConfig,
  "auto_production_enabled"
> & {
  auto_production_enabled?: boolean;
};

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
    let response: Response;
    try {
      response = await fetch(`${API_URL}${endpoint}`, {
        ...options,
        credentials: "include",
        headers: {
          ...this.getHeaders(hasBody),
          ...options.headers,
        },
      });
    } catch (err) {
      // fetch rejects (TypeError) only on network-level failure.
      reportNetworkError();
      throw err instanceof Error && err.name !== "TypeError"
        ? err
        : new Error("Network error — check your connection");
    }
    reportNetworkSuccess();

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

  async listMyTokens(): Promise<APIToken[]> {
    return this.request<APIToken[]>("/me/tokens");
  }

  async createMyToken(
    req: CreateAPITokenRequest,
  ): Promise<CreateAPITokenResponse> {
    return this.request<CreateAPITokenResponse>("/me/tokens", {
      method: "POST",
      body: JSON.stringify(req),
    });
  }

  async revokeMyToken(id: string): Promise<void> {
    await this.request<void>(`/me/tokens/${id}`, { method: "DELETE" });
  }

  // Web Push
  async getPushVapidKey(): Promise<{ public_key: string }> {
    return this.request("/push/vapid-key");
  }

  async listPushSubscriptions(): Promise<PushSubscriptionInfo[]> {
    return this.request("/push/subscriptions");
  }

  async subscribePush(
    payload: PushSubscribePayload,
  ): Promise<PushSubscriptionInfo> {
    return this.request("/push/subscriptions", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  }

  async updatePushSubscription(
    id: string,
    prefs: PushPreferencesUpdate,
  ): Promise<PushSubscriptionInfo> {
    return this.request(`/push/subscriptions/${id}`, {
      method: "PATCH",
      body: JSON.stringify(prefs),
    });
  }

  async deletePushSubscription(id: string): Promise<void> {
    await this.request<void>(`/push/subscriptions/${id}`, {
      method: "DELETE",
    });
  }

  private normalizeAutoBuildConfig(
    config: AutoBuildConfigResponse,
  ): AutoBuildConfig {
    return {
      ...config,
      auto_production_enabled: config.auto_production_enabled ?? false,
    };
  }

  async getHealth(): Promise<HealthResponse> {
    return this.request("/health", { method: "GET" }, false);
  }

  async listOrganizations(): Promise<Organization[]> {
    return this.request("/organizations");
  }

  async listGroups(): Promise<Group[]> {
    return this.request("/groups");
  }

  async createGroup(data: {
    name: string;
    project_ids?: string[];
  }): Promise<Group> {
    return this.request("/groups", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateGroup(id: string, data: { name: string }): Promise<Group> {
    return this.request(`/groups/${id}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
  }

  async deleteGroup(id: string): Promise<void> {
    await this.request<void>(`/groups/${id}`, { method: "DELETE" });
  }

  async moveProjectsToGroup(
    groupId: string,
    projectIds: string[],
  ): Promise<Group> {
    return this.request(`/groups/${groupId}/projects`, {
      method: "PUT",
      body: JSON.stringify({ project_ids: projectIds }),
    });
  }

  async listGroupMembers(groupId: string): Promise<GroupMember[]> {
    return this.request(`/groups/${groupId}/members`);
  }

  async updateGroupMember(
    groupId: string,
    userId: string,
    data: { role: "owner" | "member" },
  ): Promise<void> {
    await this.request<void>(`/groups/${groupId}/members/${userId}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
  }

  async removeGroupMember(groupId: string, userId: string): Promise<void> {
    await this.request<void>(`/groups/${groupId}/members/${userId}`, {
      method: "DELETE",
    });
  }

  async listGroupInvites(groupId: string): Promise<GroupInvite[]> {
    return this.request(`/groups/${groupId}/invites`);
  }

  async createGroupInvite(
    groupId: string,
    data: { github_username: string; role: "owner" | "member" },
  ): Promise<GroupInvite> {
    return this.request(`/groups/${groupId}/invites`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async cancelGroupInvite(groupId: string, inviteId: string): Promise<void> {
    await this.request<void>(`/groups/${groupId}/invites/${inviteId}`, {
      method: "DELETE",
    });
  }

  async listMyGroupInvites(): Promise<GroupInvite[]> {
    return this.request("/me/group-invites");
  }

  async acceptGroupInvite(inviteId: string): Promise<void> {
    await this.request<void>(`/me/group-invites/${inviteId}/accept`, {
      method: "POST",
    });
  }

  async declineGroupInvite(inviteId: string): Promise<void> {
    await this.request<void>(`/me/group-invites/${inviteId}/decline`, {
      method: "POST",
    });
  }

  // ---- App bundles ----
  async listApps(): Promise<App[]> {
    return this.request("/apps");
  }

  async createApp(data: {
    name: string;
    organization_id?: string;
    project_ids?: string[];
  }): Promise<App> {
    return this.request("/apps", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateApp(
    id: string,
    data: { name?: string; deploy_ordered?: boolean },
  ): Promise<App> {
    return this.request(`/apps/${id}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
  }

  async deleteApp(id: string): Promise<void> {
    await this.request<void>(`/apps/${id}`, { method: "DELETE" });
  }

  async getAppHealth(
    id: string,
    environment: "preview" | "production" = "production",
  ): Promise<AppHealth> {
    return this.request(`/apps/${id}/health?environment=${environment}`);
  }

  async listAppDeployments(
    appId: string,
    environment: "preview" | "production",
    limit = 20,
  ): Promise<AppDeployment[]> {
    return this.request(
      `/apps/${appId}/deployments?environment=${environment}&limit=${limit}`,
    );
  }

  async getAppTopology(
    appId: string,
    environment: "preview" | "production",
  ): Promise<AppTopology> {
    return this.request(`/apps/${appId}/topology?environment=${environment}`);
  }

  async reorderAppMembers(
    appId: string,
    projectIds: string[],
  ): Promise<Project[]> {
    return this.request(`/apps/${appId}/members/order`, {
      method: "PATCH",
      body: JSON.stringify({ project_ids: projectIds }),
    });
  }

  async addProjectsToApp(appId: string, projectIds: string[]): Promise<App> {
    return this.request(`/apps/${appId}/projects`, {
      method: "POST",
      body: JSON.stringify({ project_ids: projectIds }),
    });
  }

  async removeProjectFromApp(appId: string, projectId: string): Promise<void> {
    await this.request<void>(`/apps/${appId}/projects/${projectId}`, {
      method: "DELETE",
    });
  }

  async deployApp(
    appId: string,
    environment: "preview" | "production",
  ): Promise<{ status: string; member_count: number }> {
    return this.request(`/apps/${appId}/deploy`, {
      method: "POST",
      body: JSON.stringify({ environment }),
    });
  }

  async rollbackApp(
    appId: string,
    environment: "preview" | "production",
    releaseId: string,
  ): Promise<{ status: string }> {
    return this.request(`/apps/${appId}/rollback`, {
      method: "POST",
      body: JSON.stringify({ environment, release_id: releaseId }),
    });
  }

  async listAppReleases(
    appId: string,
    environment: "preview" | "production",
  ): Promise<AppRelease[]> {
    return this.request(`/apps/${appId}/releases?environment=${environment}`);
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

  async createProject(data: CreateProjectPayload): Promise<Project> {
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

  async listPreviewInstances(projectId: string): Promise<PreviewInstance[]> {
    return this.request(`/projects/${projectId}/preview-instances`);
  }

  async deletePreviewInstance(
    projectId: string,
    previewInstanceId: string,
    options?: { deleteVolume?: boolean },
  ): Promise<void> {
    const params = new URLSearchParams();
    if (options?.deleteVolume) params.set("delete_volume", "1");
    const query = params.toString();
    return this.request(
      `/projects/${projectId}/preview-instances/${previewInstanceId}${query ? `?${query}` : ""}`,
      { method: "DELETE" },
    );
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

  async updateDomain(
    projectId: string,
    domainId: string,
    patch: { environment?: "preview" | "production"; is_primary?: boolean },
  ): Promise<Domain> {
    return this.request(`/projects/${projectId}/domains/${domainId}`, {
      method: "PATCH",
      body: JSON.stringify(patch),
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
    return this.request(
      `/projects/${projectId}/analytics?${search.toString()}`,
    );
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
    return this.request(
      `/projects/${projectId}/analytics/verify?${search.toString()}`,
      {
        method: "POST",
      },
    );
  }

  async getProjectEmail(projectId: string): Promise<ProjectEmailPayload> {
    return this.request(`/projects/${projectId}/email`);
  }

  async saveProjectEmail(
    projectId: string,
    data: ProjectEmailSaveRequest,
  ): Promise<ProjectEmailPayload> {
    return this.request(`/projects/${projectId}/email`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async testProjectEmailSmtp(projectId: string): Promise<ProjectEmailPayload> {
    return this.request(`/projects/${projectId}/email/test-smtp`, {
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
    const config = await this.request<AutoBuildConfigResponse>(
      `/projects/${projectId}/auto-build`,
    );
    return this.normalizeAutoBuildConfig(config);
  }

  async updateAutoBuildConfig(
    projectId: string,
    data: UpdateAutoBuildConfigPayload,
  ): Promise<AutoBuildConfig> {
    const config = await this.request<AutoBuildConfigResponse>(
      `/projects/${projectId}/auto-build`,
      { method: "PUT", body: JSON.stringify(data) },
    );
    return this.normalizeAutoBuildConfig(config);
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

  async captureProjectScreenshot(
    projectId: string,
    environment: "preview" | "production",
    options?: { sync?: boolean; force?: boolean },
  ): Promise<{
    status: "ready" | "capturing" | "failed";
    deployment_id: string;
    screenshot_path?: string;
    error?: string;
  }> {
    const params = new URLSearchParams({ environment });
    if (options?.sync) params.set("sync", "1");
    if (options?.force) params.set("force", "1");
    return this.request(
      `/projects/${projectId}/screenshots/capture?${params.toString()}`,
      { method: "POST" },
    );
  }

  // Password protection
  async getProtectionStatus(projectId: string): Promise<ProtectionStatus> {
    return this.request(`/projects/${projectId}/protection`);
  }

  async updateProtection(
    projectId: string,
    data: { environment: string; enabled: boolean; password?: string },
  ): Promise<ProtectionUpdateResponse> {
    return this.request(`/projects/${projectId}/protection`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  }

  async regeneratePassword(
    projectId: string,
    data: { environment: string; password?: string },
  ): Promise<ProtectionUpdateResponse> {
    return this.request(`/projects/${projectId}/protection/regenerate`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async revealProtectionPassword(
    projectId: string,
    environment: string,
  ): Promise<RevealPasswordResponse> {
    return this.request(
      `/projects/${projectId}/protection/password?environment=${encodeURIComponent(environment)}`,
    );
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

  // ----- Services (postgres sidecar) -----

  async listServices(projectId: string): Promise<ProjectService[]> {
    return this.request<ProjectService[]>(`/projects/${projectId}/services`);
  }

  async attachService(
    projectId: string,
    body: AttachServiceRequest,
  ): Promise<ProjectService> {
    return this.request<ProjectService>(`/projects/${projectId}/services`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  }

  async detachService(projectId: string, environment: string): Promise<void> {
    await this.request<void>(`/projects/${projectId}/services/${environment}`, {
      method: "DELETE",
    });
  }

  async getServiceCredentials(
    projectId: string,
    environment: string,
  ): Promise<ServiceCredentials> {
    return this.request<ServiceCredentials>(
      `/projects/${projectId}/services/${environment}/credentials`,
    );
  }

  async regenerateServicePassword(
    projectId: string,
    environment: string,
  ): Promise<ServiceCredentials> {
    return this.request<ServiceCredentials>(
      `/projects/${projectId}/services/${environment}/regenerate-password`,
      { method: "POST" },
    );
  }

  async restartService(
    projectId: string,
    environment: string,
  ): Promise<{ status: string }> {
    return this.request<{ status: string }>(
      `/projects/${projectId}/services/${environment}/restart`,
      { method: "POST" },
    );
  }

  async resetService(
    projectId: string,
    environment: string,
    confirm: string,
  ): Promise<{ status: string }> {
    return this.request<{ status: string }>(
      `/projects/${projectId}/services/${environment}/reset`,
      {
        method: "POST",
        body: JSON.stringify({ confirm }),
      },
    );
  }

  // WebSocket URL builder
  getWebSocketUrl(path: string): string {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const wsBase = `${protocol}//${window.location.host}`;
    return `${wsBase}/ws${path}`;
  }
}

export const api = new ApiClient();
