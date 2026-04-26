import { useEffect, useMemo, useState } from "react";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CircleHelp,
  ExternalLink,
  Mail,
  Save,
  Send,
  ShieldCheck,
  TerminalSquare,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { CodePanel } from "@/components/ui/code-panel";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Textarea } from "@/components/ui/textarea";
import { LoadingState, Spinner } from "@/components/ui/spinner";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { cn } from "@/lib/utils";
import type {
  ProjectEmailPayload,
  ProjectEmailSaveRequest,
  ProjectEmailSettings,
} from "@/types/api";

type EmailFormState = ProjectEmailSaveRequest;

const emptyForm: EmailFormState = {
  provider: "webglobe",
  smtp_host: "mail.webglobe.cz",
  smtp_port: 587,
  smtp_security: "starttls",
  smtp_user: "",
  smtp_password: "",
  email_from: "",
  email_from_name: "",
  contact_email_to: "",
  recaptcha_site_key: "",
  recaptcha_secret_key: "",
  recaptcha_score_threshold: 0.5,
};

export function ProjectEmailTab({ projectId }: { projectId: string }) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState<EmailFormState>(emptyForm);

  const { data, isLoading, error } = useQuery({
    queryKey: queryKeys.projectEmail(projectId),
    queryFn: () => api.getProjectEmail(projectId),
  });

  useEffect(() => {
    if (!data) return;
    setForm(formFromSettings(data.settings));
  }, [data]);

  const saveMutation = useMutation({
    mutationFn: () => api.saveProjectEmail(projectId, normalizeForm(form)),
    onSuccess: (payload) => {
      queryClient.setQueryData(queryKeys.projectEmail(projectId), payload);
      setForm(formFromSettings(payload.settings));
      toast.success("Email settings saved");
    },
    onError: (err) => toast.error(err.message),
  });

  const testMutation = useMutation({
    mutationFn: () => api.testProjectEmailSmtp(projectId),
    onSuccess: (payload) => {
      queryClient.setQueryData(queryKeys.projectEmail(projectId), payload);
      setForm(formFromSettings(payload.settings));
      toast.success("SMTP test email sent");
    },
    onError: (err) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.projectEmail(projectId) });
      toast.error(err.message);
    },
  });

  const copyValue = async (value: string, label: string) => {
    if (!value.trim()) {
      toast.error(`${label} is not available yet`);
      return;
    }
    try {
      await navigator.clipboard.writeText(value);
      toast.success(`${label} copied`);
    } catch {
      toast.error(`Couldn't copy ${label.toLowerCase()}`);
    }
  };

  const statusCards = useMemo(() => {
    if (!data) return [];
    return buildStatusCards(data);
  }, [data]);

  if (isLoading) {
    return (
      <LoadingState
        title="Loading email setup..."
        description="Preparing SMTP defaults, reCAPTCHA state, and the install prompt."
        className="min-h-[340px]"
      />
    );
  }

  if (error || !data) {
    return (
      <div className="rounded-xl border border-destructive/30 bg-destructive/10 px-5 py-4 text-sm text-destructive-foreground">
        {error instanceof Error ? error.message : "Unknown email setup error."}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h2 className="text-lg font-semibold tracking-tight text-foreground">
            Email Support
          </h2>
          <p className="mt-1 max-w-3xl text-sm text-muted-foreground">
            Configure Webglobe SMTP and reCAPTCHA, then install a secure
            Next.js contact endpoint with a generated AI prompt.
          </p>
        </div>
        <div className="flex shrink-0 gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => saveMutation.mutate()}
            disabled={saveMutation.isPending}
          >
            {saveMutation.isPending ? (
              <Spinner data-icon="inline-start" />
            ) : (
              <Save data-icon="inline-start" />
            )}
            Save Settings
          </Button>
          <Button
            size="sm"
            onClick={() => testMutation.mutate()}
            disabled={testMutation.isPending || data.status.required.secrets_missing}
          >
            {testMutation.isPending ? (
              <Spinner data-icon="inline-start" />
            ) : (
              <Send data-icon="inline-start" />
            )}
            Test SMTP
          </Button>
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-3">
        {statusCards.map((card) => (
          <StatusCard key={card.title} {...card} />
        ))}
      </div>

      <Card>
        <CardHeader>
          <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <CardTitle>Configuration</CardTitle>
              <CardDescription>
                Saving writes these values into the shared project environment.
                Secrets are encrypted and are not returned after saving.
              </CardDescription>
            </div>
            <Badge variant={data.status.configured ? "secondary" : "outline"}>
              {data.status.configured ? "Configured" : "Needs values"}
            </Badge>
          </div>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col gap-6">
            <FormSection
              icon={Mail}
              title="Sender"
              description="Use the Webglobe mailbox that will authenticate SMTP."
            >
              <div className="grid gap-4 md:grid-cols-2">
                <Field label="SMTP Host" htmlFor="smtp-host">
                  <Input
                    id="smtp-host"
                    value={form.smtp_host}
                    onChange={(event) =>
                      patchForm({ smtp_host: event.target.value })
                    }
                    placeholder="mail.webglobe.cz"
                  />
                </Field>
                <Field label="SMTP Port" htmlFor="smtp-port">
                  <Input
                    id="smtp-port"
                    type="number"
                    min={1}
                    value={form.smtp_port}
                    onChange={(event) =>
                      patchForm({ smtp_port: Number(event.target.value) })
                    }
                  />
                </Field>
                <Field label="Security" htmlFor="smtp-security">
                  <Select
                    value={form.smtp_security}
                    onValueChange={(value) =>
                      patchForm({
                        smtp_security: value as EmailFormState["smtp_security"],
                      })
                    }
                  >
                    <SelectTrigger id="smtp-security" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem value="starttls">STARTTLS</SelectItem>
                        <SelectItem value="tls">TLS / SSL</SelectItem>
                        <SelectItem value="none">None</SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </Field>
                <Field label="SMTP User" htmlFor="smtp-user">
                  <Input
                    id="smtp-user"
                    type="email"
                    value={form.smtp_user}
                    onChange={(event) =>
                      patchForm({ smtp_user: event.target.value })
                    }
                    placeholder="noreply@acmegym.cz"
                  />
                </Field>
                <Field label="SMTP Password" htmlFor="smtp-password">
                  <Input
                    id="smtp-password"
                    type="password"
                    value={form.smtp_password ?? ""}
                    onChange={(event) =>
                      patchForm({ smtp_password: event.target.value })
                    }
                    placeholder="Leave blank to keep existing secret"
                  />
                </Field>
                <Field label="From Name" htmlFor="email-from-name">
                  <Input
                    id="email-from-name"
                    value={form.email_from_name}
                    onChange={(event) =>
                      patchForm({ email_from_name: event.target.value })
                    }
                    placeholder="acmegym"
                  />
                </Field>
                <Field label="From Address" htmlFor="email-from">
                  <Input
                    id="email-from"
                    type="email"
                    value={form.email_from}
                    onChange={(event) =>
                      patchForm({ email_from: event.target.value })
                    }
                    placeholder="noreply@acmegym.cz"
                  />
                </Field>
              </div>
            </FormSection>

            <Separator />

            <FormSection
              icon={ShieldCheck}
              title="Recipients and reCAPTCHA"
              description="Owner notifications go to explicit recipients. reCAPTCHA v3 protects the route before any email is sent."
            >
              <div className="grid gap-4 md:grid-cols-2">
                <Field
                  label="Owner Recipients"
                  htmlFor="contact-email-to"
                  className="md:col-span-2"
                >
                  <Textarea
                    id="contact-email-to"
                    value={form.contact_email_to}
                    onChange={(event) =>
                      patchForm({ contact_email_to: event.target.value })
                    }
                    placeholder="owner@acmegym.cz, sales@acmegym.cz"
                    className="min-h-20"
                  />
                </Field>
                <Field label="reCAPTCHA Site Key" htmlFor="recaptcha-site-key">
                  <Input
                    id="recaptcha-site-key"
                    value={form.recaptcha_site_key}
                    onChange={(event) =>
                      patchForm({ recaptcha_site_key: event.target.value })
                    }
                  />
                </Field>
                <Field
                  label="reCAPTCHA Secret Key"
                  htmlFor="recaptcha-secret-key"
                >
                  <Input
                    id="recaptcha-secret-key"
                    type="password"
                    value={form.recaptcha_secret_key ?? ""}
                    onChange={(event) =>
                      patchForm({ recaptcha_secret_key: event.target.value })
                    }
                    placeholder="Leave blank to keep existing secret"
                  />
                </Field>
                <Field label="Score Threshold" htmlFor="recaptcha-threshold">
                  <Input
                    id="recaptcha-threshold"
                    type="number"
                    min={0}
                    max={1}
                    step={0.1}
                    value={form.recaptcha_score_threshold}
                    onChange={(event) =>
                      patchForm({
                        recaptcha_score_threshold: Number(event.target.value),
                      })
                    }
                  />
                </Field>
              </div>
            </FormSection>

            {data.settings.last_test_error ? (
              <div className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive-foreground">
                {data.settings.last_test_error}
              </div>
            ) : null}
            {data.settings.last_tested_at ? (
              <p className="text-xs text-muted-foreground">
                Last SMTP test:{" "}
                {new Date(data.settings.last_tested_at).toLocaleString()}
              </p>
            ) : null}
          </div>
        </CardContent>
      </Card>

      <CodePanel
        title="AI Install Prompt"
        description="Paste this into Codex, Claude, or ChatGPT inside the Next.js app repository."
        value={data.install.ai_prompt}
        onCopy={() => copyValue(data.install.ai_prompt, "AI install prompt")}
        heightClassName="h-[30rem]"
      />

      <EmailHelpPanel />
    </div>
  );

  function patchForm(patch: Partial<EmailFormState>) {
    setForm((current) => ({ ...current, ...patch }));
  }
}

