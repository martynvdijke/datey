package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func setupTRMNLRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/api/trmnl/stats", h.trmnlStats)
	return r
}

func getTRMNLStats(t *testing.T, h *Handler) (int, http.Header, trmnlStats) {
	t.Helper()
	router := setupTRMNLRouter(h)
	req := httptest.NewRequest("GET", "/api/trmnl/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var stats trmnlStats
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
			t.Fatalf("response is not valid JSON: %v (body: %s)", err, w.Body.String())
		}
	}
	return w.Code, w.Header(), stats
}

func TestTRMNLStatsUnauthenticated(t *testing.T) {
	h := newTestWebHandler(t)
	code, header, _ := getTRMNLStats(t, h)

	if code != http.StatusOK {
		t.Fatalf("expected 200 without auth, got %d", code)
	}
	if ct := header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json content type, got %q", ct)
	}
}

func TestTRMNLStatsEmptyDatabase(t *testing.T) {
	h := newTestWebHandler(t)
	_, _, stats := getTRMNLStats(t, h)

	if stats.NextEvent != nil {
		t.Errorf("expected next_event null with no events, got %+v", stats.NextEvent)
	}
	if len(stats.Upcoming) != 0 {
		t.Errorf("expected empty upcoming array, got %+v", stats.Upcoming)
	}
	if stats.Stats.PeopleCount != 0 {
		t.Errorf("expected people_count 0, got %d", stats.Stats.PeopleCount)
	}
}

func TestTRMNLStatsNextEventIsEarliest(t *testing.T) {
	h := newTestWebHandler(t)
	personID := newTestPerson(t, h, "Anna")
	now := time.Now()
	// Add a margin beyond whole days so day counts are deterministic
	// (int(Hours()/24) truncates toward zero).
	newTestEvent(t, h, personID, "birthday", now.AddDate(0, 0, 3).Add(2*time.Hour))
	newTestEvent(t, h, personID, "anniversary", now.AddDate(0, 0, 1).Add(2*time.Hour))

	_, _, stats := getTRMNLStats(t, h)

	if stats.NextEvent == nil {
		t.Fatal("expected next_event to be set")
	}
	if stats.NextEvent.Name != "Anna" {
		t.Errorf("expected name Anna, got %q", stats.NextEvent.Name)
	}
	if stats.NextEvent.Type != "anniversary" {
		t.Errorf("expected earliest event type anniversary, got %q", stats.NextEvent.Type)
	}
	if stats.NextEvent.DaysRemaining != 1 {
		t.Errorf("expected days_remaining 1, got %d", stats.NextEvent.DaysRemaining)
	}
	if stats.NextEvent.Relative != "Tomorrow" {
		t.Errorf("expected relative Tomorrow, got %q", stats.NextEvent.Relative)
	}
	if len(stats.Upcoming) != 2 {
		t.Errorf("expected 2 upcoming events, got %d", len(stats.Upcoming))
	}
	// Upcoming must be sorted ascending by date.
	if stats.Upcoming[0].DaysRemaining > stats.Upcoming[1].DaysRemaining {
		t.Errorf("expected upcoming sorted ascending, got %+v", stats.Upcoming)
	}
}

func TestTRMNLStatsSummaryCounts(t *testing.T) {
	h := newTestWebHandler(t)
	annaID := newTestPerson(t, h, "Anna")
	bobID := newTestPerson(t, h, "Bob")
	now := time.Now()
	newTestEvent(t, h, annaID, "birthday", now.AddDate(0, 0, 3).Add(2*time.Hour))    // within 7
	newTestEvent(t, h, bobID, "anniversary", now.AddDate(0, 0, 20).Add(2*time.Hour)) // within 30
	newTestEvent(t, h, annaID, "holiday", now.AddDate(0, 0, 60).Add(2*time.Hour))    // outside 30

	_, _, stats := getTRMNLStats(t, h)

	if stats.Stats.PeopleCount != 2 {
		t.Errorf("expected people_count 2, got %d", stats.Stats.PeopleCount)
	}
	if stats.Stats.EventsNext7Days != 1 {
		t.Errorf("expected events_next_7_days 1, got %d", stats.Stats.EventsNext7Days)
	}
	if stats.Stats.EventsNext30Days != 2 {
		t.Errorf("expected events_next_30_days 2, got %d", stats.Stats.EventsNext30Days)
	}
	// No notification channels configured in the test registry.
	if stats.Stats.ConfiguredChannels != 0 {
		t.Errorf("expected configured_channels 0, got %d", stats.Stats.ConfiguredChannels)
	}
}

func TestTRMNLStatsRespectsReminderWindow(t *testing.T) {
	h := newTestWebHandler(t)
	h.cfg.ReminderDays = 14
	personID := newTestPerson(t, h, "Anna")
	now := time.Now()
	newTestEvent(t, h, personID, "birthday", now.AddDate(0, 0, 10)) // inside 14
	newTestEvent(t, h, personID, "custom", now.AddDate(0, 0, 20))   // outside 14

	_, _, stats := getTRMNLStats(t, h)

	if len(stats.Upcoming) != 1 {
		t.Errorf("expected 1 event within ReminderDays=14 window, got %d", len(stats.Upcoming))
	}
	if stats.NextEvent == nil || stats.NextEvent.Type != "birthday" {
		t.Errorf("expected next_event birthday, got %+v", stats.NextEvent)
	}
}
