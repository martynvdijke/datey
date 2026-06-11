package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 3.1 Test GET /api/calendar-events returns JSON array
func TestCalendarEventsAPI(t *testing.T) {
	req := adminRequest("GET", "/api/calendar-events?start=2026-01-01&end=2026-12-31", nil)
	rr := httptest.NewRecorder()
	testRouter.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/calendar-events: expected 200, got %d", rr.Code)
	}

	contentType := rr.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("expected JSON content type, got %s", contentType)
	}

	var events []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &events); err != nil {
		t.Fatalf("expected valid JSON array, got error: %v", err)
	}
}

// 3.1b Test calendar events with seeded data
func TestCalendarEventsWithData(t *testing.T) {
	contactID := seedTestContact()
	if contactID == 0 {
		t.Fatal("failed to seed test contact")
	}

	req := adminRequest("GET", "/api/calendar-events?start=2026-07-01&end=2026-07-31", nil)
	rr := httptest.NewRecorder()
	testRouter.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/calendar-events: expected 200, got %d", rr.Code)
	}

	var events []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &events); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected at least 1 event with seeded data, got 0")
	}

	ev := events[0]
	if ev["id"] == nil {
		t.Errorf("expected event to have 'id'")
	}
	if ev["title"] == nil {
		t.Errorf("expected event to have 'title'")
	}
	if ev["start"] == nil {
		t.Errorf("expected event to have 'start'")
	}
	if ev["allDay"] == nil {
		t.Errorf("expected event to have 'allDay'")
	}
}

// 3.2 Test POST /settings/logs/level changes log level
func TestSettingsLogLevelChange(t *testing.T) {
	body := `{"level": "debug"}`
	req := adminRequest("POST", "/settings/logs/level", []byte(body))
	rr := httptest.NewRecorder()
	testRouter.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("POST /settings/logs/level: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("expected JSON response, got error: %v", err)
	}

	if resp["level"] != "debug" {
		t.Errorf("expected level 'debug', got %v", resp["level"])
	}
}

// 3.2b Test invalid log level returns 400
func TestSettingsLogLevelInvalid(t *testing.T) {
	body := `{"level": "invalid"}`
	req := adminRequest("POST", "/settings/logs/level", []byte(body))
	rr := httptest.NewRecorder()
	testRouter.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("POST /settings/logs/level with invalid level: expected 400, got %d", rr.Code)
	}
}

// 3.3 Test GET /logs redirects to /settings/logs
func TestOldLogsRedirect(t *testing.T) {
	req := adminRequest("GET", "/logs", nil)
	rr := httptest.NewRecorder()
	testRouter.ServeHTTP(rr, req)

	if rr.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /logs: expected 301, got %d", rr.Code)
	}

	location := rr.Header().Get("Location")
	if location != "/settings/logs" {
		t.Errorf("expected Location: /settings/logs, got %s", location)
	}
}

// 3.4 Test POST /settings/test/gotify returns 400 for unconfigured channel
func TestTestNotificationUnconfigured(t *testing.T) {
	body := `{}`
	req := adminRequest("POST", "/settings/test/gotify", []byte(body))
	rr := httptest.NewRecorder()
	testRouter.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("POST /settings/test/gotify: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// 3.5 Test unauthenticated requests redirect to /login
func TestUnauthenticatedRedirect(t *testing.T) {
	req := unauthenticatedRequest("GET", "/settings")
	rr := httptest.NewRecorder()
	testRouter.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("GET /settings without auth: expected 303, got %d", rr.Code)
	}

	location := rr.Header().Get("Location")
	if location != "/login" {
		t.Errorf("expected Location: /login, got %s", location)
	}
}

// 3.5b Test unauthenticated access to /settings/config redirects
func TestUnauthenticatedConfigRedirect(t *testing.T) {
	req := unauthenticatedRequest("GET", "/settings/config")
	rr := httptest.NewRecorder()
	testRouter.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("GET /settings/config without auth: expected 303, got %d", rr.Code)
	}
}
