package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/datey/datey/ent/event"
	"github.com/datey/datey/ent/group"
	"github.com/datey/datey/internal/repository"
)

func setupEventEditRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/people/{id}/events/new", h.newEventForm)
	r.Post("/people/{id}/events/new", h.createEvent)
	r.Get("/groups/{id}/events/new", h.newGroupEventForm)
	r.Post("/groups/{id}/events/new", h.createGroupEvent)
	r.Get("/events/{id}/edit", h.editEventForm)
	r.Post("/events/{id}/edit", h.updateEvent)
	return r
}

func serveEventRequest(h *Handler, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	setupEventEditRouter(h).ServeHTTP(w, req)
	return w
}

func newGroupEventGroup(t *testing.T, h *Handler, name string) int {
	t.Helper()
	g, err := h.groups.Create(context.Background(), name, "")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	return g.ID
}

// TestCreateGroupEvent_PersistsMonthlySchedule verifies that the group route
// now validates and stores recurrence instead of dropping it.
func TestCreateGroupEvent_PersistsMonthlySchedule(t *testing.T) {
	h := newTestWebHandler(t)
	groupID := newGroupEventGroup(t, h, "Monthly Group")

	w := serveEventRequest(h, "POST", "/groups/"+itoa(groupID)+"/events/new",
		"type=custom&date=2026-01-15&frequency=monthly&description=Monthly")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/groups/"+itoa(groupID)+"?success=Event+created" {
		t.Fatalf("unexpected redirect %q", loc)
	}

	ev, err := h.client.Event.Query().Where(event.HasGroupWith(group.IDEQ(groupID))).Only(context.Background())
	if err != nil {
		t.Fatalf("query group event: %v", err)
	}
	if ev.Frequency != "monthly" || ev.Interval != 1 {
		t.Fatalf("stored schedule = %q/%d, want monthly/1", ev.Frequency, ev.Interval)
	}

	occs, err := h.events.ListUpcomingOccurrences(context.Background(),
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ListUpcomingOccurrences: %v", err)
	}
	want := []string{"2026-01-15", "2026-02-15", "2026-03-15", "2026-04-15"}
	if len(occs) != len(want) {
		t.Fatalf("got %d occurrences, want %d (%v)", len(occs), len(want), occs)
	}
	for i, d := range want {
		if got := occs[i].Date.Format("2006-01-02"); got != d {
			t.Fatalf("occurrence %d = %s, want %s", i, got, d)
		}
	}
}

// TestCreateGroupEvent_ValidationRetainsValues checks that a rejected group
// submission re-renders the form with field errors and the submitted values.
func TestCreateGroupEvent_ValidationRetainsValues(t *testing.T) {
	h := newTestWebHandler(t)
	groupID := newGroupEventGroup(t, h, "Validation Group")

	w := serveEventRequest(h, "POST", "/groups/"+itoa(groupID)+"/events/new",
		"type=custom&date=2026-01-15&frequency=every_n&interval=999&end_date=2026-12-31")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (form re-rendered), got %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		"Interval must be between 1 and 120 days",
		`value="2026-01-15"`,
		`value="2026-12-31"`,
		`value="999"`,
		`value="every_n" selected`,
		"Validation Group",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("re-rendered group form missing %q", want)
		}
	}

	// Nothing was persisted.
	n, err := h.client.Event.Query().Count(context.Background())
	if err != nil {
		t.Fatalf("count events: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected no events, got %d", n)
	}
}

// TestEditEvent_PersonOwner changes frequency, interval and end date of a
// person event and verifies the stored schedule.
func TestEditEvent_PersonOwner(t *testing.T) {
	h := newTestWebHandler(t)
	ctx := context.Background()

	p, err := h.client.Person.Create().
		SetName("Edit Person").
		SetNotes("test").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	ev, err := h.events.CreateForPersonWithSchedule(ctx, p.ID, "custom",
		time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), "original", "gregorian",
		nil, nil, false,
		repository.NewEventSchedule("annual", 1, nil, nil))
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	// GET renders the edit form pre-filled.
	w := serveEventRequest(h, "GET", "/events/"+itoa(ev.ID)+"/edit", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET edit: expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		"Edit Event",
		`action="/events/` + itoa(ev.ID) + `/edit"`,
		`value="2026-02-01"`,
		`value="annual" selected`,
		"Save Changes",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("edit form missing %q", want)
		}
	}

	w = serveEventRequest(h, "POST", "/events/"+itoa(ev.ID)+"/edit",
		"type=custom&date=2026-02-01&description=updated&frequency=every_n&interval=10&end_date=2027-01-31")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("POST edit: expected 303, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/people/"+itoa(p.ID)+"?success=Event+updated" {
		t.Fatalf("unexpected redirect %q", loc)
	}

	got, err := h.events.Get(ctx, ev.ID)
	if err != nil {
		t.Fatalf("reload event: %v", err)
	}
	if got.Frequency != "every_n" || got.Interval != 10 {
		t.Fatalf("stored frequency/interval = %q/%d, want every_n/10", got.Frequency, got.Interval)
	}
	if got.EndDate == nil || got.EndDate.Format("2006-01-02") != "2027-01-31" {
		t.Fatalf("stored end_date = %v", got.EndDate)
	}
	if got.Description != "updated" {
		t.Fatalf("stored description = %q", got.Description)
	}
}

