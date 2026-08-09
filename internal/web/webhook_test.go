package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/notifier"
	_ "github.com/mattn/go-sqlite3"
)

func TestSettingsTestWebhook_Success(t *testing.T) {
	var received []byte
	var gotSig string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		gotSig = r.Header.Get("X-Datey-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := enttest.Open(t, dialect.SQLite, "file:test_webhook_success?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })

	cfg := &config.Config{ReminderDays: 7, WebhookURL: server.URL, WebhookSecret: "s3cret"}
	reg := notifier.NewRegistry()
	reg.Register(notifier.NewWebhookNotifier(cfg))
	store := logstore.NewStore(100)
	h := NewHandler(cfg, client, reg, store)
	router := setupSettingsTestRouter(h)

	req := httptest.NewRequest("POST", "/settings/test/webhook", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Test sent!") {
		t.Errorf("expected success message, got: %s", w.Body.String())
	}

	var envelope map[string]any
	if err := json.Unmarshal(received, &envelope); err != nil {
		t.Fatalf("webhook server did not receive valid JSON: %v", err)
	}
	if envelope["channel"] != "webhook" {
		t.Errorf("expected channel %q, got %v", "webhook", envelope["channel"])
	}
	if envelope["title"] != "Datey Test Notification" {
		t.Errorf("expected title %q, got %v", "Datey Test Notification", envelope["title"])
	}
	if gotSig == "" || !strings.HasPrefix(gotSig, "sha256=") {
		t.Errorf("expected X-Datey-Signature header, got %q", gotSig)
	}
}

func TestSettingsTestWebhook_Unconfigured(t *testing.T) {
	mock := &mockNotifier{name: "webhook", configured: false}
	h := newTestNotificationsHandlerWithMock(t, mock)
	router := setupSettingsTestRouter(h)

	req := httptest.NewRequest("POST", "/settings/test/webhook", nil)
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

func TestSettingsTestWebhook_UnknownChannel(t *testing.T) {
	mock := &mockNotifier{name: "webhook", configured: true}
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

func TestCreateNotification_WithWebhookChannel(t *testing.T) {
	mock := &mockNotifier{name: "webhook", configured: true}
	h := newTestNotificationsHandlerWithMock(t, mock)
	router := setupNotificationsRouter(h)

	future := time.Now().Add(24 * time.Hour).Format("2006-01-02T15:04")
	body := "message=webhook+channel+test&scheduled_at=" + future + "&channels=webhook"
	req := httptest.NewRequest("POST", "/notifications/new", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", w.Code)
	}

	notifs, err := h.oneTimeNots.List(withUserContext(context.Background()))
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(notifs) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifs))
	}
	if len(notifs[0].Edges.Deliveries) != 1 {
		t.Fatalf("expected 1 delivery record, got %d", len(notifs[0].Edges.Deliveries))
	}
	if got := notifs[0].Edges.Deliveries[0].Channel; got != "webhook" {
		t.Errorf("expected delivery channel 'webhook', got '%s'", got)
	}
}
