import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { registerTool, type ToolContext } from "./_helpers.js";
import { resolveProject, fetchProjects } from "../resolve/project.js";
import { checkSafety } from "../lib/safety.js";
import { pollUntil, sleep } from "../lib/poll.js";
import { renderDryRun, renderProjectSummary, renderDeploymentSummary, renderDomainsList, renderVolumesList, renderProtection } from "../lib/format.js";
import { formatLogs } from "../lib/logs.js";
import { writeBinding, ensureGitignore, readBinding } from "../config/binding.js";
import { requireEnvironment, requireScope } from "../lib/normalize.js";
import { resolveGroupId } from "../lib/groups.js";
import { buildCreateProjectPayload } from "./projects.js";
import type {
  Project,
  Deployment,
  Domain,
  VolumeInfo,
  ProtectionStatus,
  BuildLog,
  User,
  RepoInspection,
} from "../client/types.js";

// Accept any string so we can normalize 'prod'/'live'/'staging' charitably.
const ENV = z.string().describe("'preview' (test) or 'production' (live). Also accepts: prod, live, staging, dev, test.");
const VAR_SCOPE = z.string().describe("'shared' (both envs), 'preview' (test only), or 'production' (live only). Also accepts: both, all, prod, live, staging.");

const TERMINAL_STATUSES = new Set(["live", "failed", "replaced", "rolled_back"]);