// TestEditEvent_GroupOwner verifies the edit handlers work for group events.
func TestEditEvent_GroupOwner(t *testing.T) {
	h := newTestWebHandler(t)
	ctx := context.Background()
	groupID := newGroupEventGroup(t, h, "Edit Group")

	ev, err := h.events.CreateForGroupWithSchedule(ctx, groupID, "anniversary",
		time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), "group event", "gregorian",
		nil, nil, false,
		repository.NewEventSchedule("annual", 1, nil, nil))
	if err != nil {
		t.Fatalf("create group event: %v", err)
	}

	w := serveEventRequest(h, "GET", "/events/"+itoa(ev.ID)+"/edit", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET edit: expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "/groups/"+itoa(groupID)) {
		t.Fatalf("edit form should link back to the group")
	}

	w = serveEventRequest(h, "POST", "/events/"+itoa(ev.ID)+"/edit",
		"type=anniversary&date=2026-03-10&frequency=monthly")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("POST edit: expected 303, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/groups/"+itoa(groupID)+"?success=Event+updated" {
		t.Fatalf("unexpected redirect %q", loc)
	}

	got, err := h.events.Get(ctx, ev.ID)
	if err != nil {
		t.Fatalf("reload event: %v", err)
	}
	if got.Frequency != "monthly" {
		t.Fatalf("stored frequency = %q, want monthly", got.Frequency)
	}
}

// TestEditEvent_ValidationRetainsValues checks the edit form keeps values and
// shows field errors instead of persisting.
func TestEditEvent_ValidationRetainsValues(t *testing.T) {
	h := newTestWebHandler(t)
	ctx := context.Background()

	p, err := h.client.Person.Create().
		SetName("Edit Validation").
		SetNotes("test").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	ev, err := h.events.CreateForPersonWithSchedule(ctx, p.ID, "birthday",
		time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC), "", "gregorian",
		nil, nil, false,
		repository.NewEventSchedule("annual", 1, nil, nil))
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	w := serveEventRequest(h, "POST", "/events/"+itoa(ev.ID)+"/edit",
		"type=birthday&date=2026-05-05&frequency=monthly&calendar_system=lunar&lunar_month=8&lunar_day=15")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		"Only annual repetition is supported for non-Gregorian calendars",
		`value="lunar" selected`,
		`value="8" selected`,
		`value="15" selected`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("edit form missing %q", want)
		}
	}

	// Unchanged on disk.
	got, err := h.events.Get(ctx, ev.ID)
	if err != nil {
		t.Fatalf("reload event: %v", err)
	}
	if got.Frequency != "annual" || got.CalendarSystem != "gregorian" {
		t.Fatalf("event changed despite validation errors: %q/%q", got.Frequency, got.CalendarSystem)
	}
}

// TestEditEvent_UnknownID ensures both edit handlers 404 for a missing event.
func TestEditEvent_UnknownID(t *testing.T) {
	h := newTestWebHandler(t)

	if w := serveEventRequest(h, "GET", "/events/9999/edit", ""); w.Code != http.StatusNotFound {
		t.Fatalf("GET unknown event: expected 404, got %d", w.Code)
	}
	if w := serveEventRequest(h, "POST", "/events/9999/edit", "type=custom&date=2026-01-01"); w.Code != http.StatusNotFound {
		t.Fatalf("POST unknown event: expected 404, got %d", w.Code)
	}
}

// TestEditEvent_HebrewMonth13RoundTrip ensures Adar II stores as month 12 +
// leap and is shown as month 13 again.
func TestEditEvent_HebrewMonth13RoundTrip(t *testing.T) {
	h := newTestWebHandler(t)
	ctx := context.Background()

	p, err := h.client.Person.Create().
		SetName("Hebrew Edit").
		SetNotes("test").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	ev, err := h.events.CreateForPersonWithSchedule(ctx, p.ID, "memorial",
		time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC), "", "hebrew",
		intPtr(12), intPtr(10), true,
		repository.NewEventSchedule("annual", 1, nil, nil))
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	w := serveEventRequest(h, "GET", "/events/"+itoa(ev.ID)+"/edit", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET edit: expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		`value="13" selected`,
		`value="10" selected`,
		`id="lunar_leap" name="lunar_leap" value="on" aria-label="Leap month" checked`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("edit form missing %q", want)
		}
	}

	// Round-trip an unrelated change and confirm month 12 + leap is preserved.
	w = serveEventRequest(h, "POST", "/events/"+itoa(ev.ID)+"/edit",
		"type=memorial&date=2026-03-03&calendar_system=hebrew&lunar_month=13&lunar_day=10&lunar_leap=on&frequency=annual")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("POST edit: expected 303, got %d: %s", w.Code, w.Body.String())
	}
	got, err := h.events.Get(ctx, ev.ID)
	if err != nil {
		t.Fatalf("reload event: %v", err)
	}
	if got.LunarMonth == nil || *got.LunarMonth != 12 || !got.LunarLeap {
		t.Fatalf("stored hebrew leap = %v/%v, want month 12 + leap", got.LunarMonth, got.LunarLeap)
	}
}

func intPtr(v int) *int { return &v }
