package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func setupReminderDaysRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/people/new", h.newPersonForm)
	r.Post("/people/new", h.createPerson)
	r.Get("/people/{id}/edit", h.editPersonForm)
	r.Post("/people/{id}/edit", h.updatePerson)
	return r
}

func postReminderForm(t *testing.T, router chi.Router, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestCreatePerson_ValidReminderDaysPersists(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupReminderDaysRouter(h)

	w := postReminderForm(t, router, "/people/new", url.Values{
		"first_name":    {"Alice"},
		"reminder_days": {"14, 3"},
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d: %s", w.Code, w.Body.String())
	}

	p, err := h.people.FindByName(context.Background(), "Alice")
	if err != nil {
		t.Fatalf("find created person: %v", err)
	}
	if got := p.ReminderDays; len(got) != 2 || got[0] != 3 || got[1] != 14 {
		t.Fatalf("ReminderDays = %v, want [3 14]", got)
	}
}

func TestCreatePerson_InvalidReminderDaysRerenders(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupReminderDaysRouter(h)

	w := postReminderForm(t, router, "/people/new", url.Values{
		"first_name":    {"Bob"},
		"reminder_days": {"banana"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (form re-rendered), got %d", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, "Reminder days must be") {
		t.Fatalf("expected inline reminder_days error, got: %s", body)
	}
	if _, err := h.people.FindByName(context.Background(), "Bob"); err == nil {
		t.Fatal("invalid reminder days must not create the person")
	}
}

func TestUpdatePerson_ReminderDaysOverrideAndClear(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupReminderDaysRouter(h)
	person := seedPerson(t, h, "Alice")

	w := postReminderForm(t, router, "/people/"+itoa(person.ID)+"/edit", url.Values{
		"first_name":    {"Alice"},
		"reminder_days": {"30"},
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d: %s", w.Code, w.Body.String())
	}
	updated, err := h.people.Get(context.Background(), person.ID)
	if err != nil {
		t.Fatalf("get person: %v", err)
	}
	if got := updated.ReminderDays; len(got) != 1 || got[0] != 30 {
		t.Fatalf("ReminderDays = %v, want [30]", got)
	}

	// A blank field clears the override again.
	w = postReminderForm(t, router, "/people/"+itoa(person.ID)+"/edit", url.Values{
		"first_name":    {"Alice"},
		"reminder_days": {""},
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d: %s", w.Code, w.Body.String())
	}
	cleared, err := h.people.Get(context.Background(), person.ID)
	if err != nil {
		t.Fatalf("get person: %v", err)
	}
	if len(cleared.ReminderDays) != 0 {
		t.Fatalf("ReminderDays = %v, want empty", cleared.ReminderDays)
	}
}

func TestUpdatePerson_InvalidReminderDaysRerendersWithoutPersisting(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupReminderDaysRouter(h)
	person := seedPerson(t, h, "Alice")

	w := postReminderForm(t, router, "/people/"+itoa(person.ID)+"/edit", url.Values{
		"first_name":    {"Alice"},
		"reminder_days": {"0"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (form re-rendered), got %d", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, "Reminder days must be") {
		t.Fatalf("expected inline reminder_days error, got: %s", body)
	}
	persisted, err := h.people.Get(context.Background(), person.ID)
	if err != nil {
		t.Fatalf("get person: %v", err)
	}
	if len(persisted.ReminderDays) != 0 {
		t.Fatalf("invalid reminder days must not persist, got %v", persisted.ReminderDays)
	}
}
