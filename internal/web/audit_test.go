package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/internal/auditlog"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/repository"
	"github.com/go-chi/chi/v5"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

// newAuditTestHandler creates a handler with audit recorder attached.
func newAuditTestHandler(t *testing.T) (*Handler, *ent.Client) {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_audit_web?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	cfg := &config.Config{ReminderDays: 7}
	reg := notifier.NewRegistry()
	store := logstore.NewStore(100)
	h := NewHandler(cfg, client, reg, store)
	h.SetAuditRecorder(auditlog.NewFromClient(client))
	return h, client
}

func seedAdminUser(t *testing.T, h *Handler, username string) *ent.User {
	t.Helper()
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	u, err := h.client.User.Create().
		SetUsername(username).
		SetPasswordHash(string(hash)).
		SetRole(user.RoleAdmin).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(context.Background())
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	return u
}

func seedRegularUser(t *testing.T, h *Handler, username string) *ent.User {
	t.Helper()
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	u, err := h.client.User.Create().
		SetUsername(username).
		SetPasswordHash(string(hash)).
		SetRole(user.RoleUser).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(context.Background())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u
}

// failingRepo wraps a real repo but fails
type failingAuditRepo struct{}

func (f *failingAuditRepo) Append(ctx context.Context, e *ent.AuditEntry) (*ent.AuditEntry, error) {
	return nil, errFail
}

var errFail = &testError{"injected audit failure"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

// failingAuditRecorder delegates to failing repo via auditlog.Recorder
type injectedFailRecorder struct {
	inner *auditlog.Recorder
}

func newInjectedFailRecorder() *injectedFailRecorder {
	return &injectedFailRecorder{inner: auditlog.New(&failingAuditRepo{})}
}

func (r *injectedFailRecorder) Record(req *http.Request, action, target string) {
	rinner := r.inner
	if rinner != nil {
		rinner.Record(req, action, target)
	}
}
func (r *injectedFailRecorder) RecordWithActor(ctx context.Context, actor, ip, action, target string) {
	if r.inner != nil {
		r.inner.RecordWithActor(ctx, actor, ip, action, target)
	}
}

// Test 5.2: deleting a person creates audit entry with actor and target
func TestAuditHandler_PersonDeleteCreatesEntry(t *testing.T) {
	h, client := newAuditTestHandler(t)
	admin := seedAdminUser(t, h, "auditadmin")
	person, err := h.people.Create(context.Background(), "DeleteMe", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	// Call handler directly with user context, bypassing Auth cookie check
	req := httptest.NewRequest("POST", "/people/"+itoa(person.ID)+"/delete", nil)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, admin))
	req = req.WithContext(context.WithValue(req.Context(), "user", admin)) //nolint:staticcheck
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", itoa(person.ID))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.deletePerson(w, req)

	if w.Code != http.StatusSeeOther && w.Code != http.StatusOK {
		t.Fatalf("expected redirect after delete, got %d body %s", w.Code, w.Body.String())
	}
	// Verify audit entry
	entries, err := client.AuditEntry.Query().All(context.Background())
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == "person.delete" && e.Target == itoa(person.ID) && e.ActorUsername == "auditadmin" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected audit entry person.delete target %d actor auditadmin, got %v", person.ID, entries)
	}
}

