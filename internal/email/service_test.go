package email

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lefteq/lovinka-deployik/internal/crypto"
	"github.com/lefteq/lovinka-deployik/internal/db"
)

type fakeSender struct {
	cfg     SMTPConfig
	message Message
	err     error
	calls   int
}

func (f *fakeSender) Send(_ context.Context, cfg SMTPConfig, message Message) error {
	f.calls++
	f.cfg = cfg
	f.message = message
	return f.err
}

func newServiceTestDB(t *testing.T) (*db.DB, *crypto.Encryptor, *db.Project) {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	encryptor, err := crypto.NewEncryptor("test-encryption-key")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	user := &db.User{ID: db.NewID(), GithubID: 1, Username: "email-owner", Role: "admin"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	project := &db.Project{
		Name:           "acmegym",
		GithubRepo:     "web",
		GithubOwner:    "owner",
		Branch:         "main",
		UserID:         user.ID,
		Framework:      "nextjs",
		PackageManager: "pnpm",
		BuildCommand:   "pnpm build",
		InstallCommand: "pnpm install --frozen-lockfile",
		NodeVersion:    "22",
		Status:         "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if err := database.CreateDomain(&db.Domain{
		ProjectID:   project.ID,
		DomainName:  "acmegym.cz",
		Environment: "production",
		IsPrimary:   true,
		DNSVerified: true,
		SSLStatus:   "active",
	}); err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}
	return database, encryptor, project
}

func TestServiceBuildsDomainDefaults(t *testing.T) {
	database, encryptor, project := newServiceTestDB(t)
	service := NewService(database, encryptor, nil)

	payload, err := service.GetProjectPayload(context.Background(), project)
	if err != nil {
		t.Fatalf("GetProjectPayload: %v", err)
	}

	if payload.Settings.SMTPHost != "mail.webglobe.cz" {
		t.Fatalf("smtp_host = %q, want mail.webglobe.cz", payload.Settings.SMTPHost)
	}
	if payload.Settings.SMTPPort != 587 {
		t.Fatalf("smtp_port = %d, want 587", payload.Settings.SMTPPort)
	}
	if payload.Settings.SMTPUser != "noreply@acmegym.cz" {
		t.Fatalf("smtp_user = %q, want domain-derived noreply", payload.Settings.SMTPUser)
	}
	if payload.Settings.EmailFromName != "acmegym" {
		t.Fatalf("email_from_name = %q, want project name", payload.Settings.EmailFromName)
	}
	if !payload.Status.Required.EnvMissing {
		t.Fatal("expected env vars to be missing before save")
	}
}