export function registerWorkflowTools(server: McpServer, ctx: ToolContext): void {
  registerTool(server, ctx, {
    name: "init_in_repo",
    description:
      "Bootstrap this repo: write `.deployik.json` (project + dashboard group), append `.deployik/` to .gitignore, and validate the token via whoami. Run this once per repo.",
    inputSchema: {
      project: z.string().optional(),
      project_id: z.string().optional(),
      workspace: z.string().optional(),
      default_environment: z.string().optional().describe("Optional default environment: 'preview' or 'production' (also accepts prod/live/staging)."),
    },
    handler: async (args) => {
      const user = await ctx.client.request<User>(`/auth/me`);
      const { project, source } = await resolveProject(ctx, args);
      const defaultEnvironment = args.default_environment ? requireEnvironment(args.default_environment) : undefined;
      const binding = {
        project: project.name,
        workspace: project.organization_name ?? project.organization_id,
        defaultEnvironment,
      } as const;
      writeBinding(ctx.stateDir, ctx.cwd, binding);
      const gitignoreUpdated = ensureGitignore(ctx.cwd);
      const lines = [
        `Bound this repo to Deployik project '${project.name}' (group: ${binding.workspace}).`,
        `Authenticated as ${user.username} (${user.role}).`,
        `Resolution source: ${source}.`,
        `Wrote .deployik.json (commit this — it tells teammates which project deploys here).`,
        gitignoreUpdated ? "Added `.deployik/` to .gitignore." : "`.deployik/` already in .gitignore (or no .gitignore at repo root).",
        `Private state dir: ${ctx.stateDir} (cache, token, audit — never commit).`,
      ];
      return { text: lines.join("\n"), data: { project, binding, gitignoreUpdated } };
    },
  });

  registerTool(server, ctx, {
    name: "setup_project_from_repo",
    description:
      "End-to-end: inspect the GitHub repo for monorepo/framework, create a Deployik project, optionally enable auto-build + auto-production. Returns the project + initial preview deploy id (if any).",
    inputSchema: {
      owner: z.string(),
      repo: z.string(),
      name: z.string().describe("DNS-safe slug, e.g. 'my-app'."),
      branch: z.string().default("main"),
      framework: z.enum(["nextjs", "vite", "astro", "static"]).optional(),
      package_manager: z.enum(["auto", "bun", "pnpm", "npm", "yarn"]).optional(),
      root_directory: z.string().optional().describe("If the repo is a monorepo, the app subdirectory."),
      group_id: z.string().optional().describe("Dashboard group id. Preferred over organization_id."),
      group: z.string().optional().describe("Dashboard group name, slug, or id."),
      organization_id: z.string().optional().describe("Backward-compatible alias for group_id."),
      auto_build_enabled: z.boolean().default(true),
      auto_production_enabled: z.boolean().default(false),
      resource_tier: z.enum(["nano", "small", "medium", "large"]).optional(),
      start_command: z.string().optional(),
      health_path: z.string().optional(),
    },
    handler: async (args) => {
      const inspection = await ctx.client.request<RepoInspection>(
        `/github/repos/${encodeURIComponent(args.owner)}/${encodeURIComponent(args.repo)}/inspect`,
        { query: { branch: args.branch } },
      );

      let detectedApp: RepoInspection["apps"][0] | undefined;
      if (args.root_directory !== undefined) {
        detectedApp = inspection.apps.find((a) => a.path === args.root_directory);
      } else if (inspection.apps.length > 0) {
        detectedApp = inspection.apps[0];
      }

      const groupId = await resolveGroupId(ctx.client, {
        group_id: args.group_id,
        group: args.group,
        organization_id: args.organization_id,
      });
      const payload = buildCreateProjectPayload(
        {
          name: args.name,
          github_owner: args.owner,
          github_repo: args.repo,
          branch: args.branch,
          framework: args.framework ?? detectedApp?.framework ?? "nextjs",
          package_manager: args.package_manager ?? inspection.package_manager ?? "auto",
          root_directory: args.root_directory ?? detectedApp?.path ?? "",
          output_directory: detectedApp?.output_directory ?? "",
          build_command: detectedApp?.suggested_build_command ?? "",
          install_command: "",
          node_version: "22",
          port: 3000,
          auto_build_enabled: args.auto_build_enabled,
          auto_production_enabled: args.auto_production_enabled,
          resource_tier: args.resource_tier,
          start_command: args.start_command,
          health_path: args.health_path,
        },
        groupId,
      );

      const project = await ctx.client.request<Project>(`/projects`, { method: "POST", body: payload });

      return {
        text: [
          `Created project '${project.name}' (id: ${project.id}).`,
          inspection.is_monorepo ? `Monorepo detected — picked app at '${payload.root_directory || "/"}'.` : "Single-app repo.",
          `Framework: ${payload.framework} · package manager: ${payload.package_manager}`,
          `Auto-build: ${args.auto_build_enabled ? "enabled" : "disabled"}, auto-production: ${args.auto_production_enabled ? "enabled" : "disabled"}.`,
          `Initial preview deploy is fired automatically by the server. Use list_deployments to watch.`,
        ].join("\n"),
        data: { project, inspection },
      };
    },
  });

  registerTool(server, ctx, {
    name: "deploy_project",
    description:
      "Start a new deployment. Use environment 'preview' for a test build (auto-deployed to a *.preview.example.com URL) or 'production' for the public live version. With `wait:true`, this tool waits until the build finishes (success or failure) before returning. Production deploys require `confirm:true` + `confirm_name` matching the project slug, so the AI can't accidentally push to live.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: ENV,
      branch: z.string().optional(),
      wait: z.boolean().default(false),
      timeout_ms: z.number().int().positive().max(600_000).default(180_000),
      confirm: z.boolean().optional(),
      confirm_name: z.string().optional(),
      create_tag: z.boolean().default(false),
      tag_name: z.string().optional(),
    },
    audit: true,
    handler: async (args) => {
      const environment = requireEnvironment(args.environment);
      const { project } = await resolveProject(ctx, args);
      if (environment === "production") {
        const safety = checkSafety(
          {
            toolName: "deploy_project",
            tier: "destructive_production",
            expectedName: project.name,
            impact: { project: project.name, environment: "production", branch: args.branch ?? project.branch, create_tag: args.create_tag },
          },
          { confirm: args.confirm, confirm_name: args.confirm_name },
        );
        if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      }
      const body: Record<string, unknown> = { environment };
      if (args.branch) body.branch = args.branch;
      if (args.create_tag) body.create_tag = true;
      if (args.tag_name) body.tag_name = args.tag_name;
      const deployment = await ctx.client.request<Deployment>(`/projects/${project.id}/deployments`, { method: "POST", body });

      if (!args.wait) {
        return { text: `Triggered ${environment} deploy.\n${renderDeploymentSummary(deployment)}`, data: deployment };
      }

      const final = await pollUntil<Deployment>(
        () => ctx.client.request<Deployment>(`/projects/${project.id}/deployments/${deployment.id}`),
        (d) => TERMINAL_STATUSES.has(d.status),
        { intervalMs: 2000, timeoutMs: args.timeout_ms, initialDelayMs: 1500 },
      );

      const status = final.value;
      const logs = await ctx.client.request<BuildLog[]>(`/deployments/${status.id}/logs`).catch(() => [] as BuildLog[]);
      const formatted = formatLogs(logs, { maxLines: 80, tail: true });
      const header = `Deployment ${status.id} · ${status.status}${final.done ? "" : " (timed out)"}`;
      const tail = status.status === "failed" || !final.done ? `\n\nLog tail:\n${formatted.text}` : "";
      return { text: `${header}\n${renderDeploymentSummary(status)}${tail}`, data: status };
    },
  });

  registerTool(server, ctx, {
    name: "set_secret",
    description:
      "Add or change a secret value — API key, database password, Stripe key, anything sensitive. Stored encrypted; only available to the running container at runtime. NEXT_PUBLIC_* keys are NOT allowed here (they need to be baked into the build — use set_env_var). If you don't pass `project`, the tool uses the binding from `.deployik/` in the current folder.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional().describe("Project slug like 'my-app'. Omit to use the .deployik binding."),
      environment: VAR_SCOPE.default("production"),
      key: z.string().describe("The variable name, e.g. STRIPE_SECRET_KEY."),
      value: z.string().describe("The actual secret value."),
    },
    handler: async (args) => {
      if (args.key.startsWith("NEXT_PUBLIC_")) {
        return {
          text: `NEXT_PUBLIC_* keys can't be secrets — they need to be embedded into the build so the browser can read them. Use set_env_var instead.`,
          isError: true,
        };
      }
      const environment = requireScope(args.environment);
      const { project } = await resolveProject(ctx, args);
      await ctx.client.request(`/projects/${project.id}/secrets`, {
        method: "POST",
        body: { environment, key: args.key, value: args.value },
      });
      return { text: `Saved secret '${args.key}' to ${environment} of ${project.name}.` };
    },
  });

  registerTool(server, ctx, {
    name: "set_env_var",
    description:
      "Add or change an environment variable — a non-sensitive config value or a NEXT_PUBLIC_* value that the browser can see. If you don't pass `project`, the tool uses the binding from `.deployik/` in the current folder. Use 'shared' scope to set the same value for both preview and production.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: VAR_SCOPE.default("production"),
      key: z.string().describe("Variable name. Letters, digits, underscores only. Cannot start with a number."),
      value: z.string(),
    },
    handler: async (args) => {
      const environment = requireScope(args.environment);
      const { project } = await resolveProject(ctx, args);
      await ctx.client.request(`/projects/${project.id}/env`, {
        method: "POST",
        body: { environment, key: args.key, value: args.value },
      });
      return { text: `Saved '${args.key}' to ${environment} of ${project.name}.` };
    },
  });

  registerTool(server, ctx, {
    name: "wait_for_deployment",
    description: "Poll until a deployment reaches a terminal status (live / failed / replaced / rolled_back) or the timeout elapses.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      deployment_id: z.string(),
      timeout_ms: z.number().int().positive().max(600_000).default(180_000),
    },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const result = await pollUntil<Deployment>(
        () => ctx.client.request<Deployment>(`/projects/${project.id}/deployments/${args.deployment_id}`),
        (d) => TERMINAL_STATUSES.has(d.status),
        { intervalMs: 2000, timeoutMs: args.timeout_ms },
      );
      return {
        text: result.done
          ? `Deployment ${args.deployment_id} reached '${result.value.status}' in ${result.elapsedMs}ms.\n${renderDeploymentSummary(result.value)}`
          : `Timed out waiting — last status: '${result.value.status}' after ${result.elapsedMs}ms.`,
        data: result.value,
      };
    },
  });

  registerTool(server, ctx, {
    name: "tail_latest_logs",
    description:
      "Show me the most recent build logs for a project. Finds the latest deployment, returns the tail with error lines highlighted. Use `follow:true` if a build is still running and you want to wait for it to finish before getting the logs.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: ENV,
      follow: z.boolean().default(false),
      max_lines: z.number().int().positive().max(2000).default(150),
      timeout_ms: z.number().int().positive().max(600_000).default(180_000),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const environment = requireEnvironment(args.environment);
      const { project } = await resolveProject(ctx, args);
      const deployments = await ctx.client.request<{ deployments: Deployment[]; total: number }>(
        `/projects/${project.id}/deployments`,
        { query: { environment, limit: 1 } },
      );
      const latest = (deployments.deployments ?? [])[0];
      if (!latest) {
        return { text: `(no ${environment} deployments yet for ${project.name})` };
      }

      let current = latest;
      let logs = await ctx.client.request<BuildLog[]>(`/deployments/${current.id}/logs`).catch(() => [] as BuildLog[]);

      if (args.follow && !TERMINAL_STATUSES.has(current.status)) {
        const start = Date.now();
        while (Date.now() - start < args.timeout_ms) {
          await sleep(2000);
          current = await ctx.client.request<Deployment>(`/projects/${project.id}/deployments/${current.id}`);
          logs = await ctx.client.request<BuildLog[]>(`/deployments/${current.id}/logs`).catch(() => logs);
          if (TERMINAL_STATUSES.has(current.status)) break;
        }
      }

      const formatted = formatLogs(logs, { maxLines: args.max_lines, tail: true });
      const sinceFinished = current.finished_at ? ` · finished ${current.finished_at}` : "";
      const header = `Deployment ${current.id} · ${current.status}${sinceFinished}\n${current.environment} · ${current.branch} · commit ${current.commit_sha.slice(0, 8)}\n`;
      const errorSection = formatted.errorAnchors.length > 0 ? `\nError anchors: lines ${formatted.errorAnchors.join(", ")}` : "";
      const footer = formatted.hint ? `\n\n${formatted.hint}` : "";
      return {
        text: `${header}\n${formatted.text}${errorSection}${footer}`,
        data: { deployment: current, logsMeta: { totalLines: formatted.totalLines, returnedLines: formatted.returnedLines, truncated: formatted.truncated, errorAnchors: formatted.errorAnchors } },
      };
    },
  });

  registerTool(server, ctx, {
    name: "debug_failed_deployment",
    description: "Composite: deployment meta + last 200 log lines + screenshot URL. Use as the first call when a build fails.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      deployment_id: z.string(),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const deployment = await ctx.client.request<Deployment>(`/projects/${project.id}/deployments/${args.deployment_id}`);
      const logs = await ctx.client.request<BuildLog[]>(`/deployments/${deployment.id}/logs`).catch(() => [] as BuildLog[]);
      const formatted = formatLogs(logs, { maxLines: 200, tail: true });
      const screenshotUrl = ctx.client.screenshotUrl(deployment.id);
      const sections = [
        renderDeploymentSummary(deployment),
        ``,
        `Log tail (${formatted.returnedLines}/${formatted.totalLines} lines):`,
        formatted.text,
      ];
      if (formatted.errorAnchors.length > 0) {
        sections.push(``, `Error anchors at lines: ${formatted.errorAnchors.join(", ")}`);
      }
      sections.push(``, `Screenshot URL (if captured): ${screenshotUrl}`);
      return { text: sections.join("\n"), data: { deployment, logsMeta: formatted, screenshotUrl } };
    },
  });

  registerTool(server, ctx, {
    name: "get_project_health",
    description: "Composite snapshot: project + latest deploys per env + domains + volumes + protection status. One call replaces 5+.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const [domains, volumes, protection, previewList, productionList] = await Promise.all([
        ctx.client.request<Domain[]>(`/projects/${project.id}/domains`).catch(() => [] as Domain[]),
        ctx.client.request<VolumeInfo[]>(`/projects/${project.id}/volumes`).catch(() => [] as VolumeInfo[]),
        ctx.client.request<ProtectionStatus>(`/projects/${project.id}/protection`).catch(() => ({ preview_enabled: false, production_enabled: false } as ProtectionStatus)),
        ctx.client.request<{ deployments: Deployment[]; total: number }>(`/projects/${project.id}/deployments`, {
          query: { environment: "preview", limit: 1 },
        }).catch(() => ({ deployments: [], total: 0 })),
        ctx.client.request<{ deployments: Deployment[]; total: number }>(`/projects/${project.id}/deployments`, {
          query: { environment: "production", limit: 1 },
        }).catch(() => ({ deployments: [], total: 0 })),
      ]);
      const lines = [
        renderProjectSummary(project),
        ``,
        `Latest preview:    ${previewList.deployments[0] ? `${previewList.deployments[0].status} · ${previewList.deployments[0].id}` : "(none)"}`,
        `Latest production: ${productionList.deployments[0] ? `${productionList.deployments[0].status} · ${productionList.deployments[0].id}` : "(none)"}`,
        ``,
        `Domains:`,
        renderDomainsList(domains),
        ``,
        `Volumes:`,
        renderVolumesList(volumes),
        ``,
        `Protection:`,
        renderProtection(protection),
      ];
      return {
        text: lines.join("\n"),
        data: {
          project,
          latest: { preview: previewList.deployments[0] ?? null, production: productionList.deployments[0] ?? null },
          domains,
          volumes,
          protection,
        },
      };
    },
  });

  registerTool(server, ctx, {
    name: "show_binding",
    description: "Print the current .deployik binding (project + dashboard group) for this repo, if any. Useful to confirm resolution before running other tools.",
    annotations: { readOnlyHint: true },
    handler: async () => {
      const binding = readBinding(ctx.stateDir);
      if (!binding) {
        return { text: `(no binding yet — run init_in_repo to bind this repo to a Deployik project)` };
      }
      const projects = await fetchProjects(ctx).catch(() => []);
      const project = projects.find((p) => p.name === binding.project);
      const lines = [
        `Binding (${ctx.stateDir}):`,
        `  project:             ${binding.project}`,
        `  group:               ${binding.workspace ?? "(unset)"}`,
        `  workspace:           ${binding.workspace ?? "(unset)"} (legacy key)`,
        `  defaultEnvironment:  ${binding.defaultEnvironment ?? "(unset)"}`,
      ];
      if (project) lines.push("", renderProjectSummary(project));
      return { text: lines.join("\n"), data: { binding, project } };
    },
  });

  // -----------------------------------------------------------------
  // High-intent shortcuts for non-technical users.
  //
  // These tools accept loose English ("show me my site", "what's broken",
  // "redeploy") and resolve to the same backend operations as the lower-level
  // tools, with friendlier defaults and output. The AI is free to use the
  // primitive tools instead — these are pure ergonomics.
  // -----------------------------------------------------------------

  registerTool(server, ctx, {
    name: "whats_my_url",
    description:
      "Show the live URL(s) of a project. Returns the primary preview and production URLs (with SSL status) so you can share or open them.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const domains = await ctx.client.request<Domain[]>(`/projects/${project.id}/domains`);
      const groupByEnv: Record<string, Domain[]> = { preview: [], production: [] };
      for (const d of domains) groupByEnv[d.environment].push(d);
      const pickPrimary = (list: Domain[]) =>
        list.find((d) => d.is_primary && d.dns_verified && d.ssl_status === "active") ??
        list.find((d) => d.is_primary) ??
        list[0];
      const linesOut: string[] = [`URLs for '${project.name}':`];
      for (const env of ["preview", "production"] as const) {
        const list = groupByEnv[env];
        const primary = pickPrimary(list);
        if (!primary) {
          linesOut.push(`  ${env.padEnd(11)} (no ${env} domains yet)`);
          continue;
        }
        const proto = primary.ssl_status === "active" ? "https://" : "http://";
        const flag = primary.ssl_status === "active" ? "live" : `${primary.ssl_status} SSL · DNS ${primary.dns_verified ? "ok" : "pending"}`;
        linesOut.push(`  ${env.padEnd(11)} ${proto}${primary.domain}   (${flag})`);
        const others = list.filter((d) => d !== primary);
        for (const d of others) {
          linesOut.push(`              also: ${d.ssl_status === "active" ? "https://" : "http://"}${d.domain}`);
        }
      }
      return { text: linesOut.join("\n"), data: { domains, primary: { preview: pickPrimary(groupByEnv.preview), production: pickPrimary(groupByEnv.production) } } };
    },
  });

  registerTool(server, ctx, {
    name: "whats_broken",
    description:
      "Find the most recent failed deployment — either for a specific project, or (if none specified) the freshest failure across every project this token can see. Returns the deployment summary + a log tail with error lines highlighted, so the AI can suggest a fix in one shot.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: ENV.optional(),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const envFilter = args.environment ? requireEnvironment(args.environment) : undefined;
      let candidateProjects: Project[];
      if (args.project_id || args.project) {
        const { project } = await resolveProject(ctx, args);
        candidateProjects = [project];
      } else {
        candidateProjects = await fetchProjects(ctx);
      }

      type Candidate = { project: Project; dep: Deployment };
      const failures: Candidate[] = [];
      for (const project of candidateProjects) {
        const list = await ctx.client
          .request<{ deployments: Deployment[]; total: number }>(`/projects/${project.id}/deployments`, {
            query: { status: "failed", environment: envFilter, limit: 1 },
          })
          .catch(() => ({ deployments: [], total: 0 }));
        const dep = (list.deployments ?? [])[0];
        if (dep) failures.push({ project, dep });
      }
      if (failures.length === 0) {
        return { text: `No failed deployments found${args.project || args.project_id ? " for this project" : ""}.` };
      }
      failures.sort((a, b) => (b.dep.created_at > a.dep.created_at ? 1 : -1));
      const winner = failures[0]!;
      const logs = await ctx.client.request<BuildLog[]>(`/deployments/${winner.dep.id}/logs`).catch(() => [] as BuildLog[]);
      const formatted = formatLogs(logs, { maxLines: 60, tail: true });
      const header = `Project '${winner.project.name}' has a failed ${winner.dep.environment} deployment.\n${renderDeploymentSummary(winner.dep)}`;
      const anchors = formatted.errorAnchors.length > 0 ? `\nError anchors: lines ${formatted.errorAnchors.join(", ")}` : "";
      return {
        text: `${header}\n\nLog tail (${formatted.returnedLines}/${formatted.totalLines}):\n${formatted.text}${anchors}`,
        data: { project: winner.project, deployment: winner.dep, logsMeta: formatted, otherFailures: failures.length - 1 },
      };
    },
  });

  registerTool(server, ctx, {
    name: "redeploy",
    description:
      "Re-run the last build for a project. Convenience alias around deploy_project — defaults to the preview environment and waits for the build to finish. Use this when someone says 'redeploy', 'try again', or 'kick another build'.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: ENV.default("preview"),
      wait: z.boolean().default(true),
      timeout_ms: z.number().int().positive().max(600_000).default(240_000),
      confirm: z.boolean().optional(),
      confirm_name: z.string().optional(),
    },
    audit: true,
    handler: async (args) => {
      const environment = requireEnvironment(args.environment);
      const { project } = await resolveProject(ctx, args);
      if (environment === "production") {
        const safety = checkSafety(
          {
            toolName: "redeploy",
            tier: "destructive_production",
            expectedName: project.name,
            impact: { project: project.name, environment, branch: project.branch },
          },
          { confirm: args.confirm, confirm_name: args.confirm_name },
        );
        if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      }
      const dep = await ctx.client.request<Deployment>(`/projects/${project.id}/deployments`, {
        method: "POST",
        body: { environment },
      });
      if (!args.wait) return { text: `Redeploy queued for ${project.name} (${environment}).\n${renderDeploymentSummary(dep)}`, data: dep };
      const final = await pollUntil<Deployment>(
        () => ctx.client.request<Deployment>(`/projects/${project.id}/deployments/${dep.id}`),
        (d) => TERMINAL_STATUSES.has(d.status),
        { intervalMs: 2000, timeoutMs: args.timeout_ms, initialDelayMs: 1500 },
      );
      const status = final.value;
      if (status.status === "live") {
        const domains = await ctx.client.request<Domain[]>(`/projects/${project.id}/domains`).catch(() => [] as Domain[]);
        const primary = domains.find((d) => d.environment === environment && d.is_primary) ?? domains.find((d) => d.environment === environment);
        const url = primary ? (primary.ssl_status === "active" ? "https://" : "http://") + primary.domain : null;
        return {
          text: [
            `✅ ${project.name} (${environment}) is live again.`,
            url ? `URL: ${url}` : "",
            renderDeploymentSummary(status),
          ].filter(Boolean).join("\n"),
          data: status,
        };
      }
      const logs = await ctx.client.request<BuildLog[]>(`/deployments/${status.id}/logs`).catch(() => [] as BuildLog[]);
      const formatted = formatLogs(logs, { maxLines: 80, tail: true });
      return {
        text: `❌ Redeploy of ${project.name} (${environment}) ended as '${status.status}'.\n${renderDeploymentSummary(status)}\n\nLog tail:\n${formatted.text}`,
        data: status,
      };
    },
  });

  registerTool(server, ctx, {
    name: "tail_service_logs",
    description:
      "Stream live logs from a project's postgres sidecar for a short window. Opens the /ws/projects/{id}/services/{env}/logs WebSocket, captures up to `max_lines` lines or until `duration_ms` elapses, whichever comes first. Use this to investigate connection errors, slow queries, or auth failures.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: z.enum(["preview", "production"]),
      duration_ms: z.number().int().positive().max(60_000).default(8_000),
      max_lines: z.number().int().positive().max(2000).default(200),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const result = await tailServiceLogs(ctx, project.id, args.environment, args.duration_ms, args.max_lines);
      if (result.lines.length === 0) {
        return {
          text: result.error
            ? `(no logs captured — ${result.error})`
            : `(no logs in the last ${args.duration_ms}ms — the postgres container may be idle)`,
        };
      }
      const ERR_RE = /\b(error|fatal|panic|cannot|failed)\b/i;
      const rendered = result.lines.map((l) => (ERR_RE.test(l) ? `↑ ${l}` : `  ${l}`)).join("\n");
      const header = `Postgres logs · ${project.name} · ${args.environment} (${result.lines.length} lines, captured ${result.elapsedMs}ms${result.truncated ? `, capped at max_lines=${args.max_lines}` : ""})`;
      const footer = result.error ? `\n\n(stream ended: ${result.error})` : "";
      return {
        text: `${header}\n\n${rendered}${footer}`,
        data: { lines: result.lines, elapsedMs: result.elapsedMs, truncated: result.truncated, error: result.error ?? null },
      };
    },
  });

  registerTool(server, ctx, {
    name: "tail_deployment_logs",
    description:
      "Live-tail a deployment's build log via WebSocket. Returns existing history plus any new lines emitted during the window, formatted with stderr/error highlights. The stream exits early when the deployment reaches a terminal status (live / failed / replaced / rolled_back). Pair with `trigger_deployment` for a watch-the-build flow, or use `tail_latest_logs` if you don't have a deployment_id.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      deployment_id: z.string(),
      duration_ms: z.number().int().positive().max(600_000).default(20_000),
      max_lines: z.number().int().positive().max(2000).default(300),
      include_history: z.boolean().default(true),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const deployment = await ctx.client.request<Deployment>(`/projects/${project.id}/deployments/${args.deployment_id}`);

      const history = args.include_history
        ? await ctx.client.request<BuildLog[]>(`/deployments/${deployment.id}/logs`).catch(() => [] as BuildLog[])
        : [];

      // Skip the WS hop if the build already terminated — history is final.
      const live = TERMINAL_STATUSES.has(deployment.status)
        ? { lines: [] as BuildLog[], elapsedMs: 0, truncated: false, finalStatus: deployment.status }
        : await tailDeploymentLogs(ctx, project.id, deployment.id, args.duration_ms, args.max_lines, history.length > 0 ? history[history.length - 1].line_number : 0);

      // Merge: history first, then anything new from the WS that we hadn't already seen.
      const seen = new Set(history.map((h) => h.line_number));
      const merged = [...history];
      for (const l of live.lines) {
        if (!seen.has(l.line_number)) {
          merged.push(l);
          seen.add(l.line_number);
        }
      }
      const trimmed = merged.length > args.max_lines ? merged.slice(-args.max_lines) : merged;
      const formatted = formatLogs(trimmed, { maxLines: args.max_lines, tail: true });

      const finalStatus = live.finalStatus ?? deployment.status;
      const header = [
        `Deployment ${deployment.id} · ${finalStatus}${TERMINAL_STATUSES.has(finalStatus) ? " (terminal)" : " (in progress)"}`,
        `${deployment.environment} · ${deployment.branch} · commit ${deployment.commit_sha.slice(0, 8)}`,
      ].join("\n");
      const errorSection = formatted.errorAnchors.length > 0 ? `\nError anchors: lines ${formatted.errorAnchors.join(", ")}` : "";
      const footer = "error" in live && live.error ? `\n\n(stream ended: ${live.error})` : formatted.hint ? `\n\n${formatted.hint}` : "";
      return {
        text: `${header}\n\n${formatted.text}${errorSection}${footer}`,
        data: {
          deployment_id: deployment.id,
          finalStatus,
          historyLines: history.length,
          liveLines: live.lines.length,
          elapsedMs: live.elapsedMs,
          truncated: live.truncated || formatted.truncated,
          errorAnchors: formatted.errorAnchors,
        },
      };
    },
  });
}

