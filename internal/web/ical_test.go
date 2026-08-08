package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

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
