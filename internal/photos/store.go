// Package photos stores imported profile pictures as files under
// DATA_DIR/photos/. Files are named by person ID so filenames are never
// user-controlled, and writes are atomic (temp file + rename).
package photos

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MaxSize is the maximum accepted image size (5 MB).
const MaxSize = 5 << 20

// Store manages photo files for one data directory.
type Store struct {
	dir string
}

// NewStore returns a Store rooted at <dataDir>/photos. The directory is
// created lazily on first write.
func NewStore(dataDir string) *Store {
	return &Store{dir: filepath.Join(dataDir, "photos")}
}

// Dir returns the directory photos are stored in.
func (s *Store) Dir() string { return s.dir }

// extFor maps a content type to a file extension.
func extFor(contentType string) string {
	base := strings.TrimSpace(contentType)
	if i := strings.Index(base, ";"); i >= 0 {
		base = strings.TrimSpace(base[:i])
	}
	switch base {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/avif":
		return ".avif"
	default:
		return ".img"
	}
}

// Validate checks that content type is an image and the reader yields a
// non-empty body within MaxSize. It returns the bytes read so callers can
// avoid re-reading the source.
func Validate(contentType string, r io.Reader) ([]byte, error) {
	ct := strings.TrimSpace(contentType)
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	if !strings.HasPrefix(ct, "image/") || ct == "image/" {
		return nil, fmt.Errorf("unsupported content type %q: must be an image", contentType)
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxSize+1))
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("image is empty")
	}
	if len(data) > MaxSize {
		return nil, fmt.Errorf("image exceeds size limit of %d bytes", MaxSize)
	}
	return data, nil
}

// Save writes the image bytes for a person atomically and returns the
// path relative to the store directory (e.g. "42.jpg").
func (s *Store) Save(personID int, contentType string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("image is empty")
	}
	if len(data) > MaxSize {
		return "", fmt.Errorf("image exceeds size limit of %d bytes", MaxSize)
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return "", fmt.Errorf("create photos dir: %w", err)
	}
	name := fmt.Sprintf("%d%s", personID, extFor(contentType))
	final := filepath.Join(s.dir, name)
	tmp, err := os.CreateTemp(s.dir, ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, final); err != nil {
		return "", fmt.Errorf("rename temp file: %w", err)
	}
	tmpName = "" // renamed away; nothing left to clean up
	return name, nil
}

// Resolve maps a stored relative path to an absolute filesystem path,
// refusing anything outside the store directory.
func (s *Store) Resolve(relPath string) (string, error) {
	clean := filepath.Clean("/" + relPath) // anchor, strips ../
	abs := filepath.Join(s.dir, clean)
	if abs == s.dir || !strings.HasPrefix(abs, s.dir+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid photo path %q", relPath)
	}
	return abs, nil
}

// Open returns a reader over the stored photo and its size. Returns an error
// wrapping os.ErrNotExist when the file is missing.
func (s *Store) Open(relPath string) (io.ReadCloser, int64, error) {
	abs, err := s.Resolve(relPath)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, 0, fmt.Errorf("open photo: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, fmt.Errorf("stat photo: %w", err)
	}
	return f, info.Size(), nil
}

// Delete removes the stored photo file if it exists. Missing files are not
// an error.
func (s *Store) Delete(relPath string) error {
	if relPath == "" {
		return nil
	}
	abs, err := s.Resolve(relPath)
	if err != nil {
		return err
	}
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete photo: %w", err)
	}
	return nil
}

// DeletePerson removes any photo file stored for a person regardless of
// which extension was used. Missing files are not an error.
func (s *Store) DeletePerson(personID int) error {
	exts := []string{".jpg", ".png", ".gif", ".webp", ".avif", ".img"}
	var firstErr error
	for _, ext := range exts {
		if err := s.Delete(fmt.Sprintf("%d%s", personID, ext)); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
