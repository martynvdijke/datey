package db

import (
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Backup creates a timestamped copy of the SQLite database.
// It checkpoints the WAL first to ensure all data is in the main file,
// then copies it to the backup directory.
func Backup(dbPath, backupDir string, retentionDays int) error {
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}

	// Open a temporary connection to checkpoint the WAL.
	// This ensures all pending writes are flushed to the main .db file
	// before we copy it.
	conn, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_timeout=5000")
	if err != nil {
		return fmt.Errorf("open for backup: %w", err)
	}

	if _, err := conn.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		conn.Close()
		return fmt.Errorf("wal checkpoint: %w", err)
	}
	conn.Close()

	// Copy the database file.
	timestamp := time.Now().Format("20060102_150405")
	backupFile := filepath.Join(backupDir, fmt.Sprintf("datey_%s.db", timestamp))

	src, err := os.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(backupFile)
	if err != nil {
		return fmt.Errorf("create backup: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy: %w", err)
	}

	if err := dst.Sync(); err != nil {
		return fmt.Errorf("sync backup: %w", err)
	}
	if err := dst.Close(); err != nil {
		return fmt.Errorf("close backup: %w", err)
	}
	dst = nil

	info, err := os.Stat(backupFile)
	size := int64(0)
	if err == nil {
		size = info.Size()
	}

	slog.Info("database backup created", "path", backupFile, "size_bytes", size)

	// Clean up backups older than retention days.
	if retentionDays > 0 {
		cleanupOldBackups(backupDir, retentionDays)
	}

	return nil
}

func cleanupOldBackups(dir string, retentionDays int) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
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
