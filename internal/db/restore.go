package db

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// RestorePending holds sentinel metadata for a staged restore.
type RestorePending struct {
	Filename string    `json:"filename"`
	StagedAt time.Time `json:"staged_at"`
	// PhotosStaged is the base name of the staged photo archive belonging to
	// this restore. Empty for legacy (database-only) backups and for backups
	// that had no photo sidecar.
	PhotosStaged string `json:"photos_staged,omitempty"`
}

const (
	restoreSentinelName = "restore-pending.json"
	restoreStagedName   = "restore-pending.db"
	// restoreStagedPhotosName is the staged photo archive that sits next to
	// the staged database until the restart applies the restore.
	restoreStagedPhotosName = "restore-pending-photos.tar.gz"
	// restorePhotosTempPrefix names the extraction directory created inside
	// the data dir so the final photo swap is an atomic rename.
	restorePhotosTempPrefix = ".restore-photos-"
)

// RestoreSentinelPath returns the sentinel file path in dataDir.
func RestoreSentinelPath(dataDir string) string {
	return filepath.Join(dataDir, restoreSentinelName)
}

// RestoreStagedPath returns the staged db file path in dataDir.
func RestoreStagedPath(dataDir string) string {
	return filepath.Join(dataDir, restoreStagedName)
}

