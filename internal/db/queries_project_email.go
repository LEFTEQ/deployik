package db

import (
	"database/sql"
	"fmt"
)

func (db *DB) GetProjectEmailSettings(projectID string) (*ProjectEmailSettings, error) {
	record := &ProjectEmailSettings{}
	err := db.QueryRow(
		`SELECT project_id, provider, smtp_host, smtp_port, smtp_security, smtp_user,
		        email_from, email_from_name, contact_email_to, recaptcha_site_key,
		        recaptcha_mode, recaptcha_score_threshold, status, last_tested_at,
		        last_test_error, created_at, updated_at
		 FROM project_email_settings
		 WHERE project_id = ?`,
		projectID,
	).Scan(
		&record.ProjectID,
		&record.Provider,
		&record.SMTPHost,
		&record.SMTPPort,
		&record.SMTPSecurity,
		&record.SMTPUser,
		&record.EmailFrom,
		&record.EmailFromName,
		&record.ContactEmailTo,
		&record.RecaptchaSiteKey,
		&record.RecaptchaMode,
		&record.RecaptchaScoreThreshold,
		&record.Status,
		&record.LastTestedAt,
		&record.LastTestError,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get project email settings: %w", err)
	}
	return record, nil
}

func (db *DB) UpsertProjectEmailSettings(record *ProjectEmailSettings) error {
	if record == nil {
		return fmt.Errorf("project email settings record is required")
	}

	_, err := db.Exec(
		`INSERT INTO project_email_settings (
		    project_id, provider, smtp_host, smtp_port, smtp_security, smtp_user,
		    email_from, email_from_name, contact_email_to, recaptcha_site_key,
		    recaptcha_mode, recaptcha_score_threshold, status, last_tested_at,
		    last_test_error
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id) DO UPDATE SET
		    provider = excluded.provider,
		    smtp_host = excluded.smtp_host,
		    smtp_port = excluded.smtp_port,
		    smtp_security = excluded.smtp_security,
		    smtp_user = excluded.smtp_user,
		    email_from = excluded.email_from,
		    email_from_name = excluded.email_from_name,
		    contact_email_to = excluded.contact_email_to,
		    recaptcha_site_key = excluded.recaptcha_site_key,
		    recaptcha_mode = excluded.recaptcha_mode,
		    recaptcha_score_threshold = excluded.recaptcha_score_threshold,
		    status = excluded.status,
		    last_tested_at = excluded.last_tested_at,
		    last_test_error = excluded.last_test_error,
		    updated_at = datetime('now')`,
		record.ProjectID,
		record.Provider,
		record.SMTPHost,
		record.SMTPPort,
		record.SMTPSecurity,
		record.SMTPUser,
		record.EmailFrom,
		record.EmailFromName,
		record.ContactEmailTo,
		record.RecaptchaSiteKey,
		record.RecaptchaMode,
		record.RecaptchaScoreThreshold,
		record.Status,
		record.LastTestedAt,
		record.LastTestError,
	)
	if err != nil {
		return fmt.Errorf("upsert project email settings: %w", err)
	}

	stored, err := db.GetProjectEmailSettings(record.ProjectID)
	if err != nil {
		return err
	}
	if stored != nil {
		*record = *stored
	}
	return nil
}

func (db *DB) DeleteProjectEmailSettings(projectID string) error {
	if _, err := db.Exec(`DELETE FROM project_email_settings WHERE project_id = ?`, projectID); err != nil {
		return fmt.Errorf("delete project email settings: %w", err)
	}
	return nil
}
