package db

import (
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Backup creates a timestamped copy of the SQLite database.
// It checkpoints the WAL first to ensure all data is in the main file,
// then copies it to the backup directory.
func Backup(dbPath, backupDir string, retentionDays int) error {
	backupFile, err := copyDatabase(dbPath, backupDir, "datey", "20060102_150405")
	if err != nil {
		return err
	}

	// Clean up daily backups older than retention days. Weekly backups
	// (datey_weekly_*.db) are excluded so they can outlive daily retention.
	if retentionDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -retentionDays)
		cleanupBackups(backupDir, func(name string) bool {
			return strings.HasPrefix(name, "datey_") && !strings.HasPrefix(name, "datey_weekly_")
		}, cutoff)
	}

	slog.Info("database backup created", "path", backupFile, "size_bytes", fileSize(backupFile))
	return nil
}

// BackupWeekly creates a weekly copy of the SQLite database named
// datey_weekly_YYYYMMDD.db (date-only, one file per week; a same-day re-run
// overwrites it). Old weekly backups older than retentionWeeks are pruned.
func BackupWeekly(dbPath, backupDir string, retentionWeeks int) error {
	backupFile, err := copyDatabase(dbPath, backupDir, "datey_weekly", "20060102")
	if err != nil {
		return err
	}

	if retentionWeeks > 0 {
		cutoff := time.Now().AddDate(0, 0, -7*retentionWeeks)
		cleanupBackups(backupDir, func(name string) bool {
			return strings.HasPrefix(name, "datey_weekly_")
		}, cutoff)
	}

	slog.Info("weekly database backup created", "path", backupFile, "size_bytes", fileSize(backupFile))
	return nil
}

// copyDatabase checkpoints the WAL so all writes are flushed to the main .db
// file, then copies it into backupDir under a name with the given prefix and
// timestamp layout. Returns the path of the created backup.
func copyDatabase(dbPath, backupDir, prefix, timeFormat string) (string, error) {
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	// Open a temporary connection to checkpoint the WAL.
	// This ensures all pending writes are flushed to the main .db file
	// before we copy it.
	conn, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_timeout=5000")
	if err != nil {
		return "", fmt.Errorf("open for backup: %w", err)
	}

	if _, err := conn.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		_ = conn.Close()
		return "", fmt.Errorf("wal checkpoint: %w", err)
	}
	_ = conn.Close()

	// Copy the database file.
	timestamp := time.Now().Format(timeFormat)
	backupFile := filepath.Join(backupDir, fmt.Sprintf("%s_%s.db", prefix, timestamp))

	src, err := os.Open(dbPath)
	if err != nil {
		return "", fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = src.Close() }()

	dst, err := os.Create(backupFile)
	if err != nil {
		return "", fmt.Errorf("create backup: %w", err)
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("copy: %w", err)
	}

	if err := dst.Sync(); err != nil {
		return "", fmt.Errorf("sync backup: %w", err)
	}
	if err := dst.Close(); err != nil {
		return "", fmt.Errorf("close backup: %w", err)
	}

	return backupFile, nil
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// cleanupBackups removes .db files in dir for which match returns true and
// whose modification time is before cutoff.
func cleanupBackups(dir string, match func(name string) bool, cutoff time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		slog.Warn("backup cleanup: read dir", "error", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".db" {
			continue
		}
		if !match(entry.Name()) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			path := filepath.Join(dir, entry.Name())
			if err := os.Remove(path); err != nil {
				slog.Warn("backup cleanup: remove", "path", path, "error", err)
			} else {
				slog.Debug("backup cleanup: removed old backup", "path", path)
			}
		}
	}
}
