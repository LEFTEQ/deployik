package db

import (
	"database/sql"
	"fmt"
	"time"
)

func (db *DB) CreateRefreshSession(session *RefreshSession) error {
	session.ID = NewID()
	_, err := db.Exec(
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, last_used_at)
		 VALUES (?, ?, ?, ?, ?)`,
		session.ID, session.UserID, session.TokenHash, session.ExpiresAt, session.LastUsedAt,
	)
	if err != nil {
		return fmt.Errorf("create refresh session: %w", err)
	}
	return nil
}

func (db *DB) GetActiveRefreshSessionByHash(tokenHash string) (*RefreshSession, error) {
	session := &RefreshSession{}
	err := db.QueryRow(
		`SELECT id, user_id, token_hash, expires_at, last_used_at, revoked_at, created_at
		 FROM refresh_tokens
		 WHERE token_hash = ? AND revoked_at IS NULL`,
		tokenHash,
	).Scan(
		&session.ID,
		&session.UserID,
		&session.TokenHash,
		&session.ExpiresAt,
		&session.LastUsedAt,
		&session.RevokedAt,
		&session.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get refresh session: %w", err)
	}
	if time.Now().After(session.ExpiresAt) {
		return nil, nil
	}
	return session, nil
}

func (db *DB) RotateRefreshSession(oldSessionID, userID, newTokenHash string, expiresAt time.Time) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin refresh rotation: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`UPDATE refresh_tokens
		 SET revoked_at = datetime('now'), last_used_at = datetime('now')
		 WHERE id = ? AND revoked_at IS NULL`,
		oldSessionID,
	)
	if err != nil {
		return fmt.Errorf("revoke refresh session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("refresh rotation rows affected: %w", err)
	}
	if rowsAffected != 1 {
		return sql.ErrNoRows
	}

	if _, err := tx.Exec(
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, last_used_at)
		 VALUES (?, ?, ?, ?, datetime('now'))`,
		NewID(), userID, newTokenHash, expiresAt,
	); err != nil {
		return fmt.Errorf("create rotated refresh session: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit refresh rotation: %w", err)
	}
	return nil
}

func (db *DB) RevokeRefreshSessionByHash(tokenHash string) error {
	_, err := db.Exec(
		`UPDATE refresh_tokens
		 SET revoked_at = datetime('now'), last_used_at = datetime('now')
		 WHERE token_hash = ? AND revoked_at IS NULL`,
		tokenHash,
	)
	if err != nil {
		return fmt.Errorf("revoke refresh session by hash: %w", err)
	}
	return nil
}
