package db

import (
	"database/sql"
	"fmt"
)

const pushSubscriptionColumns = `id, user_id, endpoint, p256dh, auth, device_label,
	notify_deploy_outcomes, notify_build_starts, notify_ssl_issues, created_at`

// UpsertPushSubscription registers a device for the user. Re-subscribing an
// existing endpoint refreshes its keys/label/preferences and re-owns it (a
// push endpoint is device-scoped; if another account on the same browser
// subscribes, the endpoint now belongs to that account).
func (db *DB) UpsertPushSubscription(sub *PushSubscription) error {
	if sub.ID == "" {
		sub.ID = NewID()
	}
	_, err := db.Exec(
		`INSERT INTO push_subscriptions
		   (id, user_id, endpoint, p256dh, auth, device_label,
		    notify_deploy_outcomes, notify_build_starts, notify_ssl_issues)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(endpoint) DO UPDATE SET
		   user_id = excluded.user_id,
		   p256dh = excluded.p256dh,
		   auth = excluded.auth,
		   device_label = excluded.device_label,
		   notify_deploy_outcomes = excluded.notify_deploy_outcomes,
		   notify_build_starts = excluded.notify_build_starts,
		   notify_ssl_issues = excluded.notify_ssl_issues,
		   failed_at = NULL`,
		sub.ID, sub.UserID, sub.Endpoint, sub.P256dh, sub.Auth, sub.DeviceLabel,
		sub.NotifyDeployOutcomes, sub.NotifyBuildStarts, sub.NotifySSLIssues,
	)
	if err != nil {
		return fmt.Errorf("upsert push subscription: %w", err)
	}
	// On conflict the stored row keeps its original ID — read it back so the
	// caller returns the canonical row.
	stored, err := db.GetPushSubscriptionByEndpoint(sub.Endpoint)
	if err != nil {
		return err
	}
	if stored != nil {
		*sub = *stored
	}
	return nil
}

func (db *DB) scanPushSubscription(row interface {
	Scan(dest ...any) error
}) (*PushSubscription, error) {
	sub := &PushSubscription{}
	err := row.Scan(&sub.ID, &sub.UserID, &sub.Endpoint, &sub.P256dh, &sub.Auth,
		&sub.DeviceLabel, &sub.NotifyDeployOutcomes, &sub.NotifyBuildStarts,
		&sub.NotifySSLIssues, &sub.CreatedAt)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (db *DB) GetPushSubscriptionByEndpoint(endpoint string) (*PushSubscription, error) {
	row := db.QueryRow(
		`SELECT `+pushSubscriptionColumns+` FROM push_subscriptions WHERE endpoint = ?`,
		endpoint,
	)
	sub, err := db.scanPushSubscription(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get push subscription by endpoint: %w", err)
	}
	return sub, nil
}

func (db *DB) GetPushSubscriptionForUser(id, userID string) (*PushSubscription, error) {
	row := db.QueryRow(
		`SELECT `+pushSubscriptionColumns+` FROM push_subscriptions WHERE id = ? AND user_id = ?`,
		id, userID,
	)
	sub, err := db.scanPushSubscription(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get push subscription: %w", err)
	}
	return sub, nil
}

func (db *DB) ListPushSubscriptionsForUser(userID string) ([]PushSubscription, error) {
	rows, err := db.Query(
		`SELECT `+pushSubscriptionColumns+` FROM push_subscriptions
		 WHERE user_id = ? ORDER BY created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list push subscriptions: %w", err)
	}
	defer rows.Close()

	subs := []PushSubscription{}
	for rows.Next() {
		sub, err := db.scanPushSubscription(rows)
		if err != nil {
			return nil, fmt.Errorf("scan push subscription: %w", err)
		}
		subs = append(subs, *sub)
	}
	return subs, rows.Err()
}

// ListPushSubscriptionsForProject returns subscriptions of every user who can
// access the project under the same rule as authz: the creator plus members
// of the owning organization. eventColumn picks the per-device preference
// gate and must be one of the notify_* columns (callers pass constants, never
// user input).
func (db *DB) ListPushSubscriptionsForProject(projectID, eventColumn string) ([]PushSubscription, error) {
	switch eventColumn {
	case "notify_deploy_outcomes", "notify_build_starts", "notify_ssl_issues":
	default:
		return nil, fmt.Errorf("invalid push event column %q", eventColumn)
	}
	rows, err := db.Query(
		`SELECT `+pushSubscriptionColumns+` FROM push_subscriptions
		 WHERE `+eventColumn+` = 1
		   AND user_id IN (
		     SELECT p.user_id FROM projects p WHERE p.id = ?
		     UNION
		     SELECT om.user_id
		     FROM organization_memberships om
		     JOIN projects p ON p.organization_id = om.organization_id
		     WHERE p.id = ?
		   )`,
		projectID, projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list push subscriptions for project: %w", err)
	}
	defer rows.Close()

	subs := []PushSubscription{}
	for rows.Next() {
		sub, err := db.scanPushSubscription(rows)
		if err != nil {
			return nil, fmt.Errorf("scan push subscription: %w", err)
		}
		subs = append(subs, *sub)
	}
	return subs, rows.Err()
}

// UpdatePushSubscriptionPreferences updates per-device event toggles. Nil
// pointers preserve the stored value (presence-aware PATCH semantics).
func (db *DB) UpdatePushSubscriptionPreferences(id, userID string, deployOutcomes, buildStarts, sslIssues *bool) error {
	_, err := db.Exec(
		`UPDATE push_subscriptions SET
		   notify_deploy_outcomes = COALESCE(?, notify_deploy_outcomes),
		   notify_build_starts = COALESCE(?, notify_build_starts),
		   notify_ssl_issues = COALESCE(?, notify_ssl_issues)
		 WHERE id = ? AND user_id = ?`,
		nullableBool(deployOutcomes), nullableBool(buildStarts), nullableBool(sslIssues),
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("update push subscription preferences: %w", err)
	}
	return nil
}

func (db *DB) DeletePushSubscription(id, userID string) (bool, error) {
	res, err := db.Exec(
		`DELETE FROM push_subscriptions WHERE id = ? AND user_id = ?`,
		id, userID,
	)
	if err != nil {
		return false, fmt.Errorf("delete push subscription: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeletePushSubscriptionByEndpoint removes a dead subscription after the push
// service reported it gone (404/410).
func (db *DB) DeletePushSubscriptionByEndpoint(endpoint string) error {
	_, err := db.Exec(`DELETE FROM push_subscriptions WHERE endpoint = ?`, endpoint)
	if err != nil {
		return fmt.Errorf("delete push subscription by endpoint: %w", err)
	}
	return nil
}

// MarkPushSubscriptionFailed records a transient delivery failure.
func (db *DB) MarkPushSubscriptionFailed(endpoint string) error {
	_, err := db.Exec(
		`UPDATE push_subscriptions SET failed_at = datetime('now') WHERE endpoint = ?`,
		endpoint,
	)
	if err != nil {
		return fmt.Errorf("mark push subscription failed: %w", err)
	}
	return nil
}

func nullableBool(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}
