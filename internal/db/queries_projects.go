package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// parseSQLiteDateTime parses a datetime string returned by a SQLite scalar
// aggregate (e.g. MAX(created_at)). The modernc.org/sqlite driver hands these
// back as strings even when the source column would be auto-converted as a
// direct reference. Production rows are always written via datetime('now')
// which stores "YYYY-MM-DD HH:MM:SS"; the Go-default format is included as a
// fallback for any rows inserted by passing a time.Time directly through
// database/sql. Returns nil when val is invalid (caller has already guarded
// sql.NullString.Valid).
func parseSQLiteDateTime(val string) *time.Time {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.999999999",
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05 -0700 MST", // time.Time.String() default
		"2006-01-02 15:04:05.999999999 -0700 MST",
	}
	for _, layout := range formats {
		if t, err := time.Parse(layout, val); err == nil {
			return &t
		}
	}
	return nil
}

func (db *DB) ListProjects(userID, organizationID string) ([]Project, error) {
	baseQuery := `
		SELECT p.id, p.name, p.github_repo, p.github_owner, p.branch, p.user_id,
		       COALESCE(p.organization_id, ''), COALESCE(o.name, ''), p.framework, p.package_manager,
		       p.root_directory, p.output_directory, p.build_command, p.install_command, p.node_version,
		       p.status, COALESCE(p.preview_password, ''), COALESCE(p.production_password, ''),
		       p.created_at, p.updated_at,
		       p.host_network_access, p.data_volume_enabled, COALESCE(p.data_mount_path, '/app/data'),
		       p.port, COALESCE(p.resource_tier, 'small'), p.start_command, p.health_path
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
			&p.Port, &p.ResourceTier, &p.StartCommand, &p.HealthPath); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func (db *DB) GetProject(id string) (*Project, error) {
	p := &Project{}
	var previewDeploy, productionDeploy sql.NullString
	err := db.QueryRow(
		`SELECT p.id, p.name, p.github_repo, p.github_owner, p.branch, p.user_id,
		        COALESCE(p.organization_id, ''), COALESCE(o.name, ''), p.framework, p.package_manager,
		        p.root_directory, p.output_directory, p.build_command, p.install_command, p.node_version, p.status,
		        COALESCE(p.preview_password, ''), COALESCE(p.production_password, ''),
		        p.created_at, p.updated_at,
		        p.host_network_access, p.data_volume_enabled, COALESCE(p.data_mount_path, '/app/data'),
		        p.port, COALESCE(p.resource_tier, 'small'), p.start_command, p.health_path,
		        (SELECT MAX(created_at) FROM deployments
		         WHERE project_id = p.id AND environment = 'preview' AND status = 'live'),
		        (SELECT MAX(created_at) FROM deployments
		         WHERE project_id = p.id AND environment = 'production' AND status = 'live')
		 FROM projects p
		 LEFT JOIN organizations o ON o.id = p.organization_id
		 WHERE p.id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.GithubRepo, &p.GithubOwner, &p.Branch,
		&p.UserID, &p.OrganizationID, &p.OrganizationName, &p.Framework, &p.PackageManager, &p.RootDirectory, &p.OutputDirectory, &p.BuildCommand, &p.InstallCommand, &p.NodeVersion,
		&p.Status, &p.PreviewPassword, &p.ProductionPassword,
		&p.CreatedAt, &p.UpdatedAt,
		&p.HostNetworkAccess, &p.DataVolumeEnabled, &p.DataMountPath,
		&p.Port, &p.ResourceTier, &p.StartCommand, &p.HealthPath,
		&previewDeploy, &productionDeploy)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if previewDeploy.Valid {
		p.LatestPreviewDeployAt = parseSQLiteDateTime(previewDeploy.String)
	}
	if productionDeploy.Valid {
		p.LatestProductionDeployAt = parseSQLiteDateTime(productionDeploy.String)
	}
	return p, nil
}

func (db *DB) GetProjectForUser(id, userID string) (*Project, error) {
	p := &Project{}
	var previewDeploy, productionDeploy sql.NullString
	err := db.QueryRow(
		`SELECT p.id, p.name, p.github_repo, p.github_owner, p.branch, p.user_id,
		        COALESCE(p.organization_id, ''), COALESCE(o.name, ''), p.framework, p.package_manager,
		        p.root_directory, p.output_directory, p.build_command, p.install_command, p.node_version, p.status,
		        COALESCE(p.preview_password, ''), COALESCE(p.production_password, ''),
		        p.created_at, p.updated_at,
		        p.host_network_access, p.data_volume_enabled, COALESCE(p.data_mount_path, '/app/data'),
		        p.port, COALESCE(p.resource_tier, 'small'), p.start_command, p.health_path,
		        (SELECT MAX(created_at) FROM deployments
		         WHERE project_id = p.id AND environment = 'preview' AND status = 'live'),
		        (SELECT MAX(created_at) FROM deployments
		         WHERE project_id = p.id AND environment = 'production' AND status = 'live')
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
		&p.Port, &p.ResourceTier, &p.StartCommand, &p.HealthPath,
		&previewDeploy, &productionDeploy)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get project for user: %w", err)
	}
	if previewDeploy.Valid {
		p.LatestPreviewDeployAt = parseSQLiteDateTime(previewDeploy.String)
	}
	if productionDeploy.Valid {
		p.LatestProductionDeployAt = parseSQLiteDateTime(productionDeploy.String)
	}
	return p, nil
}

func (db *DB) CreateProject(p *Project) error {
	p.ID = NewID()
	packageManager := normalizeStoredPackageManager(p.PackageManager)
	port := normalizePort(p.Port)
	resourceTier := normalizeStoredResourceTier(p.ResourceTier)
	_, err := db.Exec(
		`INSERT INTO projects (id, name, github_repo, github_owner, branch, user_id, organization_id, framework, package_manager,
		                       root_directory, output_directory, build_command, install_command, node_version, status,
		                       host_network_access, data_volume_enabled, data_mount_path, port, resource_tier,
		                       start_command, health_path)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.GithubRepo, p.GithubOwner, p.Branch, p.UserID, nullableString(p.OrganizationID), p.Framework, packageManager,
		p.RootDirectory, p.OutputDirectory, p.BuildCommand, p.InstallCommand, p.NodeVersion, p.Status,
		p.HostNetworkAccess, p.DataVolumeEnabled, p.DataMountPath, port, resourceTier,
		p.StartCommand, p.HealthPath,
	)
	if err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	p.PackageManager = packageManager
	p.Port = port
	p.ResourceTier = resourceTier
	return nil
}

