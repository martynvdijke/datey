package db

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateCandidate_Valid(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "valid.db")
	createTestDB(t, p)
	if err := ValidateCandidate(p); err != nil {
		t.Fatalf("valid file should pass: %v", err)
	}
}

func TestValidateCandidate_EmptyRejected(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "empty.db")
	if err := os.WriteFile(p, []byte{}, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := ValidateCandidate(p); err == nil {
		t.Fatal("empty file should be rejected")
	}
}

func TestValidateCandidate_WrongHeaderRejected(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.db")
	if err := os.WriteFile(p, []byte("not a sqlite file content here"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := ValidateCandidate(p); err == nil {
		t.Fatal("garbage header should be rejected")
	}
}

func TestValidateCandidate_TruncatedHeaderRejected(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "trunc.db")
	if err := os.WriteFile(p, []byte("SQLite format"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := ValidateCandidate(p); err == nil {
		t.Fatal("truncated should be rejected")
	}
}

func TestApplyPendingRestore_ValidSwap(t *testing.T) {
	dataDir := t.TempDir()
	// Create original db
	orig := filepath.Join(dataDir, "datey.db")
	createTestDB(t, orig)
	// Create staged valid db with extra table
	staged := RestoreStagedPath(dataDir)
	conn, err := sql.Open("sqlite3", staged)
	if err != nil {
		t.Fatalf("open staged: %v", err)
	}
	if _, err := conn.Exec("CREATE TABLE foo (id integer)"); err != nil {
		t.Fatalf("create foo: %v", err)
	}
	_ = conn.Close()
	if err := WriteRestorePending(dataDir, "backup.db"); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	applyPendingRestore(dataDir)
	// Sentinel cleared
	if _, err := os.Stat(RestoreSentinelPath(dataDir)); !os.IsNotExist(err) {
		t.Fatalf("sentinel should be cleared, err %v", err)
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatalf("staged should be gone")
	}
	// Check swapped db has foo table
	c, err := sql.Open("sqlite3", orig)
	if err != nil {
		t.Fatalf("open orig: %v", err)
	}
	defer c.Close()
	var name string
	if err := c.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='foo'").Scan(&name); err != nil {
		t.Fatalf("expected foo table after swap: %v", err)
	}
}

func TestApplyPendingRestore_CorruptDiscarded(t *testing.T) {
	dataDir := t.TempDir()
	orig := filepath.Join(dataDir, "datey.db")
	createTestDB(t, orig)
	staged := RestoreStagedPath(dataDir)
	if err := os.WriteFile(staged, []byte("corrupt"), 0644); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}
	if err := WriteRestorePending(dataDir, "bad.db"); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	applyPendingRestore(dataDir)
	if _, err := os.Stat(RestoreSentinelPath(dataDir)); !os.IsNotExist(err) {
		t.Fatalf("sentinel should be cleared after corrupt")
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatalf("corrupt staged should be removed")
	}
	if _, err := os.Stat(orig); err != nil {
		t.Fatalf("original should remain: %v", err)
	}
}

// tarEntry describes one entry written into a test photo archive.
type tarEntry struct {
	name     string
	typeflag byte
	body     string
	linkname string
}

// writeTarGz writes a gzip tar archive with the given entries.
func writeTarGz(t *testing.T, dst string, entries []tarEntry) {
	t.Helper()
	f, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Typeflag: e.typeflag,
			Mode:     0o644,
			Linkname: e.linkname,
			Size:     int64(len(e.body)),
		}
		if e.typeflag == tar.TypeDir {
			hdr.Size = 0
			hdr.Mode = 0o755
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header %q: %v", e.name, err)
		}
		if hdr.Size > 0 {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatalf("write tar body %q: %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
}

// stageBackup simulates what the web handler does: copy a backup's database
// into the staged path and write the sentinel for it.
func stageBackup(t *testing.T, dataDir, candidatePath, name string) {
	t.Helper()
	if err := copyFile(candidatePath, RestoreStagedPath(dataDir)); err != nil {
		t.Fatalf("stage db: %v", err)
	}
	if err := WriteRestorePending(dataDir, name); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
}

func TestRestore_RoundTripPhotos(t *testing.T) {
	dataDir := t.TempDir()
	backupDir := filepath.Join(dataDir, "backups")
	dbPath := filepath.Join(dataDir, "datey.db")
	createTestDB(t, dbPath)

	want := map[string]string{
		"42.jpg": "jpeg-payload",
		"7.png":  "png-payload",
		"9.gif":  "gif-payload",
	}
	photosDir := filepath.Join(dataDir, "photos")
	writePhotos(t, photosDir, want)

	if err := Backup(dbPath, backupDir, 30); err != nil {
		t.Fatalf("Backup: %v", err)
	}
	entries, err := ListBackups(backupDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("ListBackups: entries=%v err=%v", entries, err)
	}
	if entries[0].PhotosArchive == "" {
		t.Fatalf("backup has no photo archive: %+v", entries[0])
	}

	// Photos are lost on the fresh host.
	if err := os.RemoveAll(photosDir); err != nil {
		t.Fatalf("remove photos: %v", err)
	}

	stageBackup(t, dataDir, filepath.Join(backupDir, entries[0].Name), entries[0].Name)
	pending, err := ReadRestorePending(dataDir)
	if err != nil || pending == nil {
		t.Fatalf("read sentinel: %v %v", pending, err)
	}
	if pending.PhotosStaged == "" {
		t.Fatalf("sentinel did not record staged photos: %+v", pending)
	}

	applyPendingRestore(dataDir)

	got, err := os.ReadDir(photosDir)
	if err != nil {
		t.Fatalf("read restored photos: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d restored photos, got %d", len(want), len(got))
	}
	for name, content := range want {
		b, err := os.ReadFile(filepath.Join(photosDir, name))
		if err != nil {
			t.Fatalf("read restored %s: %v", name, err)
		}
		if string(b) != content {
			t.Errorf("photo %s = %q, want %q", name, b, content)
		}
	}
	if _, err := os.Stat(RestoreSentinelPath(dataDir)); !os.IsNotExist(err) {
		t.Errorf("sentinel should be cleared, err=%v", err)
	}
	if _, err := os.Stat(RestoreStagedPhotosPath(dataDir)); !os.IsNotExist(err) {
		t.Errorf("staged photo archive should be removed, err=%v", err)
	}
	if _, err := os.Stat(photosDir + ".old"); !os.IsNotExist(err) {
		t.Errorf("photos.old should be gone, err=%v", err)
	}
	// No extraction droppings left in the data dir.
	leftovers, _ := filepath.Glob(filepath.Join(dataDir, restorePhotosTempPrefix+"*"))
	if len(leftovers) != 0 {
		t.Errorf("temp extraction dirs left behind: %v", leftovers)
	}
}

func TestRestore_LegacyBackupWithoutSidecar(t *testing.T) {
	dataDir := t.TempDir()
	backupDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir backups: %v", err)
	}
	createTestDB(t, filepath.Join(dataDir, "datey.db"))

	// A legacy backup: only a .db file, no sidecar.
	legacyDB := filepath.Join(backupDir, "datey_20250101_000000.db")
	src, err := sql.Open("sqlite3", legacyDB)
	if err != nil {
		t.Fatalf("open legacy: %v", err)
	}
	if _, err := src.Exec("CREATE TABLE legacy (id integer)"); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	_ = src.Close()

	photosDir := filepath.Join(dataDir, "photos")
	writePhotos(t, photosDir, map[string]string{"keep.jpg": "keep"})

	stageBackup(t, dataDir, legacyDB, filepath.Base(legacyDB))
	pending, err := ReadRestorePending(dataDir)
	if err != nil || pending == nil {
		t.Fatalf("read sentinel: %v %v", pending, err)
	}
	if pending.PhotosStaged != "" {
		t.Fatalf("legacy backup should not stage photos: %+v", pending)
	}

	applyPendingRestore(dataDir)

	// Database swapped in.
	c, err := sql.Open("sqlite3", filepath.Join(dataDir, "datey.db"))
	if err != nil {
		t.Fatalf("open restored db: %v", err)
	}
	defer func() { _ = c.Close() }()
	var name string
	if err := c.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='legacy'").Scan(&name); err != nil {
		t.Fatalf("expected legacy table after restore: %v", err)
	}
	// Photos untouched.
	b, err := os.ReadFile(filepath.Join(photosDir, "keep.jpg"))
	if err != nil || string(b) != "keep" {
		t.Fatalf("existing photos should be untouched: %q err=%v", b, err)
	}
}

