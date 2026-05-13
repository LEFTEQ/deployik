package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps the database connection.
type DB struct {
	*sql.DB
}

// Open creates or opens a SQLite database at the given path.
//
// Pragmas are encoded in the DSN with `_pragma=...` so they apply to **every**
// connection the database/sql pool opens, not just the first one. Setting
// `busy_timeout` on a single conn (which the old `conn.Exec(\"PRAGMA …\")`
// loop did) silently leaves additional pool connections without a busy
// timeout, so any concurrent writer (build-log streamer running alongside an
// env-var upsert, for example) returns SQLITE_BUSY → 500 to the caller.
func Open(dbPath string) (*DB, error) {
	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create data dir: %w", err)
		}
	}

	dsn := buildDSN(dbPath)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	log.Printf("Database opened at %s", dbPath)
	return &DB{conn}, nil
}

// buildDSN encodes per-connection pragmas modernc.org/sqlite applies on every
// connection the pool opens. Keep this in sync with the docs at
// https://pkg.go.dev/modernc.org/sqlite#hdr-Connection_string
func buildDSN(dbPath string) string {
	pragmas := []string{
		"_pragma=journal_mode(WAL)",
		"_pragma=busy_timeout(5000)",
		"_pragma=foreign_keys(ON)",
		"_pragma=synchronous(NORMAL)",
		"_pragma=cache_size(-20000)",
	}
	prefix := ""
	if dbPath != ":memory:" {
		prefix = "file:"
	}
	return prefix + dbPath + "?" + joinPragmas(pragmas)
}

func joinPragmas(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += "&" + p
	}
	return out
}

// OpenMemory creates an in-memory database for testing.
//
// SQLite's `:memory:` backend scopes the database per *connection*, and Go's
// sql.DB is a connection pool — so as soon as the pool opens a second
// connection (which it will under any non-trivial concurrent load, including
// goroutines spawned by the pipeline while tests are running), queries start
// hitting a fresh, empty database and fail with "no such table: …".
//
// We force the pool to a single connection so the migrated schema is the only
// schema any query ever sees. This matches what production does implicitly
// with a file-backed DB, and keeps tests deterministic under goroutine churn.
func OpenMemory() (*DB, error) {
	database, err := Open(":memory:")
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	return database, nil
}
