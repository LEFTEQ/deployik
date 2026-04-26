package db

import "testing"

func TestProjectEmailSettingsUpsertAndDelete(t *testing.T) {
	database := newTestDB(t)
	user := &User{ID: NewID(), GithubID: 1, Username: "email-owner", Role: "admin"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	project := &Project{
		Name:           "email-demo",
		GithubRepo:     "web",
		GithubOwner:    "owner",
		Branch:         "main",
		UserID:         user.ID,
		Framework:      "nextjs",
		BuildCommand:   "bun run build",
		InstallCommand: "bun install",
		NodeVersion:    "22",
		Status:         "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	record := &ProjectEmailSettings{
		ProjectID:               project.ID,
		Provider:                EmailProviderWebglobe,
		SMTPHost:                "mail.webglobe.cz",
		SMTPPort:                587,
		SMTPSecurity:            EmailSMTPSecurityStartTLS,
		SMTPUser:                "noreply@example.com",
		EmailFrom:               "noreply@example.com",
		EmailFromName:           "Email Demo",
		ContactEmailTo:          "owner@example.com",
		RecaptchaSiteKey:        "site-key",
		RecaptchaMode:           EmailRecaptchaModeV3,
		RecaptchaScoreThreshold: 0.5,
		Status:                  EmailStatusReadyToInstall,
	}

	if err := database.UpsertProjectEmailSettings(record); err != nil {
		t.Fatalf("UpsertProjectEmailSettings: %v", err)
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		t.Fatal("expected timestamps to be populated after upsert")
	}

	got, err := database.GetProjectEmailSettings(project.ID)
	if err != nil {
		t.Fatalf("GetProjectEmailSettings: %v", err)
	}
	if got == nil {
		t.Fatal("expected settings row")
	}
	if got.Provider != EmailProviderWebglobe {
		t.Fatalf("provider = %q, want %q", got.Provider, EmailProviderWebglobe)
	}
	if got.SMTPPort != 587 {
		t.Fatalf("smtp_port = %d, want 587", got.SMTPPort)
	}
	if got.RecaptchaScoreThreshold != 0.5 {
		t.Fatalf("recaptcha_score_threshold = %v, want 0.5", got.RecaptchaScoreThreshold)
	}

	got.Status = EmailStatusSMTPTested
	got.LastTestError = ""
	if err := database.UpsertProjectEmailSettings(got); err != nil {
		t.Fatalf("UpsertProjectEmailSettings(update): %v", err)
	}
	updated, err := database.GetProjectEmailSettings(project.ID)
	if err != nil {
		t.Fatalf("GetProjectEmailSettings(updated): %v", err)
	}
	if updated.Status != EmailStatusSMTPTested {
		t.Fatalf("status = %q, want %q", updated.Status, EmailStatusSMTPTested)
	}

	if err := database.DeleteProjectEmailSettings(project.ID); err != nil {
		t.Fatalf("DeleteProjectEmailSettings: %v", err)
	}
	deleted, err := database.GetProjectEmailSettings(project.ID)
	if err != nil {
		t.Fatalf("GetProjectEmailSettings(deleted): %v", err)
	}
	if deleted != nil {
		t.Fatal("expected settings row to be deleted")
	}
}
