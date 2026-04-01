import { useAuthStore } from "@/store/auth";
import type {
  AuthResponse,
  User,
  Project,
  Deployment,
  Domain,
  EnvVariable,
  SecretVariable,
  VariableScope,
  BuildLog,
  GitHubRepo,
} from "@/types/api";

const API_URL = import.meta.env.VITE_API_URL || "/api";

class ApiClient {
  private getHeaders(hasBody = false): HeadersInit {
    const headers: HeadersInit = {};
    if (hasBody) {
      headers["Content-Type"] = "application/json";
    }
    const token = useAuthStore.getState().accessToken;
    if (token) {
      headers["Authorization"] = `Bearer ${token}`;
    }
    return headers;
  }

  private async request<T>(
    endpoint: string,
    options: RequestInit = {},
  ): Promise<T> {
    const hasBody = !!options.body;
    const response = await fetch(`${API_URL}${endpoint}`, {
      ...options,
      headers: {
        ...this.getHeaders(hasBody),
        ...options.headers,
      },
    });

    if (response.status === 401) {
      useAuthStore.getState().logout();
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

  // Auth
  async getGithubCallbackTokens(code: string): Promise<AuthResponse> {
    return this.request(
      `/auth/github/callback?code=${encodeURIComponent(code)}`,
    );
  }

  async refreshToken(refreshToken: string): Promise<AuthResponse> {
    return this.request("/auth/refresh", {
      method: "POST",
      body: JSON.stringify({ refresh_token: refreshToken }),
    });
  }

  async getMe(): Promise<User> {
    return this.request("/auth/me");
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

  // Projects
  async listProjects(): Promise<Project[]> {
    return this.request("/projects");
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
    return this.request(`/projects/${projectId}/deployments`);
  }

  async getDeployment(
    projectId: string,
    deploymentId: string,
  ): Promise<Deployment> {
    return this.request(`/projects/${projectId}/deployments/${deploymentId}`);
  }

  async triggerDeployment(
    projectId: string,
    data: { environment: string; branch?: string },
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
  ): Promise<{ dns_verified: boolean; ssl_status: string; message: string }> {
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

  // Build logs (Phase 9)
  async getBuildLogs(deploymentId: string): Promise<BuildLog[]> {
    return this.request(`/deployments/${deploymentId}/logs`);
  }

  // WebSocket URL builder
  getWebSocketUrl(path: string): string {
    const token = useAuthStore.getState().accessToken;
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const wsBase = `${protocol}//${window.location.host}`;
    return `${wsBase}/ws${path}?token=${token}`;
  }
}

export const api = new ApiClient();
