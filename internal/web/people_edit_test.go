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

func setupPeopleEditRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/people/{id}/edit", h.editPersonForm)
	r.Post("/people/{id}/edit", h.updatePerson)
	r.Get("/people/{id}", h.viewPerson)
	return r
}

func postEditForm(t *testing.T, h *Handler, router chi.Router, personID int, form map[string][]string) *httptest.ResponseRecorder {
	t.Helper()
	body := strings.NewReader(formEncode(form))
	req := httptest.NewRequest("POST", "/people/"+itoa(personID)+"/edit", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// formEncode builds an application/x-www-form-urlencoded body supporting
// repeated keys.
func formEncode(form map[string][]string) string {
	parts := make([]string, 0)
	for _, key := range []string{"name", "first_name", "middle_name", "last_name", "notes", "birthday", "groups"} {
		for _, v := range form[key] {
			parts = append(parts, key+"="+strings.ReplaceAll(v, " ", "+"))
		}
	}
	return strings.Join(parts, "&")
}

func seedBirthdayEvent(t *testing.T, h *Handler, personID int, date time.Time) {
	t.Helper()
	if _, err := h.events.CreateForPerson(context.Background(), personID, "birthday", date, ""); err != nil {
		t.Fatalf("seed birthday event: %v", err)
	}
}

// --- 4.1 Edit form render ---

func TestEditPersonForm_Prefills(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleEditRouter(h)

	person := seedPerson(t, h, "Alice")
	if _, err := h.people.Update(context.Background(), person.ID, "Alice", "Best friend", ""); err != nil {
		t.Fatalf("set notes: %v", err)
	}
	group := seedGroup(t, h, "Family")
	if err := h.groups.AddPerson(context.Background(), group.ID, person.ID); err != nil {
		t.Fatalf("add to group: %v", err)
	}
	seedBirthdayEvent(t, h, person.ID, time.Date(1990, time.June, 15, 0, 0, 0, 0, time.UTC))

	req := httptest.NewRequest("GET", "/people/"+itoa(person.ID)+"/edit", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"Edit Person", `value="Alice"`, "Best friend", `value="1990-06-15"`, `id="g-` + itoa(group.ID) + `" checked`} {
		if !strings.Contains(body, want) {
			t.Errorf("expected body to contain %q", want)
		}
	}
}

func TestEditPersonForm_NotFound(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleEditRouter(h)

	req := httptest.NewRequest("GET", "/people/999999/edit", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestEditPersonForm_YearlessBirthdayNotPrefilled(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleEditRouter(h)

	person := seedPerson(t, h, "Yearless")
	seedBirthdayEvent(t, h, person.ID, time.Date(1, time.March, 8, 0, 0, 0, 0, time.UTC))

	req := httptest.NewRequest("GET", "/people/"+itoa(person.ID)+"/edit", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "0001-03-08") {
		t.Errorf("year-less birthday should not be prefilled")
	}
}

// --- 4.2 Update ---

func TestUpdatePerson_Success(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleEditRouter(h)

	person := seedPerson(t, h, "Alice")
	group := seedGroup(t, h, "Friends")

	w := postEditForm(t, h, router, person.ID, map[string][]string{
		"name":     {"Alicia"},
		"notes":    {"Updated notes"},
		"groups":   {itoa(group.ID)},
		"birthday": {""},
	})

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "/people/"+itoa(person.ID)) {
		t.Errorf("expected redirect to person page, got %q", loc)
	}
	updated, err := h.people.Get(context.Background(), person.ID)
	if err != nil {
		t.Fatalf("get updated person: %v", err)
	}
	if updated.Name != "Alicia" || updated.Notes != "Updated notes" {
		t.Errorf("unexpected saved values: %+v", updated)
	}
}

func TestUpdatePerson_EmptyNameRejected(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleEditRouter(h)

	person := seedPerson(t, h, "Alice")

	w := postEditForm(t, h, router, person.ID, map[string][]string{"name": {""}})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 re-render, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Name is required") {
		t.Errorf("expected 'Name is required' error, got: %s", w.Body.String()[:300])
	}
	same, _ := h.people.Get(context.Background(), person.ID)
	if same.Name != "Alice" {
		t.Errorf("person should be unchanged, got %q", same.Name)
	}
}

func TestUpdatePerson_DuplicateNameRejected(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleEditRouter(h)

	a := seedPerson(t, h, "Alice")
	b := seedPerson(t, h, "Bob")

	w := postEditForm(t, h, router, a.ID, map[string][]string{"name": {"Bob"}})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 re-render, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "already exists") {
		t.Errorf("expected duplicate-name error, got: %s", w.Body.String()[:400])
	}
	same, _ := h.people.Get(context.Background(), a.ID)
	if same.Name != "Alice" {
		t.Errorf("person should be unchanged, got %q", same.Name)
	}
	_ = b
}

func TestUpdatePerson_GroupMembershipSync(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleEditRouter(h)

	person := seedPerson(t, h, "Alice")
	g1 := seedGroup(t, h, "Family")
	g2 := seedGroup(t, h, "Friends")
	if err := h.groups.AddPerson(context.Background(), g1.ID, person.ID); err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	w := postEditForm(t, h, router, person.ID, map[string][]string{
		"name":   {"Alice"},
		"groups": {itoa(g2.ID)},
	})

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	member, _ := h.groups.ListByPerson(context.Background(), person.ID)
	if len(member) != 1 || member[0].ID != g2.ID {
		t.Errorf("expected membership in group %d only, got %+v", g2.ID, member)
	}
}

// --- 4.3 Birthday upsert ---

func TestUpdatePerson_BirthdayUpdatesExisting(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleEditRouter(h)

	person := seedPerson(t, h, "Alice")
	seedBirthdayEvent(t, h, person.ID, time.Date(1990, time.January, 1, 0, 0, 0, 0, time.UTC))

	w := postEditForm(t, h, router, person.ID, map[string][]string{
		"name":     {"Alice"},
		"birthday": {"1995-05-05"},
	})

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	events, _ := h.events.ListByPerson(context.Background(), person.ID)
	if len(events) != 1 {
		t.Fatalf("expected exactly 1 birthday event, got %d", len(events))
	}
	want := time.Date(1995, time.May, 5, 0, 0, 0, 0, time.UTC)
	if !events[0].Date.Equal(want) {
		t.Errorf("expected birthday %v, got %v", want, events[0].Date)
	}
}

func TestUpdatePerson_BirthdayCreatedWhenMissing(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleEditRouter(h)

	person := seedPerson(t, h, "Alice")

	w := postEditForm(t, h, router, person.ID, map[string][]string{
		"name":     {"Alice"},
		"birthday": {"2000-02-29"},
	})

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	events, _ := h.events.ListByPerson(context.Background(), person.ID)
	if len(events) != 1 || events[0].Type != "birthday" {
		t.Fatalf("expected one birthday event, got %+v", events)
	}
}

func TestUpdatePerson_EmptyBirthdayNoOp(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleEditRouter(h)

	person := seedPerson(t, h, "Alice")
	original := time.Date(1970, time.July, 21, 0, 0, 0, 0, time.UTC)
	seedBirthdayEvent(t, h, person.ID, original)

	w := postEditForm(t, h, router, person.ID, map[string][]string{
		"name":     {"Alice"},
		"birthday": {""},
	})

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	events, _ := h.events.ListByPerson(context.Background(), person.ID)
	if len(events) != 1 || !events[0].Date.Equal(original) {
		t.Errorf("existing birthday must remain unchanged, got %+v", events)
	}
}

func TestUpdatePerson_YearlessBirthdayPreserved(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPeopleEditRouter(h)

	person := seedPerson(t, h, "Alice")
	yearless := time.Date(1, time.March, 8, 0, 0, 0, 0, time.UTC)
	seedBirthdayEvent(t, h, person.ID, yearless)

	w := postEditForm(t, h, router, person.ID, map[string][]string{
		"name":     {"Alice Renamed"},
		"birthday": {""},
	})

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	events, _ := h.events.ListByPerson(context.Background(), person.ID)
	if len(events) != 1 || !events[0].Date.Equal(yearless) {
		t.Errorf("year-less birthday must be preserved, got %+v", events)
	}
}

// --- 4.4 Detail page: settings modal only surface for photos ---

func TestViewPerson_SettingsModalContainsPhotoFormsOnly(t *testing.T) {
	h := newTestWebHandler(t)
	h.cfg.DataDir = t.TempDir()
	router := setupPeopleEditRouter(h)

	person := seedPerson(t, h, "Alice")

	req := httptest.NewRequest("GET", "/people/"+itoa(person.ID), nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()

	if !strings.Contains(body, `id="personSettingsModal"`) {
		t.Error("expected settings modal markup on detail page")
	}
	if !strings.Contains(body, `data-bs-target="#personSettingsModal"`) {
		t.Error("expected Settings button wired to the modal")
	}
	if !strings.Contains(body, `href="/people/`+itoa(person.ID)+`/edit"`) {
		t.Error("expected Edit button linking to the edit page")
	}
	// Photo forms exist exactly once (inside the modal).
	if got := strings.Count(body, `action="/people/`+itoa(person.ID)+`/photo/upload"`); got != 1 {
		t.Errorf("expected exactly 1 photo upload form, got %d", got)
	}
	if got := strings.Count(body, `action="/people/`+itoa(person.ID)+`/immich"`); got != 1 {
		t.Errorf("expected exactly 1 immich form, got %d", got)
	}
}
