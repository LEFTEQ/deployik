import type {
  ProjectEmailPayload,
  ProjectEmailSaveRequest,
  ProjectEmailSettings,
} from "../types/api";

export const SMTP_PASSWORD_KEY = "SMTP_PASSWORD";
export const RECAPTCHA_SECRET_KEY = "RECAPTCHA_SECRET_KEY";

export function getEmailReadiness(data: ProjectEmailPayload) {
  const smtpSettingsReady = hasSMTPSettings(data.settings);
  const smtpPasswordReady = !hasMissingSecret(data, SMTP_PASSWORD_KEY);
  const ownerRecipientsReady = hasText(data.settings.contact_email_to);
  const recaptchaReady =
    hasText(data.settings.recaptcha_site_key) &&
    !hasMissingSecret(data, RECAPTCHA_SECRET_KEY);

  return {
    smtpSettingsReady,
    smtpPasswordReady,
    ownerRecipientsReady,
    smtpReadyToTest: smtpSettingsReady && smtpPasswordReady,
    smtpTested: data.settings.status === "smtp_tested",
    recaptchaReady,
    installReady:
      smtpSettingsReady &&
      smtpPasswordReady &&
      ownerRecipientsReady &&
      recaptchaReady,
  };
}

export function getSMTPTestBlocker(
  data: ProjectEmailPayload,
  form: ProjectEmailSaveRequest,
) {
  if (!hasText(form.smtp_host)) {
    return "Add the SMTP host before testing.";
  }
  if (!Number(form.smtp_port)) {
    return "Add the SMTP port before testing.";
  }
  if (!hasText(form.smtp_user)) {
    return "Add the SMTP user before testing.";
  }
  if (!hasText(form.smtp_password) && hasMissingSecret(data, SMTP_PASSWORD_KEY)) {
    return "Add the SMTP password before testing.";
  }
  if (!hasText(form.email_from)) {
    return "Add the From Address before testing.";
  }
  return null;
}

function hasSMTPSettings(settings: ProjectEmailSettings) {
  return (
    hasText(settings.smtp_host) &&
    settings.smtp_port > 0 &&
    hasText(settings.smtp_user) &&
    hasText(settings.email_from)
  );
}

function hasMissingSecret(data: ProjectEmailPayload, key: string) {
  return data.status.required.missing_secrets.includes(key);
}

function hasText(value: string | null | undefined) {
  return Boolean(value?.trim());
}
