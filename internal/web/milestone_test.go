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

func TestDashboardMilestoneBadge(t *testing.T) {
	h := newTestWebHandler(t)
	ctx := context.Background()
	person, err := h.client.Person.Create().SetName("Milestone Person").SetNotes("").SetCreatedAt(time.Now()).SetUpdatedAt(time.Now()).Save(ctx)
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	now := time.Now()
	birthDate := time.Date(now.Year()-30, now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	// shift a day ahead to ensure occurrence in window
	if birthDate.After(time.Now()) {
		birthDate = birthDate.AddDate(0, 0, -1)
	}
	_, err = h.client.Event.Create().SetType("birthday").SetDate(birthDate).SetCreatedAt(time.Now()).SetPersonID(person.ID).Save(ctx)
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	// need route that calls dashboard
	r := chi.NewRouter()
	r.Get("/", h.dashboard)
	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("dashboard status %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "30th birthday") {
		t.Errorf("expected milestone badge in dashboard, body missing: %s", body[:2000])
	}
	// should also have milestones this year summary
	if !strings.Contains(body, "Milestones this year") {
		t.Errorf("expected milestones summary")
	}
}

func TestCalendarMilestoneBadge(t *testing.T) {
	h := newTestWebHandler(t)
	r := setupCalendarRouter(h)
	ctx := context.Background()
	person, _ := h.client.Person.Create().SetName("Cal Milestone").SetNotes("").SetCreatedAt(time.Now()).SetUpdatedAt(time.Now()).Save(ctx)
	_, _ = h.client.Event.Create().SetType("birthday").SetDate(time.Date(1996, 6, 15, 0, 0, 0, 0, time.UTC)).SetCreatedAt(time.Now()).SetPersonID(person.ID).Save(ctx)
	req := httptest.NewRequest("GET", "/api/calendar-events?start=2026-06-01&end=2026-06-30", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var events []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&events); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event got %d", len(events))
	}
	if events[0]["milestoneLabel"] != "30th birthday" {
		t.Errorf("expected milestoneLabel 30th birthday got %v", events[0]["milestoneLabel"])
	}
}

func TestPersonDetailMilestoneBadge(t *testing.T) {
	h := newTestWebHandler(t)
	ctx := context.Background()
	person, _ := h.client.Person.Create().SetName("Person Detail Milestone").SetNotes("").SetCreatedAt(time.Now()).SetUpdatedAt(time.Now()).Save(ctx)
	now := time.Now()
	// anniversary 25 years ago -> should be milestone
	start := time.Date(now.Year()-25, 6, 15, 0, 0, 0, 0, time.UTC)
	_, _ = h.client.Event.Create().SetType("anniversary").SetDate(start).SetCreatedAt(time.Now()).SetPersonID(person.ID).Save(ctx)
	r := chi.NewRouter()
	r.Get("/people/{id}", h.viewPerson)
	req := httptest.NewRequest("GET", "/people/"+itoa(person.ID), nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "25th anniversary") {
		t.Errorf("expected anniversary milestone badge")
	}
}
