package web

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/age"
	"github.com/datey/datey/internal/csvimport"
	"github.com/go-chi/chi/v5"
)

func setupCSVImportRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Get("/people/import/csv", h.handleCSVImportForm)
	r.Post("/people/import/csv", h.handleCSVImportPreview)
	r.Post("/people/import/csv/confirm", h.handleCSVImportConfirm)
	return r
}

func setupCSVImportAuthRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(h.Auth)
		r.Post("/people/import/csv", h.handleCSVImportPreview)
	})
	return r
}

// csvUploadRequest builds a multipart upload request for the preview endpoint.
func csvUploadRequest(t *testing.T, filename, content string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("csv_file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/people/import/csv", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("HX-Request", "true")
	return req
}

// csvConfirmRequest builds a confirm request for the given parsed rows.
func csvConfirmRequest(t *testing.T, rows []csvimport.Row) *http.Request {
	t.Helper()
	payload, err := json.Marshal(rows)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	form := url.Values{"payload": {string(payload)}}
	req := httptest.NewRequest(http.MethodPost, "/people/import/csv/confirm", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	return req
}

// parseCSVRows runs the parser and returns its rows, failing the test on error.
func parseCSVRows(t *testing.T, csvText string) []csvimport.Row {
	t.Helper()
	res, err := csvimport.Parse(strings.NewReader(csvText))
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	return res.Rows
}

func TestCSVImportForm_Renders(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCSVImportRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/people/import/csv", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `name="csv_file"`) {
		t.Errorf("upload form missing csv_file input: %s", body)
	}
	if !strings.Contains(body, `hx-post="/people/import/csv"`) {
		t.Errorf("upload form missing HTMX preview target: %s", body)
	}
	if !strings.Contains(body, "Import people from CSV") {
		t.Errorf("page missing title: %s", body)
	}
}

func TestCSVImportPreview_Counts(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCSVImportRouter(h)
	newTestPerson(t, h, "Bob Builder")

	csvText := "Name,Birthday\n" +
		"Ada Lovelace,1990-12-10\n" +
		"Bob Builder,1985-02-11\n" + // duplicate of existing person
		",1990-01-01\n" + // missing name
		"Cid,not-a-date\n" // unparseable date
	req := csvUploadRequest(t, "birthdays.csv", csvText)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `data-total="4"`) {
		t.Errorf("preview total wrong: %s", body)
	}
	if !strings.Contains(body, `data-importable="1"`) {
		t.Errorf("preview importable count wrong: %s", body)
	}
	if !strings.Contains(body, `data-skipped="3"`) {
		t.Errorf("preview skipped count wrong: %s", body)
	}
	if !strings.Contains(body, "Ada Lovelace") {
		t.Errorf("importable row missing: %s", body)
	}
	for _, reason := range []string{"duplicate of existing person", "missing name", "unparseable date"} {
		if !strings.Contains(body, reason) {
			t.Errorf("skipped reason %q missing from preview", reason)
		}
	}
	// Both sections render: the template test requirement.
	if !strings.Contains(body, `id="csv-importable"`) || !strings.Contains(body, `id="csv-import-skipped"`) {
		t.Errorf("preview must show both importable and skipped sections: %s", body)
	}
	// Nothing written by a preview.
	people, err := h.people.List(t.Context())
	if err != nil {
		t.Fatalf("list people: %v", err)
	}
	if len(people) != 1 {
		t.Errorf("people = %d, want 1 (preview must not write)", len(people))
	}
}

func TestCSVImportPreview_DuplicateWithinFile(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCSVImportRouter(h)

	req := csvUploadRequest(t, "birthdays.csv", "Name,Birthday\nAda,1990-01-01\nAda,1990-01-01\n")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "duplicate within file") {
		t.Errorf("duplicate-within-file reason missing: %s", body)
	}
	if !strings.Contains(body, `data-importable="1"`) || !strings.Contains(body, `data-skipped="1"`) {
		t.Errorf("counts wrong for duplicate file: %s", body)
	}
}

