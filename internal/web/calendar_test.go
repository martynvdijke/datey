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

func setupCalendarRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/calendar", h.calendarPage)
	r.Get("/api/calendar-events", h.calendarEvents)
	return r
}

func TestCalendarPage(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCalendarRouter(h)

	req := httptest.NewRequest("GET", "/calendar", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Calendar") {
		t.Errorf("expected Calendar in body, got: %s", w.Body.String()[:200])
	}
}

func TestCalendarEvents_Empty(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCalendarRouter(h)

	req := httptest.NewRequest("GET", "/api/calendar-events?start=2026-01-01&end=2026-12-31", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("expected JSON content type, got %s", contentType)
	}

	var events []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&events); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected empty array, got %d items", len(events))
	}
}

func TestCalendarEvents_WithData(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCalendarRouter(h)

	ctx := context.Background()

	// Create a contact and event
	contact, err := h.client.Contact.Create().
		SetName("Calendar Test").
		SetNotes("test").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}

	_, err = h.client.Event.Create().
		SetType("birthday").
		SetDate(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)).
		SetDescription("Fourth of July").
		SetCreatedAt(time.Now()).
		SetContactID(contact.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/calendar-events?start=2026-07-01&end=2026-07-31", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var events []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&events); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0]["title"] != "Calendar Test - birthday" {
		t.Errorf("unexpected title: %v", events[0]["title"])
	}
	if events[0]["type"] != "birthday" {
		t.Errorf("unexpected type: %v", events[0]["type"])
	}
	if events[0]["start"] != "2026-07-04" {
		t.Errorf("unexpected start: %v", events[0]["start"])
	}
}

func TestCalendarEvents_PersonLinkedTitle(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCalendarRouter(h)

	ctx := context.Background()

	person, err := h.client.Person.Create().
		SetName("Ada Lovelace").
		SetNotes("test").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	_, err = h.client.Event.Create().
		SetType("birthday").
		SetDate(time.Date(1815, 12, 10, 0, 0, 0, 0, time.UTC)).
		SetDescription("Born in London").
		SetNotes("Share a toast").
		SetCreatedAt(time.Now()).
		SetPersonID(person.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/calendar-events?start=2026-12-01&end=2026-12-31", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var events []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&events); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event (annual expansion), got %d", len(events))
	}
	if events[0]["title"] != "Ada Lovelace - birthday" {
		t.Errorf("expected person-linked title, got %v", events[0]["title"])
	}
	// Annual expansion: historical 1815-12-10 renders on 2026-12-10.
	if events[0]["start"] != "2026-12-10" {
		t.Errorf("expected occurrence 2026-12-10, got %v", events[0]["start"])
	}
	// Description and notes ride along for the modal.
	if events[0]["description"] != "Born in London" {
		t.Errorf("unexpected description: %v", events[0]["description"])
	}
	if events[0]["notes"] != "Share a toast" {
		t.Errorf("unexpected notes: %v", events[0]["notes"])
	}
	// Chip class keyed by type for the design-system theming.
	className, ok := events[0]["className"].([]any)
	if !ok || len(className) != 1 || className[0] != "event-birthday" {
		t.Errorf("expected className [event-birthday], got %v", events[0]["className"])
	}
}

func TestCalendarEvents_AnnualExpansionYearBoundary(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCalendarRouter(h)

	ctx := context.Background()

	person, err := h.client.Person.Create().
		SetName("Dec Birthday").
		SetNotes("test").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	// Historical Dec 25 birthday; the range spans Dec 2025 -> Jan 2027 so it
	// should yield TWO occurrences (2025-12-25 and 2026-12-25).
	_, err = h.client.Event.Create().
		SetType("birthday").
		SetDate(time.Date(2000, 12, 25, 0, 0, 0, 0, time.UTC)).
		SetCreatedAt(time.Now()).
		SetPersonID(person.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/calendar-events?start=2025-12-01&end=2026-12-31", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var events []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&events); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 occurrences across year boundary, got %d", len(events))
	}
	if events[0]["start"] != "2025-12-25" {
		t.Errorf("expected first occurrence 2025-12-25, got %v", events[0]["start"])
	}
	if events[1]["start"] != "2026-12-25" {
		t.Errorf("expected second occurrence 2026-12-25, got %v", events[1]["start"])
	}
}

func TestCalendarEvents_NoOccurrenceInRange(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCalendarRouter(h)

	ctx := context.Background()

	person, err := h.client.Person.Create().
		SetName("Summer Person").
		SetNotes("test").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	// July 4 birthday; requested range is January — no occurrence.
	_, err = h.client.Event.Create().
		SetType("birthday").
		SetDate(time.Date(1990, 7, 4, 0, 0, 0, 0, time.UTC)).
		SetCreatedAt(time.Now()).
		SetPersonID(person.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/calendar-events?start=2026-01-01&end=2026-01-31", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var events []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&events); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events in range, got %d", len(events))
	}
}
