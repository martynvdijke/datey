package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func setupPeopleNameRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/people/new", h.newPersonForm)
	r.Post("/people/new", h.createPerson)
	r.Get("/people/{id}/edit", h.editPersonForm)
	r.Post("/people/{id}/edit", h.updatePerson)
	r.Get("/people/{id}", h.viewPerson)
	return r
}

func postForm(t *testing.T, router chi.Router, path string, form map[string][]string) *httptest.ResponseRecorder {
	t.Helper()
	body := strings.NewReader(formEncode(form))
	req := httptest.NewRequest("POST", path, body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// --- Create with structured name ---

func TestCreatePerson_WithMiddleName(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleNameRouter(h)

	w := postForm(t, router, "/people/new", map[string][]string{
		"first_name":  {"John"},
		"middle_name": {"Q"},
		"last_name":   {"Public"},
	})

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", w.Code)
	}
	p, err := h.people.FindByName(context.Background(), "John Q Public")
	if err != nil {
		t.Fatalf("person not created: %v", err)
	}
	if p.FirstName == nil || *p.FirstName != "John" {
		t.Errorf("first_name = %v, want John", p.FirstName)
	}
	if p.MiddleName == nil || *p.MiddleName != "Q" {
		t.Errorf("middle_name = %v, want Q", p.MiddleName)
	}
	if p.LastName == nil || *p.LastName != "Public" {
		t.Errorf("last_name = %v, want Public", p.LastName)
	}
	if p.Name != "John Q Public" {
		t.Errorf("display name = %q, want %q", p.Name, "John Q Public")
	}
}

func TestCreatePerson_LegacySingleNameFallback(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleNameRouter(h)

	w := postForm(t, router, "/people/new", map[string][]string{
		"name": {"Solo"},
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", w.Code)
	}
	if _, err := h.people.FindByName(context.Background(), "Solo"); err != nil {
		t.Fatalf("legacy name-only create failed: %v", err)
	}
}

func TestCreatePerson_EmptyNameRejected(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleNameRouter(h)

	w := postForm(t, router, "/people/new", map[string][]string{
		"first_name":  {""},
		"middle_name": {""},
		"last_name":   {""},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 re-render, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Name is required") {
		t.Error("expected 'Name is required' error in body")
	}
}

func TestCreatePerson_WhitespaceOnlyRejected(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleNameRouter(h)

	w := postForm(t, router, "/people/new", map[string][]string{
		"first_name": {"   "},
		"last_name":  {"  "},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 re-render, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Name is required") {
		t.Error("expected 'Name is required' error for whitespace-only input")
	}
}

// --- Edit structured name ---

func TestUpdatePerson_RemoveMiddleName(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleNameRouter(h)

	person := seedPerson(t, h, "Jane Alice Doe")
	if _, err := h.people.UpdateStructured(context.Background(), person.ID, "Jane Alice Doe", "Jane", "Alice", "Doe", "", ""); err != nil {
		t.Fatalf("seed structured name: %v", err)
	}

	w := postForm(t, router, "/people/"+itoa(person.ID)+"/edit", map[string][]string{
		"first_name":  {"Jane"},
		"middle_name": {""},
		"last_name":   {"Doe"},
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", w.Code)
	}
	updated, err := h.people.Get(context.Background(), person.ID)
	if err != nil {
		t.Fatalf("get person: %v", err)
	}
	if updated.MiddleName != nil && *updated.MiddleName != "" {
		t.Errorf("middle_name = %v, want cleared", updated.MiddleName)
	}
	if updated.Name != "Jane Doe" {
		t.Errorf("display name = %q, want %q", updated.Name, "Jane Doe")
	}
}

func TestUpdatePerson_DuplicateDisplayNameRejected(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleNameRouter(h)

	seedPerson(t, h, "John Doe")
	target := seedPerson(t, h, "Other Person")

	w := postForm(t, router, "/people/"+itoa(target.ID)+"/edit", map[string][]string{
		"first_name": {"John"},
		"last_name":  {"Doe"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 re-render, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "A person with this name already exists") {
		t.Error("expected duplicate-name field error in body")
	}
	// Entered values preserved on error.
	if !strings.Contains(w.Body.String(), `value="John"`) || !strings.Contains(w.Body.String(), `value="Doe"`) {
		t.Error("expected entered first/last values preserved in re-rendered form")
	}
}

func TestUpdatePerson_CarddavPendingSyncFlag(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleNameRouter(h)

	person := seedPerson(t, h, "Synced Person")
	if _, err := h.people.SetCarddavState(context.Background(), person.ID, "uid-1", "href-1", "", "", nil, false); err != nil {
		t.Fatalf("seed carddav state: %v", err)
	}

	postForm(t, router, "/people/"+itoa(person.ID)+"/edit", map[string][]string{
		"first_name": {"Renamed"},
		"last_name":  {"Person"},
	})

	updated, err := h.people.Get(context.Background(), person.ID)
	if err != nil {
		t.Fatalf("get person: %v", err)
	}
	if !updated.CarddavPendingSync {
		t.Error("expected carddav_pending_sync to be set after local edit")
	}
}

// --- Edit form prefill ---

func TestEditForm_PrefillFromStructuredFields(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleNameRouter(h)

	person := seedPerson(t, h, "Ada Lovelace")
	if _, err := h.people.UpdateStructured(context.Background(), person.ID, "Ada Lovelace", "Ada", "", "Lovelace", "", ""); err != nil {
		t.Fatalf("seed structured name: %v", err)
	}

	req := httptest.NewRequest("GET", "/people/"+itoa(person.ID)+"/edit", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{`id="first_name"`, `value="Ada"`, `value="Lovelace"`} {
		if !strings.Contains(body, want) {
			t.Errorf("expected body to contain %q", want)
		}
	}
}

func TestEditForm_LegacyNameSplitPrefill(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleNameRouter(h)

	// Legacy row: display name only, no structured parts (simulates
	// pre-migration data).
	person := seedPerson(t, h, "Mary Jane Watson")

	req := httptest.NewRequest("GET", "/people/"+itoa(person.ID)+"/edit", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{`value="Mary"`, `value="Jane Watson"`} {
		if !strings.Contains(body, want) {
			t.Errorf("expected heuristic split prefill to contain %q", want)
		}
	}
}
