package backup

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestCreateSQLiteSnapshot(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "source.db")

	conn, err := sql.Open("sqlite", sourcePath)
	if err != nil {
		t.Fatalf("open source db: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("set journal mode: %v", err)
	}
	if _, err := conn.Exec("CREATE TABLE projects (id TEXT PRIMARY KEY, name TEXT NOT NULL)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := conn.Exec("INSERT INTO projects (id, name) VALUES ('proj_1', 'Deployik')"); err != nil {
		t.Fatalf("seed row: %v", err)
	}

	backupPath := filepath.Join(tempDir, "backup's snapshot.db")
	if err := CreateSQLiteSnapshot(sourcePath, backupPath); err != nil {
		t.Fatalf("create snapshot: %v", err)
	}

	if err := VerifySQLiteDatabase(backupPath); err != nil {
		t.Fatalf("verify snapshot: %v", err)
	}

	backupConn, err := sql.Open("sqlite", backupPath)
	if err != nil {
		t.Fatalf("open backup db: %v", err)
	}
	defer backupConn.Close()

	var count int
	if err := backupConn.QueryRow("SELECT COUNT(*) FROM projects").Scan(&count); err != nil {
		t.Fatalf("count rows in backup: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row in backup, got %d", count)
	}
}

func TestCreateSQLiteSnapshotRejectsExistingOutput(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "source.db")
	conn, err := sql.Open("sqlite", sourcePath)
	if err != nil {
		t.Fatalf("create source db: %v", err)
	}
	defer conn.Close()

	outputPath := filepath.Join(tempDir, "backup.db")
	if err := os.WriteFile(outputPath, []byte("existing"), 0o644); err != nil {
		t.Fatalf("write existing output file: %v", err)
	}

	if err := CreateSQLiteSnapshot(sourcePath, outputPath); err == nil {
		t.Fatal("expected error when output file already exists")
	}
}
