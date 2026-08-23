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

func setupStatsRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(h.Auth)
		r.Get("/stats", h.statsPage)
	})
	return r
}

func TestStatsAuth(t *testing.T) {
	h := newTestWebHandler(t)
	// authed 200 - call handler directly with user context (bypassing cookie auth)
	req := httptest.NewRequest("GET", "/stats", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	h.statsPage(w, req)
	if w.Code != http.StatusOK {
		body := w.Body.String()
		if len(body) > 500 {
			body = body[:500]
		}
		t.Fatalf("authed expected 200 got %d body %s", w.Code, body)
	}
	if !strings.Contains(w.Body.String(), "Stats Dashboard") {
		t.Error("expected Stats Dashboard in body")
	}

	// unauthed redirect via router Auth middleware
	router := setupStatsRouter(h)
	req2 := httptest.NewRequest("GET", "/stats", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusSeeOther {
		t.Fatalf("unauthed expected redirect got %d", w2.Code)
	}
	if loc := w2.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected /login redirect got %q", loc)
	}
}

func TestStatsEmptyState(t *testing.T) {
	h := newTestWebHandler(t)
	req := httptest.NewRequest("GET", "/stats", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	h.statsPage(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	body := w.Body.String()
	// each panel shows empty-state
	if count := strings.Count(body, "empty-state"); count < 5 {
		t.Errorf("expected >=5 empty-state, got %d body %s", count, body[:1000])
	}
}

func TestStatsWithData(t *testing.T) {
	h := newTestWebHandler(t)
	ctx := context.Background()
	// Create people with birthdays
	p1, _ := h.client.Person.Create().SetName("Alice").SetNotes("").SetCreatedAt(time.Now()).SetUpdatedAt(time.Now()).Save(ctx)
	p2, _ := h.client.Person.Create().SetName("Bob").SetNotes("").SetCreatedAt(time.Now()).SetUpdatedAt(time.Now()).Save(ctx)
	p3, _ := h.client.Person.Create().SetName("Unknown Person").SetNotes("").SetCreatedAt(time.Now()).SetUpdatedAt(time.Now()).Save(ctx)
	_ = p3
	// Age buckets: Alice born 1990-01-15 (should be 30-50), Bob born Feb 29 2000 (leap day)
	_, _ = h.client.Event.Create().SetType("birthday").SetDate(time.Date(1990, 1, 15, 0, 0, 0, 0, time.UTC)).SetCreatedAt(time.Now()).SetPersonID(p1.ID).Save(ctx)
	_, _ = h.client.Event.Create().SetType("birthday").SetDate(time.Date(2000, 2, 29, 0, 0, 0, 0, time.UTC)).SetCreatedAt(time.Now()).SetPersonID(p2.ID).Save(ctx)
	// Year-less birthday
	_, _ = h.client.Event.Create().SetType("birthday").SetDate(time.Date(1, 6, 8, 0, 0, 0, 0, time.UTC)).SetCreatedAt(time.Now()).SetPersonID(p3.ID).Save(ctx)

	// Also create a normal event for histogram
	p4, _ := h.client.Person.Create().SetName("Carol").SetNotes("").SetCreatedAt(time.Now()).SetUpdatedAt(time.Now()).Save(ctx)
	_, _ = h.client.Event.Create().SetType("anniversary").SetDate(time.Date(2010, 6, 15, 0, 0, 0, 0, time.UTC)).SetCreatedAt(time.Now()).SetPersonID(p4.ID).Save(ctx)

	req := httptest.NewRequest("GET", "/stats", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	h.statsPage(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	body := w.Body.String()
	// Age distribution panel present
	if !strings.Contains(body, "Age Distribution") {
		t.Error("missing Age Distribution")
	}
	if !strings.Contains(body, "unknown") {
		t.Error("missing unknown bucket")
	}
	// CSS width bars
	if !strings.Contains(body, `width:`) {
		t.Error("expected CSS width bars")
	}
	// Busiest months
	if !strings.Contains(body, "Busiest Birthday Months") {
		t.Error("missing Busiest Birthday Months")
	}
	// Events Per Month
	if !strings.Contains(body, "Events Per Month") {
		t.Error("missing Events Per Month")
	}
	// Upcoming milestones panel exists
	if !strings.Contains(body, "Upcoming Milestones") {
		t.Error("missing Upcoming Milestones")
	}
	// Missed recently
	if !strings.Contains(body, "Missed Recently") {
		t.Error("missing Missed Recently")
	}
}

func TestStatsLeapDayNoPanic(t *testing.T) {
	h := newTestWebHandler(t)
	ctx := context.Background()
	p, _ := h.client.Person.Create().SetName("Leap").SetNotes("").SetCreatedAt(time.Now()).SetUpdatedAt(time.Now()).Save(ctx)
	_, _ = h.client.Event.Create().SetType("birthday").SetDate(time.Date(2000, 2, 29, 0, 0, 0, 0, time.UTC)).SetCreatedAt(time.Now()).SetPersonID(p.ID).Save(ctx)
	req := httptest.NewRequest("GET", "/stats", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	h.statsPage(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("leap day expected 200 got %d", w.Code)
	}
}
