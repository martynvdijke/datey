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
	r.Get("/api/trmnl/birthdays", h.trmnlBirthdays)
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

func getTRMNLBirthdays(t *testing.T, h *Handler) (int, http.Header, trmnlBirthdaysResponse) {
	t.Helper()
	router := setupTRMNLRouter(h)
	req := httptest.NewRequest("GET", "/api/trmnl/birthdays", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp trmnlBirthdaysResponse
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("response is not valid JSON: %v (body: %s)", err, w.Body.String())
		}
	}
	return w.Code, w.Header(), resp
}

func TestTRMNLBirthdaysUnauthenticated(t *testing.T) {
	h := newTestWebHandler(t)
	code, header, _ := getTRMNLBirthdays(t, h)

	if code != http.StatusOK {
		t.Fatalf("expected 200 without auth, got %d", code)
	}
	if ct := header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json content type, got %q", ct)
	}
}

func TestTRMNLBirthdaysEmptyDatabase(t *testing.T) {
	h := newTestWebHandler(t)
	_, _, resp := getTRMNLBirthdays(t, h)

	if resp.NextBirthday != nil {
		t.Errorf("expected next_birthday null with no events, got %+v", resp.NextBirthday)
	}
	if len(resp.Birthdays) != 0 {
		t.Errorf("expected empty birthdays array, got %+v", resp.Birthdays)
	}
}

func TestTRMNLBirthdaysFiltersNonBirthdays(t *testing.T) {
	h := newTestWebHandler(t)
	personID := newTestPerson(t, h, "Anna")
	now := time.Now()
	midnight := func(days int) time.Time {
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, days)
	}
	newTestEvent(t, h, personID, "birthday", midnight(2))
	newTestEvent(t, h, personID, "anniversary", midnight(3))
	newTestEvent(t, h, personID, "holiday", midnight(4))

	_, _, resp := getTRMNLBirthdays(t, h)

	if len(resp.Birthdays) != 1 {
		t.Fatalf("expected only the birthday event, got %+v", resp.Birthdays)
	}
	if resp.Birthdays[0].Name != "Anna" {
		t.Errorf("expected birthday for Anna, got %+v", resp.Birthdays[0])
	}
	if resp.NextBirthday == nil || resp.NextBirthday.Name != "Anna" {
		t.Errorf("expected next_birthday to be the birthday, got %+v", resp.NextBirthday)
	}
}

func TestTRMNLBirthdaysAgeWithAndWithoutYear(t *testing.T) {
	h := newTestWebHandler(t)
	personID := newTestPerson(t, h, "Anna")
	now := time.Now()
	occ := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 3)
	// Birthday with a real birth year (30 years ago): turns 30 on the
	// upcoming occurrence (age is 29 until the birthday has occurred).
	withYear := time.Date(now.Year()-30, occ.Month(), occ.Day(), 0, 0, 0, 0, now.Location())
	newTestEvent(t, h, personID, "birthday", withYear)
	// Year-less birthday (vCard import): no usable birth year → age null.
	withoutYear := time.Date(0, occ.Month(), occ.Day(), 0, 0, 0, 0, now.Location())
	newTestEvent(t, h, personID, "birthday", withoutYear)

	_, _, resp := getTRMNLBirthdays(t, h)

	if len(resp.Birthdays) != 2 {
		t.Fatalf("expected 2 birthdays, got %+v", resp.Birthdays)
	}
	// Age present for the year-full birthday.
	if resp.Birthdays[0].Age == nil {
		t.Errorf("expected age for year-full birthday, got null")
	} else if *resp.Birthdays[0].Age != 30 {
		t.Errorf("expected age 30, got %d", *resp.Birthdays[0].Age)
	}
	// Age null for the year-less birthday.
	if resp.Birthdays[1].Age != nil {
		t.Errorf("expected null age for year-less birthday, got %d", *resp.Birthdays[1].Age)
	}
}

