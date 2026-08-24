package web

import (
	"context"
	"database/sql"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/db"
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/notifier"
	"github.com/go-chi/chi/v5"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

// newBackupRestoreTestHandler builds a handler whose DataDir and BackupDir
// point at isolated temp directories, mirroring the production admin-gated
// backup routes.
func newBackupRestoreTestHandler(t *testing.T) (*Handler, *ent.Client, chi.Router) {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_backup_restore?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	cfg := &config.Config{
		ReminderDays: 7,
		DataDir:      t.TempDir(),
		BackupDir:    t.TempDir(),
	}
	h := NewHandler(cfg, client, notifier.NewRegistry(), logstore.NewStore(100))

	router := chi.NewRouter()
	router.Use(h.CSRF)
	router.Group(func(r chi.Router) {
		r.Use(h.Admin)
		r.Post("/settings/backup/restore", h.settingsBackupRestore)
		r.Get("/settings/backup/download", h.settingsBackupDownload)
	})
	return h, client, router
}

func seedBackupRestoreUser(t *testing.T, h *Handler, username string, role user.Role) *ent.User {
	t.Helper()
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	u, err := h.client.User.Create().
		SetUsername(username).
		SetPasswordHash(string(hash)).
		SetRole(role).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(context.Background())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u
}

// createValidSQLiteFile writes a real SQLite database file at path.
func createValidSQLiteFile(t *testing.T, path string) {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := conn.Exec("CREATE TABLE IF NOT EXISTS t (id integer)"); err != nil {
		_ = conn.Close()
		t.Fatalf("init sqlite: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}
}

func postRestoreForm(router chi.Router, u *ent.User, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/settings/backup/restore", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	csrf := "test-csrf-restore"
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: csrf})
	req.Header.Set("X-CSRF-Token", csrf)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, u))
	req = req.WithContext(context.WithValue(req.Context(), "user", u)) //nolint:staticcheck
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func assertNothingStaged(t *testing.T, h *Handler) {
	t.Helper()
	if _, err := os.Stat(db.RestoreSentinelPath(h.cfg.DataDir)); !os.IsNotExist(err) {
		t.Errorf("sentinel should not exist, stat err = %v", err)
	}
	if _, err := os.Stat(db.RestoreStagedPath(h.cfg.DataDir)); !os.IsNotExist(err) {
		t.Errorf("staged db should not exist, stat err = %v", err)
	}
}

// Task 3.2: non-admin rejected without staging.
func TestBackupRestore_NonAdminRejected(t *testing.T) {
	h, _, router := newBackupRestoreTestHandler(t)
	regular := seedBackupRestoreUser(t, h, "plainuser", user.RoleUser)

	form := url.Values{}
	form.Set("confirm", "RESTORE")
	form.Set("source", "backup")
	form.Set("file", "whatever.db")
	w := postRestoreForm(router, regular, form)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d", w.Code)
	}
	assertNothingStaged(t, h)
}

// Task 3.2: missing confirmation re-renders with inline error and stages nothing.
func TestBackupRestore_MissingConfirmationRejected(t *testing.T) {
	h, _, router := newBackupRestoreTestHandler(t)
	admin := seedBackupRestoreUser(t, h, "backupadmin", user.RoleAdmin)

	createValidSQLiteFile(t, filepath.Join(h.cfg.BackupDir, "good.db"))

	form := url.Values{}
	form.Set("source", "backup")
	form.Set("file", "good.db")
	w := postRestoreForm(router, admin, form)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 re-render for missing confirmation, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Type RESTORE to confirm.") {
		t.Error("expected inline confirmation error in rendered page")
	}
	assertNothingStaged(t, h)

	// Wrong phrase is equally rejected.
	form.Set("confirm", "restore")
	w2 := postRestoreForm(router, admin, form)
	if w2.Code != http.StatusOK || !strings.Contains(w2.Body.String(), "Type RESTORE to confirm.") {
		t.Errorf("lowercase confirm must be rejected; code=%d", w2.Code)
	}
	assertNothingStaged(t, h)
}

