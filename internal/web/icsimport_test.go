package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

const testICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Datey//Test//EN\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:test-1\r\n" +
	"SUMMARY:Test Birthday Party\r\n" +
	"DTSTART;VALUE=DATE:20260812\r\n" +
	"RRULE:FREQ=YEARLY;INTERVAL=1\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func setupICSImportRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Get("/calendar", h.calendarPage)
	r.Post("/calendar/import", h.handleImportICS)
	r.Post("/calendar/import/confirm", h.handleConfirmICS)
	return r
}

func icsUploadRequest(url, fileContent, personID, eventType string) (*http.Request, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("ics_file", "test.ics")
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write([]byte(fileContent)); err != nil {
		return nil, err
	}
	if err := w.WriteField("person_id", personID); err != nil {
		return nil, err
	}
	if err := w.WriteField("type", eventType); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	req := httptest.NewRequest(http.MethodPost, url, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("HX-Request", "true")
	return req, nil
}

func TestImportICS_PreviewShowsParsedEvents(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupICSImportRouter(h)
	ctx := withUserContext(t.Context())

	personID := newTestPerson(t, h, "Dana Vreede")
	now := time.Now()

	req, err := icsUploadRequest("/calendar/import", testICS, itoa(personID), "birthday")
	if err != nil {
		t.Fatalf("build upload request: %v", err)
	}
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Test Birthday Party") {
		t.Errorf("preview missing event summary: %s", body)
	}
	if !strings.Contains(body, "Dana Vreede") {
		t.Errorf("preview missing person name: %s", body)
	}
	if !strings.Contains(body, "2026-08-12") {
		t.Errorf("preview missing parsed date: %s", body)
	}
	if rr.Header().Get("HX-Trigger") == "" {
		t.Error("missing HX-Trigger toast header")
	}
	_ = now
}

func TestImportICS_ConfirmCreatesEvents(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupICSImportRouter(h)
	ctx := withUserContext(t.Context())

	personID := newTestPerson(t, h, "Dana Vreede")

	// Build preview payload directly (same shape the confirm handler expects).
	payload, _ := json.Marshal([]icsPreviewItem{{
		Summary:     "Test Birthday Party",
		Date:        "2026-08-12",
		Type:        "birthday",
		PersonID:    personID,
		PersonName:  "Dana Vreede",
		IsDuplicate: false,
		RecurYearly: true,
	}})

	form := "payload=" + string(payload)
	req := httptest.NewRequest(http.MethodPost, "/calendar/import/confirm", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Imported") {
		t.Errorf("results partial missing: %s", rr.Body.String())
	}

	events, err := h.events.ListByPerson(ctx, personID)
	if err != nil {
		t.Fatalf("list person events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Type != "birthday" {
		t.Errorf("event type = %q, want birthday", events[0].Type)
	}
	want := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	if !events[0].Date.Equal(want) {
		t.Errorf("event date = %v, want %v", events[0].Date, want)
	}
}

func TestImportICS_DuplicateSkippedOnConfirm(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupICSImportRouter(h)
	ctx := withUserContext(t.Context())

	personID := newTestPerson(t, h, "Dana Vreede")
	want := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	if _, err := h.events.CreateForPerson(ctx, personID, "birthday", want, "existing"); err != nil {
		t.Fatalf("seed event: %v", err)
	}

	// First: preview flags the duplicate.
	req, err := icsUploadRequest("/calendar/import", testICS, itoa(personID), "birthday")
	if err != nil {
		t.Fatalf("build upload request: %v", err)
	}
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("preview status = %d; body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "duplicate") {
		t.Errorf("preview should flag duplicate: %s", rr.Body.String())
	}

	// Second: confirm with duplicate payload skips it.
	payload, _ := json.Marshal([]icsPreviewItem{{
		Summary:     "Test Birthday Party",
		Date:        "2026-08-12",
		Type:        "birthday",
		PersonID:    personID,
		PersonName:  "Dana Vreede",
		IsDuplicate: true,
		RecurYearly: true,
	}})

	form := "payload=" + string(payload)
	req2 := httptest.NewRequest(http.MethodPost, "/calendar/import/confirm", strings.NewReader(form))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.Header.Set("HX-Request", "true")
	req2 = req2.WithContext(ctx)

	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("confirm status = %d; body: %s", rr2.Code, rr2.Body.String())
	}
	if !strings.Contains(rr2.Body.String(), "0") {
		t.Errorf("results should show 0 imported: %s", rr2.Body.String())
	}

	events, err := h.events.ListByPerson(ctx, personID)
	if err != nil {
		t.Fatalf("list person events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1 (duplicate skipped)", len(events))
	}
}

func TestImportICS_InvalidFileShowsError(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupICSImportRouter(h)
	ctx := withUserContext(t.Context())

	personID := newTestPerson(t, h, "Dana Vreede")

	req, err := icsUploadRequest("/calendar/import", "this is not an ics file", itoa(personID), "custom")
	if err != nil {
		t.Fatalf("build upload request: %v", err)
	}
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		// Error path still returns the preview partial (200) with a toast.
		t.Fatalf("status = %d; body: %s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("HX-Trigger") == "" {
		t.Error("expected error toast via HX-Trigger")
	}

	events, err := h.events.ListByPerson(ctx, personID)
	if err != nil {
		t.Fatalf("list person events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %d, want 0 (nothing created on parse error)", len(events))
	}
}
