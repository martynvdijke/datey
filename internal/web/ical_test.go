package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/internal/repository"
	"github.com/go-chi/chi/v5"
)

func setupICalRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/ical.ics", h.icalFeedGlobal)
	r.Get("/ical/{personID}.ics", h.icalFeedPerson)
	return r
}

func enableFeed(h *Handler) {
	h.cfg.ICalEnabled = true
	h.cfg.ICalFeedKey = "testsecretkey"
}

func newTestPerson(t *testing.T, h *Handler, name string) int {
	t.Helper()
	p, err := h.people.Create(context.Background(), name, "", "")
	if err != nil {
		t.Fatalf("create person %q: %v", name, err)
	}
	return p.ID
}

func newTestEvent(t *testing.T, h *Handler, personID int, eventType string, date time.Time) {
	t.Helper()
	if _, err := h.events.CreateForPerson(context.Background(), personID, eventType, date, ""); err != nil {
		t.Fatalf("create event: %v", err)
	}
}

func TestICalFeedDisabledReturns404(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupICalRouter(h)

	req := httptest.NewRequest("GET", "/ical.ics?key=testsecretkey", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when feed disabled, got %d", w.Code)
	}
}

func TestICalFeedMissingKeyReturns404(t *testing.T) {
	h := newTestWebHandler(t)
	enableFeed(h)
	router := setupICalRouter(h)

	req := httptest.NewRequest("GET", "/ical.ics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing key, got %d", w.Code)
	}
}

func TestICalFeedWrongKeyReturns404(t *testing.T) {
	h := newTestWebHandler(t)
	enableFeed(h)
	router := setupICalRouter(h)

	req := httptest.NewRequest("GET", "/ical.ics?key=wrongkey", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for wrong key, got %d", w.Code)
	}
}

func TestICalFeedGlobalReturnsCalendar(t *testing.T) {
	h := newTestWebHandler(t)
	enableFeed(h)
	personID := newTestPerson(t, h, "Anna")
	newTestEvent(t, h, personID, "birthday", time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC))
	router := setupICalRouter(h)

	req := httptest.NewRequest("GET", "/ical.ics?key=testsecretkey", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String()[:200])
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/calendar; charset=utf-8" {
		t.Errorf("expected text/calendar content type, got %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "BEGIN:VCALENDAR") {
		t.Errorf("expected VCALENDAR document, got:\n%s", body)
	}
	if !strings.Contains(body, "SUMMARY:Anna - birthday") {
		t.Errorf("expected event summary, got:\n%s", body)
	}
	if !strings.Contains(body, "RRULE:FREQ=YEARLY") {
		t.Errorf("expected yearly recurrence for birthday, got:\n%s", body)
	}
}

func TestICalFeedTimedEvents(t *testing.T) {
	h := newTestWebHandler(t)
	enableFeed(h)
	h.cfg.ICalEventStart = "09:00"
	h.cfg.ICalDurationMinutes = 60
	personID := newTestPerson(t, h, "Anna")
	newTestEvent(t, h, personID, "birthday", time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC))
	router := setupICalRouter(h)

	req := httptest.NewRequest("GET", "/ical.ics?key=testsecretkey", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "DTSTART:20260815T090000") {
		t.Errorf("expected timed DTSTART, got:\n%s", body)
	}
	if !strings.Contains(body, "DTEND:20260815T100000") {
		t.Errorf("expected DTEND = start+duration, got:\n%s", body)
	}
}

func TestICalFeedPersonOnlyIncludesThatPerson(t *testing.T) {
	h := newTestWebHandler(t)
	enableFeed(h)
	annaID := newTestPerson(t, h, "Anna")
	bobID := newTestPerson(t, h, "Bob")
	newTestEvent(t, h, annaID, "birthday", time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC))
	newTestEvent(t, h, bobID, "birthday", time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC))
	router := setupICalRouter(h)

	req := httptest.NewRequest("GET", "/ical/"+strconv.Itoa(annaID)+".ics?key=testsecretkey", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "SUMMARY:Anna - birthday") {
		t.Errorf("expected Anna's event, got:\n%s", body)
	}
	if strings.Contains(body, "Bob") {
		t.Errorf("per-person feed must not include other people, got:\n%s", body)
	}
}

func TestICalFeedUnknownPersonReturns404(t *testing.T) {
	h := newTestWebHandler(t)
	enableFeed(h)
	router := setupICalRouter(h)

	req := httptest.NewRequest("GET", "/ical/99999.ics?key=testsecretkey", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown person, got %d", w.Code)
	}
}

// newScheduledEvent creates a Gregorian person event with an explicit schedule.
func newScheduledEvent(t *testing.T, h *Handler, personID int, eventType string, date time.Time, freq string, interval int, endDate *time.Time, count *int) {
	t.Helper()
	sched := repository.NewEventSchedule(freq, interval, endDate, count)
	if _, err := h.events.CreateForPersonWithSchedule(context.Background(), personID, eventType, date, "", "gregorian", nil, nil, false, sched); err != nil {
		t.Fatalf("create scheduled event: %v", err)
	}
}