func TestServiceSaveSettingsWritesEnvAndSecretsAndBuildsPrompt(t *testing.T) {
	database, encryptor, project := newServiceTestDB(t)
	service := NewService(database, encryptor, nil)

	payload, err := service.SaveProjectSettings(context.Background(), project, SaveRequest{
		Provider:                string(db.EmailProviderWebglobe),
		SMTPHost:                "mail.webglobe.cz",
		SMTPPort:                587,
		SMTPSecurity:            string(db.EmailSMTPSecurityStartTLS),
		SMTPUser:                "noreply@acmegym.cz",
		SMTPPassword:            "smtp-password",
		EmailFrom:               "noreply@acmegym.cz",
		EmailFromName:           "AcmeGym",
		ContactEmailTo:          "owner@acmegym.cz, sales@acmegym.cz",
		RecaptchaSiteKey:        "site-key",
		RecaptchaSecretKey:      "secret-key",
		RecaptchaScoreThreshold: 0.5,
	})
	if err != nil {
		t.Fatalf("SaveProjectSettings: %v", err)
	}

	if payload.Status.Required.EnvMissing {
		t.Fatal("expected required env vars to be present after save")
	}
	if payload.Status.Required.SecretsMissing {
		t.Fatal("expected required secrets to be present after save")
	}

	envs, err := database.ListProjectVariables(project.ID, "shared", db.VariableKindEnv)
	if err != nil {
		t.Fatalf("ListProjectVariables(env): %v", err)
	}
	secrets, err := database.ListProjectVariables(project.ID, "shared", db.VariableKindSecret)
	if err != nil {
		t.Fatalf("ListProjectVariables(secret): %v", err)
	}
	if !hasEncryptedVariable(t, encryptor, envs, "SMTP_HOST", "mail.webglobe.cz") {
		t.Fatal("missing SMTP_HOST env var")
	}
	if !hasEncryptedVariable(t, encryptor, envs, "NEXT_PUBLIC_RECAPTCHA_SITE_KEY", "site-key") {
		t.Fatal("missing NEXT_PUBLIC_RECAPTCHA_SITE_KEY env var")
	}
	if !hasEncryptedVariable(t, encryptor, secrets, "SMTP_PASSWORD", "smtp-password") {
		t.Fatal("missing SMTP_PASSWORD secret")
	}
	if !hasEncryptedVariable(t, encryptor, secrets, "RECAPTCHA_SECRET_KEY", "secret-key") {
		t.Fatal("missing RECAPTCHA_SECRET_KEY secret")
	}

	prompt := payload.Install.AIPrompt
	for _, expected := range []string{
		"Integrate a secure contact form email flow",
		"Next.js Node runtime API route",
		"nodemailer",
		"Google reCAPTCHA v3",
		"owner notification email",
		"branded confirmation email",
		"plain-text fallback",
		"SMTP_HOST",
		"RECAPTCHA_SECRET_KEY",
		"Do not expose SMTP credentials",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q\n%s", expected, prompt)
		}
	}
}

func TestServiceTestSMTPUsesStoredSecretsAndRecordsSuccess(t *testing.T) {
	database, encryptor, project := newServiceTestDB(t)
	sender := &fakeSender{}
	service := NewService(database, encryptor, sender)

	if _, err := service.SaveProjectSettings(context.Background(), project, SaveRequest{
		SMTPHost:                "mail.webglobe.cz",
		SMTPPort:                587,
		SMTPSecurity:            string(db.EmailSMTPSecurityStartTLS),
		SMTPUser:                "noreply@acmegym.cz",
		SMTPPassword:            "smtp-password",
		EmailFrom:               "noreply@acmegym.cz",
		EmailFromName:           "AcmeGym",
		ContactEmailTo:          "owner@acmegym.cz",
		RecaptchaSiteKey:        "site-key",
		RecaptchaSecretKey:      "secret-key",
		RecaptchaScoreThreshold: 0.5,
	}); err != nil {
		t.Fatalf("SaveProjectSettings: %v", err)
	}

	payload, err := service.TestProjectSMTP(context.Background(), project)
	if err != nil {
		t.Fatalf("TestProjectSMTP: %v", err)
	}

	if sender.calls != 1 {
		t.Fatalf("sender calls = %d, want 1", sender.calls)
	}
	if sender.cfg.Password != "smtp-password" {
		t.Fatalf("smtp password = %q, want decrypted password", sender.cfg.Password)
	}
	if len(sender.message.To) != 1 || sender.message.To[0] != "owner@acmegym.cz" {
		t.Fatalf("message.To = %#v, want owner recipient", sender.message.To)
	}
	if payload.Settings.Status != string(db.EmailStatusSMTPTested) {
		t.Fatalf("status = %q, want smtp_tested", payload.Settings.Status)
	}
	if payload.Settings.LastTestedAt == nil {
		t.Fatal("expected last_tested_at to be populated")
	}
}

