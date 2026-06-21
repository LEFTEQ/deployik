package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// CreateApp creates an app inside an organization and optionally moves the given
// projects into it. Mirrors CreateGroup. The caller is responsible for verifying
// the user may write to the organization + projects (the HTTP handler does this).
func (db *DB) CreateApp(input *AppCreate) (*App, error) {
	if input == nil {
		return nil, fmt.Errorf("create app: input is nil")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("create app: name is required")
	}
	orgID := strings.TrimSpace(input.OrganizationID)
	if orgID == "" {
		return nil, fmt.Errorf("create app: organization_id is required")
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("create app: begin: %w", err)
	}
	defer tx.Rollback()

	slug, err := reserveUniqueAppSlug(tx, orgID, slugifyOrganizationName(name))
	if err != nil {
		return nil, fmt.Errorf("create app: %w", err)
	}

	var displayOrder int
	if err := tx.QueryRow(
		`SELECT COALESCE(MAX(display_order), 0) + 1 FROM apps WHERE organization_id = ?`,
		orgID,
	).Scan(&displayOrder); err != nil {
		return nil, fmt.Errorf("create app: next display order: %w", err)
	}

	appID := NewID()
	if _, err := tx.Exec(
		`INSERT INTO apps (id, organization_id, name, slug, display_order)
		 VALUES (?, ?, ?, ?, ?)`,
		appID, orgID, name, slug, displayOrder,
	); err != nil {
		return nil, fmt.Errorf("create app: insert: %w", err)
	}
	for _, projectID := range uniqueNonEmpty(input.ProjectIDs) {
		if err := moveProjectToAppTx(tx, appID, projectID); err != nil {
			return nil, fmt.Errorf("create app: move project: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create app: commit: %w", err)
	}

	app, err := db.GetApp(appID)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, fmt.Errorf("create app: created app not found")
	}
	return app, nil
}

// reserveUniqueAppSlug returns base, or base-2, base-3, … unique within the org.
func reserveUniqueAppSlug(tx *sql.Tx, orgID, base string) (string, error) {
	if base == "" {
		base = "app"
	}
	candidate := base
	for n := 2; ; n++ {
		var exists int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM apps WHERE organization_id = ? AND slug = ?`,
			orgID, candidate,
		).Scan(&exists); err != nil {
			return "", fmt.Errorf("reserve slug: %w", err)
		}
		if exists == 0 {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", base, n)
	}
}

// moveProjectToAppTx sets a project's app_id within a transaction. The project
// must live in the same organization as the app — enforced here (not just in the
// handler) so the workspace boundary can't be crossed via this path. A project
// in a different org (or missing/deleted) matches zero rows → ErrProjectNotMovable.
func moveProjectToAppTx(tx *sql.Tx, appID, projectID string) error {
	res, err := tx.Exec(
		`UPDATE projects SET app_id = ?, updated_at = datetime('now')
		 WHERE id = ? AND status != 'deleted'
		   AND organization_id = (SELECT organization_id FROM apps WHERE id = ?)`,
		appID, projectID, appID,
	)
	if err != nil {
		return fmt.Errorf("move project to app: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("move project to app: rows affected: %w", err)
	}
	if affected == 0 {
		return ErrProjectNotMovable
	}
	return nil
}

func (db *DB) appSelect(where string, args ...any) (*App, error) {
	app := &App{}
	query := `SELECT a.id, a.organization_id, a.name, a.slug, a.deploy_ordered, a.display_order,
	                 COALESCE((SELECT COUNT(*) FROM projects p
	                           WHERE p.app_id = a.id AND p.status != 'deleted'), 0),
	                 a.created_at, a.updated_at
	          FROM apps a ` + where
	err := db.QueryRow(query, args...).Scan(
		&app.ID, &app.OrganizationID, &app.Name, &app.Slug, &app.DeployOrdered,
		&app.DisplayOrder, &app.ProjectCount, &app.CreatedAt, &app.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}
	return app, nil
}

// GetApp fetches an app by id (no access check).
func (db *DB) GetApp(appID string) (*App, error) {
	return db.appSelect(`WHERE a.id = ?`, appID)
}

// GetAppForUser fetches an app only if the user is a member of its organization.
func (db *DB) GetAppForUser(appID, userID string) (*App, error) {
	return db.appSelect(
		`WHERE a.id = ? AND EXISTS (
		   SELECT 1 FROM organization_memberships om
		   WHERE om.organization_id = a.organization_id AND om.user_id = ?
		 )`,
		appID, userID,
	)
}

// ListAppsForUser lists apps across every organization the user belongs to.
func (db *DB) ListAppsForUser(userID string) ([]App, error) {
	rows, err := db.Query(
		`SELECT a.id, a.organization_id, a.name, a.slug, a.deploy_ordered, a.display_order,
		        COALESCE((SELECT COUNT(*) FROM projects p
		                  WHERE p.app_id = a.id AND p.status != 'deleted'), 0),
		        a.created_at, a.updated_at
		 FROM apps a
		 JOIN organization_memberships om ON om.organization_id = a.organization_id
		 WHERE om.user_id = ?
		 ORDER BY a.display_order ASC, lower(a.name) ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list apps for user: %w", err)
	}
	defer rows.Close()

	var apps []App
	for rows.Next() {
		var app App
		if err := rows.Scan(
			&app.ID, &app.OrganizationID, &app.Name, &app.Slug, &app.DeployOrdered,
			&app.DisplayOrder, &app.ProjectCount, &app.CreatedAt, &app.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan app: %w", err)
		}
		apps = append(apps, app)
	}
	return apps, rows.Err()
}

