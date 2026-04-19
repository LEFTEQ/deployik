package db

import (
	"database/sql"
	"fmt"
)

func (db *DB) ListDomains(projectID string) ([]Domain, error) {
	rows, err := db.Query(
		`SELECT id, project_id, domain, environment, is_auto, dns_verified, ssl_status, ssl_expires_at, created_at
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
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.DomainName, &d.Environment,
			&d.IsAuto, &d.DNSVerified, &d.SSLStatus, &d.SSLExpiresAt, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan domain: %w", err)
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}

func (db *DB) GetDomainByID(id string) (*Domain, error) {
	d := &Domain{}
	err := db.QueryRow(
		`SELECT id, project_id, domain, environment, is_auto, dns_verified, ssl_status, ssl_expires_at, created_at
		 FROM domains WHERE id = ?`, id,
	).Scan(&d.ID, &d.ProjectID, &d.DomainName, &d.Environment,
		&d.IsAuto, &d.DNSVerified, &d.SSLStatus, &d.SSLExpiresAt, &d.CreatedAt)
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
		`SELECT id, project_id, domain, environment, is_auto, dns_verified, ssl_status, ssl_expires_at, created_at
		 FROM domains WHERE domain = ?`, domain,
	).Scan(&d.ID, &d.ProjectID, &d.DomainName, &d.Environment,
		&d.IsAuto, &d.DNSVerified, &d.SSLStatus, &d.SSLExpiresAt, &d.CreatedAt)
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
		`INSERT INTO domains (id, project_id, domain, environment, is_auto, dns_verified, ssl_status)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.ProjectID, d.DomainName, d.Environment, d.IsAuto, d.DNSVerified, d.SSLStatus,
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

func (db *DB) UpdateDomainSSL(id, status string, expiresAt sql.NullTime) error {
	_, err := db.Exec(`UPDATE domains SET ssl_status = ?, ssl_expires_at = ? WHERE id = ?`, status, expiresAt, id)
	if err != nil {
		return fmt.Errorf("update domain ssl: %w", err)
	}
	return nil
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

func (db *DB) ListActiveDomainProvisionTargets() ([]DomainProvisionTarget, error) {
	rows, err := db.Query(
		`SELECT p.id, p.name, d.domain, d.environment,
		        CASE
		            WHEN d.environment = 'preview'    AND p.preview_password    IS NOT NULL THEN 1
		            WHEN d.environment = 'production' AND p.production_password IS NOT NULL THEN 1
		            ELSE 0
		        END AS password_protected,
		        p.port
		 FROM domains d
		 JOIN projects p ON p.id = d.project_id
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
		if err := rows.Scan(&target.ProjectID, &target.ProjectName, &target.DomainName, &target.Environment, &pwProtected, &target.Port); err != nil {
			return nil, fmt.Errorf("scan active domain provision target: %w", err)
		}
		target.PasswordProtected = pwProtected == 1
		targets = append(targets, target)
	}
	return targets, rows.Err()
}
