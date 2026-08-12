package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func setupHARouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/api/homeassistant/stats", h.homeAssistantStats)
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
