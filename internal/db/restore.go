package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// RestorePending holds sentinel metadata for a staged restore.
type RestorePending struct {
	Filename string    `json:"filename"`
	StagedAt time.Time `json:"staged_at"`
}

const (
	restoreSentinelName = "restore-pending.json"
	restoreStagedName   = "restore-pending.db"
)

// RestoreSentinelPath returns the sentinel file path in dataDir.
func RestoreSentinelPath(dataDir string) string {
	return filepath.Join(dataDir, restoreSentinelName)
}

// RestoreStagedPath returns the staged db file path in dataDir.
func RestoreStagedPath(dataDir string) string {
	return filepath.Join(dataDir, restoreStagedName)
}

// ReadRestorePending reads the sentinel if present; returns nil if absent.
func ReadRestorePending(dataDir string) (*RestorePending, error) {
	p := RestoreSentinelPath(dataDir)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var rp RestorePending
	if err := json.Unmarshal(b, &rp); err != nil {
		return nil, fmt.Errorf("parse restore sentinel: %w", err)
	}
	return &rp, nil
}

// WriteRestorePending writes sentinel file with filename and current time.
func WriteRestorePending(dataDir, filename string) error {
	rp := RestorePending{Filename: filename, StagedAt: time.Now().UTC()}
	b, err := json.Marshal(rp)
	if err != nil {
		return err
	}
	path := RestoreSentinelPath(dataDir)
	return os.WriteFile(path, b, 0644)
}

// ClearRestorePending removes the sentinel file if present.
func ClearRestorePending(dataDir string) error {
	p := RestoreSentinelPath(dataDir)
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ValidateCandidate checks that path is a valid SQLite file.
// It verifies non-empty, magic header "SQLite format 3\x00", and
// PRAGMA integrity_check == "ok" on a temp copy opened read-only.
func ValidateCandidate(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat candidate: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("candidate is empty")
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open candidate: %w", err)
	}
	defer func() { _ = f.Close() }()
	header := make([]byte, 16)
	n, err := io.ReadFull(f, header)
	if err != nil {
		return fmt.Errorf("read header: %w", err)
	}
	if n != 16 {
		return fmt.Errorf("candidate too small")
	}
	if string(header) != "SQLite format 3\x00" {
		return fmt.Errorf("not a SQLite file")
	}
	_ = f.Close()

	// Copy to temp file to avoid locking original and validate.
	tmp, err := os.CreateTemp("", "datey-restore-validate-*.db")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := copyFile(path, tmpPath); err != nil {
		return fmt.Errorf("copy to temp: %w", err)
	}

	conn, err := sql.Open("sqlite3", tmpPath+"?mode=ro&_query_only=1")
	if err != nil {
		return fmt.Errorf("open temp copy: %w", err)
	}
	defer func() { _ = conn.Close() }()
	row := conn.QueryRow("PRAGMA integrity_check")
	var result string
	if err := row.Scan(&result); err != nil {
		return fmt.Errorf("integrity_check failed: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("integrity_check: %s", result)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	return out.Close()
}

// applyPendingRestore checks sentinel+staged file before DB open.
// If valid, atomically renames staged file over datey.db, clears sentinel, logs success.
// If staged file missing/corrupt, discards staged file, clears sentinel, logs failure, continues.
// Never returns error that would prevent boot.
func applyPendingRestore(dataDir string) {
	pending, err := ReadRestorePending(dataDir)
	if err != nil {
		slog.Warn("restore: read sentinel", "error", err)
		// Clear potentially corrupt sentinel.
		_ = ClearRestorePending(dataDir)
		_ = os.Remove(RestoreStagedPath(dataDir))
		return
	}
	if pending == nil {
		return
	}
	staged := RestoreStagedPath(dataDir)
	if _, err := os.Stat(staged); err != nil {
		slog.Warn("restore: staged file missing, clearing sentinel", "sentinel", pending.Filename)
		_ = ClearRestorePending(dataDir)
		return
	}
	if err := ValidateCandidate(staged); err != nil {
		slog.Error("restore: staged file corrupt, discarding", "error", err, "file", staged)
		_ = os.Remove(staged)
		_ = ClearRestorePending(dataDir)
		return
	}
	dbPath := filepath.Join(dataDir, "datey.db")
	if err := os.Rename(staged, dbPath); err != nil {
		slog.Error("restore: swap failed", "error", err)
		_ = os.Remove(staged)
		_ = ClearRestorePending(dataDir)
		return
	}
	_ = ClearRestorePending(dataDir)
	slog.Info("restore: applied staged database", "filename", pending.Filename)
}
