package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func setupHARouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/api/homeassistant/stats", h.homeAssistantStats)
	r.Get("/api/homeassistant/calendar", h.homeAssistantCalendar)
	return r
}

func enableHAFeed(h *Handler) {
	h.cfg.HomeAssistantEnabled = true
	h.cfg.HomeAssistantKey = "testhakey"
}

func TestHAFeedDisabledReturns404(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupHARouter(h)

	req := httptest.NewRequest("GET", "/api/homeassistant/stats?key=testhakey", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when disabled, got %d", w.Code)
	}
}

func TestHAFeedMissingKeyReturns404(t *testing.T) {
	h := newTestWebHandler(t)
	enableHAFeed(h)
	router := setupHARouter(h)

	req := httptest.NewRequest("GET", "/api/homeassistant/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 without key, got %d", w.Code)
	}
}

func TestHAFeedWrongKeyReturns404(t *testing.T) {
	h := newTestWebHandler(t)
	enableHAFeed(h)
	router := setupHARouter(h)

	req := httptest.NewRequest("GET", "/api/homeassistant/stats?key=wrongkey", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 with wrong key, got %d", w.Code)
	}
}

func TestHAFeedStatsShape(t *testing.T) {
	h := newTestWebHandler(t)
	enableHAFeed(h)

	personID := newTestPerson(t, h, "Dana")
	// Annual events are reported on their occurrence date (midnight of the
	// month/day), so day counts are anchored to midnight-based dates:
	// midnight 4 days out is deterministically 3 whole days away.
	now := time.Now()
	midnight := func(days int) time.Time {
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, days)
	}
	newTestEvent(t, h, personID, "birthday", midnight(4))
	newTestEvent(t, h, personID, "anniversary", midnight(20))

	router := setupHARouter(h)
	req := httptest.NewRequest("GET", "/api/homeassistant/stats?key=testhakey", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}

	var stats homeAssistantStats
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if stats.NextEvent != "Dana's Birthday" {
		t.Errorf("next_event: got %q want %q", stats.NextEvent, "Dana's Birthday")
	}
	if stats.NextEventDays != 3 {
		t.Errorf("next_event_days: got %d want 3", stats.NextEventDays)
	}
	if stats.NextEventDate == "" {
		t.Error("next_event_date should not be empty")
	}
	if stats.EventsIn7Days != 1 {
		t.Errorf("events_in_7_days: got %d want 1", stats.EventsIn7Days)
	}
	if stats.EventsIn30Days != 2 {
		t.Errorf("events_in_30_days: got %d want 2", stats.EventsIn30Days)
	}

	// ReminderDays defaults to 7 in the test handler, so the 20-day
	// anniversary counts toward the summary but is excluded from the list.
	if len(stats.Upcoming) != 1 {
		t.Fatalf("upcoming: got %d entries want 1", len(stats.Upcoming))
	}
	ev := stats.Upcoming[0]
	if ev.Name != "Dana" || ev.Type != "birthday" || ev.Days != 3 {
		t.Errorf("upcoming[0]: got %+v", ev)
	}
	if ev.Date == "" {
		t.Error("upcoming[0].date should not be empty")
	}
}

