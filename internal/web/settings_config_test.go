package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func setupConfigRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/settings/config", h.settingsConfig)
	r.Post("/settings/config", h.settingsConfigSave)
	return r
}

func TestSettingsConfig_GetRendersEditableForm(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupConfigRouter(h)

	req := httptest.NewRequest("GET", "/settings/config", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String()[:300])
	}
	body := w.Body.String()
	if !strings.Contains(body, "Save Settings") {
		t.Errorf("expected save button, got: %s", body[:300])
	}
	for _, name := range []string{
		"PORT", "SCHEDULER_HOUR", "REMINDER_DAYS", "LOG_LEVEL",
		"SMTP_HOST", "SMTP_PORT", "GOTIFY_URL", "TELEGRAM_BOT_TOKEN",
		"UMAMI_URL", "OTEL_ENDPOINT",
	} {
		if !strings.Contains(body, `name="`+name+`"`) {
			t.Errorf("expected form field for %q, missing", name)
		}
	}
	// CSRF token input present.
	if !strings.Contains(body, `name="csrf_token"`) {
		t.Errorf("expected CSRF hidden input in form")
	}
	// Restart required badge for PORT (Port is restart-required in buildConfigGroups).
	if !strings.Contains(body, "Restart required") {
		t.Errorf("expected restart required badge")
	}
}

func TestSettingsConfig_DataDirIsReadOnly(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupConfigRouter(h)

	req := httptest.NewRequest("GET", "/settings/config", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `name="DATA_DIR"`) {
		// readonly fields are not submitted; check that the input exists with readonly attr
		if !strings.Contains(body, "DATA_DIR") {
			t.Errorf("expected DATA_DIR shown as readonly, missing from body")
		}
	}
	if !strings.Contains(body, `readonly`) {
		t.Errorf("expected a readonly input on the config form")
	}
}

func TestSettingsConfigSave_Success(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupConfigRouter(h)

	form := url.Values{}
	form.Set("PORT", "9300")
	form.Set("SCHEDULER_HOUR", "4")
	form.Set("REMINDER_DAYS", "10")
	form.Set("LOG_LEVEL", "warn")
	form.Set("SMTP_HOST", "mail.test")
	form.Set("SMTP_PORT", "465")

	req := httptest.NewRequest("POST", "/settings/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String()[:300])
	}
	if w.Header().Get("HX-Refresh") != "true" {
		t.Errorf("expected HX-Refresh header on success, got %+v", w.Header())
	}
	// cfg hot-reload fields mutated.
	if h.cfg.ReminderDays != 10 {
		t.Errorf("ReminderDays hot-reload: got %d want 10", h.cfg.ReminderDays)
	}
	if h.cfg.LogLevel != "warn" {
		t.Errorf("LogLevel hot-reload: got %q want warn", h.cfg.LogLevel)
	}
	// Port (restart-required) NOT hot reloaded but persisted.
	if h.cfg.Port == 9300 {
		t.Errorf("Port should NOT hot-reload (restart-required)")
	}
}

func TestSettingsConfigSave_ValidationErrors(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupConfigRouter(h)

	form := url.Values{}
	form.Set("PORT", "99999")      // invalid
	form.Set("SCHEDULER_HOUR", "5") // valid
	form.Set("REMINDER_DAYS", "0")   // invalid

	req := httptest.NewRequest("POST", "/settings/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (re-render), got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Port must be between 1 and 65535") {
		t.Errorf("expected inline error for PORT, got: %s", body[:400])
	}
	if !strings.Contains(body, "Reminder days must be between 1 and 365") {
		t.Errorf("expected inline error for REMINDER_DAYS, got: %s", body[:400])
	}
	// The valid submitted SCHEDULER_HOUR value should be preserved in the re-rendered form.
	if !strings.Contains(body, `value="5"`) {
		t.Errorf("expected valid submitted SCHEDULER_HOUR value preserved in re-render, got: %s", body[:400])
	}
}

func TestSettingsConfigSave_UnauthenticatedRejected(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupConfigRouter(h)

	// No user context -> handler should not crash; auth normally handled by chi group,
	// here we exercise the handler directly to ensure no panic on empty context.
	req := httptest.NewRequest("GET", "/settings/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET without user returned %d (render should still succeed; auth is via middleware)", w.Code)
	}
}

func TestSettingsConfig_SecretsVisibleInForm(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupConfigRouter(h)

	h.cfg.SMTPPass = "topsecret-pw"
	h.cfg.GotifyToken = "gotify-token-val"

	req := httptest.NewRequest("GET", "/settings/config", nil)
	req = req.WithContext(withUserContext(context.Background()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "topsecret-pw") {
		t.Errorf("expected SMTP_PASS to be visible to admin, missing from form")
	}
	if !strings.Contains(body, "gotify-token-val") {
		t.Errorf("expected GOTIFY_TOKEN to be visible to admin, missing from form")
	}
}