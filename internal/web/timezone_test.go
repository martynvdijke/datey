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

// setupPeopleCreateRouter wires only the person-create endpoints used by the
// timezone form tests.
func setupPeopleCreateRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/people/new", h.newPersonForm)
	r.Post("/people/new", h.createPerson)
	return r
}

// postPersonForm posts an arbitrary person form (including the timezone fields
// the shared formEncode helper does not know about).
func postPersonForm(t *testing.T, h *Handler, router chi.Router, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestPersonCreateStoresTimezone(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleCreateRouter(h)

	w := postPersonForm(t, h, router, "/people/new", url.Values{
		"first_name": {"Dana"},
		"timezone":   {"Europe/Berlin"},
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", w.Code, w.Body.String())
	}

	p, err := h.people.FindByName(context.Background(), "Dana")
	if err != nil {
		t.Fatalf("find person: %v", err)
	}
	if p.Timezone != "Europe/Berlin" {
		t.Errorf("timezone = %q, want Europe/Berlin", p.Timezone)
	}
}

func TestPersonCreateAcceptsCustomTimezoneFallback(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleCreateRouter(h)

	w := postPersonForm(t, h, router, "/people/new", url.Values{
		"first_name":      {"Yuki"},
		"timezone":        {"__custom__"},
		"timezone_custom": {"Asia/Tokyo"},
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", w.Code, w.Body.String())
	}
	p, err := h.people.FindByName(context.Background(), "Yuki")
	if err != nil {
		t.Fatalf("find person: %v", err)
	}
	if p.Timezone != "Asia/Tokyo" {
		t.Errorf("timezone = %q, want Asia/Tokyo", p.Timezone)
	}
}

func TestPersonCreateRejectsInvalidTimezone(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleCreateRouter(h)

	w := postPersonForm(t, h, router, "/people/new", url.Values{
		"first_name": {"Nobody"},
		"timezone":   {"Mars/Olympus_Mons"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (re-rendered form)", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, "Unknown time zone") {
		t.Errorf("expected inline timezone error, got: %s", body)
	}
	if _, err := h.people.FindByName(context.Background(), "Nobody"); err == nil {
		t.Error("person was created despite the invalid timezone")
	}
}

func TestPersonCreateEmptyTimezoneInherits(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleCreateRouter(h)

	w := postPersonForm(t, h, router, "/people/new", url.Values{
		"first_name": {"Inherit"},
		"timezone":   {""},
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", w.Code, w.Body.String())
	}
	p, err := h.people.FindByName(context.Background(), "Inherit")
	if err != nil {
		t.Fatalf("find person: %v", err)
	}
	if p.Timezone != "" {
		t.Errorf("timezone = %q, want empty (inherit)", p.Timezone)
	}
}

func TestPersonFormRendersTimezoneSelect(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleCreateRouter(h)

	req := httptest.NewRequest("GET", "/people/new", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		`name="timezone"`,
		`name="timezone_custom"`,
		`value="Europe/Berlin"`,
		"Use instance default",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("form is missing %q", want)
		}
	}
}

func TestPersonEditUpdatesAndClearsTimezone(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleEditRouter(h)
	person := seedPerson(t, h, "Alice")

	w := postPersonForm(t, h, router, "/people/"+itoa(person.ID)+"/edit", url.Values{
		"first_name": {"Alice"},
		"timezone":   {"America/New_York"},
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", w.Code, w.Body.String())
	}
	p, err := h.people.Get(context.Background(), person.ID)
	if err != nil {
		t.Fatalf("reload person: %v", err)
	}
	if p.Timezone != "America/New_York" {
		t.Fatalf("timezone = %q, want America/New_York", p.Timezone)
	}

	// An empty selection clears the override so the instance default applies.
	w = postPersonForm(t, h, router, "/people/"+itoa(person.ID)+"/edit", url.Values{
		"first_name": {"Alice"},
		"timezone":   {""},
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", w.Code, w.Body.String())
	}
	p, err = h.people.Get(context.Background(), person.ID)
	if err != nil {
		t.Fatalf("reload person: %v", err)
	}
	if p.Timezone != "" {
		t.Errorf("timezone = %q, want empty after clearing", p.Timezone)
	}
}

func TestPersonEditRejectsInvalidTimezone(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleEditRouter(h)
	person := seedPerson(t, h, "Alice")

	w := postPersonForm(t, h, router, "/people/"+itoa(person.ID)+"/edit", url.Values{
		"first_name": {"Alice"},
		"timezone":   {"Nowhere/Special"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (re-rendered form)", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, "Unknown time zone") {
		t.Errorf("expected inline timezone error, got: %s", body)
	}
	p, err := h.people.Get(context.Background(), person.ID)
	if err != nil {
		t.Fatalf("reload person: %v", err)
	}
	if p.Timezone != "" {
		t.Errorf("timezone = %q, want unchanged (empty)", p.Timezone)
	}
}

func TestPersonDetailDisplaysTimezone(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleEditRouter(h)
	person := seedPerson(t, h, "Alice")
	if _, err := h.people.SetTimezone(context.Background(), person.ID, "Australia/Sydney"); err != nil {
		t.Fatalf("set timezone: %v", err)
	}

	req := httptest.NewRequest("GET", "/people/"+itoa(person.ID), nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, "Australia/Sydney") {
		t.Errorf("person detail does not show the timezone")
	}
}

func TestPersonDetailShowsInheritedTimezone(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleEditRouter(h)
	person := seedPerson(t, h, "Bob")

	req := httptest.NewRequest("GET", "/people/"+itoa(person.ID), nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, "Instance default") {
		t.Errorf("person detail does not mark the inherited timezone")
	}
}