// Task 3.2: valid backup from BACKUP_DIR stages sentinel + staged file.
func TestBackupRestore_ValidBackupStagesSentinel(t *testing.T) {
	h, _, router := newBackupRestoreTestHandler(t)
	admin := seedBackupRestoreUser(t, h, "backupadmin2", user.RoleAdmin)

	createValidSQLiteFile(t, filepath.Join(h.cfg.BackupDir, "nightly.db"))

	form := url.Values{}
	form.Set("confirm", "RESTORE")
	form.Set("source", "backup")
	form.Set("file", "nightly.db")
	w := postRestoreForm(router, admin, form)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect after staging, got %d body=%s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "/settings/backup") {
		t.Errorf("expected redirect to /settings/backup, got %q", loc)
	}

	pending, err := db.ReadRestorePending(h.cfg.DataDir)
	if err != nil || pending == nil {
		t.Fatalf("expected sentinel after valid restore, err=%v", err)
	}
	if pending.Filename != "nightly.db" {
		t.Errorf("sentinel filename = %q, want nightly.db", pending.Filename)
	}
	if _, err := os.Stat(db.RestoreStagedPath(h.cfg.DataDir)); err != nil {
		t.Errorf("staged db missing: %v", err)
	}
}

// Task 3.2: multipart upload path works end-to-end.
func TestBackupRestore_UploadPathWorks(t *testing.T) {
	h, _, router := newBackupRestoreTestHandler(t)
	admin := seedBackupRestoreUser(t, h, "backupadmin3", user.RoleAdmin)

	uploadPath := filepath.Join(t.TempDir(), "uploaded.db")
	createValidSQLiteFile(t, uploadPath)
	uploadBytes, err := os.ReadFile(uploadPath)
	if err != nil {
		t.Fatalf("read upload: %v", err)
	}

	var buf strings.Builder
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "uploaded.db")
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	if _, err := fw.Write(uploadBytes); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := mw.WriteField("confirm", "RESTORE"); err != nil {
		t.Fatalf("write field: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	req := httptest.NewRequest("POST", "/settings/backup/restore", strings.NewReader(buf.String()))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	csrf := "test-csrf-restore"
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: csrf})
	req.Header.Set("X-CSRF-Token", csrf)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, admin))
	req = req.WithContext(context.WithValue(req.Context(), "user", admin)) //nolint:staticcheck
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 after upload staging, got %d body=%s", w.Code, w.Body.String())
	}
	pending, err := db.ReadRestorePending(h.cfg.DataDir)
	if err != nil || pending == nil {
		t.Fatalf("expected sentinel after upload restore, err=%v", err)
	}
	if pending.Filename != "uploaded.db" {
		t.Errorf("sentinel filename = %q, want uploaded.db", pending.Filename)
	}
	staged, err := os.ReadFile(db.RestoreStagedPath(h.cfg.DataDir))
	if err != nil {
		t.Fatalf("staged db missing: %v", err)
	}
	if string(staged) != string(uploadBytes) {
		t.Error("staged content differs from uploaded content")
	}
}

// Task 3.4: path traversal in file= is rejected before anything is staged.
func TestBackupRestore_PathTraversalRejected(t *testing.T) {
	h, _, router := newBackupRestoreTestHandler(t)
	admin := seedBackupRestoreUser(t, h, "backupadmin4", user.RoleAdmin)

	for _, name := range []string{"../../etc/passwd", "..\\windows\\evil.db", "/etc/passwd", "sub/dir/x.db"} {
		form := url.Values{}
		form.Set("confirm", "RESTORE")
		form.Set("source", "backup")
		form.Set("file", name)
		w := postRestoreForm(router, admin, form)

		if w.Code != http.StatusOK {
			t.Errorf("file=%q: expected 200 re-render with error, got %d", name, w.Code)
		}
		if !strings.Contains(w.Body.String(), "Invalid filename.") {
			t.Errorf("file=%q: expected invalid-filename error in response", name)
		}
		assertNothingStaged(t, h)
	}
}

// Task 2.5: download serves the current database as an attachment.
func TestBackupRestore_DownloadCurrentDB(t *testing.T) {
	h, _, router := newBackupRestoreTestHandler(t)
	admin := seedBackupRestoreUser(t, h, "backupadmin5", user.RoleAdmin)

	dbPath := filepath.Join(h.cfg.DataDir, "datey.db")
	createValidSQLiteFile(t, dbPath)

	req := httptest.NewRequest("GET", "/settings/backup/download", nil)
	csrf := "test-csrf-restore"
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: csrf})
	req.Header.Set("X-CSRF-Token", csrf)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, admin))
	req = req.WithContext(context.WithValue(req.Context(), "user", admin)) //nolint:staticcheck
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 download, got %d", w.Code)
	}
	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") || !strings.Contains(cd, "datey.db") {
		t.Errorf("expected attachment Content-Disposition with datey.db, got %q", cd)
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat source db: %v", err)
	}
	if int64(w.Body.Len()) != info.Size() {
		t.Errorf("downloaded %d bytes, want %d", w.Body.Len(), info.Size())
	}
}
