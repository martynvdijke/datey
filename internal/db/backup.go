package db

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/datey/datey/internal/photos"
)

// photoArchiveSuffix is appended to the stem of a database backup to name the
// photo sidecar archive. Kept as a separate file so that a database backup
// stays a plain, validatable SQLite file and legacy backups keep working.
const photoArchiveSuffix = "-photos.tar.gz"

// PhotoArchivePath returns the sidecar photo archive path that belongs to the
// given database backup file (e.g. "datey_20260101_120000.db" ->
// "datey_20260101_120000-photos.tar.gz"). It returns "" for names that are
// not .db files.
func PhotoArchivePath(dbBackupFile string) string {
	if !strings.HasSuffix(dbBackupFile, ".db") {
		return ""
	}
	return strings.TrimSuffix(dbBackupFile, ".db") + photoArchiveSuffix
}

// BackupEntry describes one database backup in a backup directory, together
// with its photo sidecar when one exists.
type BackupEntry struct {
	Name          string
	Size          int64
	Modified      time.Time
	PhotosArchive string // base name of the sidecar archive, "" when absent
	PhotosSize    int64
}

// ListBackups reports the database backups in dir. Each entry also reports the
// photo sidecar that belongs to it, so callers can tell a self-contained
// backup from a legacy database-only one.
func ListBackups(dir string) ([]BackupEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []BackupEntry
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".db" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		entry := BackupEntry{Name: e.Name(), Size: info.Size(), Modified: info.ModTime()}
		if sidecar := PhotoArchivePath(e.Name()); sidecar != "" {
			if si, err := os.Stat(filepath.Join(dir, sidecar)); err == nil && si.Mode().IsRegular() {
				entry.PhotosArchive = sidecar
				entry.PhotosSize = si.Size()
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

// Backup creates a timestamped copy of the SQLite database along with a
// sidecar archive of the photo files under <dataDir>/photos, making the
// backup self-contained.
// It checkpoints the WAL first to ensure all data is in the main file,
// then copies it to the backup directory.
func Backup(dbPath, backupDir string, retentionDays int) error {
	backupFile, err := copyDatabase(dbPath, backupDir, "datey", "20060102_150405")
	if err != nil {
		return err
	}

	photoCount, photoBytes, err := writePhotoArchive(filepath.Dir(dbPath), backupFile)
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

	slog.Info("database backup created",
		"path", backupFile, "size_bytes", fileSize(backupFile),
		"photos_archive", PhotoArchivePath(backupFile), "photo_count", photoCount, "photo_bytes", photoBytes)
	return nil
}

// BackupWeekly creates a weekly copy of the SQLite database named
// datey_weekly_YYYYMMDD.db (date-only, one file per week; a same-day re-run
// overwrites it), plus the photo sidecar for that backup. Old weekly backups
// older than retentionWeeks are pruned together with their sidecars.
func BackupWeekly(dbPath, backupDir string, retentionWeeks int) error {
	backupFile, err := copyDatabase(dbPath, backupDir, "datey_weekly", "20060102")
	if err != nil {
		return err
	}

	photoCount, photoBytes, err := writePhotoArchive(filepath.Dir(dbPath), backupFile)
	if err != nil {
		return err
	}

	if retentionWeeks > 0 {
		cutoff := time.Now().AddDate(0, 0, -7*retentionWeeks)
		cleanupBackups(backupDir, func(name string) bool {
			return strings.HasPrefix(name, "datey_weekly_")
		}, cutoff)
	}

	slog.Info("weekly database backup created",
		"path", backupFile, "size_bytes", fileSize(backupFile),
		"photos_archive", PhotoArchivePath(backupFile), "photo_count", photoCount, "photo_bytes", photoBytes)
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

// writePhotoArchive captures every photo file below <dataDir>/photos into a
// gzip-compressed tar next to the database backup. A missing photo directory
// yields an empty archive rather than an error, so restoring a backup taken
// when no photos existed still clears stale photos on the target host.
// Returns the number of files and total bytes captured.
func writePhotoArchive(dataDir, dbBackupFile string) (int, int64, error) {
	archivePath := PhotoArchivePath(dbBackupFile)
	if archivePath == "" {
		return 0, 0, fmt.Errorf("photo archive: %q is not a .db file", dbBackupFile)
	}
	photosDir := photos.NewStore(dataDir).Dir()

	// Build in the destination directory so the final rename is atomic.
	tmp, err := os.CreateTemp(filepath.Dir(archivePath), ".photos-*.tar.gz.tmp")
	if err != nil {
		return 0, 0, fmt.Errorf("create photo archive: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if tmpPath != "" {
			_ = os.Remove(tmpPath)
		}
	}()

	gz := gzip.NewWriter(tmp)
	tw := tar.NewWriter(gz)

	var count int
	var total int64

	if info, err := os.Stat(photosDir); err == nil && info.IsDir() {
		walkErr := filepath.WalkDir(photosDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			// Skip files that are not regular (symlinks, devices) and the
			// store's own in-flight temp files (".tmp-*"): neither belongs in
			// a backup.
			if !d.Type().IsRegular() {
				slog.Warn("photo backup: skipping non-regular file", "path", path)
				return nil
			}
			if strings.HasPrefix(d.Name(), ".tmp-") {
				return nil
			}
			rel, err := filepath.Rel(photosDir, path)
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			hdr := &tar.Header{
				Name:     filepath.ToSlash(rel),
				Mode:     0o644,
				Size:     info.Size(),
				ModTime:  info.ModTime(),
				Typeflag: tar.TypeReg,
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			n, copyErr := io.Copy(tw, f)
			closeErr := f.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			count++
			total += n
			return nil
		})
		if walkErr != nil {
			_ = tw.Close()
			_ = gz.Close()
			_ = tmp.Close()
			return 0, 0, fmt.Errorf("archive photos: %w", walkErr)
		}
	} else if err != nil && !os.IsNotExist(err) {
		_ = tw.Close()
		_ = gz.Close()
		_ = tmp.Close()
		return 0, 0, fmt.Errorf("stat photos dir: %w", err)
	}

	if err := tw.Close(); err != nil {
		_ = gz.Close()
		_ = tmp.Close()
		return 0, 0, fmt.Errorf("close tar: %w", err)
	}
	if err := gz.Close(); err != nil {
		_ = tmp.Close()
		return 0, 0, fmt.Errorf("close gzip: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return 0, 0, fmt.Errorf("sync photo archive: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return 0, 0, fmt.Errorf("close photo archive: %w", err)
	}
	if err := os.Rename(tmpPath, archivePath); err != nil {
		return 0, 0, fmt.Errorf("rename photo archive: %w", err)
	}
	tmpPath = "" // renamed away; nothing left to clean up

	return count, total, nil
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// cleanupBackups removes .db files in dir for which match returns true and
// whose modification time is before cutoff. The photo sidecar of every
// removed backup is removed too, so pruning never leaves orphan archives.
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
				continue
			}
			slog.Debug("backup cleanup: removed old backup", "path", path)
			if sidecar := PhotoArchivePath(path); sidecar != "" {
				if err := os.Remove(sidecar); err != nil && !os.IsNotExist(err) {
					slog.Warn("backup cleanup: remove photo archive", "path", sidecar, "error", err)
				}
			}
		}
	}
}
