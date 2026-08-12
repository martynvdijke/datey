package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func setupUpcomingAPIRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/api/upcoming", h.upcomingAPI)
	return r
}

func enableUpcomingAPI(h *Handler) {
	h.cfg.UpcomingAPIEnabled = true
	h.cfg.UpcomingAPIKey = "testapikey"
}

func TestUpcomingAPIDisabledReturns404(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupUpcomingAPIRouter(h)

	req := httptest.NewRequest("GET", "/api/upcoming?key=testapikey", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when disabled, got %d", w.Code)
	}
}

func TestUpcomingAPIMissingKeyReturns404(t *testing.T) {
	h := newTestWebHandler(t)
	enableUpcomingAPI(h)
	router := setupUpcomingAPIRouter(h)

	req := httptest.NewRequest("GET", "/api/upcoming", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 without key, got %d", w.Code)
	}
}

func TestUpcomingAPIWrongKeyReturns404(t *testing.T) {
	h := newTestWebHandler(t)
	enableUpcomingAPI(h)
	router := setupUpcomingAPIRouter(h)

	req := httptest.NewRequest("GET", "/api/upcoming?key=wrongkey", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 with wrong key, got %d", w.Code)
	}
}

func TestUpcomingAPIReturnsEvents(t *testing.T) {
	h := newTestWebHandler(t)
	enableUpcomingAPI(h)

	personID := newTestPerson(t, h, "Dana")
	newTestEvent(t, h, personID, "birthday", time.Now().AddDate(0, 0, 3))
	newTestEvent(t, h, personID, "anniversary", time.Now().AddDate(0, 0, 1))

	router := setupUpcomingAPIRouter(h)
	req := httptest.NewRequest("GET", "/api/upcoming?key=testapikey", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json content type, got %q", ct)
	}

	var out []upcomingEvent
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not valid JSON: %v\n%s", err, w.Body.String())
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 events, got %d: %s", len(out), w.Body.String())
	}
	if out[0].Person != "Dana" || out[0].Type != "anniversary" {
		t.Errorf("expected first event person=Dana type=anniversary, got %+v", out[0])
	}
	if out[1].Person != "Dana" || out[1].Type != "birthday" {
		t.Errorf("expected second event person=Dana type=birthday, got %+v", out[1])
	}
	if out[1].DaysRemaining < 2 || out[1].DaysRemaining > 3 {
		t.Errorf("expected days_remaining 2-3, got %d", out[1].DaysRemaining)
	}
	if out[0].Date > out[1].Date {
		t.Errorf("expected ascending date order, got %q then %q", out[0].Date, out[1].Date)
	}
}

func TestUpcomingAPICustomDays(t *testing.T) {
	h := newTestWebHandler(t)
	enableUpcomingAPI(h)

	personID := newTestPerson(t, h, "Dana")
	// Event outside the default 7-day reminder window.
	newTestEvent(t, h, personID, "holiday", time.Now().AddDate(0, 0, 20))

	router := setupUpcomingAPIRouter(h)

	// Default horizon (7 days) excludes the event.
	req := httptest.NewRequest("GET", "/api/upcoming?key=testapikey", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var out []upcomingEvent
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected 0 events with default horizon, got %d", len(out))
	}

	// days=30 includes it.
	req = httptest.NewRequest("GET", "/api/upcoming?key=testapikey&days=30", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(out) != 1 || out[0].Type != "holiday" {
		t.Fatalf("expected 1 holiday event with days=30, got %+v", out)
	}
}

func TestUpcomingAPIInvalidDaysReturns400(t *testing.T) {
	h := newTestWebHandler(t)
	enableUpcomingAPI(h)
	router := setupUpcomingAPIRouter(h)

	for _, raw := range []string{"abc", "0", "-5"} {
		req := httptest.NewRequest("GET", "/api/upcoming?key=testapikey&days="+raw, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("days=%q: expected 400, got %d", raw, w.Code)
		}
	}
}

func TestUpcomingAPIDaysCappedAt365(t *testing.T) {
	h := newTestWebHandler(t)
	enableUpcomingAPI(h)

	personID := newTestPerson(t, h, "Dana")
	newTestEvent(t, h, personID, "birthday", time.Now().AddDate(0, 0, 10))
	// A non-annual type is used for the far event: annual types would be
	// expanded to their same-year occurrence, which legitimately falls
	// inside the capped horizon.
	newTestEvent(t, h, personID, "meeting", time.Now().AddDate(0, 0, 400))

	router := setupUpcomingAPIRouter(h)
	// days=1000 requests a year+; cap at 365 excludes the 400-day event.
	req := httptest.NewRequest("GET", "/api/upcoming?key=testapikey&days=1000", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out []upcomingEvent
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(out) != 1 || out[0].Type != "birthday" {
		t.Fatalf("expected only the 10-day birthday event after cap, got %+v", out)
	}
}

func TestUpcomingAPIEmptyArray(t *testing.T) {
	h := newTestWebHandler(t)
	enableUpcomingAPI(h)
	router := setupUpcomingAPIRouter(h)

	req := httptest.NewRequest("GET", "/api/upcoming?key=testapikey", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out []upcomingEvent
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if out == nil || len(out) != 0 {
		t.Fatalf("expected empty JSON array, got %s", w.Body.String())
	}
}