func fetchGlobalFeed(t *testing.T, h *Handler) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/ical.ics?key=testsecretkey", nil)
	w := httptest.NewRecorder()
	setupICalRouter(h).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	return w.Body.String()
}

func TestICalFeedMonthlyEmitsRDATEDates(t *testing.T) {
	h := newTestWebHandler(t)
	enableFeed(h)
	personID := newTestPerson(t, h, "Anna")
	newScheduledEvent(t, h, personID, "custom",
		time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC), "monthly", 1, nil, nil)

	body := fetchGlobalFeed(t, h)
	if strings.Contains(body, "FREQ=YEARLY") {
		t.Errorf("monthly event must not be published as yearly, got:\n%s", body)
	}
	if strings.Contains(body, "RRULE") {
		t.Errorf("monthly event must not carry an RRULE, got:\n%s", body)
	}
	if !strings.Contains(body, "DTSTART;VALUE=DATE:20260131") {
		t.Errorf("expected the anchor as DTSTART, got:\n%s", body)
	}
	// The engine clamps Jan 31 to the end of short months.
	for _, want := range []string{"RDATE;VALUE=DATE:20260228", "RDATE;VALUE=DATE:20260331"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q, got:\n%s", want, body)
		}
	}
}

func TestICalFeedOnceDoesNotRepeat(t *testing.T) {
	h := newTestWebHandler(t)
	enableFeed(h)
	personID := newTestPerson(t, h, "Anna")
	newScheduledEvent(t, h, personID, "custom",
		time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC), "once", 1, nil, nil)

	body := fetchGlobalFeed(t, h)
	if strings.Contains(body, "RRULE") || strings.Contains(body, "RDATE") {
		t.Errorf("one-off event must not repeat, got:\n%s", body)
	}
	if !strings.Contains(body, "DTSTART;VALUE=DATE:20260704") {
		t.Errorf("expected the one-off DTSTART, got:\n%s", body)
	}
	if strings.Count(body, "BEGIN:VEVENT") != 1 {
		t.Errorf("expected exactly one VEVENT, got:\n%s", body)
	}
}

func TestICalFeedAnnualCarriesYearlyRRule(t *testing.T) {
	h := newTestWebHandler(t)
	enableFeed(h)
	personID := newTestPerson(t, h, "Anna")
	newScheduledEvent(t, h, personID, "birthday",
		time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC), "annual", 1, nil, nil)

	body := fetchGlobalFeed(t, h)
	if !strings.Contains(body, "RRULE:FREQ=YEARLY") {
		t.Errorf("expected yearly rule for an annual event, got:\n%s", body)
	}
	if strings.Contains(body, "RDATE") {
		t.Errorf("annual event must not materialize RDATEs, got:\n%s", body)
	}
}

func TestICalFeedAnnualHonoursEndDate(t *testing.T) {
	h := newTestWebHandler(t)
	enableFeed(h)
	personID := newTestPerson(t, h, "Anna")
	end := time.Date(2030, 8, 15, 0, 0, 0, 0, time.UTC)
	newScheduledEvent(t, h, personID, "birthday",
		time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC), "annual", 1, &end, nil)

	body := fetchGlobalFeed(t, h)
	if !strings.Contains(body, "RRULE:FREQ=YEARLY;UNTIL=20300815") {
		t.Errorf("expected UNTIL bound from end_date, got:\n%s", body)
	}
}

func TestICalFeedAnnualHonoursOccurrenceCount(t *testing.T) {
	h := newTestWebHandler(t)
	enableFeed(h)
	personID := newTestPerson(t, h, "Anna")
	count := 3
	newScheduledEvent(t, h, personID, "birthday",
		time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC), "annual", 1, nil, &count)

	body := fetchGlobalFeed(t, h)
	if !strings.Contains(body, "RRULE:FREQ=YEARLY;COUNT=3") {
		t.Errorf("expected COUNT bound from occurrence_count, got:\n%s", body)
	}
}

func TestICalFeedWeeklyIsMaterialized(t *testing.T) {
	h := newTestWebHandler(t)
	enableFeed(h)
	personID := newTestPerson(t, h, "Anna")
	newScheduledEvent(t, h, personID, "custom",
		time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), "weekly", 1, nil, nil)

	body := fetchGlobalFeed(t, h)
	if strings.Contains(body, "RRULE") {
		t.Errorf("weekly event must not carry an RRULE, got:\n%s", body)
	}
	for _, want := range []string{"RDATE;VALUE=DATE:20260112", "RDATE;VALUE=DATE:20260119"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q, got:\n%s", want, body)
		}
	}
}