// UpdateAppName renames an app.
func (db *DB) UpdateAppName(appID, name string) (*App, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("update app name: name is required")
	}
	if _, err := db.Exec(
		`UPDATE apps SET name = ?, updated_at = datetime('now') WHERE id = ?`,
		name, appID,
	); err != nil {
		return nil, fmt.Errorf("update app name: %w", err)
	}
	return db.GetApp(appID)
}

// SetAppDeployOrdered toggles whether an app honors member deploy_order during a
// coordinated deploy (true) or deploys all members in parallel (false).
func (db *DB) SetAppDeployOrdered(appID string, ordered bool) (*App, error) {
	if _, err := db.Exec(
		`UPDATE apps SET deploy_ordered = ?, updated_at = datetime('now') WHERE id = ?`,
		ordered, appID,
	); err != nil {
		return nil, fmt.Errorf("set app deploy_ordered: %w", err)
	}
	return db.GetApp(appID)
}

// DeleteApp deletes an app. Member projects survive: projects.app_id is set NULL
// by the ON DELETE SET NULL foreign key.
func (db *DB) DeleteApp(appID string) error {
	if _, err := db.Exec(`DELETE FROM apps WHERE id = ?`, appID); err != nil {
		return fmt.Errorf("delete app: %w", err)
	}
	return nil
}

// AddProjectsToApp moves projects into an app (sets projects.app_id).
func (db *DB) AddProjectsToApp(appID string, projectIDs []string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("add projects to app: begin: %w", err)
	}
	defer tx.Rollback()
	for _, projectID := range uniqueNonEmpty(projectIDs) {
		if err := moveProjectToAppTx(tx, appID, projectID); err != nil {
			return fmt.Errorf("add projects to app: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("add projects to app: commit: %w", err)
	}
	return nil
}

// SetAppMemberOrder assigns deploy_order to each member by its index in
// projectIDs (0-based). All ids must be members of the app; otherwise the whole
// change rolls back with an error.
func (db *DB) SetAppMemberOrder(appID string, projectIDs []string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin reorder: %w", err)
	}
	defer tx.Rollback()

	for i, pid := range projectIDs {
		res, err := tx.Exec(
			`UPDATE projects SET deploy_order = ?, updated_at = datetime('now')
			 WHERE id = ? AND app_id = ? AND status != 'deleted'`,
			i, pid, appID,
		)
		if err != nil {
			return fmt.Errorf("reorder member %s: %w", pid, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("reorder rows affected: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("project %s is not a member of app %s", pid, appID)
		}
	}
	return tx.Commit()
}

// RemoveProjectFromApp detaches a project from its app (app_id = NULL).
func (db *DB) RemoveProjectFromApp(projectID string) error {
	if _, err := db.Exec(
		`UPDATE projects SET app_id = NULL, updated_at = datetime('now')
		 WHERE id = ? AND status != 'deleted'`,
		projectID,
	); err != nil {
		return fmt.Errorf("remove project from app: %w", err)
	}
	return nil
}

// IsOrganizationMember reports whether the user belongs to the organization.
func (db *DB) IsOrganizationMember(orgID, userID string) (bool, error) {
	var exists int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM organization_memberships WHERE organization_id = ? AND user_id = ?`,
		orgID, userID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("is organization member: %w", err)
	}
	return exists > 0, nil
}

// ListProjectsByApp returns the member projects of an app (for the unified view).
func (db *DB) ListProjectsByApp(appID string) ([]Project, error) {
	rows, err := db.Query(
		`SELECT p.id, p.name, p.github_repo, p.github_owner, p.branch, p.user_id,
		        COALESCE(p.organization_id, ''), COALESCE(o.name, ''), COALESCE(p.app_id, ''),
		        p.framework, p.package_manager, p.root_directory, p.output_directory,
		        p.build_command, p.install_command, p.node_version, p.status,
		        p.created_at, p.updated_at,
		        p.host_network_access, p.data_volume_enabled, COALESCE(p.data_mount_path, '/app/data'),
		        p.port, COALESCE(p.resource_tier, 'small'), p.start_command, p.health_path,
		        p.build_filter_enabled, p.watch_paths, p.deploy_order
		 FROM projects p
		 LEFT JOIN organizations o ON o.id = p.organization_id
		 WHERE p.app_id = ? AND p.status != 'deleted'
		 ORDER BY p.deploy_order ASC, p.name ASC`,
		appID,
	)
	if err != nil {
		return nil, fmt.Errorf("list projects by app: %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		var watchPaths sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.GithubRepo, &p.GithubOwner, &p.Branch,
			&p.UserID, &p.OrganizationID, &p.OrganizationName, &p.AppID,
			&p.Framework, &p.PackageManager, &p.RootDirectory, &p.OutputDirectory,
			&p.BuildCommand, &p.InstallCommand, &p.NodeVersion, &p.Status,
			&p.CreatedAt, &p.UpdatedAt,
			&p.HostNetworkAccess, &p.DataVolumeEnabled, &p.DataMountPath,
			&p.Port, &p.ResourceTier, &p.StartCommand, &p.HealthPath,
			&p.BuildFilterEnabled, &watchPaths, &p.DeployOrder); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		p.WatchPaths = scanWatchPaths(watchPaths)
		projects = append(projects, p)
	}
	return projects, rows.Err()
}