function EmailHelpPanel() {
  const items = [
    {
      title: "Webglobe SMTP",
      body:
        "Use the mailbox address as SMTP_USER. Webglobe lists mail.webglobe.cz with port 587 for TLS, port 465 for SSL, and authenticated outgoing mail.",
    },
    {
      title: "Google reCAPTCHA v3",
      body:
        "Create a v3 key pair, add the production and preview domains, store the site key as NEXT_PUBLIC_RECAPTCHA_SITE_KEY, and keep the secret server-only.",
    },
    {
      title: "Next.js contact route",
      body:
        "Use a Node runtime API route for Nodemailer. Verify reCAPTCHA before validation passes into the mail send, and never accept recipients from the browser.",
    },
    {
      title: "Troubleshooting",
      body:
        "Authentication failures usually mean a mailbox password mismatch. Missing submissions usually mean the app was not redeployed after env changes or the route is running on Edge.",
    },
  ];

  return (
    <div className="rounded-lg border bg-card text-card-foreground">
      <div className="flex flex-col gap-2 border-b px-5 py-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <CircleHelp className="text-muted-foreground" />
            <h3 className="font-medium">Help and how-to</h3>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            Operational notes for Webglobe mailboxes, reCAPTCHA, and Next.js contact forms.
          </p>
        </div>
        <Button variant="outline" size="sm" asChild>
          <a
            href="https://www.webglobe.cz/poradna/jake-parametry-pro-nastaveni-emailoveho-klienta-pouzit"
            target="_blank"
            rel="noreferrer"
          >
            Webglobe SMTP
            <ExternalLink data-icon="inline-end" />
          </a>
        </Button>
      </div>
      <div className="grid gap-0 md:grid-cols-2">
        {items.map((item, index) => (
          <div
            key={item.title}
            className={cn(
              "flex flex-col gap-1 px-5 py-4",
              index % 2 === 0 && "md:border-r",
              index < 2 && "border-b",
            )}
          >
            <h4 className="text-sm font-medium">{item.title}</h4>
            <p className="text-sm text-muted-foreground">{item.body}</p>
          </div>
        ))}
      </div>
    </div>
  );
}

