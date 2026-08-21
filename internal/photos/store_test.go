package photos

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(t.TempDir())
}

func TestValidate_AcceptsImage(t *testing.T) {
	data, err := Validate("image/png", strings.NewReader("not really a png but non-empty"))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty data")
	}
}

func TestValidate_StripsContentTypeParams(t *testing.T) {
	if _, err := Validate("image/jpeg; charset=binary", strings.NewReader("x")); err != nil {
		t.Errorf("expected params to be stripped, got %v", err)
	}
}

func TestValidate_RejectsNonImage(t *testing.T) {
	for _, ct := range []string{"text/plain", "application/pdf", "image/", ""} {
		if _, err := Validate(ct, strings.NewReader("data")); err == nil {
			t.Errorf("content type %q: expected error", ct)
		}
	}
}

func TestValidate_RejectsEmpty(t *testing.T) {
	if _, err := Validate("image/png", strings.NewReader("")); err == nil {
		t.Error("expected error for empty body")
	}
}

func TestValidate_RejectsOversize(t *testing.T) {
	big := bytes.Repeat([]byte{0xff}, MaxSize+1)
	if _, err := Validate("image/png", bytes.NewReader(big)); err == nil {
		t.Error("expected error for oversize image")
	}
}

func TestSaveOpenRoundtrip(t *testing.T) {
	s := newTestStore(t)

	rel, err := s.Save(7, "image/png", []byte("png-bytes"))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if rel != "7.png" {
		t.Errorf("expected rel path 7.png, got %q", rel)
	}

	rc, size, err := s.Open(rel)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, []byte("png-bytes")) {
		t.Errorf("roundtrip mismatch: got %q", got)
	}
	if size != int64(len("png-bytes")) {
		t.Errorf("expected size %d, got %d", len("png-bytes"), size)
	}
}

func TestSave_ExtensionMapping(t *testing.T) {
	s := newTestStore(t)
	cases := map[string]string{
		"image/jpeg": ".jpg",
		"image/gif":  ".gif",
		"image/webp": ".webp",
		"image/avif": ".avif",
		"text/plain": ".img",
	}
	for ct, ext := range cases {
		rel, err := s.Save(1, ct, []byte("data"))
		if err != nil {
			t.Fatalf("Save(%s): %v", ct, err)
		}
		if filepath.Ext(rel) != ext {
			t.Errorf("content type %s: expected ext %s, got %q", ct, ext, rel)
		}
	}
}

func TestSave_OverwritesSamePerson(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Save(3, "image/png", []byte("first")); err != nil {
		t.Fatalf("Save first: %v", err)
	}
	rel, err := s.Save(3, "image/jpeg", []byte("second"))
	if err != nil {
		t.Fatalf("Save second: %v", err)
	}
	rc, _, err := s.Open(rel)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != "second" {
		t.Errorf("expected overwrite content 'second', got %q", got)
	}
}

func TestSave_RejectsEmptyAndOversize(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Save(1, "image/png", nil); err == nil {
		t.Error("expected error for empty data")
	}
	if _, err := s.Save(1, "image/png", bytes.Repeat([]byte{0}, MaxSize+1)); err == nil {
		t.Error("expected error for oversize data")
	}
}

func TestResolve_RejectsTraversal(t *testing.T) {
	s := newTestStore(t)
	// Anchoring strips any traversal: every path resolves inside the store.
	for _, p := range []string{"../escape.png", "../../etc/passwd"} {
		abs, err := s.Resolve(p)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", p, err)
		}
		if !strings.HasPrefix(abs, s.Dir()+string(os.PathSeparator)) {
			t.Errorf("path %q escaped store dir: %q", p, abs)
		}
	}
	// Absolute-looking paths are anchored inside the store, not escaped.
	abs, err := s.Resolve("/etc/passwd")
	if err != nil {
		t.Fatalf("Resolve anchored absolute path: %v", err)
	}
	if !strings.HasPrefix(abs, s.Dir()+string(os.PathSeparator)) {
		t.Errorf("absolute path escaped store dir: %q", abs)
	}
	abs2, err := s.Resolve("sub/3.png")
	if err != nil {
		t.Fatalf("Resolve valid: %v", err)
	}
	if !strings.HasPrefix(abs2, s.Dir()+string(os.PathSeparator)) {
		t.Errorf("resolved path escapes store dir: %q", abs2)
	}
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)
	rel, err := s.Save(9, "image/png", []byte("bye"))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Delete(rel); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := s.Open(rel); err == nil {
		t.Error("expected open after delete to fail")
	}
	// Deleting again is not an error.
	if err := s.Delete(rel); err != nil {
		t.Errorf("repeat Delete: %v", err)
	}
	if err := s.Delete(""); err != nil {
		t.Errorf("empty path Delete: %v", err)
	}
}

func TestDeletePerson_RemovesAllExtensions(t *testing.T) {
	s := newTestStore(t)
	for i, ct := range []string{"image/png", "image/jpeg", "text/plain"} {
		if _, err := s.Save(100+i, ct, []byte("x")); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	// Save under every extension for one person by writing directly.
	if err := os.MkdirAll(s.Dir(), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for _, ext := range []string{"200.jpg", "200.png", "200.img"} {
		if err := os.WriteFile(filepath.Join(s.Dir(), ext), []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	if err := s.DeletePerson(200); err != nil {
		t.Fatalf("DeletePerson: %v", err)
	}
	for _, ext := range []string{"200.jpg", "200.png", "200.img"} {
		if _, err := os.Stat(filepath.Join(s.Dir(), ext)); !os.IsNotExist(err) {
			t.Errorf("%s still exists after DeletePerson", ext)
		}
	}
}
