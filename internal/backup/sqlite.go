package backup

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// CreateSQLiteSnapshot writes a consistent on-disk snapshot of a live SQLite database.
// It uses VACUUM INTO so the backup is transactionally consistent even when the source
// database is running in WAL mode.
func CreateSQLiteSnapshot(sourcePath, outputPath string) error {
	sourcePath = strings.TrimSpace(sourcePath)
	outputPath = strings.TrimSpace(outputPath)

	if sourcePath == "" {
		return fmt.Errorf("source database path is required")
	}
	if outputPath == "" {
		return fmt.Errorf("output path is required")
	}
	if sourcePath == ":memory:" {
		return fmt.Errorf("cannot snapshot an in-memory database")
	}

	if _, err := os.Stat(sourcePath); err != nil {
		return fmt.Errorf("stat source database: %w", err)
	}
	if _, err := os.Stat(outputPath); err == nil {
		return fmt.Errorf("output path already exists: %s", outputPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat output path: %w", err)
	}

	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	conn, err := sql.Open("sqlite", sourcePath)
	if err != nil {
		return fmt.Errorf("open source database: %w", err)
	}
	defer conn.Close()

	if _, err := conn.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return fmt.Errorf("set busy_timeout pragma: %w", err)
	}

	tempPath := filepath.Join(
		outputDir,
		fmt.Sprintf(".%s.%d.tmp", filepath.Base(outputPath), time.Now().UnixNano()),
	)
	_ = os.Remove(tempPath)
	defer os.Remove(tempPath)

	if _, err := conn.Exec(fmt.Sprintf("VACUUM INTO %s", sqliteStringLiteral(tempPath))); err != nil {
		return fmt.Errorf("vacuum into backup file: %w", err)
	}

	if err := VerifySQLiteDatabase(tempPath); err != nil {
		return fmt.Errorf("verify snapshot: %w", err)
	}

	if err := os.Rename(tempPath, outputPath); err != nil {
		return fmt.Errorf("move snapshot into place: %w", err)
	}

	return nil
}

// VerifySQLiteDatabase runs an integrity check against a SQLite file.
func VerifySQLiteDatabase(dbPath string) error {
	dbPath = strings.TrimSpace(dbPath)
	if dbPath == "" {
		return fmt.Errorf("database path is required")
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer conn.Close()

	var result string
	if err := conn.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("run integrity_check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("integrity_check failed: %s", result)
	}

	return nil
}

func sqliteStringLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