function formFromSettings(settings: ProjectEmailSettings): EmailFormState {
  return {
    provider: settings.provider,
    smtp_host: settings.smtp_host || emptyForm.smtp_host,
    smtp_port: settings.smtp_port || emptyForm.smtp_port,
    smtp_security: settings.smtp_security || emptyForm.smtp_security,
    smtp_user: settings.smtp_user,
    smtp_password: "",
    email_from: settings.email_from,
    email_from_name: settings.email_from_name,
    contact_email_to: settings.contact_email_to,
    recaptcha_site_key: settings.recaptcha_site_key,
    recaptcha_secret_key: "",
    recaptcha_score_threshold:
      settings.recaptcha_score_threshold || emptyForm.recaptcha_score_threshold,
  };
}

function normalizeForm(form: EmailFormState): ProjectEmailSaveRequest {
  return {
    ...form,
    smtp_host: form.smtp_host.trim(),
    smtp_port: Number(form.smtp_port) || 587,
    smtp_user: form.smtp_user.trim(),
    smtp_password: form.smtp_password?.trim(),
    email_from: form.email_from.trim(),
    email_from_name: form.email_from_name.trim(),
    contact_email_to: form.contact_email_to.trim(),
    recaptcha_site_key: form.recaptcha_site_key.trim(),
    recaptcha_secret_key: form.recaptcha_secret_key?.trim(),
    recaptcha_score_threshold: Number(form.recaptcha_score_threshold) || 0.5,
  };
}