// RestoreStagedPhotosPath returns the staged photo archive path in dataDir.
func RestoreStagedPhotosPath(dataDir string) string {
	return filepath.Join(dataDir, restoreStagedPhotosName)
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
// If a photo sidecar exists for the chosen backup, it is staged alongside the
// database so the restart step can swap in both. A missing sidecar is not an
// error: legacy backups restore their database only.
func WriteRestorePending(dataDir, filename string) error {
	return WriteRestorePendingFrom(dataDir, filename, filename)
}

// WriteRestorePendingFrom is WriteRestorePending for a backup whose file may
// live outside DataDir (for example a custom BACKUP_DIR): the sentinel keeps
// displayName, while the photo sidecar is looked up next to sourcePath.
func WriteRestorePendingFrom(dataDir, displayName, sourcePath string) error {
	// Drop any stale staged archive from a previous, abandoned staging.
	_ = os.Remove(RestoreStagedPhotosPath(dataDir))

	rp := RestorePending{Filename: displayName, StagedAt: time.Now().UTC()}

	if src, ok := findPhotoArchive(dataDir, sourcePath); ok {
		if err := copyFile(src, RestoreStagedPhotosPath(dataDir)); err != nil {
			slog.Warn("restore: failed to stage photo archive, restoring database only",
				"archive", src, "error", err)
			_ = os.Remove(RestoreStagedPhotosPath(dataDir))
		} else {
			rp.PhotosStaged = restoreStagedPhotosName
			slog.Info("restore: staged photo archive", "archive", src)
		}
	}

	b, err := json.Marshal(rp)
	if err != nil {
		return err
	}
	path := RestoreSentinelPath(dataDir)
	return os.WriteFile(path, b, 0644)
}

// findPhotoArchive locates the photo sidecar belonging to the named backup.
// The restore entry point only knows the backup's base name, so the default
// backup directory (<dataDir>/backups) is searched as well as the directory
// of filename when it carries one.
func findPhotoArchive(dataDir, filename string) (string, bool) {
	sidecar := PhotoArchivePath(filepath.Base(filename))
	if sidecar == "" {
		return "", false
	}
	var candidates []string
	if dir := filepath.Dir(filename); dir != "." {
		candidates = append(candidates, filepath.Join(dir, sidecar))
	}
	candidates = append(candidates,
		filepath.Join(dataDir, "backups", sidecar),
		filepath.Join(dataDir, sidecar),
	)
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.Mode().IsRegular() {
			return c, true
		}
	}
	return "", false
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
// If valid, atomically renames staged file over datey.db, then swaps the
// staged photos directory into place, clears sentinel, logs success.
// If staged file missing/corrupt, discards staged file, clears sentinel, logs failure, continues.
// A missing or invalid photo archive never blocks the database restore: the
// existing photos are then left untouched.
// Never returns error that would prevent boot.
func applyPendingRestore(dataDir string) {
	pending, err := ReadRestorePending(dataDir)
	if err != nil {
		slog.Warn("restore: read sentinel", "error", err)
		// Clear potentially corrupt sentinel.
		_ = ClearRestorePending(dataDir)
		_ = os.Remove(RestoreStagedPath(dataDir))
		discardStagedPhotos(dataDir, "")
		return
	}
	if pending == nil {
		return
	}
	staged := RestoreStagedPath(dataDir)
	if _, err := os.Stat(staged); err != nil {
		slog.Warn("restore: staged file missing, clearing sentinel", "sentinel", pending.Filename)
		_ = ClearRestorePending(dataDir)
		discardStagedPhotos(dataDir, pending.PhotosStaged)
		return
	}
	if err := ValidateCandidate(staged); err != nil {
		slog.Error("restore: staged file corrupt, discarding", "error", err, "file", staged)
		_ = os.Remove(staged)
		_ = ClearRestorePending(dataDir)
		discardStagedPhotos(dataDir, pending.PhotosStaged)
		return
	}

	// Extract and validate the photos before touching anything, so a bad
	// archive degrades to a database-only restore.
	var photoDir string
	var photoCount int
	var photoBytes int64
	if pending.PhotosStaged != "" {
		archive := filepath.Join(dataDir, filepath.Base(pending.PhotosStaged))
		if _, err := os.Stat(archive); err != nil {
			slog.Warn("restore: staged photo archive missing, restoring database only", "archive", archive)
		} else {
			dir, n, b, err := extractPhotoArchive(dataDir, archive)
			if err != nil {
				slog.Error("restore: staged photo archive invalid, restoring database only",
					"error", err, "archive", archive)
			} else {
				photoDir, photoCount, photoBytes = dir, n, b
			}
		}
	}

	dbPath := filepath.Join(dataDir, "datey.db")
	if err := os.Rename(staged, dbPath); err != nil {
		slog.Error("restore: swap failed", "error", err)
		_ = os.Remove(staged)
		_ = ClearRestorePending(dataDir)
		discardStagedPhotos(dataDir, pending.PhotosStaged)
		if photoDir != "" {
			_ = os.RemoveAll(photoDir)
		}
		return
	}

	if photoDir != "" {
		if err := swapPhotosDir(dataDir, photoDir); err != nil {
			slog.Error("restore: photo swap failed, keeping existing photos", "error", err)
			_ = os.RemoveAll(photoDir)
		} else {
			slog.Info("restore: applied staged photos",
				"files", photoCount, "bytes", photoBytes)
		}
	}

	_ = ClearRestorePending(dataDir)
	discardStagedPhotos(dataDir, pending.PhotosStaged)
	slog.Info("restore: applied staged database", "filename", pending.Filename)
}

// discardStagedPhotos removes the staged archive file and any extraction
// directory left in dataDir. It never touches the live photos directory.
func discardStagedPhotos(dataDir, stagedName string) {
	if stagedName != "" {
		_ = os.Remove(filepath.Join(dataDir, filepath.Base(stagedName)))
	}
	_ = os.Remove(RestoreStagedPhotosPath(dataDir))
}

// validArchiveEntryName rejects archive entries that could escape the target
// directory: absolute paths, drive letters, any ".." element, backslashes and
// NUL bytes.
func validArchiveEntryName(name string) error {
	if name == "" {
		return fmt.Errorf("empty entry name")
	}
	if strings.ContainsRune(name, 0) {
		return fmt.Errorf("entry name contains NUL")
	}
	if strings.Contains(name, "\\") {
		return fmt.Errorf("entry name contains backslash")
	}
	if path.IsAbs(name) || filepath.IsAbs(name) {
		return fmt.Errorf("absolute entry name")
	}
	if filepath.VolumeName(name) != "" {
		return fmt.Errorf("entry name has a volume")
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return fmt.Errorf("parent-directory entry name")
		}
	}
	return nil
}

