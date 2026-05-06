package db

import (
	"database/sql"
	"fmt"
)

func (db *DB) ListDomains(projectID string) ([]Domain, error) {
	rows, err := db.Query(
		`SELECT id, project_id, COALESCE(preview_instance_id, ''), domain, environment, is_auto, is_primary, dns_verified, ssl_status, ssl_expires_at, created_at
		 FROM domains WHERE project_id = ?
		 ORDER BY is_auto DESC, created_at ASC`, projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list domains: %w", err)
	}
	defer rows.Close()

	var domains []Domain
	for rows.Next() {
		var d Domain
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.PreviewInstanceID, &d.DomainName, &d.Environment,
			&d.IsAuto, &d.IsPrimary, &d.DNSVerified, &d.SSLStatus, &d.SSLExpiresAt, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan domain: %w", err)
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}

func (db *DB) GetDomainByID(id string) (*Domain, error) {
	d := &Domain{}
	err := db.QueryRow(
		`SELECT id, project_id, COALESCE(preview_instance_id, ''), domain, environment, is_auto, is_primary, dns_verified, ssl_status, ssl_expires_at, created_at
		 FROM domains WHERE id = ?`, id,
	).Scan(&d.ID, &d.ProjectID, &d.PreviewInstanceID, &d.DomainName, &d.Environment,
		&d.IsAuto, &d.IsPrimary, &d.DNSVerified, &d.SSLStatus, &d.SSLExpiresAt, &d.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get domain by id: %w", err)
	}
	return d, nil
}

func (db *DB) GetDomainByName(domain string) (*Domain, error) {
	d := &Domain{}
	err := db.QueryRow(
		`SELECT id, project_id, COALESCE(preview_instance_id, ''), domain, environment, is_auto, is_primary, dns_verified, ssl_status, ssl_expires_at, created_at
		 FROM domains WHERE domain = ?`, domain,
	).Scan(&d.ID, &d.ProjectID, &d.PreviewInstanceID, &d.DomainName, &d.Environment,
		&d.IsAuto, &d.IsPrimary, &d.DNSVerified, &d.SSLStatus, &d.SSLExpiresAt, &d.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get domain: %w", err)
	}
	return d, nil
}

func (db *DB) CreateDomain(d *Domain) error {
	d.ID = NewID()
	_, err := db.Exec(
		`INSERT INTO domains (id, project_id, preview_instance_id, domain, environment, is_auto, is_primary, dns_verified, ssl_status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.ProjectID, nullableString(d.PreviewInstanceID), d.DomainName, d.Environment, d.IsAuto, d.IsPrimary, d.DNSVerified, d.SSLStatus,
	)
	if err != nil {
		return fmt.Errorf("create domain: %w", err)
	}
	return nil
}

func (db *DB) UpdateDomainDNS(id string, verified bool) error {
	_, err := db.Exec(`UPDATE domains SET dns_verified = ? WHERE id = ?`, verified, id)
	if err != nil {
		return fmt.Errorf("update domain dns: %w", err)
	}
	return nil
}

func (db *DB) UpdateDomainSSL(id, status string, expiresAt NullableTime) error {
	_, err := db.Exec(`UPDATE domains SET ssl_status = ?, ssl_expires_at = ? WHERE id = ?`, status, expiresAt, id)
	if err != nil {
		return fmt.Errorf("update domain ssl: %w", err)
	}
	return nil
}

func (db *DB) UpdateDomainEnvironment(id, environment, previewInstanceID string) error {
	_, err := db.Exec(`UPDATE domains SET environment = ?, preview_instance_id = ?, is_primary = 0 WHERE id = ?`, environment, nullableString(previewInstanceID), id)
	if err != nil {
		return fmt.Errorf("update domain environment: %w", err)
	}
	return nil
}

func (db *DB) UpdateDomainPreviewInstance(id, previewInstanceID string) error {
	_, err := db.Exec(`UPDATE domains SET preview_instance_id = ? WHERE id = ?`, nullableString(previewInstanceID), id)
	if err != nil {
		return fmt.Errorf("update domain preview instance: %w", err)
	}
	return nil
}

// SetDomainPrimary flips the primary flag within one project/environment scope
// in a single transaction so the partial unique index never sees two primaries.
func (db *DB) SetDomainPrimary(projectID, environment, domainID string) error {
	target, err := db.GetDomainByID(domainID)
	if err != nil {
		return err
	}
	if target == nil {
		return sql.ErrNoRows
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("set primary begin: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`UPDATE domains SET is_primary = 0
		 WHERE project_id = ? AND environment = ? AND COALESCE(preview_instance_id, '') = ? AND id != ?`,
		projectID, environment, target.PreviewInstanceID, domainID,
	); err != nil {
		return fmt.Errorf("set primary clear siblings: %w", err)
	}

	if _, err := tx.Exec(
		`UPDATE domains SET is_primary = 1
		 WHERE id = ? AND project_id = ? AND environment = ?`,
		domainID, projectID, environment,
	); err != nil {
		return fmt.Errorf("set primary assign: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("set primary commit: %w", err)
	}

	return nil
}

// GetPrimaryDomain returns the canonical SSL-active domain for the given
// project/environment. Prefers the row flagged is_primary=1, falling back to
// the oldest SSL-active domain so reconciles and screenshots have something to
// point at while a newly-added primary still has SSL pending. Returns
// (nil, nil) when no domain in the env has SSL provisioned yet.
func (db *DB) GetPrimaryDomain(projectID, environment string) (*Domain, error) {
	return db.GetPrimaryDomainForTarget(projectID, environment, "")
}

func (db *DB) GetPrimaryDomainForTarget(projectID, environment, previewInstanceID string) (*Domain, error) {
	d := &Domain{}
	err := db.QueryRow(
		`SELECT id, project_id, COALESCE(preview_instance_id, ''), domain, environment, is_auto, is_primary, dns_verified, ssl_status, ssl_expires_at, created_at
		 FROM domains
		 WHERE project_id = ? AND environment = ? AND COALESCE(preview_instance_id, '') = ? AND ssl_status = 'active'
		 ORDER BY is_primary DESC, created_at ASC
		 LIMIT 1`, projectID, environment, previewInstanceID,
	).Scan(&d.ID, &d.ProjectID, &d.PreviewInstanceID, &d.DomainName, &d.Environment,
		&d.IsAuto, &d.IsPrimary, &d.DNSVerified, &d.SSLStatus, &d.SSLExpiresAt, &d.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get primary domain: %w", err)
	}
	return d, nil
}

func (db *DB) DeleteDomain(id string) error {
	_, err := db.Exec(`DELETE FROM domains WHERE id = ? AND is_auto = 0`, id)
	if err != nil {
		return fmt.Errorf("delete domain: %w", err)
	}
	return nil
}

func (db *DB) DeleteDomainForProject(projectID, id string) error {
	_, err := db.Exec(`DELETE FROM domains WHERE id = ? AND project_id = ? AND is_auto = 0`, id, projectID)
	if err != nil {
		return fmt.Errorf("delete domain for project: %w", err)
	}
	return nil
}

func (db *DB) DeleteAllDomainsForProject(projectID string) error {
	_, err := db.Exec(`DELETE FROM domains WHERE project_id = ?`, projectID)
	if err != nil {
		return fmt.Errorf("delete all domains for project: %w", err)
	}
	return nil
}

func (db *DB) DeleteDomainsForPreviewInstance(projectID, previewInstanceID string) error {
	_, err := db.Exec(
		`DELETE FROM domains WHERE project_id = ? AND preview_instance_id = ?`,
		projectID, previewInstanceID,
	)
	if err != nil {
		return fmt.Errorf("delete domains for preview instance: %w", err)
	}
	return nil
}

func (db *DB) ListActiveDomainProvisionTargets() ([]DomainProvisionTarget, error) {
	rows, err := db.Query(
		`SELECT p.id, p.name, d.domain, d.environment,
		        COALESCE(d.preview_instance_id, ''), COALESCE(pi.branch, ''), COALESCE(pi.branch_slug, ''), COALESCE(pi.is_default, 0),
		        CASE
		            WHEN d.environment = 'preview'    AND p.preview_password    IS NOT NULL THEN 1
		            WHEN d.environment = 'production' AND p.production_password IS NOT NULL THEN 1
		            ELSE 0
		        END AS password_protected,
		        p.port
		 FROM domains d
		 JOIN projects p ON p.id = d.project_id
		 LEFT JOIN preview_instances pi ON pi.id = d.preview_instance_id
		 WHERE p.status = 'active' AND d.ssl_status = 'active'
		 ORDER BY p.name ASC, d.environment ASC, d.created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list active domain provision targets: %w", err)
	}
	defer rows.Close()

	var targets []DomainProvisionTarget
	for rows.Next() {
		var target DomainProvisionTarget
		var pwProtected int
		var previewDefault int
		if err := rows.Scan(
			&target.ProjectID, &target.ProjectName, &target.DomainName, &target.Environment,
			&target.PreviewInstanceID, &target.PreviewBranch, &target.PreviewBranchSlug, &previewDefault,
			&pwProtected, &target.Port,
		); err != nil {
			return nil, fmt.Errorf("scan active domain provision target: %w", err)
		}
		target.PreviewInstanceDefault = previewDefault == 1
		target.PasswordProtected = pwProtected == 1
		targets = append(targets, target)
	}
	return targets, rows.Err()
}
