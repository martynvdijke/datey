package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// TestCalendarEvents_HebrewAndHijri verifies that Hebrew and Hijri events flow
// through the shared occurrence expansion used by the calendar/API.
func TestCalendarEvents_HebrewAndHijri(t *testing.T) {
	h := newTestWebHandler(t)
	personID := newTestPerson(t, h, "Calendar Hebrew")

	hebrewMonth, hebrewDay := 7, 1 // Rosh Hashanah
	_, err := h.events.CreateForPersonWithCalendar(
		nilContext(), personID, "birthday", time.Date(1990, 9, 23, 0, 0, 0, 0, time.UTC), "",
		"hebrew", &hebrewMonth, &hebrewDay, false)
	if err != nil {
		t.Fatalf("create hebrew event: %v", err)
	}
	hijriMonth, hijriDay := 9, 1 // 1 Ramadan
	_, err = h.events.CreateForPersonWithCalendar(
		nilContext(), personID, "custom", time.Date(1990, 3, 1, 0, 0, 0, 0, time.UTC), "",
		"hijri", &hijriMonth, &hijriDay, false)
	if err != nil {
		t.Fatalf("create hijri event: %v", err)
	}

	router := setupCalendarRouter(h)
	req := httptest.NewRequest("GET", "/api/calendar-events?start=2025-01-01&end=2025-12-31", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var events []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &events); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := map[string]string{}
	for _, e := range events {
		got[e["type"].(string)] = e["start"].(string)
	}
	if got["birthday"] != "2025-09-23" {
		t.Errorf("hebrew occurrence = %q, want 2025-09-23", got["birthday"])
	}
	if got["custom"] != "2025-03-01" {
		t.Errorf("hijri occurrence = %q, want 2025-03-01", got["custom"])
	}
}

// TestDashboardJSON_MemorialYearsSince verifies a memorial renders elapsed
// years and never a milestone badge.
func TestDashboardJSON_MemorialYearsSince(t *testing.T) {
	h := newTestWebHandler(t)
	personID := newTestPerson(t, h, "Memorial Person")
	// Anniversary tomorrow: five completed years since the original date.
	original := time.Now().AddDate(-5, 0, 1)
	newTestEvent(t, h, personID, "memorial", original)

	router := chi.NewRouter()
	router.Get("/api/dashboard", h.dashboardJSON)
	req := httptest.NewRequest("GET", "/api/dashboard", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body struct {
		Today []UpcomingEventDTO `json:"today"`
		Week  []UpcomingEventDTO `json:"this_week"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var found *UpcomingEventDTO
	for _, list := range [][]UpcomingEventDTO{body.Today, body.Week} {
		for i := range list {
			if list[i].Type == "memorial" {
				found = &list[i]
			}
		}
	}
	if found == nil {
		t.Fatalf("memorial event not found in response: %s", w.Body.String())
	}
	if found.YearsSince == nil || *found.YearsSince != 5 {
		t.Errorf("years_since = %v, want 5", found.YearsSince)
	}
	if found.YearsSinceText == nil || *found.YearsSinceText != "5 years since" {
		t.Errorf("years_since_text = %v", found.YearsSinceText)
	}
	if found.IsMilestone {
		t.Errorf("memorial must not carry a milestone badge: %v", found.MilestoneLabel)
	}
}
