package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/mail"
	"net/smtp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

const (
	defaultSMTPHost        = "mail.webglobe.cz"
	defaultSMTPPort        = 587
	defaultRecaptchaScore  = 0.5
	sharedEnvironment      = "shared"
	smtpPasswordKey        = "SMTP_PASSWORD"
	recaptchaSecretKeyName = "RECAPTCHA_SECRET_KEY"
)

var requiredEnvKeys = []string{
	"SMTP_HOST",
	"SMTP_PORT",
	"SMTP_SECURE",
	"SMTP_USER",
	"EMAIL_FROM",
	"EMAIL_FROM_NAME",
	"CONTACT_EMAIL_TO",
	"NEXT_PUBLIC_RECAPTCHA_SITE_KEY",
	"RECAPTCHA_SCORE_THRESHOLD",
}

var requiredSecretKeys = []string{
	smtpPasswordKey,
	recaptchaSecretKeyName,
}

type Service struct {
	DB        *db.DB
	Encryptor *crypto.Encryptor
	Sender    Sender
}

type Sender interface {
	Send(ctx context.Context, cfg SMTPConfig, message Message) error
}

type SMTPConfig struct {
	Host     string
	Port     int
	Security db.EmailSMTPSecurity
	Username string
	Password string
	From     string
	FromName string
}

type Message struct {
	To      []string
	Subject string
	Text    string
}

type SaveRequest struct {
	Provider                string  `json:"provider"`
	SMTPHost                string  `json:"smtp_host"`
	SMTPPort                int     `json:"smtp_port"`
	SMTPSecurity            string  `json:"smtp_security"`
	SMTPUser                string  `json:"smtp_user"`
	SMTPPassword            string  `json:"smtp_password"`
	EmailFrom               string  `json:"email_from"`
	EmailFromName           string  `json:"email_from_name"`
	ContactEmailTo          string  `json:"contact_email_to"`
	RecaptchaSiteKey        string  `json:"recaptcha_site_key"`
	RecaptchaSecretKey      string  `json:"recaptcha_secret_key"`
	RecaptchaScoreThreshold float64 `json:"recaptcha_score_threshold"`
}

type ProjectPayload struct {
	Settings SettingsPayload `json:"settings"`
	Status   StatusPayload   `json:"status"`
	Install  InstallPayload  `json:"install"`
}

type SettingsPayload struct {
	ProjectID               string     `json:"project_id"`
	Provider                string     `json:"provider"`
	SMTPHost                string     `json:"smtp_host"`
	SMTPPort                int        `json:"smtp_port"`
	SMTPSecurity            string     `json:"smtp_security"`
	SMTPUser                string     `json:"smtp_user"`
	EmailFrom               string     `json:"email_from"`
	EmailFromName           string     `json:"email_from_name"`
	ContactEmailTo          string     `json:"contact_email_to"`
	RecaptchaSiteKey        string     `json:"recaptcha_site_key"`
	RecaptchaMode           string     `json:"recaptcha_mode"`
	RecaptchaScoreThreshold float64    `json:"recaptcha_score_threshold"`
	Status                  string     `json:"status"`
	LastTestedAt            *time.Time `json:"last_tested_at,omitempty"`
	LastTestError           string     `json:"last_test_error,omitempty"`
}

type StatusPayload struct {
	Configured bool           `json:"configured"`
	Required   RequiredStatus `json:"required"`
}

type RequiredStatus struct {
	EnvMissing     bool     `json:"env_missing"`
	SecretsMissing bool     `json:"secrets_missing"`
	MissingEnv     []string `json:"missing_env"`
	MissingSecrets []string `json:"missing_secrets"`
}

type InstallPayload struct {
	AIPrompt string   `json:"ai_prompt"`
	EnvKeys  []string `json:"env_keys"`
}

func NewService(database *db.DB, encryptor *crypto.Encryptor, sender Sender) *Service {
	if sender == nil {
		sender = SMTPSender{}
	}
	return &Service{DB: database, Encryptor: encryptor, Sender: sender}
}

func (s *Service) GetProjectPayload(ctx context.Context, project *db.Project) (ProjectPayload, error) {
	if project == nil {
		return ProjectPayload{}, fmt.Errorf("project is required")
	}

	record, err := s.DB.GetProjectEmailSettings(project.ID)
	if err != nil {
		return ProjectPayload{}, err
	}
	if record == nil {
		record = s.defaultSettings(ctx, project)
	}
	return s.buildPayload(project, record)
}

