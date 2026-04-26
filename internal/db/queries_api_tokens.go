package db

import (
	"database/sql"
	"fmt"
	"time"
)

// CreateAPIToken inserts a new token. The caller must set TokenHash; the raw
// token is never persisted. ID is auto-assigned if empty.
func (db *DB) CreateAPIToken(token *APIToken) error {
	if token.ID == "" {
		token.ID = NewID()
	}
	var expiresAt any
	if token.ExpiresAt.Valid {
		expiresAt = token.ExpiresAt.Time
	} else {
		expiresAt = nil
	}
	_, err := db.Exec(
		`INSERT INTO api_tokens (id, user_id, name, token_hash, expires_at)
		 VALUES (?, ?, ?, ?, ?)`,
		token.ID, token.UserID, token.Name, token.TokenHash, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("create api token: %w", err)
	}
	return nil
}

// GetAPITokenByHash returns the token only when it is active: not revoked,
// not past its expires_at. Not-found, revoked, and expired all collapse to
// (nil, nil) so the middleware can treat them identically.
func (db *DB) GetAPITokenByHash(tokenHash string) (*APIToken, error) {
	t := &APIToken{}
	err := db.QueryRow(
		`SELECT id, user_id, name, token_hash, last_used_at, expires_at, revoked_at, created_at
		 FROM api_tokens
		 WHERE token_hash = ? AND revoked_at IS NULL`,
		tokenHash,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.LastUsedAt, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get api token: %w", err)
	}
	if t.ExpiresAt.Valid && time.Now().After(t.ExpiresAt.Time) {
		return nil, nil
	}
	return t, nil
}

// ListAPITokensForUser returns ALL tokens for a user, including revoked and
// expired ones — the UI shows them with status badges so the user can audit
// what was issued.
func (db *DB) ListAPITokensForUser(userID string) ([]APIToken, error) {
	rows, err := db.Query(
		`SELECT id, user_id, name, token_hash, last_used_at, expires_at, revoked_at, created_at
		 FROM api_tokens
		 WHERE user_id = ?
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list api tokens: %w", err)
	}
	defer rows.Close()

	var tokens []APIToken
	for rows.Next() {
		var t APIToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.LastUsedAt, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan api token: %w", err)
		}
		tokens = append(tokens, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows api tokens: %w", err)
	}
	return tokens, nil
}

// RevokeAPIToken sets revoked_at on a token, but only when the requesting
// user owns it — a strict ownership check guards against IDOR even though
// the caller is already authenticated.
func (db *DB) RevokeAPIToken(id, userID string) error {
	result, err := db.Exec(
		`UPDATE api_tokens
		 SET revoked_at = datetime('now')
		 WHERE id = ? AND user_id = ? AND revoked_at IS NULL`,
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("revoke api token: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("revoke rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// TouchAPITokenLastUsed is fire-and-forget from the middleware. Errors are
// non-fatal — a missed touch is acceptable; an auth failure is not.
func (db *DB) TouchAPITokenLastUsed(id string) error {
	_, err := db.Exec(
		`UPDATE api_tokens SET last_used_at = datetime('now') WHERE id = ?`,
		id,
	)
	if err != nil {
		return fmt.Errorf("touch api token: %w", err)
	}
	return nil
}
