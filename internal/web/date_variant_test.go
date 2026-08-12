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

// upcomingDec25 returns a Dec 25 occurrence date in the future so the seeded
// event is guaranteed to show up in every page regardless of when the test
// runs.
func upcomingDec25(t *testing.T) time.Time {
	t.Helper()
	now := time.Now()
	cand := time.Date(now.Year(), 12, 25, 0, 0, 0, 0, now.Location())
	if !cand.After(now) {
		cand = cand.AddDate(1, 0, 0)
	}
	return cand
}

func setupVariantRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.dashboard)
	r.Get("/people", h.listPeople)
	r.Get("/calendar", h.calendarPage)
	return r
}

func TestDateVariant_TRMNLFeed(t *testing.T) {
	for _, tc := range []struct {
		variant string
		want    string
	}{
		{"european", "25 Dec"},
		{"us", "Dec 25"},
	} {
		t.Run(tc.variant, func(t *testing.T) {
			h := newTestWebHandler(t)
			h.cfg.DateVariant = tc.variant
			h.cfg.ReminderDays = 365
			personID := newTestPerson(t, h, "Anna")
			newTestEvent(t, h, personID, "birthday", upcomingDec25(t))

			_, _, stats := getTRMNLStats(t, h)
			if stats.NextEvent == nil {
				t.Fatal("expected next_event to be set")
			}
			if stats.NextEvent.Date != tc.want {
				t.Errorf("date = %q, want %q", stats.NextEvent.Date, tc.want)
			}
		})
	}
}

func TestDateVariant_PeopleList(t *testing.T) {
	for _, tc := range []struct {
		variant string
		want    string
	}{
		{"european", "25 Dec"},
		{"us", "Dec 25"},
	} {
		t.Run(tc.variant, func(t *testing.T) {
			h := newTestWebHandler(t)
			h.cfg.DateVariant = tc.variant
			h.cfg.ReminderDays = 365
			router := setupVariantRouter(h)

			p, err := h.people.Create(context.Background(), "Anna", "", "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := h.events.CreateForPerson(context.Background(), p.ID, "birthday", upcomingDec25(t), ""); err != nil {
				t.Fatal(err)
			}

			req := httptest.NewRequest("GET", "/people", nil)
			req = req.WithContext(withUserContext(req.Context()))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", w.Code)
			}
			if !strings.Contains(w.Body.String(), tc.want) {
				t.Errorf("expected %q in people list body, got:\n%s", tc.want, w.Body.String())
			}
		})
	}
}

func TestDateVariant_CalendarPage(t *testing.T) {
	// The calendar page only lists the next 30 days, so seed an event inside
	// that window and derive the expected rendering from the same layouts.
	target := time.Now().AddDate(0, 0, 14)
	for _, tc := range []struct {
		variant string
		want    string
	}{
		{"european", target.Format("2 Jan 2006")},
		{"us", target.Format("Jan 2, 2006")},
	} {
		t.Run(tc.variant, func(t *testing.T) {
			h := newTestWebHandler(t)
			h.cfg.DateVariant = tc.variant
			h.cfg.ReminderDays = 365
			router := setupVariantRouter(h)

			p, err := h.people.Create(context.Background(), "Anna", "", "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := h.events.CreateForPerson(context.Background(), p.ID, "birthday", target, ""); err != nil {
				t.Fatal(err)
			}

			req := httptest.NewRequest("GET", "/calendar", nil)
			req = req.WithContext(withUserContext(req.Context()))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", w.Code)
			}
			if !strings.Contains(w.Body.String(), tc.want) {
				t.Errorf("expected %q in calendar body, got:\n%s", tc.want, w.Body.String())
			}
		})
	}
}

func TestDateVariant_Dashboard(t *testing.T) {
	for _, tc := range []struct {
		variant string
		want    string
	}{
		{"european", "25 Dec"},
		{"us", "Dec 25"},
	} {
		t.Run(tc.variant, func(t *testing.T) {
			h := newTestWebHandler(t)
			h.cfg.DateVariant = tc.variant
			h.cfg.ReminderDays = 365
			router := setupVariantRouter(h)

			p, err := h.people.Create(context.Background(), "Anna", "", "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := h.events.CreateForPerson(context.Background(), p.ID, "birthday", upcomingDec25(t), ""); err != nil {
				t.Fatal(err)
			}

			req := httptest.NewRequest("GET", "/", nil)
			req = req.WithContext(withUserContext(req.Context()))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", w.Code)
			}
			if !strings.Contains(w.Body.String(), tc.want) {
				t.Errorf("expected %q in dashboard body, got:\n%s", tc.want, w.Body.String())
			}
		})
	}
}