func (s *Service) SaveProjectSettings(ctx context.Context, project *db.Project, req SaveRequest) (ProjectPayload, error) {
	if project == nil {
		return ProjectPayload{}, fmt.Errorf("project is required")
	}

	record := s.recordFromRequest(project, req)
	if err := s.DB.UpsertProjectEmailSettings(record); err != nil {
		return ProjectPayload{}, err
	}
	if err := s.writeVariables(record, req); err != nil {
		return ProjectPayload{}, err
	}
	return s.buildPayload(project, record)
}

func (s *Service) TestProjectSMTP(ctx context.Context, project *db.Project) (ProjectPayload, error) {
	if project == nil {
		return ProjectPayload{}, fmt.Errorf("project is required")
	}
	record, err := s.DB.GetProjectEmailSettings(project.ID)
	if err != nil {
		return ProjectPayload{}, err
	}
	if record == nil {
		return ProjectPayload{}, fmt.Errorf("email settings are not configured")
	}

	password, err := s.decryptedSecret(project.ID, smtpPasswordKey)
	if err != nil {
		return ProjectPayload{}, err
	}
	recipients := splitRecipients(record.ContactEmailTo)
	if len(recipients) == 0 {
		recipients = splitRecipients(record.EmailFrom)
	}
	if len(recipients) == 0 {
		recipients = splitRecipients(record.SMTPUser)
	}
	if len(recipients) == 0 {
		return ProjectPayload{}, fmt.Errorf("from address or owner recipient is required")
	}

	message := Message{
		To:      recipients,
		Subject: "Deployik email integration test",
		Text:    fmt.Sprintf("Deployik successfully reached the SMTP configuration for %s at %s.", project.Name, time.Now().UTC().Format(time.RFC3339)),
	}
	sendErr := s.Sender.Send(ctx, SMTPConfig{
		Host:     record.SMTPHost,
		Port:     record.SMTPPort,
		Security: record.SMTPSecurity,
		Username: record.SMTPUser,
		Password: password,
		From:     record.EmailFrom,
		FromName: record.EmailFromName,
	}, message)

	record.LastTestedAt = nullTimeNow()
	if sendErr != nil {
		record.Status = db.EmailStatusError
		record.LastTestError = sendErr.Error()
		_ = s.DB.UpsertProjectEmailSettings(record)
		payload, _ := s.buildPayload(project, record)
		return payload, fmt.Errorf("send smtp test: %w", sendErr)
	}

	record.Status = db.EmailStatusSMTPTested
	record.LastTestError = ""
	if err := s.DB.UpsertProjectEmailSettings(record); err != nil {
		return ProjectPayload{}, err
	}
	return s.buildPayload(project, record)
}

func (s *Service) defaultSettings(_ context.Context, project *db.Project) *db.ProjectEmailSettings {
	primaryDomain := s.primaryProductionDomain(project.ID)
	defaultAddress := ""
	if primaryDomain != "" {
		defaultAddress = "noreply@" + primaryDomain
	}
	projectName := strings.TrimSpace(project.Name)
	return &db.ProjectEmailSettings{
		ProjectID:               project.ID,
		Provider:                db.EmailProviderWebglobe,
		SMTPHost:                defaultSMTPHost,
		SMTPPort:                defaultSMTPPort,
		SMTPSecurity:            db.EmailSMTPSecurityStartTLS,
		SMTPUser:                defaultAddress,
		EmailFrom:               defaultAddress,
		EmailFromName:           projectName,
		ContactEmailTo:          "",
		RecaptchaMode:           db.EmailRecaptchaModeV3,
		RecaptchaScoreThreshold: defaultRecaptchaScore,
		Status:                  db.EmailStatusNotConfigured,
	}
}

func (s *Service) primaryProductionDomain(projectID string) string {
	domains, err := s.DB.ListDomains(projectID)
	if err != nil {
		return ""
	}
	sort.SliceStable(domains, func(i, j int) bool {
		if domains[i].IsPrimary != domains[j].IsPrimary {
			return domains[i].IsPrimary
		}
		if domains[i].Environment != domains[j].Environment {
			return domains[i].Environment == "production"
		}
		return domains[i].DomainName < domains[j].DomainName
	})
	for _, domain := range domains {
		if domain.Environment == "production" && !domain.IsAuto {
			return domain.DomainName
		}
	}
	for _, domain := range domains {
		if domain.Environment == "production" {
			return domain.DomainName
		}
	}
	if len(domains) > 0 {
		return domains[0].DomainName
	}
	return ""
}