func TestServiceTestSMTPFallsBackToFromAddressWhenOwnerRecipientsEmpty(t *testing.T) {
	database, encryptor, project := newServiceTestDB(t)
	sender := &fakeSender{}
	service := NewService(database, encryptor, sender)

	if _, err := service.SaveProjectSettings(context.Background(), project, SaveRequest{
		SMTPHost:                "mail.webglobe.cz",
		SMTPPort:                587,
		SMTPSecurity:            string(db.EmailSMTPSecurityStartTLS),
		SMTPUser:                "noreply@acmegym.cz",
		SMTPPassword:            "smtp-password",
		EmailFrom:               "noreply@acmegym.cz",
		EmailFromName:           "AcmeGym",
		ContactEmailTo:          "",
		RecaptchaSiteKey:        "",
		RecaptchaScoreThreshold: 0.5,
	}); err != nil {
		t.Fatalf("SaveProjectSettings: %v", err)
	}

	payload, err := service.TestProjectSMTP(context.Background(), project)
	if err != nil {
		t.Fatalf("TestProjectSMTP: %v", err)
	}

	if sender.calls != 1 {
		t.Fatalf("sender calls = %d, want 1", sender.calls)
	}
	if len(sender.message.To) != 1 || sender.message.To[0] != "noreply@acmegym.cz" {
		t.Fatalf("message.To = %#v, want from-address fallback", sender.message.To)
	}
	if payload.Settings.Status != string(db.EmailStatusSMTPTested) {
		t.Fatalf("status = %q, want smtp_tested", payload.Settings.Status)
	}
}

func TestServiceTestSMTPRecordsFailure(t *testing.T) {
	database, encryptor, project := newServiceTestDB(t)
	sender := &fakeSender{err: errors.New("auth failed")}
	service := NewService(database, encryptor, sender)

	if _, err := service.SaveProjectSettings(context.Background(), project, SaveRequest{
		SMTPHost:                "mail.webglobe.cz",
		SMTPPort:                587,
		SMTPSecurity:            string(db.EmailSMTPSecurityStartTLS),
		SMTPUser:                "noreply@acmegym.cz",
		SMTPPassword:            "smtp-password",
		EmailFrom:               "noreply@acmegym.cz",
		EmailFromName:           "AcmeGym",
		ContactEmailTo:          "owner@acmegym.cz",
		RecaptchaSiteKey:        "site-key",
		RecaptchaSecretKey:      "secret-key",
		RecaptchaScoreThreshold: 0.5,
	}); err != nil {
		t.Fatalf("SaveProjectSettings: %v", err)
	}

	payload, err := service.TestProjectSMTP(context.Background(), project)
	if err == nil {
		t.Fatal("expected TestProjectSMTP to return sender error")
	}
	if !strings.Contains(err.Error(), "auth failed") {
		t.Fatalf("expected auth failed error, got %v", err)
	}
	if payload.Settings.Status != string(db.EmailStatusError) {
		t.Fatalf("status = %q, want error", payload.Settings.Status)
	}
	if !strings.Contains(payload.Settings.LastTestError, "auth failed") {
		t.Fatalf("last_test_error = %q, want auth failure", payload.Settings.LastTestError)
	}
}

func TestSplitRecipientsIgnoresDelimiterOnlyInput(t *testing.T) {
	if got := splitRecipients(",,;\n"); len(got) != 0 {
		t.Fatalf("splitRecipients delimiter-only = %#v, want empty", got)
	}
}