async function tailDeploymentLogs(
  ctx: ToolContext,
  projectId: string,
  deploymentId: string,
  durationMs: number,
  maxLines: number,
  highestSeenLine: number,
): Promise<{ lines: BuildLog[]; elapsedMs: number; truncated: boolean; finalStatus?: string; error?: string }> {
  const { WebSocket } = await import("ws");
  const url = ctx.client.wsUrl(`/ws/deployments/${deploymentId}/logs`);
  const lines: BuildLog[] = [];
  const start = Date.now();

  return new Promise((resolve) => {
    const ws = new WebSocket(url, {
      headers: { Authorization: ctx.client.bearerHeader() },
      handshakeTimeout: 10_000,
    });

    let truncated = false;
    let settled = false;
    let finalStatus: string | undefined;
    const finish = (error?: string) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      clearInterval(statusPoll);
      try { ws.close(); } catch { /* ignore */ }
      resolve({ lines, elapsedMs: Date.now() - start, truncated, finalStatus, error });
    };

    const timer = setTimeout(() => finish(), durationMs);

    // Poll the deployment status every 2s — exit early when the build terminates.
    const statusPoll = setInterval(async () => {
      try {
        const dep = await ctx.client.request<Deployment>(`/projects/${projectId}/deployments/${deploymentId}`);
        if (TERMINAL_STATUSES.has(dep.status)) {
          finalStatus = dep.status;
          // Give the WS one tick to flush any final lines before closing.
          setTimeout(() => finish(), 250);
        }
      } catch {
        // status fetch failures shouldn't kill the stream — just keep going.
      }
    }, 2000);

    ws.on("message", (data: Buffer | ArrayBuffer | Buffer[]) => {
      const text = Buffer.isBuffer(data)
        ? data.toString("utf8")
        : Array.isArray(data)
          ? Buffer.concat(data).toString("utf8")
          : Buffer.from(data as ArrayBuffer).toString("utf8");
      let parsed: unknown;
      try {
        parsed = JSON.parse(text);
      } catch {
        return; // server only sends JSON frames; ignore anything else
      }
      if (!parsed || typeof parsed !== "object") return;
      const raw = parsed as Record<string, unknown>;
      const lineNumber = typeof raw.line_number === "number" ? raw.line_number : 0;
      if (lineNumber > 0 && lineNumber <= highestSeenLine) return; // dedupe replay
      const log: BuildLog = {
        id: 0,
        deployment_id: typeof raw.deployment_id === "string" ? raw.deployment_id : deploymentId,
        line_number: lineNumber,
        content: typeof raw.content === "string" ? raw.content : "",
        stream: raw.stream === "stderr" ? "stderr" : "stdout",
        timestamp: "",
      };
      if (lines.length >= maxLines) {
        truncated = true;
        finish();
        return;
      }
      lines.push(log);
    });

    ws.on("error", (err: Error) => finish(err.message));
    ws.on("close", (code: number, reason: Buffer) => {
      const reasonText = reason?.toString("utf8");
      finish(code !== 1000 && code !== 1005 ? `closed (code ${code}${reasonText ? `: ${reasonText}` : ""})` : undefined);
    });
  });
}

