package db

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// writePhotos creates photo files (relative name -> content) under dir.
func writePhotos(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create photos dir: %v", err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write photo %s: %v", name, err)
		}
	}
}

// readArchive returns entry name -> content for a gzip tar archive.
func readArchive(t *testing.T, archive string) map[string]string {
	t.Helper()
	f, err := os.Open(archive)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	out := map[string]string{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read tar entry: %v", err)
		}
		out[hdr.Name] = string(b)
	}
	return out
}

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

func TestBackup_CapturesPhotosArchive(t *testing.T) {
	dataDir := t.TempDir()
	backupDir := filepath.Join(dataDir, "backups")
	dbPath := filepath.Join(dataDir, "datey.db")
	createTestDB(t, dbPath)
	writePhotos(t, filepath.Join(dataDir, "photos"), map[string]string{
		"42.jpg": "jpeg-bytes",
		"7.png":  "png-bytes",
	})

	if err := Backup(dbPath, backupDir, 30); err != nil {
		t.Fatalf("Backup: %v", err)
	}

	entries, err := ListBackups(backupDir)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(entries))
	}
	e := entries[0]
	if e.PhotosArchive != PhotoArchivePath(e.Name) {
		t.Fatalf("listing did not report photo archive: %+v", e)
	}
	if e.PhotosSize == 0 {
		t.Fatalf("listing reported zero photos size: %+v", e)
	}
	if _, err := os.Stat(filepath.Join(backupDir, e.PhotosArchive)); err != nil {
		t.Fatalf("photo archive missing: %v", err)
	}

	got := readArchive(t, filepath.Join(backupDir, e.PhotosArchive))
	if len(got) != 2 || got["42.jpg"] != "jpeg-bytes" || got["7.png"] != "png-bytes" {
		t.Fatalf("unexpected archive contents: %#v", got)
	}
}

func TestBackup_NoPhotosDirStillSucceeds(t *testing.T) {
	dataDir := t.TempDir()
	backupDir := filepath.Join(dataDir, "backups")
	dbPath := filepath.Join(dataDir, "datey.db")
	createTestDB(t, dbPath)

	// No photos directory at all.
	if err := Backup(dbPath, backupDir, 30); err != nil {
		t.Fatalf("Backup without photos dir: %v", err)
	}
	entries, err := ListBackups(backupDir)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(entries) != 1 || entries[0].PhotosArchive == "" {
		t.Fatalf("expected a backup with an empty photo archive, got %+v", entries)
	}
	if got := readArchive(t, filepath.Join(backupDir, entries[0].PhotosArchive)); len(got) != 0 {
		t.Fatalf("expected empty archive, got %#v", got)
	}
}

func TestBackupWeekly_CapturesPhotosArchive(t *testing.T) {
	dataDir := t.TempDir()
	backupDir := filepath.Join(dataDir, "backups")
	dbPath := filepath.Join(dataDir, "datey.db")
	createTestDB(t, dbPath)
	writePhotos(t, filepath.Join(dataDir, "photos"), map[string]string{"1.webp": "webp"})

	if err := BackupWeekly(dbPath, backupDir, 52); err != nil {
		t.Fatalf("BackupWeekly: %v", err)
	}
	entries, err := ListBackups(backupDir)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(entries) != 1 || entries[0].PhotosArchive == "" {
		t.Fatalf("expected weekly backup with photo archive, got %+v", entries)
	}
	if got := readArchive(t, filepath.Join(backupDir, entries[0].PhotosArchive)); got["1.webp"] != "webp" {
		t.Fatalf("unexpected archive contents: %#v", got)
	}
}

func TestCleanup_PrunesPhotoSidecarWithDatabase(t *testing.T) {
	dataDir := t.TempDir()
	backupDir := filepath.Join(dataDir, "backups")
	dbPath := filepath.Join(dataDir, "datey.db")
	createTestDB(t, dbPath)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir backups: %v", err)
	}

	oldDB := filepath.Join(backupDir, "datey_20200101_000000.db")
	oldPhotos := PhotoArchivePath(oldDB)
	for _, p := range []string{oldDB, oldPhotos} {
		if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
			t.Fatalf("write %s: %v", filepath.Base(p), err)
		}
	}
	oldTime := time.Now().AddDate(0, 0, -400)
	for _, p := range []string{oldDB, oldPhotos} {
		if err := os.Chtimes(p, oldTime, oldTime); err != nil {
			t.Fatalf("set old mtime: %v", err)
		}
	}

	if err := Backup(dbPath, backupDir, 30); err != nil {
		t.Fatalf("Backup: %v", err)
	}

	if _, err := os.Stat(oldDB); !os.IsNotExist(err) {
		t.Errorf("expected old database pruned, stat err=%v", err)
	}
	if _, err := os.Stat(oldPhotos); !os.IsNotExist(err) {
		t.Errorf("expected old photo sidecar pruned with its database, stat err=%v", err)
	}
	// The fresh backup keeps its own sidecar.
	entries, err := ListBackups(backupDir)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(entries) != 1 || entries[0].PhotosArchive == "" {
		t.Fatalf("fresh backup should keep a sidecar, got %+v", entries)
	}
}

func TestPhotoArchivePath_LeavesDatabaseNameAlone(t *testing.T) {
	if got := PhotoArchivePath("datey_20260101_120000.db"); got != "datey_20260101_120000-photos.tar.gz" {
		t.Errorf("PhotoArchivePath = %q", got)
	}
	if got := PhotoArchivePath("datey_weekly_20260101.db"); got != "datey_weekly_20260101-photos.tar.gz" {
		t.Errorf("PhotoArchivePath weekly = %q", got)
	}
	if got := PhotoArchivePath("notes.txt"); got != "" {
		t.Errorf("PhotoArchivePath(non-db) = %q, want empty", got)
	}
}