func TestPickPrimaryProductionDomainPrecedence(t *testing.T) {
	cases := []struct {
		name    string
		domains []db.Domain
		want    string
	}{
		{
			name:    "empty",
			domains: nil,
			want:    "",
		},
		{
			name: "primary production wins even when auto",
			domains: []db.Domain{
				{DomainName: "z-custom.com", Environment: "production", IsAuto: false},
				{DomainName: "auto.preview.example.com", Environment: "production", IsAuto: true, IsPrimary: true},
			},
			want: "auto.preview.example.com",
		},
		{
			name: "no primary: production non-auto beats production auto",
			domains: []db.Domain{
				{DomainName: "auto.preview.example.com", Environment: "production", IsAuto: true},
				{DomainName: "acmesite.cz", Environment: "production", IsAuto: false},
			},
			want: "acmesite.cz",
		},
		{
			name: "no primary: only production auto exists",
			domains: []db.Domain{
				{DomainName: "auto.preview.example.com", Environment: "production", IsAuto: true},
			},
			want: "auto.preview.example.com",
		},
		{
			name: "primary preview is ignored when production exists",
			domains: []db.Domain{
				{DomainName: "preview-primary.com", Environment: "preview", IsPrimary: true, IsAuto: false},
				{DomainName: "prod.com", Environment: "production", IsAuto: false},
			},
			want: "prod.com",
		},
		{
			name: "no production: prefer non-auto preview",
			domains: []db.Domain{
				{DomainName: "auto.preview.example.com", Environment: "preview", IsAuto: true},
				{DomainName: "staging.acmesite.cz", Environment: "preview", IsAuto: false},
			},
			want: "staging.acmesite.cz",
		},
		{
			name: "fallback to first when only auto preview exists",
			domains: []db.Domain{
				{DomainName: "auto.preview.example.com", Environment: "preview", IsAuto: true},
			},
			want: "auto.preview.example.com",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pickPrimaryProductionDomain(tc.domains)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildSiteContext(t *testing.T) {
	t.Run("empty domains", func(t *testing.T) {
		ctx := buildSiteContext(nil)
		if ctx.SiteURL != "" {
			t.Fatalf("SiteURL = %q, want empty", ctx.SiteURL)
		}
		if len(ctx.AllowedHosts) != 0 || len(ctx.ProductionHosts) != 0 || len(ctx.PreviewHosts) != 0 {
			t.Fatalf("expected all host slices empty, got %#v", ctx)
		}
	})

	t.Run("mixed preview + production + auto + duplicates", func(t *testing.T) {
		ctx := buildSiteContext([]db.Domain{
			{DomainName: "acmesite.preview.example.com", Environment: "preview", IsAuto: true},
			{DomainName: "acmegym.cz", Environment: "production", IsAuto: false, IsPrimary: true},
			{DomainName: "www.acmegym.cz", Environment: "production", IsAuto: false},
			{DomainName: "acmegym.cz", Environment: "production", IsAuto: false, IsPrimary: true}, // dup
			{DomainName: "  ", Environment: "production"},                                           // blank — ignored
		})
		if ctx.SiteURL != "https://acmegym.cz" {
			t.Fatalf("SiteURL = %q, want https://acmegym.cz", ctx.SiteURL)
		}
		wantAll := []string{"acmegym.cz", "acmesite.preview.example.com", "www.acmegym.cz"}
		if !equalStrings(ctx.AllowedHosts, wantAll) {
			t.Fatalf("AllowedHosts = %#v, want %#v", ctx.AllowedHosts, wantAll)
		}
		wantProd := []string{"acmegym.cz", "www.acmegym.cz"}
		if !equalStrings(ctx.ProductionHosts, wantProd) {
			t.Fatalf("ProductionHosts = %#v, want %#v", ctx.ProductionHosts, wantProd)
		}
		wantPrev := []string{"acmesite.preview.example.com"}
		if !equalStrings(ctx.PreviewHosts, wantPrev) {
			t.Fatalf("PreviewHosts = %#v, want %#v", ctx.PreviewHosts, wantPrev)
		}
	})
}

func TestServiceWritesSiteVariablesAndPromptIncludesContext(t *testing.T) {
	database, encryptor, project := newServiceTestDB(t)
	// Add a www variant + a preview auto-domain so the allowlist is non-trivial.
	if err := database.CreateDomain(&db.Domain{
		ProjectID:   project.ID,
		DomainName:  "www.acmegym.cz",
		Environment: "production",
		DNSVerified: true,
		SSLStatus:   "active",
	}); err != nil {
		t.Fatalf("CreateDomain www: %v", err)
	}
	if err := database.CreateDomain(&db.Domain{
		ProjectID:   project.ID,
		DomainName:  "acmegym.preview.example.com",
		Environment: "preview",
		IsAuto:      true,
		DNSVerified: true,
		SSLStatus:   "active",
	}); err != nil {
		t.Fatalf("CreateDomain preview: %v", err)
	}

	service := NewService(database, encryptor, nil)
	payload, err := service.SaveProjectSettings(context.Background(), project, SaveRequest{
		SMTPHost:                "mail.webglobe.cz",
		SMTPPort:                587,
		SMTPSecurity:            string(db.EmailSMTPSecurityStartTLS),
		SMTPUser:                "noreply@acmegym.cz",
		SMTPPassword:            "smtp-password",
		EmailFrom:               "noreply@acmegym.cz",
		EmailFromName:           "AcmeGym",
		ContactEmailTo:          "owner@acmegym.cz",
		RecaptchaSiteKey:        "site-key",
		RecaptchaSecretKey:      "secret-key",
		RecaptchaScoreThreshold: 0.5,
	})
	if err != nil {
		t.Fatalf("SaveProjectSettings: %v", err)
	}

	envs, err := database.ListProjectVariables(project.ID, "shared", db.VariableKindEnv)
	if err != nil {
		t.Fatalf("ListProjectVariables: %v", err)
	}
	if !hasEncryptedVariable(t, encryptor, envs, "SITE_URL", "https://acmegym.cz") {
		t.Fatal("SITE_URL not provisioned to https://acmegym.cz")
	}
	if !hasEncryptedVariable(t, encryptor, envs, "RECAPTCHA_ALLOWED_HOSTS",
		"acmegym.cz,acmegym.preview.example.com,www.acmegym.cz") {
		t.Fatalf("RECAPTCHA_ALLOWED_HOSTS not provisioned with all hostnames; envs=%#v", envs)
	}

	// 9 original + SITE_URL + RECAPTCHA_ALLOWED_HOSTS = 11 env keys
	if len(envs) != len(requiredEnvKeys) {
		t.Fatalf("env count = %d, want %d", len(envs), len(requiredEnvKeys))
	}

	prompt := payload.Install.AIPrompt
	for _, expected := range []string{
		"Deployment context",
		"Production hostnames: acmegym.cz, www.acmegym.cz",
		"Preview hostnames: acmegym.preview.example.com",
		"Canonical site URL: https://acmegym.cz",
		"X-Real-IP",
		"APPENDS to X-Forwarded-For",
		"SITE_URL",
		"RECAPTCHA_ALLOWED_HOSTS",
		"5 requests per 10 minutes",
		"i18n / locale setup",
		"Use SITE_URL to build absolute URLs",
		"Do not invent a new env var for this",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q\n--- prompt ---\n%s", expected, prompt)
		}
	}
}

func TestGetProjectPayloadBackfillsSiteVariablesForLegacyProjects(t *testing.T) {
	database, encryptor, project := newServiceTestDB(t)

	// Persist email settings WITHOUT writing the new env vars (simulates a
	// project that was email-configured before this change).
	record := &db.ProjectEmailSettings{
		ProjectID:               project.ID,
		Provider:                db.EmailProviderWebglobe,
		SMTPHost:                "mail.webglobe.cz",
		SMTPPort:                587,
		SMTPSecurity:            db.EmailSMTPSecurityStartTLS,
		SMTPUser:                "noreply@acmegym.cz",
		EmailFrom:               "noreply@acmegym.cz",
		EmailFromName:           "AcmeGym",
		ContactEmailTo:          "owner@acmegym.cz",
		RecaptchaSiteKey:        "site-key",
		RecaptchaMode:           db.EmailRecaptchaModeV3,
		RecaptchaScoreThreshold: 0.5,
		Status:                  db.EmailStatusReadyToInstall,
	}
	if err := database.UpsertProjectEmailSettings(record); err != nil {
		t.Fatalf("UpsertProjectEmailSettings: %v", err)
	}

	// Confirm the env table is empty before backfill.
	envsBefore, err := database.ListProjectVariableKeys(project.ID, db.VariableKindEnv)
	if err != nil {
		t.Fatalf("ListProjectVariableKeys before: %v", err)
	}
	if len(envsBefore) != 0 {
		t.Fatalf("expected no env vars before backfill, got %#v", envsBefore)
	}

	service := NewService(database, encryptor, nil)
	if _, err := service.GetProjectPayload(context.Background(), project); err != nil {
		t.Fatalf("GetProjectPayload: %v", err)
	}

	// After the read, only SITE_URL + RECAPTCHA_ALLOWED_HOSTS should be backfilled
	// (the other 9 are filled by Save, not by Get).
	envsAfter, err := database.ListProjectVariables(project.ID, "shared", db.VariableKindEnv)
	if err != nil {
		t.Fatalf("ListProjectVariables after: %v", err)
	}
	if !hasEncryptedVariable(t, encryptor, envsAfter, "SITE_URL", "https://acmegym.cz") {
		t.Fatalf("SITE_URL not backfilled; envs=%#v", envsAfter)
	}
	if !hasEncryptedVariable(t, encryptor, envsAfter, "RECAPTCHA_ALLOWED_HOSTS", "acmegym.cz") {
		t.Fatalf("RECAPTCHA_ALLOWED_HOSTS not backfilled; envs=%#v", envsAfter)
	}
	if len(envsAfter) != 2 {
		t.Fatalf("expected only the 2 backfilled vars, got %d: %#v", len(envsAfter), envsAfter)
	}

	// Idempotency: a second read should not duplicate or change anything.
	if _, err := service.GetProjectPayload(context.Background(), project); err != nil {
		t.Fatalf("GetProjectPayload (second): %v", err)
	}
	envsAfterSecond, _ := database.ListProjectVariables(project.ID, "shared", db.VariableKindEnv)
	if len(envsAfterSecond) != 2 {
		t.Fatalf("backfill is not idempotent: %d vars after second read", len(envsAfterSecond))
	}
}

func TestGetProjectPayloadEmptyDomainsDoesNotPanic(t *testing.T) {
	database, encryptor, _ := newServiceTestDB(t)

	user := &db.User{ID: db.NewID(), GithubID: 99, Username: "lonely", Role: "admin"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	bare := &db.Project{
		Name:           "noprod",
		GithubRepo:     "x",
		GithubOwner:    "y",
		Branch:         "main",
		UserID:         user.ID,
		Framework:      "nextjs",
		PackageManager: "auto",
		BuildCommand:   "b",
		InstallCommand: "i",
		NodeVersion:    "22",
		Status:         "active",
	}
	if err := database.CreateProject(bare); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	service := NewService(database, encryptor, nil)
	payload, err := service.GetProjectPayload(context.Background(), bare)
	if err != nil {
		t.Fatalf("GetProjectPayload: %v", err)
	}
	if !strings.Contains(payload.Install.AIPrompt, "Production hostnames: (none)") {
		t.Fatalf("expected '(none)' for empty production hostnames\n%s", payload.Install.AIPrompt)
	}
	if !strings.Contains(payload.Install.AIPrompt, "Canonical site URL: (none yet") {
		t.Fatalf("expected fallback for empty SITE_URL\n%s", payload.Install.AIPrompt)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasEncryptedVariable(t *testing.T, encryptor *crypto.Encryptor, variables []db.ProjectVariable, key, want string) bool {
	t.Helper()
	for _, variable := range variables {
		if variable.Key != key {
			continue
		}
		got, err := encryptor.Decrypt(variable.Value)
		if err != nil {
			t.Fatalf("Decrypt(%s): %v", key, err)
		}
		return got == want
	}
	return false
}
