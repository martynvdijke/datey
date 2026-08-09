package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/notifier"
	"github.com/go-chi/chi/v5"
	_ "github.com/mattn/go-sqlite3"
)

// setupSettingsTestRouter mounts the settings test-notification route only.
func setupSettingsTestRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Post("/settings/test/{channel}", h.testNotification)
	return r
}

func TestSettingsTestNtfy_Success(t *testing.T) {
	var received []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := enttest.Open(t, dialect.SQLite, "file:test_ntfy_success?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })

	cfg := &config.Config{ReminderDays: 7, NtfyURL: server.URL, NtfyTopic: "reminders"}
	reg := notifier.NewRegistry()
	reg.Register(notifier.NewNtfyNotifier(cfg))
	store := logstore.NewStore(100)
	h := NewHandler(cfg, client, reg, store)
	router := setupSettingsTestRouter(h)

	req := httptest.NewRequest("POST", "/settings/test/ntfy", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Test sent!") {
		t.Errorf("expected success message, got: %s", w.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(received, &payload); err != nil {
		t.Fatalf("ntfy server did not receive valid JSON: %v", err)
	}
	if payload["topic"] != "reminders" {
		t.Errorf("expected topic %q, got %v", "reminders", payload["topic"])
	}
	if payload["title"] != "Datey Test Notification" {
		t.Errorf("expected title %q, got %v", "Datey Test Notification", payload["title"])
	}
}

func TestSettingsTestNtfy_Unconfigured(t *testing.T) {
	mock := &mockNotifier{name: "ntfy", configured: false}
	h := newTestNotificationsHandlerWithMock(t, mock)
	router := setupSettingsTestRouter(h)

	req := httptest.NewRequest("POST", "/settings/test/ntfy", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if len(mock.sent) != 0 {
		t.Errorf("expected no sent messages, got %d", len(mock.sent))
	}
}

func TestSettingsTestNtfy_UnknownChannel(t *testing.T) {
	mock := &mockNotifier{name: "ntfy", configured: true}
	h := newTestNotificationsHandlerWithMock(t, mock)
	router := setupSettingsTestRouter(h)

	req := httptest.NewRequest("POST", "/settings/test/nope", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown channel, got %d", w.Code)
	}
}

// silence unused-import guard for context in some build modes
var _ = context.Background
