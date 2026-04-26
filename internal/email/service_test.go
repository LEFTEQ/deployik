package email

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
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
		EmailFromName:           "acmegym",
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
		EmailFromName:           "acmegym",
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
		EmailFromName:           "acmegym",
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
		EmailFromName:           "acmegym",
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