func (s *Service) recordFromRequest(project *db.Project, req SaveRequest) *db.ProjectEmailSettings {
	defaults := s.defaultSettings(context.Background(), project)
	provider := db.EmailProvider(strings.TrimSpace(req.Provider))
	if provider != db.EmailProviderWebglobe && provider != db.EmailProviderSMTP {
		provider = db.EmailProviderWebglobe
	}
	host := firstNonEmpty(req.SMTPHost, defaults.SMTPHost)
	port := req.SMTPPort
	if port <= 0 {
		port = defaults.SMTPPort
	}
	security := db.EmailSMTPSecurity(strings.TrimSpace(req.SMTPSecurity))
	if security != db.EmailSMTPSecurityStartTLS && security != db.EmailSMTPSecurityTLS && security != db.EmailSMTPSecurityNone {
		security = db.EmailSMTPSecurityStartTLS
	}
	threshold := req.RecaptchaScoreThreshold
	if threshold <= 0 {
		threshold = defaultRecaptchaScore
	}
	status := db.EmailStatusReadyToInstall
	if strings.TrimSpace(req.SMTPUser) == "" ||
		strings.TrimSpace(req.EmailFrom) == "" ||
		strings.TrimSpace(req.ContactEmailTo) == "" ||
		strings.TrimSpace(req.RecaptchaSiteKey) == "" {
		status = db.EmailStatusNotConfigured
	}
	return &db.ProjectEmailSettings{
		ProjectID:               project.ID,
		Provider:                provider,
		SMTPHost:                host,
		SMTPPort:                port,
		SMTPSecurity:            security,
		SMTPUser:                strings.TrimSpace(req.SMTPUser),
		EmailFrom:               strings.TrimSpace(req.EmailFrom),
		EmailFromName:           firstNonEmpty(req.EmailFromName, defaults.EmailFromName),
		ContactEmailTo:          normalizeRecipients(req.ContactEmailTo),
		RecaptchaSiteKey:        strings.TrimSpace(req.RecaptchaSiteKey),
		RecaptchaMode:           db.EmailRecaptchaModeV3,
		RecaptchaScoreThreshold: threshold,
		Status:                  status,
	}
}