func TestHAFeedEmptyState(t *testing.T) {
	h := newTestWebHandler(t)
	enableHAFeed(h)
	router := setupHARouter(h)

	req := httptest.NewRequest("GET", "/api/homeassistant/stats?key=testhakey", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var stats homeAssistantStats
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if stats.NextEvent != "" {
		t.Errorf("next_event: got %q want empty", stats.NextEvent)
	}
	if stats.NextEventDate != "" {
		t.Errorf("next_event_date: got %q want empty", stats.NextEventDate)
	}
	if stats.EventsIn7Days != 0 || stats.EventsIn30Days != 0 {
		t.Errorf("expected zero counts, got 7d=%d 30d=%d", stats.EventsIn7Days, stats.EventsIn30Days)
	}
	if len(stats.Upcoming) != 0 {
		t.Errorf("upcoming: got %d entries want 0", len(stats.Upcoming))
	}
}

// Calendar endpoint tests

func TestHACalendarDisabledReturns404(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupHARouter(h)
	req := httptest.NewRequest("GET", "/api/homeassistant/calendar?key=testhakey&start=2026-01-01&end=2026-01-31", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when disabled, got %d", w.Code)
	}
}

func TestHACalendarMissingKeyReturns404(t *testing.T) {
	h := newTestWebHandler(t)
	enableHAFeed(h)
	router := setupHARouter(h)
	req := httptest.NewRequest("GET", "/api/homeassistant/calendar?start=2026-01-01&end=2026-01-31", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 without key, got %d", w.Code)
	}
}

func TestHACalendarWrongKeyReturns404(t *testing.T) {
	h := newTestWebHandler(t)
	enableHAFeed(h)
	router := setupHARouter(h)
	req := httptest.NewRequest("GET", "/api/homeassistant/calendar?key=wrong&start=2026-01-01&end=2026-01-31", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 with wrong key, got %d", w.Code)
	}
}

func TestHACalendarValidRangeReturnsEvents(t *testing.T) {
	h := newTestWebHandler(t)
	enableHAFeed(h)
	personID := newTestPerson(t, h, "Dana")
	newTestEvent(t, h, personID, "birthday", time.Date(1990, 3, 15, 0, 0, 0, 0, time.UTC))
	router := setupHARouter(h)
	req := httptest.NewRequest("GET", "/api/homeassistant/calendar?key=testhakey&start=2026-03-01&end=2026-04-01", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var events []homeAssistantCalendarEvent
	if err := json.Unmarshal(w.Body.Bytes(), &events); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Start.Date != "2026-03-15" {
		t.Errorf("start.date got %q want 2026-03-15", ev.Start.Date)
	}
	if ev.End.Date != "2026-03-16" {
		t.Errorf("end.date got %q want 2026-03-16", ev.End.Date)
	}
	if ev.Summary == "" || ev.UID == "" {
		t.Errorf("summary/uid should not be empty: %+v", ev)
	}
	// Summary should contain person's name
	if !contains(ev.Summary, "Dana") {
		t.Errorf("summary should contain Dana, got %q", ev.Summary)
	}
}

func TestHACalendarOutsideRangeExcluded(t *testing.T) {
	h := newTestWebHandler(t)
	enableHAFeed(h)
	personID := newTestPerson(t, h, "Dana")
	newTestEvent(t, h, personID, "birthday", time.Date(1990, 7, 4, 0, 0, 0, 0, time.UTC))
	router := setupHARouter(h)
	req := httptest.NewRequest("GET", "/api/homeassistant/calendar?key=testhakey&start=2026-01-01&end=2026-02-01", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var events []homeAssistantCalendarEvent
	if err := json.Unmarshal(w.Body.Bytes(), &events); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events outside range, got %d", len(events))
	}
}

func TestHACalendarEndExclusive(t *testing.T) {
	h := newTestWebHandler(t)
	enableHAFeed(h)
	personID := newTestPerson(t, h, "Dana")
	newTestEvent(t, h, personID, "birthday", time.Date(1990, 3, 15, 0, 0, 0, 0, time.UTC))
	router := setupHARouter(h)
	// end is exclusive, event on end date should not appear
	req := httptest.NewRequest("GET", "/api/homeassistant/calendar?key=testhakey&start=2026-03-01&end=2026-03-15", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var events []homeAssistantCalendarEvent
	if err := json.Unmarshal(w.Body.Bytes(), &events); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events when occurrence == end (exclusive), got %d", len(events))
	}
}

func TestHACalendarEmptyState(t *testing.T) {
	h := newTestWebHandler(t)
	enableHAFeed(h)
	router := setupHARouter(h)
	req := httptest.NewRequest("GET", "/api/homeassistant/calendar?key=testhakey&start=2026-01-01&end=2026-01-31", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var events []homeAssistantCalendarEvent
	if err := json.Unmarshal(w.Body.Bytes(), &events); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected empty array, got %d", len(events))
	}
	// Must be JSON array not null
	if string(w.Body.Bytes()) == "null\n" {
		t.Error("expected [] not null")
	}
}

func TestHACalendarInvalidDateReturns400(t *testing.T) {
	h := newTestWebHandler(t)
	enableHAFeed(h)
	router := setupHARouter(h)
	cases := []string{
		"/api/homeassistant/calendar?key=testhakey&start=invalid&end=2026-01-31",
		"/api/homeassistant/calendar?key=testhakey&start=2026-01-01&end=invalid",
		"/api/homeassistant/calendar?key=testhakey&start=2026-01-01",
		"/api/homeassistant/calendar?key=testhakey&end=2026-01-31",
		"/api/homeassistant/calendar?key=testhakey&start=2026-02-01&end=2026-01-01",
	}
	for _, url := range cases {
		req := httptest.NewRequest("GET", url, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("url %q expected 400, got %d", url, w.Code)
		}
	}
}

func TestHACalendarRangeTooLargeReturns400(t *testing.T) {
	h := newTestWebHandler(t)
	enableHAFeed(h)
	router := setupHARouter(h)
	req := httptest.NewRequest("GET", "/api/homeassistant/calendar?key=testhakey&start=2026-01-01&end=2027-01-02", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for >365 day range, got %d", w.Code)
	}
}

func TestHACalendarLunarConversion(t *testing.T) {
	h := newTestWebHandler(t)
	enableHAFeed(h)
	personID := newTestPerson(t, h, "Luna")
	// Create lunar event: lunar 1/1
	lm, ld := 1, 1
	_, err := h.events.CreateForPersonWithCalendar(nilContext(), personID, "birthday", time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC), "", "lunar", &lm, &ld, false)
	if err != nil {
		// fallback via ent client
		_, err = h.client.Event.Create().SetType("birthday").SetDate(time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)).SetCreatedAt(time.Now()).SetPersonID(personID).SetCalendarSystem("lunar").SetLunarMonth(1).SetLunarDay(1).Save(nilContext())
		if err != nil {
			t.Fatalf("create lunar event: %v", err)
		}
	}
	// Lunar 1/1 in 2026 is 2026-02-17 (known conversion)
	// Query range covering that date
	router := setupHARouter(h)
	req := httptest.NewRequest("GET", "/api/homeassistant/calendar?key=testhakey&start=2026-02-17&end=2026-02-18", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var events []homeAssistantCalendarEvent
	if err := json.Unmarshal(w.Body.Bytes(), &events); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 lunar event on converted date, got %d: %s", len(events), w.Body.String())
	}
	if events[0].Start.Date != "2026-02-17" {
		t.Errorf("lunar occurrence expected 2026-02-17, got %q", events[0].Start.Date)
	}
}

func nilContext() context.Context { return context.Background() }

func contains(s, substr string) bool { return strings.Contains(s, substr) }
