package web

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func setupVCardImportRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Post("/people/import", h.handleImportVCard)
	return r
}

const vcfContact = "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:%s\r\nNOTE:%s\r\nEND:VCARD\r\n"

func vcardUploadRequest(url string, files map[string]string, fields map[string]string) (*http.Request, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for fname, content := range files {
		fw, err := w.CreateFormFile("vcf_file", fname)
		if err != nil {
			return nil, err
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			return nil, err
		}
	}
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	req := httptest.NewRequest(http.MethodPost, url, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("HX-Request", "true")
	return req, nil
}

func TestImportVCard_MultipleFiles(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupVCardImportRouter(h)

	req, err := vcardUploadRequest("/people/import", map[string]string{
		"a.vcf": sprintfVCF("Alice One", "From A"),
		"b.vcf": sprintfVCF("Bob Two", "From B"),
	}, nil)
	if err != nil {
		t.Fatalf("build upload request: %v", err)
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Alice One") || !strings.Contains(body, "Bob Two") {
		t.Errorf("results missing both contacts: %s", body)
	}
	if !strings.Contains(body, "2</strong> imported") {
		t.Errorf("summary missing aggregate count: %s", body)
	}

	people, err := h.people.List(t.Context())
	if err != nil {
		t.Fatalf("list people: %v", err)
	}
	if len(people) != 2 {
		t.Fatalf("people = %d, want 2", len(people))
	}
}

func TestImportVCard_OneValidOneInvalidFile(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupVCardImportRouter(h)

	req, err := vcardUploadRequest("/people/import", map[string]string{
		"valid.vcf":  sprintfVCF("Good One", "ok"),
		"broken.vcf": "this is not a vcard",
	}, nil)
	if err != nil {
		t.Fatalf("build upload request: %v", err)
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Good One") {
		t.Errorf("valid contact missing from results: %s", body)
	}
	if !strings.Contains(body, "1</strong> imported") {
		t.Errorf("summary missing import count: %s", body)
	}
	if !strings.Contains(body, "1</strong> skipped") {
		t.Errorf("summary missing skip count: %s", body)
	}

	people, err := h.people.List(t.Context())
	if err != nil {
		t.Fatalf("list people: %v", err)
	}
	if len(people) != 1 {
		t.Fatalf("people = %d, want 1 (invalid file must not abort import)", len(people))
	}
}

func TestImportVCard_OverwriteUpdatesExisting(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupVCardImportRouter(h)

	newTestPerson(t, h, "Dana Vreede")

	req, err := vcardUploadRequest("/people/import", map[string]string{
		"dana.vcf": sprintfVCF("Dana Vreede", "Refreshed notes"),
	}, map[string]string{"overwrite": "true"})
	if err != nil {
		t.Fatalf("build upload request: %v", err)
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Updated") {
		t.Errorf("results missing Updated status: %s", body)
	}
	if !strings.Contains(body, "1</strong> updated") {
		t.Errorf("summary missing updated count: %s", body)
	}

	people, err := h.people.List(t.Context())
	if err != nil {
		t.Fatalf("list people: %v", err)
	}
	if len(people) != 1 {
		t.Fatalf("people = %d, want 1 (overwrite must not create duplicates)", len(people))
	}
	if people[0].Notes != "Refreshed notes" {
		t.Errorf("notes = %q, want updated notes", people[0].Notes)
	}
	if people[0].VcardData == "" {
		t.Error("vcard data not stored on overwrite")
	}
}

func TestImportVCard_OverwriteUncheckedStillSkips(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupVCardImportRouter(h)

	newTestPerson(t, h, "Dana Vreede")

	req, err := vcardUploadRequest("/people/import", map[string]string{
		"dana.vcf": sprintfVCF("Dana Vreede", "Should not apply"),
	}, nil)
	if err != nil {
		t.Fatalf("build upload request: %v", err)
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, "Updated") {
		t.Errorf("unexpected Updated status: %s", body)
	}
	if !strings.Contains(body, "Duplicate name") {
		t.Errorf("expected Duplicate name skip: %s", body)
	}

	people, err := h.people.List(t.Context())
	if err != nil {
		t.Fatalf("list people: %v", err)
	}
	if len(people) != 1 || people[0].Notes != "" {
		t.Fatalf("person should be unchanged, got %d people, notes=%q", len(people), people[0].Notes)
	}
}

func TestImportVCard_OverwriteCreatesBirthdayEvent(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupVCardImportRouter(h)

	id := newTestPerson(t, h, "Dana Vreede")

	vcf := "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Dana Vreede\r\nBDAY:19980129\r\nEND:VCARD\r\n"
	req, err := vcardUploadRequest("/people/import", map[string]string{
		"dana.vcf": vcf,
	}, map[string]string{"overwrite": "true"})
	if err != nil {
		t.Fatalf("build upload request: %v", err)
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	events, err := h.events.ListByPerson(t.Context(), id)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 || events[0].Type != "birthday" {
		t.Fatalf("events = %d (type %q), want 1 birthday", len(events), events[0].Type)
	}
}

func TestImportVCard_OverwriteNoDuplicateBirthdayEvent(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupVCardImportRouter(h)

	id := newTestPerson(t, h, "Dana Vreede")
	newTestEvent(t, h, id, "birthday", time.Date(1998, 1, 29, 0, 0, 0, 0, time.UTC))

	vcf := "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Dana Vreede\r\nBDAY:19980129\r\nEND:VCARD\r\n"
	req, err := vcardUploadRequest("/people/import", map[string]string{
		"dana.vcf": vcf,
	}, map[string]string{"overwrite": "true"})
	if err != nil {
		t.Fatalf("build upload request: %v", err)
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	events, err := h.events.ListByPerson(t.Context(), id)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1 (no duplicate birthday)", len(events))
	}
}

func TestImportVCard_YearlessBirthdayCreatesEvent(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupVCardImportRouter(h)

	vcf := "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Dana Vreede\r\nBDAY:--0608\r\nEND:VCARD\r\n"
	req, err := vcardUploadRequest("/people/import", map[string]string{
		"dana.vcf": vcf,
	}, nil)
	if err != nil {
		t.Fatalf("build upload request: %v", err)
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Jun 8") {
		t.Errorf("import result should show month/day 'Jun 8', got: %s", body)
	}
	if strings.Contains(body, "Jun 8, 1") {
		t.Errorf("import result must not show 'Jun 8, 1': %s", body)
	}

	people, err := h.people.List(t.Context())
	if err != nil {
		t.Fatalf("list people: %v", err)
	}
	if len(people) != 1 {
		t.Fatalf("people = %d, want 1", len(people))
	}

	events, err := h.events.ListByPerson(t.Context(), people[0].ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 || events[0].Type != "birthday" {
		t.Fatalf("events = %d (type %q), want 1 birthday", len(events), events[0].Type)
	}
	if events[0].Date.Month() != time.June || events[0].Date.Day() != 8 {
		t.Errorf("event date = %v, want June 8", events[0].Date)
	}
	if got := events[0].Date.Year(); got > 1 {
		t.Errorf("event date year = %d, want <= 1", got)
	}
	if events[0].Description != "Birthday of Dana Vreede" {
		t.Errorf("description = %q, want 'Birthday of Dana Vreede'", events[0].Description)
	}
}

func TestImportVCard_YearlessDuplicateImportNoSecondEvent(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupVCardImportRouter(h)

	vcf := "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Dana Vreede\r\nBDAY:--0608\r\nEND:VCARD\r\n"
	for i := 1; i <= 2; i++ {
		req, err := vcardUploadRequest("/people/import", map[string]string{
			"dana.vcf": vcf,
		}, nil)
		if err != nil {
			t.Fatalf("build upload request: %v", err)
		}
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("import %d: status = %d, want 200; body: %s", i, rr.Code, rr.Body.String())
		}
	}

	people, err := h.people.List(t.Context())
	if err != nil {
		t.Fatalf("list people: %v", err)
	}
	if len(people) != 1 {
		t.Fatalf("people = %d, want 1 (duplicate must be skipped)", len(people))
	}

	events, err := h.events.ListByPerson(t.Context(), people[0].ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1 (no duplicate birthday)", len(events))
	}
}

func sprintfVCF(name, notes string) string {
	return fmt.Sprintf(vcfContact, name, notes)
}
