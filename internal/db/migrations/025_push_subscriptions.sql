-- Web Push subscriptions for the installed PWA (one row per browser/device).
--
-- The endpoint plus the p256dh/auth keys are everything needed to send a push
-- to that device via the Web Push protocol (VAPID-signed). The endpoint is
-- unique globally — re-subscribing the same browser upserts the row.
--
-- Notification preferences are per-device booleans (a phone may want failure
-- pushes while a desktop has them muted). Event types map to the senders:
--   notify_deploy_outcomes — pipeline finalize (deployment live/failed)
--   notify_build_starts    — webhook-triggered deployment created
--   notify_ssl_issues      — domain SSL provisioning failures
--
-- failed_at marks the last delivery failure; rows are hard-deleted when the
-- push service reports the subscription gone (404/410).

CREATE TABLE IF NOT EXISTS push_subscriptions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint TEXT NOT NULL UNIQUE,
    p256dh TEXT NOT NULL,
    auth TEXT NOT NULL,
    device_label TEXT NOT NULL DEFAULT '',
    notify_deploy_outcomes INTEGER NOT NULL DEFAULT 1,
    notify_build_starts INTEGER NOT NULL DEFAULT 1,
    notify_ssl_issues INTEGER NOT NULL DEFAULT 1,
    failed_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_push_subscriptions_user_id ON push_subscriptions(user_id);
