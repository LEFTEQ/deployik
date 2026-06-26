import { describe, expect, test } from "bun:test";

import {
  getEmailReadiness,
  getSMTPTestBlocker,
  RECAPTCHA_SECRET_KEY,
} from "../src/lib/email-readiness";
import type { ProjectEmailPayload, ProjectEmailSaveRequest } from "../src/types/api";

describe("email readiness", () => {
  test("allows SMTP testing when only the reCAPTCHA secret is missing", () => {
    const payload = makePayload({
      secrets_missing: true,
      missing_secrets: [RECAPTCHA_SECRET_KEY],
    });
    const form = makeForm();

    expect(getSMTPTestBlocker(payload, form)).toBeNull();
    expect(getEmailReadiness(payload)).toMatchObject({
      smtpReadyToTest: true,
      recaptchaReady: false,
      installReady: false,
    });
  });

  test("allows SMTP testing with a newly typed unsaved SMTP password", () => {
    const payload = makePayload({
      secrets_missing: true,
      missing_secrets: ["SMTP_PASSWORD", RECAPTCHA_SECRET_KEY],
    });
    const form = makeForm({
      smtp_password: "new-password",
    });

    expect(getSMTPTestBlocker(payload, form)).toBeNull();
  });

  test("allows SMTP testing before owner recipients are configured", () => {
    const payload = makePayload({
      env_missing: false,
      missing_env: [],
    });
    payload.settings.contact_email_to = "";
    const form = makeForm({
      contact_email_to: "",
    });

    expect(getSMTPTestBlocker(payload, form)).toBeNull();
    expect(getEmailReadiness(payload)).toMatchObject({
      smtpReadyToTest: true,
      installReady: false,
    });
  });

  test("blocks SMTP testing when neither stored nor current SMTP password exists", () => {
    const payload = makePayload({
      secrets_missing: true,
      missing_secrets: ["SMTP_PASSWORD"],
    });

    expect(getSMTPTestBlocker(payload, makeForm())).toBe(
      "Add the SMTP password before testing.",
    );
  });
});

function makePayload(
  required: Partial<ProjectEmailPayload["status"]["required"]> = {},
): ProjectEmailPayload {
  return {
    settings: {
      project_id: "project-1",
      provider: "webglobe",
      smtp_host: "mail.webglobe.cz",
      smtp_port: 587,
      smtp_security: "starttls",
      smtp_user: "noreply@acmegym.cz",
      email_from: "noreply@acmegym.cz",
      email_from_name: "AcmeGym",
      contact_email_to: "info@acmegym.cz",
      recaptcha_site_key: "site-key",
      recaptcha_mode: "v3",
      recaptcha_score_threshold: 0.5,
      status: "ready_to_install",
    },
    status: {
      configured: false,
      required: {
        env_missing: false,
        secrets_missing: false,
        missing_env: [],
        missing_secrets: [],
        ...required,
      },
    },
    install: {
      ai_prompt: "",
      env_keys: [],
    },
  };
}

function makeForm(
  overrides: Partial<ProjectEmailSaveRequest> = {},
): ProjectEmailSaveRequest {
  return {
    provider: "webglobe",
    smtp_host: "mail.webglobe.cz",
    smtp_port: 587,
    smtp_security: "starttls",
    smtp_user: "noreply@acmegym.cz",
    smtp_password: "",
    email_from: "noreply@acmegym.cz",
    email_from_name: "AcmeGym",
    contact_email_to: "info@acmegym.cz",
    recaptcha_site_key: "site-key",
    recaptcha_secret_key: "",
    recaptcha_score_threshold: 0.5,
    ...overrides,
  };
}