func TestTRMNLBirthdaysRelativeAndReminderWindow(t *testing.T) {
	h := newTestWebHandler(t)
	h.cfg.ReminderDays = 14
	personID := newTestPerson(t, h, "Anna")
	now := time.Now()
	midnight := func(days int) time.Time {
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, days)
	}
	newTestEvent(t, h, personID, "birthday", midnight(2))  // Tomorrow
	newTestEvent(t, h, personID, "birthday", midnight(10)) // In 9 days (within 14)
	newTestEvent(t, h, personID, "birthday", midnight(20)) // outside 14

	_, _, resp := getTRMNLBirthdays(t, h)

	if len(resp.Birthdays) != 2 {
		t.Fatalf("expected 2 birthdays within ReminderDays=14, got %d", len(resp.Birthdays))
	}
	if resp.Birthdays[0].Relative != "Tomorrow" {
		t.Errorf("expected relative Tomorrow for +2 days, got %q", resp.Birthdays[0].Relative)
	}
	// Beyond 7 days the relative label is empty (same as the stats feed).
	if resp.Birthdays[1].DaysRemaining != 9 {
		t.Errorf("expected days_remaining 9 for +10 days, got %d", resp.Birthdays[1].DaysRemaining)
	}
	if resp.Birthdays[1].Relative != "" {
		t.Errorf("expected empty relative beyond 7 days, got %q", resp.Birthdays[1].Relative)
	}
	if resp.NextBirthday == nil || resp.NextBirthday.Relative != "Tomorrow" {
		t.Errorf("expected next_birthday Tomorrow, got %+v", resp.NextBirthday)
	}
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
	// Annual events are reported on their occurrence date (midnight of the
	// month/day), so day counts are anchored to midnight-based dates:
	// midnight 2 days out is deterministically 1 whole day away, midnight
	// 4 days out is 3 whole days away.
	midnight := func(days int) time.Time {
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, days)
	}
	newTestEvent(t, h, personID, "birthday", midnight(4))
	newTestEvent(t, h, personID, "anniversary", midnight(2))

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
}

func TestTRMNLStatsFallsBackToNextBirthday(t *testing.T) {
	h := newTestWebHandler(t)
	personID := newTestPerson(t, h, "Anna")
	now := time.Now()
	midnight := func(days int) time.Time {
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, days)
	}
	// Birthday far outside the reminder window (default 7): the regular
	// upcoming list is empty, so the feed must fall back to the next
	// birthday so it never reports "No upcoming events".
	newTestEvent(t, h, personID, "birthday", midnight(40))

	_, _, stats := getTRMNLStats(t, h)

	if stats.NextEvent == nil {
		t.Fatal("expected next_event to be set via birthday fallback")
	}
	if stats.NextEvent.Name != "Anna" {
		t.Errorf("expected name Anna, got %q", stats.NextEvent.Name)
	}
	if stats.NextEvent.Type != "birthday" {
		t.Errorf("expected type birthday, got %q", stats.NextEvent.Type)
	}
	if stats.NextEvent.DaysRemaining != 39 {
		t.Errorf("expected days_remaining 39, got %d", stats.NextEvent.DaysRemaining)
	}
	if len(stats.Upcoming) != 1 {
		t.Errorf("expected 1 upcoming event (the fallback), got %d", len(stats.Upcoming))
	}
	// The fallback is outside the 30-day horizon: summary counts stay 0.
	if stats.Stats.EventsNext7Days != 0 {
		t.Errorf("expected events_next_7_days 0, got %d", stats.Stats.EventsNext7Days)
	}
	if stats.Stats.EventsNext30Days != 0 {
		t.Errorf("expected events_next_30_days 0, got %d", stats.Stats.EventsNext30Days)
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

func TestTRMNLBirthdaysFallsBackBeyondWindow(t *testing.T) {
	h := newTestWebHandler(t)
	h.cfg.ReminderDays = 14
	personID := newTestPerson(t, h, "Anna")
	now := time.Now()
	midnight := func(days int) time.Time {
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, days)
	}
	// Birthday far outside the reminder window: the feed must fall back to
	// the next birthday so it never reports "No upcoming birthdays".
	newTestEvent(t, h, personID, "birthday", midnight(60))

	_, _, resp := getTRMNLBirthdays(t, h)

	if resp.NextBirthday == nil {
		t.Fatal("expected next_birthday to be set via birthday fallback")
	}
	if resp.NextBirthday.Name != "Anna" {
		t.Errorf("expected name Anna, got %q", resp.NextBirthday.Name)
	}
	if resp.NextBirthday.DaysRemaining != 59 {
		t.Errorf("expected days_remaining 59, got %d", resp.NextBirthday.DaysRemaining)
	}
	if len(resp.Birthdays) != 1 {
		t.Errorf("expected 1 birthday (the fallback), got %d", len(resp.Birthdays))
	}
}