// extractPhotoArchive unpacks the archive into a fresh temporary directory
// inside dataDir (same filesystem, so the later swap is an atomic rename).
// Every entry is validated first; symlinks, hardlinks, devices and traversal
// attempts abort the extraction and the temp directory is removed.
func extractPhotoArchive(dataDir, archive string) (string, int, int64, error) {
	tmpDir, err := os.MkdirTemp(dataDir, restorePhotosTempPrefix+"*")
	if err != nil {
		return "", 0, 0, fmt.Errorf("create photo temp dir: %w", err)
	}

	f, err := os.Open(archive)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", 0, 0, fmt.Errorf("open photo archive: %w", err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", 0, 0, fmt.Errorf("read photo archive: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	var count int
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = os.RemoveAll(tmpDir)
			return "", 0, 0, fmt.Errorf("read tar entry: %w", err)
		}
		if err := validArchiveEntryName(hdr.Name); err != nil {
			_ = os.RemoveAll(tmpDir)
			return "", 0, 0, fmt.Errorf("unsafe archive entry %q: %w", hdr.Name, err)
		}
		target := filepath.Join(tmpDir, filepath.FromSlash(hdr.Name))
		if target != tmpDir && !strings.HasPrefix(target, tmpDir+string(os.PathSeparator)) {
			_ = os.RemoveAll(tmpDir)
			return "", 0, 0, fmt.Errorf("entry %q escapes target directory", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				_ = os.RemoveAll(tmpDir)
				return "", 0, 0, fmt.Errorf("create photo dir: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				_ = os.RemoveAll(tmpDir)
				return "", 0, 0, fmt.Errorf("create photo dir: %w", err)
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				_ = os.RemoveAll(tmpDir)
				return "", 0, 0, fmt.Errorf("create photo file: %w", err)
			}
			n, copyErr := io.Copy(out, io.LimitReader(tr, hdr.Size))
			closeErr := out.Close()
			if copyErr != nil {
				_ = os.RemoveAll(tmpDir)
				return "", 0, 0, fmt.Errorf("extract photo %q: %w", hdr.Name, copyErr)
			}
			if closeErr != nil {
				_ = os.RemoveAll(tmpDir)
				return "", 0, 0, fmt.Errorf("close photo %q: %w", hdr.Name, closeErr)
			}
			count++
			total += n
		default:
			// Symlinks, hardlinks, devices, FIFOs: refuse the whole archive.
			_ = os.RemoveAll(tmpDir)
			return "", 0, 0, fmt.Errorf("unsupported archive entry type %q for %q", hdr.Typeflag, hdr.Name)
		}
	}

	return tmpDir, count, total, nil
}

// swapPhotosDir replaces <dataDir>/photos with the freshly extracted
// directory. The existing photos are renamed aside first and only deleted
// after the new directory is in place, so a failure keeps the old photos.
func swapPhotosDir(dataDir, stagedDir string) error {
	photosDir := filepath.Join(dataDir, "photos")
	oldDir := photosDir + ".old"

	// Remove a leftover from a previous failed swap (never the live dir).
	_ = os.RemoveAll(oldDir)

	moved := false
	if _, err := os.Lstat(photosDir); err == nil {
		if err := os.Rename(photosDir, oldDir); err != nil {
			return fmt.Errorf("set aside existing photos: %w", err)
		}
		moved = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat photos dir: %w", err)
	}

	if err := os.Rename(stagedDir, photosDir); err != nil {
		if moved {
			_ = os.Rename(oldDir, photosDir) // roll back
		}
		return fmt.Errorf("install staged photos: %w", err)
	}

	if moved {
		_ = os.RemoveAll(oldDir)
	}
	return nil
}
