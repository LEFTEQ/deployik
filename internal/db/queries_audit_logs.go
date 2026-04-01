package db

import (
	"fmt"
	"strings"
)

func (db *DB) CreateAuditLog(entry *AuditLog) error {
	_, err := db.Exec(
		`INSERT INTO audit_logs (user_id, action, resource_type, resource_id, project_id, deployment_id, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		nullableAuditValue(entry.UserID),
		entry.Action,
		nullableAuditValue(entry.ResourceType),
		nullableAuditValue(entry.ResourceID),
		nullableAuditValue(entry.ProjectID),
		nullableAuditValue(entry.DeploymentID),
		entry.Metadata,
	)
	if err != nil {
		return fmt.Errorf("create audit log: %w", err)
	}
	return nil
}

func nullableAuditValue(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
