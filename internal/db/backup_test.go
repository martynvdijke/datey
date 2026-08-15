package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// createTestDB creates a real (empty) SQLite file at path and returns it.
func createTestDB(t *testing.T, path string) {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	if _, err := conn.Exec("CREATE TABLE IF NOT EXISTS t (id integer)"); err != nil {
		_ = conn.Close()
		t.Fatalf("init test db: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close test db: %v", err)
	}
}

var weeklyFileRE = regexp.MustCompile(`^datey_weekly_\d{8}\.db$`)

func weeklyFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && weeklyFileRE.MatchString(e.Name()) {
			files = append(files, e.Name())
		}
	}
	return files
}

func TestBackupWeekly_CreatesFile(t *testing.T) {
	backupDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "datey.db")
	createTestDB(t, dbPath)

	if err := BackupWeekly(dbPath, backupDir, 52); err != nil {
		t.Fatalf("BackupWeekly: %v", err)
	}

	files := weeklyFiles(t, backupDir)
	if len(files) != 1 {
		t.Fatalf("expected 1 weekly backup, got %d: %v", len(files), files)
	}
}

func TestBackupWeekly_SameDayIsIdempotent(t *testing.T) {
	backupDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "datey.db")
	createTestDB(t, dbPath)

	if err := BackupWeekly(dbPath, backupDir, 52); err != nil {
		t.Fatalf("first BackupWeekly: %v", err)
	}
	if err := BackupWeekly(dbPath, backupDir, 52); err != nil {
		t.Fatalf("second BackupWeekly: %v", err)
	}

	if files := weeklyFiles(t, backupDir); len(files) != 1 {
		t.Fatalf("expected 1 weekly backup after re-run (same-day overwrite), got %d: %v", len(files), files)
	}
}

func TestBackupWeekly_CleanupOldWeekly(t *testing.T) {
	backupDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "datey.db")
	createTestDB(t, dbPath)

	old := filepath.Join(backupDir, "datey_weekly_20200101.db")
	if err := os.WriteFile(old, []byte("old"), 0644); err != nil {
		t.Fatalf("write old weekly: %v", err)
	}
	oldTime := time.Now().AddDate(0, 0, -400)
	if err := os.Chtimes(old, oldTime, oldTime); err != nil {
		t.Fatalf("set old mtime: %v", err)
	}

	if err := BackupWeekly(dbPath, backupDir, 52); err != nil {
		t.Fatalf("BackupWeekly: %v", err)
	}

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("expected old weekly backup pruned (older than 52 weeks), stat err=%v", err)
	}
	if files := weeklyFiles(t, backupDir); len(files) != 1 {
		t.Errorf("expected only the fresh weekly backup to remain, got %d: %v", len(files), files)
	}
}

func TestDailyCleanup_DoesNotTouchWeekly(t *testing.T) {
	backupDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "datey.db")
	createTestDB(t, dbPath)

	// Seed an old daily backup and an old weekly backup, both older than the
	// 30-day daily retention.
	oldDaily := filepath.Join(backupDir, "datey_20200101_000000.db")
	oldWeekly := filepath.Join(backupDir, "datey_weekly_20200101.db")
	for _, p := range []string{oldDaily, oldWeekly} {
		if err := os.WriteFile(p, []byte("old"), 0644); err != nil {
			t.Fatalf("write %s: %v", filepath.Base(p), err)
		}
	}
	oldTime := time.Now().AddDate(0, 0, -400)
	if err := os.Chtimes(oldDaily, oldTime, oldTime); err != nil {
		t.Fatalf("set daily mtime: %v", err)
	}
	if err := os.Chtimes(oldWeekly, oldTime, oldTime); err != nil {
		t.Fatalf("set weekly mtime: %v", err)
	}

	if err := Backup(dbPath, backupDir, 30); err != nil {
		t.Fatalf("Backup: %v", err)
	}

	if _, err := os.Stat(oldDaily); !os.IsNotExist(err) {
		t.Errorf("expected old daily backup pruned by daily retention, stat err=%v", err)
	}
	if _, err := os.Stat(oldWeekly); err != nil {
		t.Errorf("expected old weekly backup untouched by daily cleanup, stat err=%v", err)
	}
}
