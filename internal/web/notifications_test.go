package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/notifier"
	"github.com/go-chi/chi/v5"
	_ "github.com/mattn/go-sqlite3"
)

func newTestNotificationsHandler(t *testing.T) *Handler {
	t.Helper()

	client := enttest.Open(t, dialect.SQLite, "file:test_notif_handler?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })

	cfg := &config.Config{
		ReminderDays: 7,
	}
	reg := notifier.NewRegistry()
	store := logstore.NewStore(100)

	return NewHandler(cfg, client, reg, store)
}

func withUserContext(ctx context.Context) context.Context {
	u := &ent.User{
		ID:      1,
		Username: "testuser",
		Role:    user.RoleAdmin,
	}
	return context.WithValue(ctx, userContextKey, u)
}

func setupNotificationsRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/notifications", h.notificationsList)
	r.Get("/notifications/new", h.newNotificationForm)
	r.Post("/notifications/new", h.createNotification)
	r.Post("/notifications/{id}/delete", h.deleteNotification)
	r.Get("/api/notifications", h.apiNotifications)
	return r
}

func TestNotificationsList_Empty(t *testing.T) {
	h := newTestNotificationsHandler(t)
	router := setupNotificationsRouter(h)

	req := httptest.NewRequest("GET", "/notifications", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "One-Time Notifications") {
		t.Errorf("expected page title, got body: %s", w.Body.String()[:200])
	}
}

func TestNotificationsNewForm(t *testing.T) {
	h := newTestNotificationsHandler(t)
	router := setupNotificationsRouter(h)

	req := httptest.NewRequest("GET", "/notifications/new", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Create One-Time Notification") {
		t.Errorf("expected form title, got: %s", w.Body.String()[:200])
	}
}

func TestCreateNotification_Success(t *testing.T) {
	h := newTestNotificationsHandler(t)
	router := setupNotificationsRouter(h)

	future := time.Now().Add(24 * time.Hour).Format("2006-01-02T15:04")
	body := "message=Test+notification&scheduled_at=" + future
	req := httptest.NewRequest("POST", "/notifications/new", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/notifications" {
		t.Errorf("expected redirect to /notifications, got %s", loc)
	}
}

func TestCreateNotification_MissingMessage(t *testing.T) {
	h := newTestNotificationsHandler(t)
	router := setupNotificationsRouter(h)

	future := time.Now().Add(24 * time.Hour).Format("2006-01-02T15:04")
	body := "message=&scheduled_at=" + future
	req := httptest.NewRequest("POST", "/notifications/new", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with validation error, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Message is required") {
		t.Errorf("expected validation error message, got: %s", w.Body.String()[:500])
	}
}

func TestCreateNotification_PastTime(t *testing.T) {
	h := newTestNotificationsHandler(t)
	router := setupNotificationsRouter(h)

	past := time.Now().Add(-1 * time.Hour).Format("2006-01-02T15:04")
	body := "message=Test&scheduled_at=" + past
	req := httptest.NewRequest("POST", "/notifications/new", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with validation error, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "must be in the future") {
		t.Errorf("expected future validation error, got: %s", w.Body.String()[:500])
	}
}

func TestDeleteNotification(t *testing.T) {
	h := newTestNotificationsHandler(t)
	router := setupNotificationsRouter(h)

	// First create a notification
	future := time.Now().Add(24 * time.Hour)
	n, err := h.oneTimeNots.Create(withUserContext(context.Background()), "to delete", future)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Delete it
	req := httptest.NewRequest("POST", "/notifications/"+itoa(n.ID)+"/delete", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Verify it's gone
	notifs, err := h.oneTimeNots.List(withUserContext(context.Background()))
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	for _, existing := range notifs {
		if existing.ID == n.ID {
			t.Errorf("notification %d should have been deleted", n.ID)
		}
	}
}

func TestAPINotifications(t *testing.T) {
	h := newTestNotificationsHandler(t)
	router := setupNotificationsRouter(h)

	future := time.Now().Add(24 * time.Hour)
	_, err := h.oneTimeNots.Create(withUserContext(context.Background()), "api test", future)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/notifications", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(result))
	}
	if result[0]["message"] != "api test" {
		t.Errorf("expected message 'api test', got '%v'", result[0]["message"])
	}
	if result[0]["status"] != "pending" {
		t.Errorf("expected status 'pending', got '%v'", result[0]["status"])
	}
}

// itoa is a simple int to string conversion to avoid importing strconv in tests
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