async function tailServiceLogs(
  ctx: ToolContext,
  projectId: string,
  environment: string,
  durationMs: number,
  maxLines: number,
): Promise<{ lines: string[]; elapsedMs: number; truncated: boolean; error?: string }> {
  // Lazy import — keeps `ws` out of the import graph for the rest of the
  // toolset (which is fetch-only) so a packaging issue can't break unrelated tools.
  const { WebSocket } = await import("ws");
  const url = ctx.client.wsUrl(`/ws/projects/${projectId}/services/${environment}/logs`);
  const lines: string[] = [];
  const start = Date.now();

  return new Promise((resolve) => {
    const ws = new WebSocket(url, {
      headers: { Authorization: ctx.client.bearerHeader() },
      handshakeTimeout: 10_000,
    });

    let truncated = false;
    let settled = false;
    const finish = (error?: string) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      try { ws.close(); } catch { /* ignore */ }
      resolve({ lines, elapsedMs: Date.now() - start, truncated, error });
    };

    const timer = setTimeout(() => finish(), durationMs);

    ws.on("message", (data: Buffer | ArrayBuffer | Buffer[]) => {
      const text = Buffer.isBuffer(data)
        ? data.toString("utf8")
        : Array.isArray(data)
          ? Buffer.concat(data).toString("utf8")
          : Buffer.from(data as ArrayBuffer).toString("utf8");
      // Server emits one line per frame, but tolerate joined lines defensively.
      for (const line of text.split(/\r?\n/)) {
        if (!line) continue;
        if (lines.length >= maxLines) {
          truncated = true;
          finish();
          return;
        }
        lines.push(line);
      }
    });

    ws.on("error", (err: Error) => finish(err.message));
    ws.on("close", (code: number, reason: Buffer) => {
      const reasonText = reason?.toString("utf8");
      finish(code !== 1000 && code !== 1005 ? `closed (code ${code}${reasonText ? `: ${reasonText}` : ""})` : undefined);
    });
  });
}
