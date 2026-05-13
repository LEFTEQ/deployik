export class ApiError extends Error {
  readonly status: number;
  readonly endpoint: string;
  readonly body: unknown;
  readonly hint?: string;

  constructor(opts: { status: number; endpoint: string; body: unknown; message: string; hint?: string }) {
    super(opts.message);
    this.name = "ApiError";
    this.status = opts.status;
    this.endpoint = opts.endpoint;
    this.body = opts.body;
    this.hint = opts.hint;
  }
}

export function hintForStatus(status: number, endpoint: string): string | undefined {
  if (status === 401) {
    return "Token is missing, expired, or revoked. Set DEPLOYIK_TOKEN to a fresh `dpk_…` token from Account → Access tokens.";
  }
  if (status === 403) {
    return "Token is valid but the owner cannot access this resource. Check the project's workspace membership.";
  }
  if (status === 404) {
    if (endpoint.includes("/projects/")) {
      return "Project or deployment not found. Call `list_projects` to see what this token can reach, or run `init_in_repo` to bind this folder to an existing project.";
    }
    return "Resource not found at this URL.";
  }
  if (status === 409) {
    return "Conflict — the resource is in a state that blocks this operation (e.g. a volume in use by a running container, or a duplicate key).";
  }
  if (status === 429) {
    return "Rate limited. Back off and retry in a few seconds.";
  }
  if (status >= 500) {
    return "Deployik server error. Retry; if it persists, check the deployik server logs.";
  }
  return undefined;
}

export function asString(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (typeof err === "string") return err;
  try {
    return JSON.stringify(err);
  } catch {
    return String(err);
  }
}
