package db

import "testing"

func TestMigration026CreatesAppsSchema(t *testing.T) {
	database := newTestDB(t)

	// apps table exists
	var tableName string
	err := database.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='apps'`,
	).Scan(&tableName)
	if err != nil {
		t.Fatalf("apps table not found: %v", err)
	}

	// projects.app_id column exists and defaults to NULL
	var appIDColumns int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('projects') WHERE name='app_id'`,
	).Scan(&appIDColumns); err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	if appIDColumns != 1 {
		t.Fatalf("expected projects.app_id column, found %d", appIDColumns)
	}
}
