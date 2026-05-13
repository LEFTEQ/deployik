import { z } from "zod";
import type { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { registerTool, type ToolContext } from "./_helpers.js";
import { resolveProject } from "../resolve/project.js";
import { checkSafety } from "../lib/safety.js";
import { renderDryRun } from "../lib/format.js";
import type { ProjectEmailPayload, ProjectEmailSaveRequest } from "../client/types.js";

export function registerEmailTools(server: McpServer, ctx: ToolContext): void {
  registerTool(server, ctx, {
    name: "get_project_email",
    description: "Get the email/contact-form setup payload for a project. Includes the AI install prompt and required env keys.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const payload = await ctx.client.request<ProjectEmailPayload>(`/projects/${project.id}/email`);
      return { text: JSON.stringify(payload, null, 2), data: payload };
    },
  });

  registerTool(server, ctx, {
    name: "update_project_email",
    description: "Save Webglobe SMTP + reCAPTCHA settings. Writes shared env vars + encrypted secrets.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      provider: z.enum(["webglobe", "smtp"]).default("webglobe"),
      smtp_host: z.string(),
      smtp_port: z.number().int().positive(),
      smtp_security: z.enum(["starttls", "tls", "none"]).default("starttls"),
      smtp_user: z.string(),
      smtp_password: z.string().optional(),
      email_from: z.string(),
      email_from_name: z.string(),
      contact_email_to: z.string(),
      recaptcha_site_key: z.string(),
      recaptcha_secret_key: z.string().optional(),
      recaptcha_score_threshold: z.number().min(0).max(1).default(0.5),
    },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const body: ProjectEmailSaveRequest = {
        provider: args.provider,
        smtp_host: args.smtp_host,
        smtp_port: args.smtp_port,
        smtp_security: args.smtp_security,
        smtp_user: args.smtp_user,
        email_from: args.email_from,
        email_from_name: args.email_from_name,
        contact_email_to: args.contact_email_to,
        recaptcha_site_key: args.recaptcha_site_key,
        recaptcha_score_threshold: args.recaptcha_score_threshold,
      };
      if (args.smtp_password) body.smtp_password = args.smtp_password;
      if (args.recaptcha_secret_key) body.recaptcha_secret_key = args.recaptcha_secret_key;
      const payload = await ctx.client.request<ProjectEmailPayload>(`/projects/${project.id}/email`, {
        method: "PUT",
        body,
      });
      return { text: `Saved email settings for ${project.name}.`, data: payload };
    },
  });

  registerTool(server, ctx, {
    name: "test_project_smtp",
    description: "Send an audited SMTP test email using the stored settings. Requires `confirm: true`.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      confirm: z.boolean().optional(),
    },
    audit: true,
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const safety = checkSafety(
        {
          toolName: "test_project_smtp",
          tier: "destructive",
          impact: { project: project.name, note: "Sends a real email to the configured contact recipient." },
        },
        { confirm: args.confirm },
      );
      if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      const payload = await ctx.client.request<ProjectEmailPayload>(`/projects/${project.id}/email/test-smtp`, {
        method: "POST",
      });
      return { text: `SMTP test triggered for ${project.name}. status=${payload.settings.status ?? "?"}`, data: payload };
    },
  });
}
