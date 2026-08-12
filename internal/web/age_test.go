package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/internal/age"
	"github.com/go-chi/chi/v5"
)

func setupAgeRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/people/{id}", h.viewPerson)
	r.Get("/people", h.listPeople)
	return r
}

func TestPersonDetail_ShowsAgeForBirthdayWithYear(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupAgeRouter(h)

	p, err := h.people.Create(context.Background(), "Dana Vreede", "", "")
	if err != nil {
		t.Fatal(err)
	}
	birthDate := time.Date(1996, 8, 12, 0, 0, 0, 0, time.UTC)
	if _, err := h.events.CreateForPerson(context.Background(), p.ID, "birthday", birthDate, ""); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", fmt.Sprintf("/people/%d", p.ID), nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	exp, ok := age.AgeAt(birthDate, time.Now())
	if !ok {
		t.Fatal("expected a derivable age for a dated birthday")
	}
	if want := fmt.Sprintf("Age %d", exp); !strings.Contains(w.Body.String(), want) {
		t.Errorf("expected %q in person detail, got:\n%s", want, w.Body.String())
	}
}

func TestPersonDetail_NoAgeForYearlessBirthday(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupAgeRouter(h)

	p, err := h.people.Create(context.Background(), "No Year Person", "", "")
	if err != nil {
		t.Fatal(err)
	}
	// Year 0001 = date-only entry without a usable birth year.
	if _, err := h.events.CreateForPerson(context.Background(), p.ID, "birthday",
		time.Date(1, 8, 12, 0, 0, 0, 0, time.UTC), ""); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", fmt.Sprintf("/people/%d", p.ID), nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "Age ") {
		t.Errorf("expected no age text for a yearless birthday, got:\n%s", body)
	}
	if !strings.Contains(body, "No Year Person") {
		t.Errorf("expected the person page to still render, got:\n%s", body)
	}
}

func TestPersonDetail_YearlessBirthdayShowsMonthDayOnly(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupAgeRouter(h)

	p, err := h.people.Create(context.Background(), "No Year Person", "", "")
	if err != nil {
		t.Fatal(err)
	}
	// Year 0 = what ParseBDAY produces for year-less vCard values (--0812).
	if _, err := h.events.CreateForPerson(context.Background(), p.ID, "birthday",
		time.Date(0, 8, 12, 0, 0, 0, 0, time.UTC), ""); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", fmt.Sprintf("/people/%d", p.ID), nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "12 Aug") {
		t.Errorf("expected day-first '12 Aug' in person detail (european default), got:\n%s", body)
	}
	if strings.Contains(body, "12 Aug 1") {
		t.Errorf("expected no year in date for a yearless birthday, got:\n%s", body)
	}
	if strings.Contains(body, "Age ") {
		t.Errorf("expected no age text for a yearless birthday, got:\n%s", body)
	}
}

func TestPeopleList_NoAgeForYearlessBirthday(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupAgeRouter(h)

	p, err := h.people.Create(context.Background(), "No Year Person", "", "")
	if err != nil {
		t.Fatal(err)
	}
	// Year 0 = what ParseBDAY produces for year-less vCard values (--0812).
	if _, err := h.events.CreateForPerson(context.Background(), p.ID, "birthday",
		time.Date(0, 8, 12, 0, 0, 0, 0, time.UTC), ""); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/people", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "No Year Person") {
		t.Errorf("expected the person to appear in people list, got:\n%s", body)
	}
	if strings.Contains(body, `<span class="fw-normal text-muted">`) {
		t.Errorf("expected no age span for a yearless birthday, got:\n%s", body)
	}
}

func TestDashboard_NoAgeForYearlessBirthday(t *testing.T) {
	// The dashboard gates age display on AgeInfo.HasAge, computed via
	// age.InfoFor(e.Date, now) for birthday events. A year-less date must
	// never produce an age there.
	info := age.InfoFor(time.Date(0, 8, 12, 0, 0, 0, 0, time.UTC), time.Now())
	if info.HasAge {
		t.Errorf("expected HasAge=false for a yearless birthday, got %+v", info)
	}
}

func TestPeopleList_ShowsAge(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupAgeRouter(h)

	p, err := h.people.Create(context.Background(), "Dana Vreede", "", "")
	if err != nil {
		t.Fatal(err)
	}
	birthDate := time.Date(1996, 8, 12, 0, 0, 0, 0, time.UTC)
	if _, err := h.events.CreateForPerson(context.Background(), p.ID, "birthday", birthDate, ""); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/people", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	exp, ok := age.AgeAt(birthDate, time.Now())
	if !ok {
		t.Fatal("expected a derivable age for a dated birthday")
	}
	body := w.Body.String()
	if !strings.Contains(body, "Dana Vreede") {
		t.Errorf("expected the person to appear in people list, got:\n%s", body)
	}
	if want := fmt.Sprintf(`<span class="fw-normal text-muted">%d</span>`, exp); !strings.Contains(body, want) {
		t.Errorf("expected %q in people list, got:\n%s", want, body)
	}
}
