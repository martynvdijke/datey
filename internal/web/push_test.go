package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/internal/notifier"
	"github.com/go-chi/chi/v5"
)

func setupPushRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/push/vapid-public-key", h.pushVAPIDPublicKey)
	r.Post("/push/subscribe", h.pushSubscribe)
	r.Post("/push/unsubscribe", h.pushUnsubscribe)
	return r
}

func enableWebPush(h *Handler) {
	h.cfg.PushEnabled = true
	h.cfg.PushVAPIDPublicKey = "test-public-key"
	h.cfg.PushVAPIDPrivateKey = "test-private-key"
	h.notifReg.Register(notifier.NewWebPushNotifier(h.cfg, h.pushSubs))
}

func seedPushTestUser(t *testing.T, h *Handler) {
	t.Helper()
	ctx := context.Background()
	_, err := h.client.User.Create().
		SetUsername("testuser").
		SetPasswordHash("hash").
		SetRole(user.RoleUser).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func TestPushVAPIDPublicKey_Unconfigured(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPushRouter(h)

	req := httptest.NewRequest("GET", "/push/vapid-public-key", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when unconfigured, got %d", w.Code)
	}
}

func TestPushVAPIDPublicKey_Configured(t *testing.T) {
	h := newTestWebHandler(t)
	enableWebPush(h)
	router := setupPushRouter(h)

	req := httptest.NewRequest("GET", "/push/vapid-public-key", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["public_key"] != "test-public-key" {
		t.Errorf("public_key = %q, want %q", body["public_key"], "test-public-key")
	}
}

func TestPushSubscribe_Unauthenticated(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPushRouter(h)

	req := httptest.NewRequest("POST", "/push/subscribe", strings.NewReader(`{"endpoint":"https://push.example/x","p256dh":"a","auth":"b"}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without user, got %d", w.Code)
	}
}

func TestPushSubscribe_InvalidJSON(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPushRouter(h)

	req := httptest.NewRequest("POST", "/push/subscribe", strings.NewReader("not-json"))
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestPushSubscribe_EmptyFields(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPushRouter(h)

	req := httptest.NewRequest("POST", "/push/subscribe", strings.NewReader(`{"endpoint":"","p256dh":"","auth":""}`))
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty fields, got %d", w.Code)
	}
}

func TestPushSubscribe_Success(t *testing.T) {
	h := newTestWebHandler(t)
	seedPushTestUser(t, h)
	router := setupPushRouter(h)

	body := `{"endpoint":"https://push.example/sub","p256dh":"abc","auth":"def"}`
	req := httptest.NewRequest("POST", "/push/subscribe", bytes.NewBufferString(body))
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (body: %s)", w.Code, w.Body.String())
	}
	count, err := h.pushSubs.Count(req.Context())
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("subscription count = %d, want 1", count)
	}
}

func TestPushUnsubscribe_EmptyEndpoint(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPushRouter(h)

	req := httptest.NewRequest("POST", "/push/unsubscribe", strings.NewReader(`{"endpoint":""}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty endpoint, got %d", w.Code)
	}
}

func TestPushUnsubscribe_Success(t *testing.T) {
	h := newTestWebHandler(t)
	seedPushTestUser(t, h)
	ctx := context.Background()
	if _, err := h.pushSubs.Upsert(ctx, 1, "https://push.example/sub", "abc", "def"); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	router := setupPushRouter(h)

	req := httptest.NewRequest("POST", "/push/unsubscribe", strings.NewReader(`{"endpoint":"https://push.example/sub"}`))
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	count, err := h.pushSubs.Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("subscription count = %d, want 0", count)
	}
}