func (s *Service) writeVariables(record *db.ProjectEmailSettings, req SaveRequest) error {
	envValues := map[string]string{
		"SMTP_HOST":                      record.SMTPHost,
		"SMTP_PORT":                      strconv.Itoa(record.SMTPPort),
		"SMTP_SECURE":                    string(record.SMTPSecurity),
		"SMTP_USER":                      record.SMTPUser,
		"EMAIL_FROM":                     record.EmailFrom,
		"EMAIL_FROM_NAME":                record.EmailFromName,
		"CONTACT_EMAIL_TO":               record.ContactEmailTo,
		"NEXT_PUBLIC_RECAPTCHA_SITE_KEY": record.RecaptchaSiteKey,
		"RECAPTCHA_SCORE_THRESHOLD":      strconv.FormatFloat(record.RecaptchaScoreThreshold, 'f', -1, 64),
	}
	for key, value := range envValues {
		if err := s.upsertEncryptedVariable(record.ProjectID, db.VariableKindEnv, key, value); err != nil {
			return err
		}
	}
	if strings.TrimSpace(req.SMTPPassword) != "" {
		if err := s.upsertEncryptedVariable(record.ProjectID, db.VariableKindSecret, smtpPasswordKey, req.SMTPPassword); err != nil {
			return err
		}
	}
	if strings.TrimSpace(req.RecaptchaSecretKey) != "" {
		if err := s.upsertEncryptedVariable(record.ProjectID, db.VariableKindSecret, recaptchaSecretKeyName, req.RecaptchaSecretKey); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) upsertEncryptedVariable(projectID string, kind db.VariableKind, key, value string) error {
	if s.Encryptor == nil {
		return fmt.Errorf("encryptor is required")
	}
	oppositeKind := db.VariableKindSecret
	if kind == db.VariableKindSecret {
		oppositeKind = db.VariableKindEnv
	}
	oppositeKeys, err := s.DB.ListProjectVariableKeys(projectID, oppositeKind)
	if err != nil {
		return err
	}
	for _, existingKey := range oppositeKeys {
		if existingKey == key {
			return fmt.Errorf("%s already exists in the opposite variable store", key)
		}
	}
	encrypted, err := s.Encryptor.Encrypt(value)
	if err != nil {
		return err
	}
	return s.DB.UpsertProjectVariable(&db.ProjectVariable{
		ProjectID:   projectID,
		Environment: sharedEnvironment,
		Kind:        kind,
		Key:         key,
		Value:       encrypted,
	})
}

func (s *Service) decryptedSecret(projectID, key string) (string, error) {
	if s.Encryptor == nil {
		return "", fmt.Errorf("encryptor is required")
	}
	secrets, err := s.DB.ListProjectVariables(projectID, sharedEnvironment, db.VariableKindSecret)
	if err != nil {
		return "", err
	}
	for _, secret := range secrets {
		if secret.Key != key {
			continue
		}
		value, err := s.Encryptor.Decrypt(secret.Value)
		if err != nil {
			return "", fmt.Errorf("decrypt %s: %w", key, err)
		}
		return value, nil
	}
	return "", fmt.Errorf("%s secret is missing", key)
}

func (s *Service) buildPayload(project *db.Project, record *db.ProjectEmailSettings) (ProjectPayload, error) {
	required, err := s.requiredStatus(project.ID)
	if err != nil {
		return ProjectPayload{}, err
	}
	settings := SettingsPayload{
		ProjectID:               record.ProjectID,
		Provider:                string(record.Provider),
		SMTPHost:                record.SMTPHost,
		SMTPPort:                record.SMTPPort,
		SMTPSecurity:            string(record.SMTPSecurity),
		SMTPUser:                record.SMTPUser,
		EmailFrom:               record.EmailFrom,
		EmailFromName:           record.EmailFromName,
		ContactEmailTo:          record.ContactEmailTo,
		RecaptchaSiteKey:        record.RecaptchaSiteKey,
		RecaptchaMode:           string(record.RecaptchaMode),
		RecaptchaScoreThreshold: record.RecaptchaScoreThreshold,
		Status:                  string(record.Status),
		LastTestError:           record.LastTestError,
	}
	if record.LastTestedAt.Valid {
		settings.LastTestedAt = &record.LastTestedAt.Time
	}
	return ProjectPayload{
		Settings: settings,
		Status: StatusPayload{
			Configured: !required.EnvMissing && !required.SecretsMissing,
			Required:   required,
		},
		Install: InstallPayload{
			AIPrompt: buildAIPrompt(project, record),
			EnvKeys:  append(append([]string{}, requiredEnvKeys...), requiredSecretKeys...),
		},
	}, nil
}

func (s *Service) requiredStatus(projectID string) (RequiredStatus, error) {
	envKeys, err := s.DB.ListProjectVariableKeys(projectID, db.VariableKindEnv)
	if err != nil {
		return RequiredStatus{}, err
	}
	secretKeys, err := s.DB.ListProjectVariableKeys(projectID, db.VariableKindSecret)
	if err != nil {
		return RequiredStatus{}, err
	}
	status := RequiredStatus{
		MissingEnv:     missingKeys(requiredEnvKeys, envKeys),
		MissingSecrets: missingKeys(requiredSecretKeys, secretKeys),
	}
	status.EnvMissing = len(status.MissingEnv) > 0
	status.SecretsMissing = len(status.MissingSecrets) > 0
	return status, nil
}

func buildAIPrompt(project *db.Project, record *db.ProjectEmailSettings) string {
	projectName := "this project"
	framework := "unknown"
	packageManager := "auto"
	rootDirectory := "."
	if project != nil {
		projectName = firstNonEmpty(project.Name, projectName)
		framework = firstNonEmpty(project.Framework, framework)
		packageManager = firstNonEmpty(project.PackageManager, packageManager)
		rootDirectory = firstNonEmpty(project.RootDirectory, rootDirectory)
	}

	var prompt strings.Builder
	prompt.WriteString("You are modifying an existing web application.\n\n")
	prompt.WriteString("Goal:\n")
	prompt.WriteString("Integrate a secure contact form email flow into this app using a Next.js Node runtime API route.\n\n")
	prompt.WriteString("Project context:\n")
	prompt.WriteString(fmt.Sprintf("- Project name: %s\n", projectName))
	prompt.WriteString(fmt.Sprintf("- Framework preset: %s\n", framework))
	prompt.WriteString(fmt.Sprintf("- Package manager: %s\n", packageManager))
	prompt.WriteString(fmt.Sprintf("- Root directory: %s\n\n", rootDirectory))
	prompt.WriteString("Email config is already provisioned as environment variables and secrets:\n")
	for _, key := range requiredEnvKeys {
		prompt.WriteString(fmt.Sprintf("- %s\n", key))
	}
	for _, key := range requiredSecretKeys {
		prompt.WriteString(fmt.Sprintf("- %s\n", key))
	}
	prompt.WriteString("\nRequirements:\n")
	prompt.WriteString("1. Inspect the existing contact form, homepage, design tokens, and routing style before editing.\n")
	prompt.WriteString("2. Add or update a Next.js Node runtime API route for contact submissions; do not use Edge runtime for nodemailer.\n")
	prompt.WriteString("3. Use nodemailer with SMTP_HOST, SMTP_PORT, SMTP_SECURE, SMTP_USER, and SMTP_PASSWORD.\n")
	prompt.WriteString("   Interpret SMTP_SECURE=tls as Nodemailer secure: true. Interpret SMTP_SECURE=starttls as secure: false with requireTLS: true.\n")
	prompt.WriteString("4. Verify Google reCAPTCHA v3 server-side before sending mail. Check success, action, hostname, and RECAPTCHA_SCORE_THRESHOLD.\n")
	prompt.WriteString("5. Validate submitted fields and adapt to the existing form shape. Never let the browser choose recipients or raw HTML.\n")
	prompt.WriteString("6. Send an owner notification email to CONTACT_EMAIL_TO with Reply-To set to the submitter email when valid.\n")
	prompt.WriteString("7. Send a branded confirmation email to the submitter after the owner notification succeeds.\n")
	prompt.WriteString("8. Create HTML emails that match the app's visual language and include a plain-text fallback for both email flows.\n")
	prompt.WriteString("9. Add a small in-memory IP rate limit for the contact route suitable for low-volume Docker-hosted sites.\n")
	prompt.WriteString("10. Do not expose SMTP credentials or RECAPTCHA_SECRET_KEY to the browser. Only NEXT_PUBLIC_RECAPTCHA_SITE_KEY may be client-visible.\n")
	prompt.WriteString("11. Return changed files and a short verification checklist.\n\n")
	prompt.WriteString("Provider defaults:\n")
	prompt.WriteString(fmt.Sprintf("- SMTP host: %s\n", record.SMTPHost))
	prompt.WriteString(fmt.Sprintf("- SMTP port: %d\n", record.SMTPPort))
	prompt.WriteString(fmt.Sprintf("- SMTP security: %s\n", record.SMTPSecurity))
	prompt.WriteString(fmt.Sprintf("- From address: %s\n", record.EmailFrom))
	prompt.WriteString(fmt.Sprintf("- Owner recipients: %s\n", record.ContactEmailTo))
	return prompt.String()
}

func missingKeys(required, existing []string) []string {
	seen := make(map[string]struct{}, len(existing))
	for _, key := range existing {
		seen[key] = struct{}{}
	}
	var missing []string
	for _, key := range required {
		if _, ok := seen[key]; !ok {
			missing = append(missing, key)
		}
	}
	return missing
}

func normalizeRecipients(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	})
	var recipients []string
	seen := map[string]struct{}{}
	for _, part := range parts {
		recipient := strings.TrimSpace(part)
		if recipient == "" {
			continue
		}
		if _, ok := seen[recipient]; ok {
			continue
		}
		seen[recipient] = struct{}{}
		recipients = append(recipients, recipient)
	}
	return strings.Join(recipients, ",")
}

