import { ApiError, hintForStatus } from "./errors.js";

export interface DeployikClientOptions {
  baseUrl: string;
  token: string;
  /** Default request timeout in ms. */
  timeoutMs?: number;
  /** Optional fetch override for tests. */
  fetchImpl?: typeof fetch;
  /** User-Agent string sent with every request. */
  userAgent?: string;
}

type RequestOptions = {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  query?: Record<string, string | number | boolean | undefined | null>;
  body?: unknown;
  /** Suppress 4xx-to-error mapping; the caller wants the raw response. */
  rawError?: boolean;
};

export class DeployikClient {
  readonly baseUrl: string;
  private readonly token: string;
  private readonly timeoutMs: number;
  private readonly fetchImpl: typeof fetch;
  private readonly userAgent: string;

  constructor(opts: DeployikClientOptions) {
    this.baseUrl = opts.baseUrl.replace(/\/+$/, "");
    this.token = opts.token;
    this.timeoutMs = opts.timeoutMs ?? 30_000;
    this.fetchImpl = opts.fetchImpl ?? fetch;
    this.userAgent = opts.userAgent ?? "deployik-mcp/0.1.0";
  }

  async request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
    // Transient 5xx (commonly SQLITE_BUSY on the Deployik server during a live
    // build pipeline) are auto-retried with exponential backoff. Idempotent
    // methods (GET, PUT, DELETE) and the env/secret upsert POST endpoints are
    // safe to retry — they all use ON CONFLICT or full-set replacement.
    const method = opts.method ?? "GET";
    const maxAttempts = retryableMethod(method, path) ? 4 : 1;
    let lastErr: ApiError | undefined;
    for (let attempt = 1; attempt <= maxAttempts; attempt++) {
      try {
        return await this.requestOnce<T>(path, opts);
      } catch (err) {
        if (err instanceof ApiError && err.status >= 500 && err.status < 600 && attempt < maxAttempts) {
          lastErr = err;
          await sleep(Math.min(150 * 2 ** (attempt - 1), 1500));
          continue;
        }
        throw err;
      }
    }
    throw lastErr!;
  }

  private async requestOnce<T>(path: string, opts: RequestOptions): Promise<T> {
    const url = this.buildUrl(path, opts.query);
    const headers: Record<string, string> = {
      Authorization: `Bearer ${this.token}`,
      Accept: "application/json",
      "User-Agent": this.userAgent,
    };
    const init: RequestInit = {
      method: opts.method ?? "GET",
      headers,
    };
    if (opts.body !== undefined) {
      headers["Content-Type"] = "application/json";
      init.body = JSON.stringify(opts.body);
    }

    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), this.timeoutMs);
    init.signal = controller.signal;

    let response: Response;
    try {
      response = await this.fetchImpl(url, init);
    } catch (err) {
      clearTimeout(timeout);
      if (err instanceof Error && err.name === "AbortError") {
        throw new ApiError({
          status: 0,
          endpoint: path,
          body: null,
          message: `Request timed out after ${this.timeoutMs}ms: ${path}`,
          hint: "Set DEPLOYIK_TIMEOUT_MS to a larger value, or check that DEPLOYIK_URL is reachable from this host (VPN connected?).",
        });
      }
      throw new ApiError({
        status: 0,
        endpoint: path,
        body: null,
        message: `Network error calling ${path}: ${(err as Error).message}`,
        hint: "Check DEPLOYIK_URL and your network connection (VPN may not be active).",
      });
    }
    clearTimeout(timeout);

    const text = await response.text();
    let parsed: unknown = null;
    if (text) {
      try {
        parsed = JSON.parse(text);
      } catch {
        parsed = text;
      }
    }

    if (!response.ok) {
      if (opts.rawError) {
        // caller wants the body as-is; surface as a typed throw anyway
      }
      const errMsg =
        (parsed && typeof parsed === "object" && (parsed as Record<string, unknown>).error) ||
        (parsed && typeof parsed === "object" && (parsed as Record<string, unknown>).message) ||
        text ||
        `Request failed (${response.status})`;
      throw new ApiError({
        status: response.status,
        endpoint: path,
        body: parsed,
        message: String(errMsg),
        hint: hintForStatus(response.status, path),
      });
    }

    if (parsed === null) return undefined as T;
    return parsed as T;
  }

  buildUrl(path: string, query?: RequestOptions["query"]): string {
    const normalized = path.startsWith("/") ? path : `/${path}`;
    const base = `${this.baseUrl}${normalized.startsWith("/api/") ? normalized : `/api${normalized}`}`;
    if (!query) return base;
    const params = new URLSearchParams();
    for (const [k, v] of Object.entries(query)) {
      if (v === undefined || v === null) continue;
      params.set(k, String(v));
    }
    const qs = params.toString();
    return qs ? `${base}?${qs}` : base;
  }

  /** Build a screenshot URL (no fetch — just the URL). */
  screenshotUrl(deploymentId: string): string {
    return `${this.baseUrl}/api/deployments/${encodeURIComponent(deploymentId)}/screenshot`;
  }
}

function retryableMethod(method: string, path: string): boolean {
  // GET / PUT / DELETE are HTTP-idempotent.
  if (method === "GET" || method === "PUT" || method === "DELETE") return true;
  // The Deployik env/secret single-upsert POST endpoints are application-level
  // idempotent (UPSERT with ON CONFLICT) so retrying a transient 5xx is safe.
  if (method === "POST" && /\/env\b|\/secrets\b/.test(path) && !path.endsWith("/test-smtp")) return true;
  return false;
}

function sleep(ms: number): Promise<void> {
  return new Promise((res) => setTimeout(res, ms));
}