// Test 5.2: failed login creates auth.login_failure with attempted username
func TestAuditHandler_FailedLoginCreatesEntry(t *testing.T) {
	h, _ := newAuditTestHandler(t)
	seedAdminUser(t, h, "realuser")
	router := chi.NewRouter()
	router.Use(h.CSRF)
	router.Post("/login", h.loginPost)

	form := url.Values{}
	form.Set("username", "realuser")
	form.Set("password", "wrongpassword")
	csrf := "test-csrf-login"
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: csrf})
	req.Header.Set("X-CSRF-Token", csrf)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should render login page with error (200)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for failed login, got %d", w.Code)
	}
	// Query audit
	repo := repository.NewAuditEntryRepository(h.client)
	entries, _ := repo.List(context.Background(), repository.AuditFilter{Action: "auth.login_failure"})
	if len(entries) == 0 {
		t.Fatal("expected auth.login_failure entry")
	}
	found := false
	for _, e := range entries {
		if e.Target == "realuser" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected attempted username in target, got %#v", entries[0])
	}
	// Also test non-existent user
	form2 := url.Values{}
	form2.Set("username", "nosuchuser")
	form2.Set("password", "whatever123")
	req2 := httptest.NewRequest("POST", "/login", strings.NewReader(form2.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.AddCookie(&http.Cookie{Name: "csrf_token", Value: csrf})
	req2.Header.Set("X-CSRF-Token", csrf)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	entries2, _ := repo.List(context.Background(), repository.AuditFilter{Action: "auth.login_failure"})
	hasNosuch := false
	for _, e := range entries2 {
		if e.Target == "nosuchuser" {
			hasNosuch = true
		}
	}
	if !hasNosuch {
		t.Error("expected audit for nonexistent user login failure")
	}
}

func TestAuditViewer_NonAdminRejected(t *testing.T) {
	h, _ := newAuditTestHandler(t)
	regular := seedRegularUser(t, h, "regular")
	// Test Admin middleware directly with user context
	handler := h.Admin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/settings/audit", nil)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, regular))
	req = req.WithContext(context.WithValue(req.Context(), "user", regular)) //nolint:staticcheck
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d", w.Code)
	}
}

func TestAuditViewer_FilterNarrowsResults(t *testing.T) {
	h, client := newAuditTestHandler(t)
	admin := seedAdminUser(t, h, "admin1")
	ctx := context.Background()
	repo := repository.NewAuditEntryRepository(client)
	_, _ = repo.Append(ctx, &ent.AuditEntry{CreatedAt: time.Now(), ActorUsername: "admin1", Action: "person.delete", Target: "1", SourceIP: "1.1.1.1"})
	_, _ = repo.Append(ctx, &ent.AuditEntry{CreatedAt: time.Now(), ActorUsername: "admin1", Action: "user.create", Target: "2", SourceIP: "1.1.1.1"})
	_, _ = repo.Append(ctx, &ent.AuditEntry{CreatedAt: time.Now(), ActorUsername: "admin1", Action: "person.delete", Target: "3", SourceIP: "1.1.1.1"})

	req := httptest.NewRequest("GET", "/settings/audit?action=person.delete", nil)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, admin))
	req = req.WithContext(context.WithValue(req.Context(), "user", admin)) //nolint:staticcheck
	w := httptest.NewRecorder()
	h.auditLog(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "person.delete") {
		t.Error("expected filtered results to contain person.delete")
	}
	if strings.Contains(body, "user.create") {
		t.Error("filtered results should not contain user.create")
	}
}

func TestAuditViewer_EmptyState(t *testing.T) {
	h, _ := newAuditTestHandler(t)
	admin := seedAdminUser(t, h, "adminEmpty")
	req := httptest.NewRequest("GET", "/settings/audit", nil)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, admin))
	req = req.WithContext(context.WithValue(req.Context(), "user", admin)) //nolint:staticcheck
	w := httptest.NewRecorder()
	h.auditLog(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "No audit entries") {
		t.Error("expected empty-state copy")
	}
}

func TestAuditFailureInjection_DoesNotBreakMutation(t *testing.T) {
	h, _ := newAuditTestHandler(t)
	h.SetAuditRecorder(newInjectedFailRecorder())
	admin := seedAdminUser(t, h, "adminFail")
	person, err := h.people.Create(context.Background(), "WillDelete", "", "")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	req := httptest.NewRequest("POST", "/people/"+itoa(person.ID)+"/delete", nil)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, admin))
	req = req.WithContext(context.WithValue(req.Context(), "user", admin)) //nolint:staticcheck
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", itoa(person.ID))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.deletePerson(w, req)

	if w.Code != http.StatusSeeOther && w.Code != http.StatusOK {
		t.Fatalf("expected success despite audit failure, got %d body %s", w.Code, w.Body.String())
	}
	_, err = h.people.Get(context.Background(), person.ID)
	if err == nil {
		t.Error("expected person to be deleted even when audit fails")
	}
}
