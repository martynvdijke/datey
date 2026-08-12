package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func setupNotifyBirthdaysRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/people/{id}", h.viewPerson)
	r.Post("/people/{id}/notify-birthdays", h.toggleNotifyBirthdays)
	return r
}

func TestPersonDetail_ShowsBirthdayToggleChecked(t *testing.T) {
	h := newTestWebHandler(t)
	id := newTestPerson(t, h, "Dana")

	router := setupNotifyBirthdaysRouter(h)
	req := httptest.NewRequest("GET", "/people/"+strconv.Itoa(id), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `name="notify_birthdays" value="on"`) {
		t.Error("expected notify_birthdays checkbox in person detail page")
	}
	if !strings.Contains(w.Body.String(), `id="notify-birthdays-toggle" checked`) {
		t.Error("expected toggle checked by default (notify_birthdays defaults true)")
	}
}

func TestToggleNotifyBirthdaysOptsOut(t *testing.T) {
	h := newTestWebHandler(t)
	id := newTestPerson(t, h, "Dana")

	router := setupNotifyBirthdaysRouter(h)
	form := url.Values{}
	// Checkbox unchecked → no notify_birthdays value is submitted.
	req := httptest.NewRequest("POST", "/people/"+strconv.Itoa(id)+"/notify-birthdays", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", w.Code)
	}

	person, err := h.people.Get(req.Context(), id)
	if err != nil {
		t.Fatalf("get person: %v", err)
	}
	if person.NotifyBirthdays {
		t.Error("expected notify_birthdays false after opting out")
	}
}

func TestToggleNotifyBirthdaysReEnables(t *testing.T) {
	h := newTestWebHandler(t)
	id := newTestPerson(t, h, "Dana")
	if _, err := h.people.SetNotifyBirthdays(context.Background(), id, false); err != nil {
		t.Fatalf("seed opt-out: %v", err)
	}

	router := setupNotifyBirthdaysRouter(h)
	form := url.Values{"notify_birthdays": {"on"}}
	req := httptest.NewRequest("POST", "/people/"+strconv.Itoa(id)+"/notify-birthdays", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", w.Code)
	}

	person, err := h.people.Get(req.Context(), id)
	if err != nil {
		t.Fatalf("get person: %v", err)
	}
	if !person.NotifyBirthdays {
		t.Error("expected notify_birthdays true after re-enabling")
	}
}
