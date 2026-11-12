package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/ent/event"
	"github.com/datey/datey/ent/person"
)

// postEventForm posts to the create-event route and returns the recorder.
func postEventForm(t *testing.T, h *Handler, personID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/people/"+personID+"/events/new", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	setupEventsRouter(h).ServeHTTP(w, req)
	return w
}

func newEventFormPerson(t *testing.T, h *Handler) int {
	t.Helper()
	p, err := h.client.Person.Create().
		SetName("Frequency Test").
		SetNotes("test").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	return p.ID
}

func TestCreateEvent_UnknownFrequencyRejected(t *testing.T) {
	h := newTestWebHandler(t)
	w := postEventForm(t, h, "1", "type=birthday&date=2026-07-04&frequency=fortnightly")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (form re-rendered), got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Unknown repeat frequency") {
		t.Fatalf("expected frequency error, got: %s", w.Body.String()[:300])
	}
}

func TestCreateEvent_IntervalOutOfRangeRejected(t *testing.T) {
	h := newTestWebHandler(t)
	for _, interval := range []string{"0", "121", "-3"} {
		w := postEventForm(t, h, "1", "type=custom&date=2026-07-04&frequency=every_n&interval="+interval)
		if w.Code != http.StatusOK {
			t.Fatalf("interval %s: expected 200, got %d", interval, w.Code)
		}
		if !strings.Contains(w.Body.String(), "Interval must be between 1 and 120 days") {
			t.Fatalf("interval %s: expected interval error, got: %s", interval, w.Body.String()[:300])
		}
	}
}

func TestCreateEvent_OccurrenceCountBelowOneRejected(t *testing.T) {
	h := newTestWebHandler(t)
	for _, count := range []string{"0", "-2", "abc"} {
		w := postEventForm(t, h, "1", "type=custom&date=2026-07-04&frequency=weekly&occurrence_count="+count)
		if w.Code != http.StatusOK {
			t.Fatalf("count %s: expected 200, got %d", count, w.Code)
		}
		if !strings.Contains(w.Body.String(), "Number of occurrences must be at least 1") {
			t.Fatalf("count %s: expected count error, got: %s", count, w.Body.String()[:300])
		}
	}
}

func TestCreateEvent_EndDateBeforeStartRejected(t *testing.T) {
	h := newTestWebHandler(t)
	w := postEventForm(t, h, "1", "type=custom&date=2026-07-04&frequency=weekly&end_date=2026-07-03")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "End date cannot be before the start date") {
		t.Fatalf("expected end date error, got: %s", w.Body.String()[:300])
	}
}

func TestCreateEvent_NonGregorianNonAnnualRejected(t *testing.T) {
	h := newTestWebHandler(t)
	w := postEventForm(t, h, "1", "type=birthday&date=2026-07-04&calendar_system=lunar&lunar_month=8&lunar_day=15&frequency=monthly")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Only annual repetition is supported for non-Gregorian calendars") {
		t.Fatalf("expected non-gregorian error, got: %s", w.Body.String()[:300])
	}
}

func TestCreateEvent_PersistsSchedule(t *testing.T) {
	h := newTestWebHandler(t)
	personID := newEventFormPerson(t, h)
	body := "type=custom&date=2026-07-04&frequency=every_n&interval=14&end_date=2026-12-31&occurrence_count=5"
	w := postEventForm(t, h, itoa(personID), body)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", w.Code, w.Body.String()[:300])
	}
	ev, err := h.client.Event.Query().
		Where(event.HasPersonWith(person.IDEQ(personID))).
		Only(context.Background())
	if err != nil {
		t.Fatalf("query event: %v", err)
	}
	if ev.Frequency != "every_n" || ev.Interval != 14 {
		t.Fatalf("stored frequency/interval = %q/%d", ev.Frequency, ev.Interval)
	}
	if ev.OccurrenceCount == nil || *ev.OccurrenceCount != 5 {
		t.Fatalf("stored occurrence_count = %v", ev.OccurrenceCount)
	}
	if ev.EndDate == nil || ev.EndDate.Format("2006-01-02") != "2026-12-31" {
		t.Fatalf("stored end_date = %v", ev.EndDate)
	}
}

func TestCreateEvent_DefaultsToAnnual(t *testing.T) {
	h := newTestWebHandler(t)
	personID := newEventFormPerson(t, h)
	w := postEventForm(t, h, itoa(personID), "type=birthday&date=2026-07-04")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", w.Code, w.Body.String()[:300])
	}
	ev, err := h.client.Event.Query().Only(context.Background())
	if err != nil {
		t.Fatalf("query event: %v", err)
	}
	if ev.Frequency != "annual" || ev.Interval != 1 {
		t.Fatalf("default schedule = %q/%d", ev.Frequency, ev.Interval)
	}
	if ev.EndDate != nil || ev.OccurrenceCount != nil {
		t.Fatalf("expected no bounds, got %v/%v", ev.EndDate, ev.OccurrenceCount)
	}
}

func TestCreateEvent_ShowsScheduleFields(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupEventsRouter(h)
	req := httptest.NewRequest("GET", "/people/1/events/new", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body := w.Body.String()
	for _, want := range []string{`id="frequency"`, `id="interval"`, `id="end_date"`, `id="occurrence_count"`, "Every N days"} {
		if !strings.Contains(body, want) {
			t.Fatalf("event form missing %q", want)
		}
	}
}