func TestCSVImportPreview_OversizedFileRejected(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCSVImportRouter(h)

	var b strings.Builder
	b.WriteString("Name,Birthday\n")
	for b.Len() <= csvImportMaxUploadSize+1024 {
		b.WriteString("Padding Person,1990-01-01\n")
	}
	req := csvUploadRequest(t, "huge.csv", b.String())

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	trigger := strings.ToLower(rr.Header().Get("HX-Trigger"))
	if !strings.Contains(trigger, "too large") {
		t.Fatalf("oversized upload not rejected with too-large message; HX-Trigger = %q, status %d", trigger, rr.Code)
	}
	people, err := h.people.List(t.Context())
	if err != nil {
		t.Fatalf("list people: %v", err)
	}
	if len(people) != 0 {
		t.Errorf("people = %d, want 0 (oversized file must not import)", len(people))
	}
}

func TestCSVImportPreview_RejectsUnsupportedFileType(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCSVImportRouter(h)

	req := csvUploadRequest(t, "birthdays.pdf", "Name,Birthday\nAda,1990-01-01\n")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if !strings.Contains(strings.ToLower(rr.Header().Get("HX-Trigger")), "only .csv") {
		t.Fatalf("expected unsupported-type rejection, HX-Trigger = %q", rr.Header().Get("HX-Trigger"))
	}
}

func TestCSVImportPreview_UnknownGroupReported(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCSVImportRouter(h)

	req := csvUploadRequest(t, "birthdays.csv", "Name,Birthday,Groups\nAda,1990-01-01,Chess Club\n")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, `id="csv-import-unknown-groups"`) || !strings.Contains(body, "Chess Club") {
		t.Fatalf("unknown group not reported in preview: %s", body)
	}
	groups, err := h.groups.List(t.Context())
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("groups = %d, want 0 (preview must not create groups)", len(groups))
	}
}

func TestCSVImportConfirm_CreatesPeopleBirthdaysGroupsAndTags(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCSVImportRouter(h)

	rows := parseCSVRows(t, "Name,Birthday,Notes,Groups,Tags\n"+
		"Ada Lovelace,1990-12-10,Math,Chess Club;Friends,smart\n"+
		"Bob Builder,1985-02-11,,Chess Club,\n")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, csvConfirmRequest(t, rows))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "<strong>2</strong>") {
		t.Errorf("results summary missing imported count: %s", rr.Body.String())
	}

	people, err := h.people.List(t.Context())
	if err != nil {
		t.Fatalf("list people: %v", err)
	}
	if len(people) != 2 {
		t.Fatalf("people = %d, want 2", len(people))
	}
	for _, p := range people {
		events, err := h.events.ListByPerson(t.Context(), p.ID)
		if err != nil {
			t.Fatalf("list events for %q: %v", p.Name, err)
		}
		if len(events) != 1 || events[0].Type != "birthday" {
			t.Fatalf("%q events = %+v, want one birthday", p.Name, events)
		}
		if events[0].Description != "Birthday of "+p.Name {
			t.Errorf("%q description = %q", p.Name, events[0].Description)
		}
	}

	groups, err := h.groups.List(t.Context())
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2 (%v)", len(groups), groupNames(groups))
	}
	for _, g := range groups {
		if g.Name == "Chess Club" {
			members, err := h.groups.ListPeopleInGroup(t.Context(), g.ID)
			if err != nil {
				t.Fatalf("list group members: %v", err)
			}
			if len(members) != 2 {
				t.Errorf("Chess Club members = %d, want 2", len(members))
			}
		}
	}

	tags, err := h.tags.ListAll(t.Context())
	if err != nil {
		t.Fatalf("list tags: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "smart" {
		t.Errorf("tags = %+v, want one 'smart' tag", tags)
	}
}

