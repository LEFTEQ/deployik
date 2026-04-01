package db

import (
	"database/sql"
	"fmt"
)

func (db *DB) GetUserByGithubID(githubID int64) (*User, error) {
	u := &User{}
	err := db.QueryRow(
		`SELECT id, github_id, username, avatar_url, github_token, role, created_at
		 FROM users WHERE github_id = ?`, githubID,
	).Scan(&u.ID, &u.GithubID, &u.Username, &u.AvatarURL, &u.GithubToken, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by github_id: %w", err)
	}
	return u, nil
}

func (db *DB) GetUserByID(id string) (*User, error) {
	u := &User{}
	err := db.QueryRow(
		`SELECT id, github_id, username, avatar_url, github_token, role, created_at
		 FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.GithubID, &u.Username, &u.AvatarURL, &u.GithubToken, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}

func (db *DB) UpsertUser(u *User) error {
	_, err := db.Exec(
		`INSERT INTO users (id, github_id, username, avatar_url, github_token, role)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(github_id) DO UPDATE SET
		   username = excluded.username,
		   avatar_url = excluded.avatar_url,
		   github_token = excluded.github_token,
		   role = excluded.role`,
		u.ID, u.GithubID, u.Username, u.AvatarURL, u.GithubToken, u.Role,
	)
	if err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}
	return nil
}
