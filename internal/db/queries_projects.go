package db

import (
	"database/sql"
	"fmt"
	"strings"
)

func (db *DB) ListProjects(userID, organizationID string) ([]Project, error) {
	baseQuery := `
		SELECT p.id, p.name, p.github_repo, p.github_owner, p.branch, p.user_id,
		       COALESCE(p.organization_id, ''), COALESCE(o.name, ''), p.framework, p.package_manager,
		       p.root_directory, p.output_directory, p.build_command, p.install_command, p.node_version,
		       p.status, COALESCE(p.preview_password, ''), COALESCE(p.production_password, ''),
		       p.created_at, p.updated_at,
		       p.host_network_access, p.data_volume_enabled, COALESCE(p.data_mount_path, '/app/data'),
		       p.port
		FROM projects p
		LEFT JOIN organizations o ON o.id = p.organization_id
		WHERE p.status != 'deleted'
		  AND (
		    p.user_id = ?
		    OR EXISTS (
		      SELECT 1
		      FROM organization_memberships om
		      WHERE om.organization_id = p.organization_id AND om.user_id = ?
		    )
		  )`

	args := []any{userID, userID}
	if trimmedOrgID := strings.TrimSpace(organizationID); trimmedOrgID != "" {
		baseQuery += ` AND p.organization_id = ?`
		args = append(args, trimmedOrgID)
	}
	baseQuery += ` ORDER BY p.updated_at DESC`

	rows, err := db.Query(baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.GithubRepo, &p.GithubOwner, &p.Branch,
			&p.UserID, &p.OrganizationID, &p.OrganizationName, &p.Framework, &p.PackageManager, &p.RootDirectory, &p.OutputDirectory, &p.BuildCommand, &p.InstallCommand, &p.NodeVersion,
			&p.Status, &p.PreviewPassword, &p.ProductionPassword,
			&p.CreatedAt, &p.UpdatedAt,
			&p.HostNetworkAccess, &p.DataVolumeEnabled, &p.DataMountPath,
			&p.Port); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func (db *DB) GetProject(id string) (*Project, error) {
	p := &Project{}
	err := db.QueryRow(
		`SELECT p.id, p.name, p.github_repo, p.github_owner, p.branch, p.user_id,
		        COALESCE(p.organization_id, ''), COALESCE(o.name, ''), p.framework, p.package_manager,
		        p.root_directory, p.output_directory, p.build_command, p.install_command, p.node_version, p.status,
		        COALESCE(p.preview_password, ''), COALESCE(p.production_password, ''),
		        p.created_at, p.updated_at,
		        p.host_network_access, p.data_volume_enabled, COALESCE(p.data_mount_path, '/app/data'),
		        p.port
		 FROM projects p
		 LEFT JOIN organizations o ON o.id = p.organization_id
		 WHERE p.id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.GithubRepo, &p.GithubOwner, &p.Branch,
		&p.UserID, &p.OrganizationID, &p.OrganizationName, &p.Framework, &p.PackageManager, &p.RootDirectory, &p.OutputDirectory, &p.BuildCommand, &p.InstallCommand, &p.NodeVersion,
		&p.Status, &p.PreviewPassword, &p.ProductionPassword,
		&p.CreatedAt, &p.UpdatedAt,
		&p.HostNetworkAccess, &p.DataVolumeEnabled, &p.DataMountPath,
		&p.Port)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	return p, nil
}

func (db *DB) GetProjectForUser(id, userID string) (*Project, error) {
	p := &Project{}
	err := db.QueryRow(
		`SELECT p.id, p.name, p.github_repo, p.github_owner, p.branch, p.user_id,
		        COALESCE(p.organization_id, ''), COALESCE(o.name, ''), p.framework, p.package_manager,
		        p.root_directory, p.output_directory, p.build_command, p.install_command, p.node_version, p.status,
		        COALESCE(p.preview_password, ''), COALESCE(p.production_password, ''),
		        p.created_at, p.updated_at,
		        p.host_network_access, p.data_volume_enabled, COALESCE(p.data_mount_path, '/app/data'),
		        p.port
		 FROM projects p
		 LEFT JOIN organizations o ON o.id = p.organization_id
		 WHERE p.id = ?
		   AND (
		     p.user_id = ?
		     OR EXISTS (
		       SELECT 1
		       FROM organization_memberships om
		       WHERE om.organization_id = p.organization_id AND om.user_id = ?
		     )
		   )`, id, userID, userID,
	).Scan(&p.ID, &p.Name, &p.GithubRepo, &p.GithubOwner, &p.Branch,
		&p.UserID, &p.OrganizationID, &p.OrganizationName, &p.Framework, &p.PackageManager, &p.RootDirectory, &p.OutputDirectory, &p.BuildCommand, &p.InstallCommand, &p.NodeVersion,
		&p.Status, &p.PreviewPassword, &p.ProductionPassword,
		&p.CreatedAt, &p.UpdatedAt,
		&p.HostNetworkAccess, &p.DataVolumeEnabled, &p.DataMountPath,
		&p.Port)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get project for user: %w", err)
	}
	return p, nil
}

func (db *DB) CreateProject(p *Project) error {
	p.ID = NewID()
	packageManager := normalizeStoredPackageManager(p.PackageManager)
	port := normalizePort(p.Port)
	_, err := db.Exec(
		`INSERT INTO projects (id, name, github_repo, github_owner, branch, user_id, organization_id, framework, package_manager,
		                       root_directory, output_directory, build_command, install_command, node_version, status,
		                       host_network_access, data_volume_enabled, data_mount_path, port)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.GithubRepo, p.GithubOwner, p.Branch, p.UserID, nullableString(p.OrganizationID), p.Framework, packageManager,
		p.RootDirectory, p.OutputDirectory, p.BuildCommand, p.InstallCommand, p.NodeVersion, p.Status,
		p.HostNetworkAccess, p.DataVolumeEnabled, p.DataMountPath, port,
	)
	if err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	p.PackageManager = packageManager
	p.Port = port
	return nil
}

func (db *DB) UpdateProject(p *Project) error {
	packageManager := normalizeStoredPackageManager(p.PackageManager)
	port := normalizePort(p.Port)
	_, err := db.Exec(
		`UPDATE projects SET name = ?, branch = ?, framework = ?, package_manager = ?, root_directory = ?, output_directory = ?, build_command = ?,
		        install_command = ?, node_version = ?, status = ?,
		        host_network_access = ?, data_volume_enabled = ?, data_mount_path = ?, port = ?,
		        updated_at = datetime('now')
		 WHERE id = ?`,
		p.Name, p.Branch, p.Framework, packageManager, p.RootDirectory, p.OutputDirectory, p.BuildCommand, p.InstallCommand,
		p.NodeVersion, p.Status,
		p.HostNetworkAccess, p.DataVolumeEnabled, p.DataMountPath, port, p.ID,
	)
	if err != nil {
		return fmt.Errorf("update project: %w", err)
	}
	p.PackageManager = packageManager
	p.Port = port
	return nil
}

// normalizePort clamps the port to a valid range, defaulting to 3000 when unset.
// Outer layers (API handlers) are expected to have already validated the range;
// this is a defense-in-depth fallback so we never persist an invalid port.
func normalizePort(port int) int {
	if port < 1 || port > 65535 {
		return 3000
	}
	return port
}

func (db *DB) DeleteProject(id string) error {
	_, err := db.Exec(
		`UPDATE projects
		 SET name = CASE
		                WHEN status = 'deleted' THEN name
		                ELSE name || '--deleted-' || lower(substr(id, 1, 8))
		            END,
		     status = 'deleted',
		     updated_at = datetime('now')
		 WHERE id = ?`,
		id,
	)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	return nil
}

func (db *DB) ListProjectsWithLatestDeployment(userID, orgID string) ([]ProjectWithLatestDeployment, error) {
	baseQuery := `
		SELECT p.id, p.name, p.github_repo, p.github_owner, p.branch, p.user_id,
		       COALESCE(p.organization_id, ''), COALESCE(o.name, ''), p.framework, p.package_manager,
		       p.root_directory, p.output_directory, p.build_command, p.install_command, p.node_version,
		       p.status, COALESCE(p.preview_password, ''), COALESCE(p.production_password, ''),
		       p.created_at, p.updated_at,
		       p.host_network_access, p.data_volume_enabled, COALESCE(p.data_mount_path, '/app/data'),
		       p.port,
		       ld.id, ld.status, ld.branch, ld.commit_sha, ld.commit_message, ld.created_at
		FROM projects p
		LEFT JOIN organizations o ON o.id = p.organization_id
		LEFT JOIN (
		    SELECT d1.*
		    FROM deployments d1
		    INNER JOIN (
		        SELECT project_id, MAX(created_at) AS max_created
		        FROM deployments
		        GROUP BY project_id
		    ) d2 ON d1.project_id = d2.project_id AND d1.created_at = d2.max_created
		) ld ON ld.project_id = p.id
		WHERE p.status != 'deleted'
		  AND (
		    p.user_id = ?
		    OR EXISTS (
		      SELECT 1
		      FROM organization_memberships om
		      WHERE om.organization_id = p.organization_id AND om.user_id = ?
		    )
		  )`

	args := []any{userID, userID}
	if trimmedOrgID := strings.TrimSpace(orgID); trimmedOrgID != "" {
		baseQuery += ` AND p.organization_id = ?`
		args = append(args, trimmedOrgID)
	}
	baseQuery += ` ORDER BY p.updated_at DESC`

	rows, err := db.Query(baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("list projects with latest deployment: %w", err)
	}
	defer rows.Close()

	var projects []ProjectWithLatestDeployment
	for rows.Next() {
		var pw ProjectWithLatestDeployment
		p := &pw.Project
		var ldID, ldStatus, ldBranch, ldCommitSHA, ldCommitMsg sql.NullString
		var ldCreatedAt sql.NullTime
		if err := rows.Scan(
			&p.ID, &p.Name, &p.GithubRepo, &p.GithubOwner, &p.Branch,
			&p.UserID, &p.OrganizationID, &p.OrganizationName, &p.Framework, &p.PackageManager,
			&p.RootDirectory, &p.OutputDirectory, &p.BuildCommand, &p.InstallCommand, &p.NodeVersion,
			&p.Status, &p.PreviewPassword, &p.ProductionPassword,
			&p.CreatedAt, &p.UpdatedAt,
			&p.HostNetworkAccess, &p.DataVolumeEnabled, &p.DataMountPath,
			&p.Port,
			&ldID, &ldStatus, &ldBranch, &ldCommitSHA, &ldCommitMsg, &ldCreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan project with latest deployment: %w", err)
		}
		if ldID.Valid {
			pw.LatestDeploymentID = &ldID.String
		}
		if ldStatus.Valid {
			pw.LatestDeploymentStatus = &ldStatus.String
		}
		if ldBranch.Valid {
			pw.LatestDeploymentBranch = &ldBranch.String
		}
		if ldCommitSHA.Valid {
			pw.LatestDeploymentCommitSHA = &ldCommitSHA.String
		}
		if ldCommitMsg.Valid {
			pw.LatestDeploymentCommitMsg = &ldCommitMsg.String
		}
		if ldCreatedAt.Valid {
			pw.LatestDeploymentCreatedAt = &ldCreatedAt.Time
		}
		projects = append(projects, pw)
	}
	return projects, rows.Err()
}

func normalizeStoredPackageManager(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	switch trimmed {
	case "", "auto":
		return "auto"
	case "bun", "pnpm", "npm", "yarn":
		return trimmed
	default:
		return "auto"
	}
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

// GetProjectPassword returns the encrypted password for the given environment ("preview" or "production").
// Returns an empty string if no password is set.
func (db *DB) GetProjectPassword(projectID, environment string) (string, error) {
	var col string
	switch environment {
	case "preview":
		col = "preview_password"
	case "production":
		col = "production_password"
	default:
		return "", fmt.Errorf("invalid environment: %s", environment)
	}

	var val sql.NullString
	err := db.QueryRow(
		fmt.Sprintf(`SELECT %s FROM projects WHERE id = ?`, col), projectID,
	).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get project password: %w", err)
	}
	if !val.Valid {
		return "", nil
	}
	return val.String, nil
}

// SetProjectPassword stores an encrypted password for the given environment.
func (db *DB) SetProjectPassword(projectID, environment, encryptedPassword string) error {
	var col string
	switch environment {
	case "preview":
		col = "preview_password"
	case "production":
		col = "production_password"
	default:
		return fmt.Errorf("invalid environment: %s", environment)
	}

	_, err := db.Exec(
		fmt.Sprintf(`UPDATE projects SET %s = ?, updated_at = datetime('now') WHERE id = ?`, col),
		encryptedPassword, projectID,
	)
	if err != nil {
		return fmt.Errorf("set project password: %w", err)
	}
	return nil
}

// ClearProjectPassword removes the password for the given environment (sets it to NULL).
func (db *DB) ClearProjectPassword(projectID, environment string) error {
	var col string
	switch environment {
	case "preview":
		col = "preview_password"
	case "production":
		col = "production_password"
	default:
		return fmt.Errorf("invalid environment: %s", environment)
	}

	_, err := db.Exec(
		fmt.Sprintf(`UPDATE projects SET %s = NULL, updated_at = datetime('now') WHERE id = ?`, col),
		projectID,
	)
	if err != nil {
		return fmt.Errorf("clear project password: %w", err)
	}
	return nil
}
