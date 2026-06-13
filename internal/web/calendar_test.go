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