func (db *DB) UpdateProject(p *Project) error {
	packageManager := normalizeStoredPackageManager(p.PackageManager)
	port := normalizePort(p.Port)
	resourceTier := normalizeStoredResourceTier(p.ResourceTier)
	_, err := db.Exec(
		`UPDATE projects SET name = ?, branch = ?, framework = ?, package_manager = ?, root_directory = ?, output_directory = ?, build_command = ?,
		        install_command = ?, node_version = ?, status = ?,
		        host_network_access = ?, data_volume_enabled = ?, data_mount_path = ?, port = ?, resource_tier = ?,
		        start_command = ?, health_path = ?,
		        updated_at = datetime('now')
		 WHERE id = ?`,
		p.Name, p.Branch, p.Framework, packageManager, p.RootDirectory, p.OutputDirectory, p.BuildCommand, p.InstallCommand,
		p.NodeVersion, p.Status,
		p.HostNetworkAccess, p.DataVolumeEnabled, p.DataMountPath, port, resourceTier,
		p.StartCommand, p.HealthPath, p.ID,
	)
	if err != nil {
		return fmt.Errorf("update project: %w", err)
	}
	p.PackageManager = packageManager
	p.Port = port
	p.ResourceTier = resourceTier
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
		       p.port, COALESCE(p.resource_tier, 'small'), p.start_command, p.health_path,
		       (SELECT MAX(created_at) FROM deployments
		        WHERE project_id = p.id AND environment = 'preview' AND status = 'live'),
		       (SELECT MAX(created_at) FROM deployments
		        WHERE project_id = p.id AND environment = 'production' AND status = 'live'),
		       ld.id, ld.status, ld.branch, ld.commit_sha, ld.commit_message, ld.created_at,
		       ld.environment, ld.screenshot_path
		FROM projects p
		LEFT JOIN organizations o ON o.id = p.organization_id
		LEFT JOIN (
		    -- Exactly one latest deployment per project. created_at is
		    -- second-resolution (datetime('now')) so a webhook fan-out can write
		    -- preview + production rows with an identical timestamp; a plain
		    -- MAX(created_at) join would then emit one duplicate project row per
		    -- tied deployment. ROW_NUMBER with id (a time-ordered ULID) as the
		    -- tiebreaker collapses ties deterministically to the newest row.
		    SELECT * FROM (
		        SELECT d.*,
		               ROW_NUMBER() OVER (
		                   PARTITION BY d.project_id
		                   ORDER BY d.created_at DESC, d.id DESC
		               ) AS rn
		        FROM deployments d
		    ) ranked
		    WHERE ranked.rn = 1
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
		var ldEnvironment, ldScreenshotPath sql.NullString
		var ldCreatedAt NullableTime
		var previewDeploy, productionDeploy sql.NullString
		if err := rows.Scan(
			&p.ID, &p.Name, &p.GithubRepo, &p.GithubOwner, &p.Branch,
			&p.UserID, &p.OrganizationID, &p.OrganizationName, &p.Framework, &p.PackageManager,
			&p.RootDirectory, &p.OutputDirectory, &p.BuildCommand, &p.InstallCommand, &p.NodeVersion,
			&p.Status, &p.PreviewPassword, &p.ProductionPassword,
			&p.CreatedAt, &p.UpdatedAt,
			&p.HostNetworkAccess, &p.DataVolumeEnabled, &p.DataMountPath,
			&p.Port, &p.ResourceTier, &p.StartCommand, &p.HealthPath,
			&previewDeploy, &productionDeploy,
			&ldID, &ldStatus, &ldBranch, &ldCommitSHA, &ldCommitMsg, &ldCreatedAt,
			&ldEnvironment, &ldScreenshotPath,
		); err != nil {
			return nil, fmt.Errorf("scan project with latest deployment: %w", err)
		}
		if previewDeploy.Valid {
			p.LatestPreviewDeployAt = parseSQLiteDateTime(previewDeploy.String)
		}
		if productionDeploy.Valid {
			p.LatestProductionDeployAt = parseSQLiteDateTime(productionDeploy.String)
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
		if ldEnvironment.Valid {
			pw.LatestDeploymentEnvironment = &ldEnvironment.String
		}
		if ldScreenshotPath.Valid && ldScreenshotPath.String != "" {
			pw.LatestDeploymentScreenshotPath = &ldScreenshotPath.String
		}
		projects = append(projects, pw)
	}
	return projects, rows.Err()
}

// normalizeStoredResourceTier guards the DB write path against empty strings
// (a freshly-decoded Project before defaults are applied) and rejects unknown
// values. The DB CHECK constraint is the final backstop, but defaulting here
// keeps INSERT/UPDATE callers from having to know the canonical default.
func normalizeStoredResourceTier(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "nano":
		return "nano"
	case "small", "":
		return "small"
	case "medium":
		return "medium"
	case "large":
		return "large"
	default:
		return "small"
	}
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