func TestRestore_EmptyPhotosBackupRestoresDBOnly(t *testing.T) {
	dataDir := t.TempDir()
	backupDir := filepath.Join(dataDir, "backups")
	dbPath := filepath.Join(dataDir, "datey.db")
	createTestDB(t, dbPath)

	// No photos dir: backup captures an empty archive.
	if err := Backup(dbPath, backupDir, 30); err != nil {
		t.Fatalf("Backup: %v", err)
	}
	entries, err := ListBackups(backupDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("ListBackups: entries=%v err=%v", entries, err)
	}

	stageBackup(t, dataDir, filepath.Join(backupDir, entries[0].Name), entries[0].Name)
	applyPendingRestore(dataDir)

	if _, err := os.Stat(filepath.Join(dataDir, "datey.db")); err != nil {
		t.Fatalf("database should be restored: %v", err)
	}
	restored, err := os.ReadDir(filepath.Join(dataDir, "photos"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read photos: %v", err)
	}
	if len(restored) != 0 {
		t.Fatalf("expected no photos after DB-only restore, got %d", len(restored))
	}
}

func TestRestore_RejectsMaliciousPhotoArchive(t *testing.T) {
	absEscapeTarget := filepath.Join(os.TempDir(), "datey-evil-abs-test")
	_ = os.Remove(absEscapeTarget) // guard against leftovers from an earlier run

	cases := []struct {
		name    string
		entries []tarEntry
		outside string // path that must not be created, relative to dataDir's parent
	}{
		{
			name:    "parent traversal",
			entries: []tarEntry{{name: "../evil", typeflag: tar.TypeReg, body: "pwned"}},
			outside: "evil",
		},
		{
			name:    "nested traversal",
			entries: []tarEntry{{name: "sub/../../evil2", typeflag: tar.TypeReg, body: "pwned"}},
			outside: "evil2",
		},
		{
			name:    "absolute path",
			entries: []tarEntry{{name: absEscapeTarget, typeflag: tar.TypeReg, body: "pwned"}},
		},
		{
			name:    "symlink",
			entries: []tarEntry{{name: "link", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"}},
		},
		{
			name:    "hardlink",
			entries: []tarEntry{{name: "hard", typeflag: tar.TypeLink, linkname: "/etc/passwd"}},
		},
		{
			name: "valid then traversal",
			entries: []tarEntry{
				{name: "ok.jpg", typeflag: tar.TypeReg, body: "fine"},
				{name: "../../escape", typeflag: tar.TypeReg, body: "pwned"},
			},
			outside: "escape",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			createTestDB(t, filepath.Join(dataDir, "datey.db"))
			photosDir := filepath.Join(dataDir, "photos")
			writePhotos(t, photosDir, map[string]string{"keep.jpg": "keep"})

			// Staged database that would be swapped in.
			stagedDB := RestoreStagedPath(dataDir)
			conn, err := sql.Open("sqlite3", stagedDB)
			if err != nil {
				t.Fatalf("open staged: %v", err)
			}
			if _, err := conn.Exec("CREATE TABLE foo (id integer)"); err != nil {
				t.Fatalf("create foo: %v", err)
			}
			_ = conn.Close()

			writeTarGz(t, RestoreStagedPhotosPath(dataDir), tc.entries)
			// Model a hostile staged state: archive plus the sentinel that
			// references it.
			rp := RestorePending{Filename: "backup.db", StagedAt: time.Now().UTC(), PhotosStaged: restoreStagedPhotosName}
			b, err := json.Marshal(rp)
			if err != nil {
				t.Fatalf("marshal sentinel: %v", err)
			}
			if err := os.WriteFile(RestoreSentinelPath(dataDir), b, 0o644); err != nil {
				t.Fatalf("write sentinel: %v", err)
			}

			applyPendingRestore(dataDir)

			if tc.outside != "" {
				if _, err := os.Stat(filepath.Join(filepath.Dir(dataDir), tc.outside)); !os.IsNotExist(err) {
					t.Errorf("archive escaped the target: %s exists (err=%v)", tc.outside, err)
				}
			}
			if _, err := os.Stat(absEscapeTarget); !os.IsNotExist(err) {
				t.Errorf("absolute entry escaped the target (err=%v)", err)
			}
			if _, err := os.Lstat(filepath.Join(photosDir, "link")); !os.IsNotExist(err) {
				t.Errorf("symlink entry was installed (err=%v)", err)
			}

			// Existing photos survive; the database restore still proceeds.
			got, err := os.ReadDir(photosDir)
			if err != nil {
				t.Fatalf("read photos: %v", err)
			}
			if len(got) != 1 || got[0].Name() != "keep.jpg" {
				t.Errorf("existing photos should be untouched, got %v", got)
			}
			b2, err := os.ReadFile(filepath.Join(photosDir, "keep.jpg"))
			if err != nil || string(b2) != "keep" {
				t.Errorf("existing photo content changed: %q err=%v", b2, err)
			}

			if _, err := os.Stat(RestoreSentinelPath(dataDir)); !os.IsNotExist(err) {
				t.Errorf("sentinel should be cleared (err=%v)", err)
			}
			if _, err := os.Stat(RestoreStagedPhotosPath(dataDir)); !os.IsNotExist(err) {
				t.Errorf("staged archive should be removed (err=%v)", err)
			}
			leftovers, _ := filepath.Glob(filepath.Join(dataDir, restorePhotosTempPrefix+"*"))
			if len(leftovers) != 0 {
				t.Errorf("temp extraction dirs left behind: %v", leftovers)
			}

			// The database restore itself still happened.
			c, err := sql.Open("sqlite3", filepath.Join(dataDir, "datey.db"))
			if err != nil {
				t.Fatalf("open db: %v", err)
			}
			defer func() { _ = c.Close() }()
			var name string
			if err := c.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='foo'").Scan(&name); err != nil {
				t.Errorf("expected database restore to proceed: %v", err)
			}
		})
	}
}