func splitRecipients(value string) []string {
	if value == "" {
		return nil
	}
	normalized := normalizeRecipients(value)
	if normalized == "" {
		return nil
	}
	return strings.Split(normalized, ",")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func nullTimeNow() db.NullableTime {
	return db.NullableTimeNow()
}

type SMTPSender struct{}

func (SMTPSender) Send(ctx context.Context, cfg SMTPConfig, message Message) error {
	if strings.TrimSpace(cfg.Host) == "" {
		return fmt.Errorf("smtp host is required")
	}
	if cfg.Port <= 0 {
		return fmt.Errorf("smtp port is required")
	}
	if strings.TrimSpace(cfg.From) == "" {
		return fmt.Errorf("from address is required")
	}
	if len(message.To) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}

	address := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	var err error
	if cfg.Security == db.EmailSMTPSecurityTLS {
		conn, err = tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
			ServerName: cfg.Host,
			MinVersion: tls.VersionTLS12,
		})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return err
	}

	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer client.Close()

	if cfg.Security == db.EmailSMTPSecurityStartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("smtp server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}

	if strings.TrimSpace(cfg.Username) != "" {
		auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(cfg.From); err != nil {
		return err
	}
	for _, recipient := range message.To {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := io.WriteString(writer, renderTextEmail(cfg, message)); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func renderTextEmail(cfg SMTPConfig, message Message) string {
	from := (&mail.Address{Name: cfg.FromName, Address: cfg.From}).String()
	headers := []string{
		"From: " + sanitizeHeader(from),
		"To: " + sanitizeHeader(strings.Join(message.To, ", ")),
		"Subject: " + sanitizeHeader(message.Subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
	}
	return strings.Join(headers, "\r\n") + "\r\n\r\n" + message.Text + "\r\n"
}

func sanitizeHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}
