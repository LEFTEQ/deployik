package db

import (
	"database/sql"
	"fmt"
)

func (db *DB) ListProjects(userID string) ([]Project, error) {
	rows, err := db.Query(
		`SELECT id, name, github_repo, github_owner, branch, user_id, framework,
		        root_directory, output_directory, build_command, install_command, node_version, status, created_at, updated_at
		 FROM projects WHERE user_id = ? AND status != 'deleted'
		 ORDER BY updated_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.GithubRepo, &p.GithubOwner, &p.Branch,
			&p.UserID, &p.Framework, &p.RootDirectory, &p.OutputDirectory, &p.BuildCommand, &p.InstallCommand, &p.NodeVersion,
			&p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func (db *DB) GetProject(id string) (*Project, error) {
	p := &Project{}
	err := db.QueryRow(
		`SELECT id, name, github_repo, github_owner, branch, user_id, framework,
		        root_directory, output_directory, build_command, install_command, node_version, status, created_at, updated_at
		 FROM projects WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.GithubRepo, &p.GithubOwner, &p.Branch,
		&p.UserID, &p.Framework, &p.RootDirectory, &p.OutputDirectory, &p.BuildCommand, &p.InstallCommand, &p.NodeVersion,
		&p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	return p, nil
}

func (db *DB) CreateProject(p *Project) error {
	p.ID = NewID()
	_, err := db.Exec(
		`INSERT INTO projects (id, name, github_repo, github_owner, branch, user_id, framework,
		                       root_directory, output_directory, build_command, install_command, node_version, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.GithubRepo, p.GithubOwner, p.Branch, p.UserID, p.Framework,
		p.RootDirectory, p.OutputDirectory, p.BuildCommand, p.InstallCommand, p.NodeVersion, p.Status,
	)
	if err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	return nil
}

func (db *DB) UpdateProject(p *Project) error {
	_, err := db.Exec(
		`UPDATE projects SET name = ?, branch = ?, framework = ?, root_directory = ?, output_directory = ?, build_command = ?,
		        install_command = ?, node_version = ?, status = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		p.Name, p.Branch, p.Framework, p.RootDirectory, p.OutputDirectory, p.BuildCommand, p.InstallCommand,
		p.NodeVersion, p.Status, p.ID,
	)
	if err != nil {
		return fmt.Errorf("update project: %w", err)
	}
	return nil
}

func (db *DB) DeleteProject(id string) error {
	_, err := db.Exec(
		`UPDATE projects SET status = 'deleted', updated_at = datetime('now') WHERE id = ?`, id,
	)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	return nil
}
