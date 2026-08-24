package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
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