func TestCSVImportConfirm_YearlessBirthdayHasNoAge(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCSVImportRouter(h)

	rows := parseCSVRows(t, "Name,Birthday\nDana Vreede,--0608\n")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, csvConfirmRequest(t, rows))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
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
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	date := events[0].Date
	if date.Month() != 6 || date.Day() != 8 {
		t.Errorf("date = %v, want June 8", date)
	}
	if date.Year() > 1 {
		t.Errorf("year = %d, want <= 1 for a year-less birthday", date.Year())
	}
	if info := age.InfoFor(date, date.AddDate(40, 0, 0)); info.HasAge {
		t.Errorf("age = %+v, want no age for a year-less birthday", info)
	}
}

func TestCSVImportConfirm_SkipsExistingPerson(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCSVImportRouter(h)
	newTestPerson(t, h, "Ada Lovelace")

	rows := parseCSVRows(t, "Name,Birthday\nAda Lovelace,1990-12-10\nBob Builder,1985-02-11\n")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, csvConfirmRequest(t, rows))

	body := rr.Body.String()
	if !strings.Contains(body, "<strong>1</strong>") {
		t.Errorf("summary should report 1 imported and 1 skipped: %s", body)
	}
	people, err := h.people.List(t.Context())
	if err != nil {
		t.Fatalf("list people: %v", err)
	}
	if len(people) != 2 {
		t.Errorf("people = %d, want 2 (existing must not be duplicated)", len(people))
	}
	// The existing person must not gain a birthday event from the import.
	existing, err := h.people.FindByName(t.Context(), "Ada Lovelace")
	if err != nil {
		t.Fatalf("find existing person: %v", err)
	}
	events, err := h.events.ListByPerson(t.Context(), existing.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("existing person gained %d events, want 0 (additive only)", len(events))
	}
}

func TestCSVImportConfirm_IdempotentResubmit(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCSVImportRouter(h)

	rows := parseCSVRows(t, "Name,Birthday\nAda Lovelace,1990-12-10\n")
	for i := 1; i <= 2; i++ {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, csvConfirmRequest(t, rows))
		if rr.Code != http.StatusOK {
			t.Fatalf("submit %d: status = %d, body: %s", i, rr.Code, rr.Body.String())
		}
	}

	people, err := h.people.List(t.Context())
	if err != nil {
		t.Fatalf("list people: %v", err)
	}
	if len(people) != 1 {
		t.Fatalf("people = %d, want 1 after re-submit", len(people))
	}
	events, err := h.events.ListByPerson(t.Context(), people[0].ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("events = %d, want 1 after re-submit", len(events))
	}
}

func TestCSVImportConfirm_RejectsMissingPayload(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCSVImportRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/people/import/csv/confirm", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if !strings.Contains(strings.ToLower(rr.Header().Get("HX-Trigger")), "no import data") {
		t.Errorf("expected rejection toast, HX-Trigger = %q", rr.Header().Get("HX-Trigger"))
	}
}

func TestCSVImport_UnauthenticatedRejected(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCSVImportAuthRouter(h)

	req := csvUploadRequest(t, "birthdays.csv", "Name,Birthday\nAda,1990-01-01\n")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 redirect to login", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
}

func TestCSVImportPreview_NonHTMXReturnsPage(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCSVImportRouter(h)

	req := csvUploadRequest(t, "birthdays.csv", "Name,Birthday\nAda,1990-01-01\n")
	req.Header.Del("HX-Request")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "<!DOCTYPE html>") {
		t.Errorf("non-HTMX preview should render the full page")
	}
	if !strings.Contains(string(body), `id="csv-importable"`) {
		t.Errorf("full page preview missing importable section")
	}
}

func groupNames(groups []*ent.Group) []string {
	names := make([]string, 0, len(groups))
	for _, g := range groups {
		names = append(names, g.Name)
	}
	return names
}