function buildStatusCards(data: ProjectEmailPayload) {
  const settingsReady = !data.status.required.env_missing;
  const secretsReady = !data.status.required.secrets_missing;
  const smtpTested = data.settings.status === "smtp_tested";
  return [
    {
      title: "SMTP",
      description: smtpTested
        ? "Test email sent successfully."
        : settingsReady && secretsReady
          ? "Configured, ready to test."
          : "Needs sender credentials.",
      icon: Mail,
      state: smtpTested ? "complete" : settingsReady ? "pending" : "missing",
      badge: smtpTested ? "Tested" : settingsReady ? "Ready" : "Missing",
    },
    {
      title: "reCAPTCHA",
      description: secretsReady
        ? "v3 keys are provisioned."
        : "Add site and secret keys.",
      icon: ShieldCheck,
      state: secretsReady ? "complete" : "missing",
      badge: secretsReady ? "Ready" : "Missing",
    },
    {
      title: "Install Prompt",
      description: data.status.configured
        ? "Prompt is ready for the app repo."
        : "Prompt will include missing-key guidance.",
      icon: TerminalSquare,
      state: data.status.configured ? "complete" : "pending",
      badge: data.status.configured ? "Ready" : "Draft",
    },
  ] as const;
}

function StatusCard({
  title,
  description,
  icon: Icon,
  state,
  badge,
}: ReturnType<typeof buildStatusCards>[number]) {
  const complete = state === "complete";
  const missing = state === "missing";
  return (
    <Card>
      <CardHeader className="gap-3">
        <div className="flex items-start justify-between gap-3">
          <div
            className={cn(
              "flex size-10 items-center justify-center rounded-lg border",
              complete
                ? "border-success/30 bg-success/10 text-success"
                : missing
                  ? "border-destructive/30 bg-destructive/10 text-destructive"
                  : "border-warning/30 bg-warning/10 text-warning",
            )}
          >
            <Icon />
          </div>
          <Badge variant={complete ? "secondary" : "outline"}>{badge}</Badge>
        </div>
        <div>
          <CardTitle className="text-base">{title}</CardTitle>
          <CardDescription>{description}</CardDescription>
        </div>
      </CardHeader>
    </Card>
  );
}

function FormSection({
  icon: Icon,
  title,
  description,
  children,
}: {
  icon: typeof Mail;
  title: string;
  description: string;
  children: React.ReactNode;
}) {
  return (
    <section className="flex flex-col gap-4">
      <div className="flex items-start gap-3">
        <div className="flex size-9 shrink-0 items-center justify-center rounded-lg border bg-muted/40 text-muted-foreground">
          <Icon />
        </div>
        <div>
          <h3 className="font-medium text-foreground">{title}</h3>
          <p className="text-sm text-muted-foreground">{description}</p>
        </div>
      </div>
      {children}
    </section>
  );
}

function Field({
  label,
  htmlFor,
  className,
  children,
}: {
  label: string;
  htmlFor: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
    </div>
  );
}
