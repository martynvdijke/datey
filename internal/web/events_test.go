package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func setupEventsRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/people/{id}/events/new", h.newEventForm)
	r.Post("/people/{id}/events/new", h.createEvent)
	r.Post("/events/{id}/delete", h.deleteEvent)
	return r
}

func TestNewEventForm(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupEventsRouter(h)

	req := httptest.NewRequest("GET", "/people/1/events/new", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Add Event") {
		t.Errorf("expected Add Event form, got: %s", w.Body.String()[:200])
	}
}

func TestCreateEvent_InvalidDate(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupEventsRouter(h)

	body := "type=birthday&date=invalid&description=test"
	req := httptest.NewRequest("POST", "/people/1/events/new", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateEvent_InvalidContactID(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupEventsRouter(h)

	// chi still routes "abc" as {id} with value "abc"
	req := httptest.NewRequest("POST", "/people/abc/events/new", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid contact ID, got %d", w.Code)
	}
}

func TestCreateEvent_Success(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupEventsRouter(h)

	ctx := context.Background()

	// Create a person first
	person, err := h.client.Person.Create().
		SetName("Event Test").
		SetNotes("test").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	body := "type=birthday&date=2026-07-04&description=Test+event"
	req := httptest.NewRequest("POST", "/people/"+itoa(person.ID)+"/events/new", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	expected := "/people/" + itoa(person.ID)
	if loc != expected {
		t.Errorf("expected redirect to %s, got %s", expected, loc)
	}
}

func TestDeleteEvent(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupEventsRouter(h)

	ctx := context.Background()

	// Create a person and event
	person, err := h.client.Person.Create().
		SetName("Event Delete").
		SetNotes("test").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	event, err := h.client.Event.Create().
		SetType("birthday").
		SetDate(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)).
		SetDescription("to delete").
		SetCreatedAt(time.Now()).
		SetPersonID(person.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	req := httptest.NewRequest("POST", "/events/"+itoa(event.ID)+"/delete", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Verify it's gone
	_, err = h.client.Event.Get(ctx, event.ID)
	if err == nil {
		t.Errorf("event should have been deleted")
	}
}
