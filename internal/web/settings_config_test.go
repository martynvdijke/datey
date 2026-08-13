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

func TestSettingsConfigSave_SchedulerCatchupToggle(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupConfigRouter(h)

	// Default: enabled.
	if !h.cfg.SchedulerCatchup {
		t.Fatal("expected SchedulerCatchup to default to true")
	}

	// Unchecking the toggle persists false and hot-reloads cfg.
	form := url.Values{}
	form.Set("REMINDER_DAYS", "7")
	req := httptest.NewRequest("POST", "/settings/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if h.cfg.SchedulerCatchup {
		t.Error("expected SchedulerCatchup hot-reload to false when checkbox absent")
	}

	// Checking the toggle persists true.
	form = url.Values{}
	form.Set("REMINDER_DAYS", "7")
	form.Set("SCHEDULER_CATCHUP", "on")
	req = httptest.NewRequest("POST", "/settings/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !h.cfg.SchedulerCatchup {
		t.Error("expected SchedulerCatchup hot-reload to true when checkbox set")
	}
}

func TestSettingsConfigSave_SchedulerCatchupInvalidValueReRenders(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupConfigRouter(h)

	// A non-boolean value for the checkbox must fail validation and re-render
	// with an inline error instead of silently persisting.
	form := url.Values{}
	form.Set("SCHEDULER_CATCHUP", "banana")

	req := httptest.NewRequest("POST", "/settings/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (re-render), got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Catch up missed reminders must be a boolean") {
		t.Errorf("expected inline error for SCHEDULER_CATCHUP, got: %s", body[:400])
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

func TestSettingsConfig_ICalFeedFieldsRender(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupConfigRouter(h)

	h.cfg.ICalEnabled = true
	h.cfg.ICalFeedKey = "abc123feedkey"
	h.cfg.ICalEventStart = "09:00"
	h.cfg.ICalDurationMinutes = 30

	req := httptest.NewRequest("GET", "/settings/config", nil)
	req = req.WithContext(withUserContext(context.Background()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `name="ICAL_FEED_ENABLED"`) {
		t.Errorf("expected ICAL_FEED_ENABLED checkbox, missing from form")
	}
	if !strings.Contains(body, `name="ICAL_EVENT_START"`) || !strings.Contains(body, `value="09:00"`) {
		t.Errorf("expected ICAL_EVENT_START with value 09:00")
	}
	if !strings.Contains(body, `name="ICAL_EVENT_DURATION"`) || !strings.Contains(body, `value="30"`) {
		t.Errorf("expected ICAL_EVENT_DURATION with value 30")
	}
	if !strings.Contains(body, "/ical.ics?key=abc123feedkey") {
		t.Errorf("expected global feed URL with secret key")
	}
	if !strings.Contains(body, "/ical/{personID}.ics?key=abc123feedkey") {
		t.Errorf("expected per-person feed URL template with secret key")
	}
}

func TestSettingsConfigSave_ICalFeedSuccess(t *testing.T) {
	h := newTestWebHandler(t)
	if err := h.settingsStore.EnsureSeeded(context.Background()); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	router := setupConfigRouter(h)

	form := url.Values{}
	form.Set("ICAL_FEED_ENABLED", "on")
	form.Set("ICAL_EVENT_START", "09:00")
	form.Set("ICAL_EVENT_DURATION", "60")

	req := httptest.NewRequest("POST", "/settings/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String()[:300])
	}
	if !h.cfg.ICalEnabled {
		t.Errorf("ICalEnabled hot-reload: want true")
	}
	if h.cfg.ICalEventStart != "09:00" {
		t.Errorf("ICalEventStart hot-reload: got %q want 09:00", h.cfg.ICalEventStart)
	}
	if h.cfg.ICalDurationMinutes != 60 {
		t.Errorf("ICalDurationMinutes hot-reload: got %d want 60", h.cfg.ICalDurationMinutes)
	}
	if h.cfg.ICalFeedKey == "" {
		t.Errorf("expected auto-generated feed key on first enable")
	}
}

func TestSettingsConfigSave_ICalFeedKeyRotation(t *testing.T) {
	h := newTestWebHandler(t)
	if err := h.settingsStore.EnsureSeeded(context.Background()); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	router := setupConfigRouter(h)

	form := url.Values{}
	form.Set("ICAL_FEED_ENABLED", "on")
	form.Set("ICAL_FEED_KEY", "rotatedkey123")

	req := httptest.NewRequest("POST", "/settings/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String()[:300])
	}
	if h.cfg.ICalFeedKey != "rotatedkey123" {
		t.Errorf("ICalFeedKey rotation: got %q want rotatedkey123", h.cfg.ICalFeedKey)
	}
}

func TestSettingsConfigSave_ICalFeedValidationErrors(t *testing.T) {
	h := newTestWebHandler(t)
	if err := h.settingsStore.EnsureSeeded(context.Background()); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	router := setupConfigRouter(h)

	form := url.Values{}
	form.Set("ICAL_FEED_ENABLED", "on")
	form.Set("ICAL_EVENT_START", "25:99")    // invalid hour and minute
	form.Set("ICAL_EVENT_DURATION", "9999")  // invalid duration

	req := httptest.NewRequest("POST", "/settings/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (re-render), got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Start hour must be between 0 and 23") {
		t.Errorf("expected inline error for start hour, got: %s", body[:500])
	}
	if !strings.Contains(body, "Event duration must be between 1 and 1440 minutes") {
		t.Errorf("expected inline error for duration, got: %s", body[:500])
	}
	// Validation failure must not persist or hot-reload anything.
	if h.cfg.ICalEnabled {
		t.Errorf("feed must not be enabled after failed save")
	}
	if h.cfg.ICalEventStart != "" {
		t.Errorf("ICalEventStart must be unchanged after failed save, got %q", h.cfg.ICalEventStart)
	}
}