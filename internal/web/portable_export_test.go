package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/internal/portable"
	"github.com/go-chi/chi/v5"
)

func setupPortableRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Get("/settings/portable/export", h.handlePortableExport)
	r.Get("/settings/portable/import", h.handlePortableImportForm)
	r.Post("/settings/portable/import", h.handlePortableImportPreview)
	r.Post("/settings/portable/import/confirm", h.handlePortableImportConfirm)
	return r
}

func portableUploadRequest(t *testing.T, filename, body string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("portable_file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write([]byte(body)); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/settings/portable/import", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func portableConfirmRequest(t *testing.T, env portable.Envelope) *http.Request {
	t.Helper()
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	form := url.Values{"envelope": {string(data)}}
	req := httptest.NewRequest(http.MethodPost, "/settings/portable/import/confirm", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func samplePortableEnvelope() portable.Envelope {
	return portable.Envelope{
		Format:     portable.Format,
		Version:    portable.Version,
		ExportedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		People: []portable.Person{
			{Name: "Zoe Example", FirstName: "Zoe", LastName: "Example", Notes: "From an old instance", Groups: []string{"Team"}, Tags: []string{"beta"}},
		},
		Groups: []portable.Group{{Name: "Team", Description: "Work"}},
		Tags:   []portable.Tag{{Name: "beta"}},
		Events: []portable.Event{{Person: "Zoe Example", Type: "birthday", Date: "1995-01-02", Description: "Birthday"}},
	}
}

func TestPortableExport_DownloadsJSON(t *testing.T) {
	h := newTestWebHandler(t)
	newTestPerson(t, h, "Ada Lovelace")
	router := setupPortableRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/settings/portable/export", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
	cd := rr.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") || !strings.Contains(cd, `filename="datey-portable-`) {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}
	if !json.Valid(rr.Body.Bytes()) {
		t.Fatalf("export is not valid JSON: %s", rr.Body.String())
	}
	var env portable.Envelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal export: %v", err)
	}
	if env.Format != portable.Format || env.Version != portable.Version {
		t.Errorf("identity = %q/%d", env.Format, env.Version)
	}
	if len(env.People) != 1 || env.People[0].Name != "Ada Lovelace" {
		t.Errorf("people = %#v", env.People)
	}
}

func TestPortableImportForm_Renders(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPortableRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/settings/portable/import", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `name="portable_file"`) {
		t.Errorf("upload form missing file input: %s", body)
	}
	if !strings.Contains(body, "/settings/portable/export") {
		t.Errorf("page missing export link")
	}
}

func TestPortableImportPreview_NoWrites(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPortableRouter(h)

	env := samplePortableEnvelope()
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := portableUploadRequest(t, "backup.json", string(data))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "What this import would do") {
		t.Errorf("preview missing summary: %s", body)
	}
	if !strings.Contains(body, `name="envelope"`) {
		t.Errorf("preview missing confirm payload: %s", body)
	}

	people, err := h.people.List(t.Context())
	if err != nil {
		t.Fatalf("list people: %v", err)
	}
	if len(people) != 0 {
		t.Errorf("preview wrote %d people", len(people))
	}
	if groups, _ := h.groups.List(t.Context()); len(groups) != 0 {
		t.Errorf("preview wrote %d groups", len(groups))
	}
	if events, _ := h.events.List(t.Context()); len(events) != 0 {
		t.Errorf("preview wrote %d events", len(events))
	}
}

func TestPortableImportPreview_RejectsUnusableFiles(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPortableRouter(h)

	cases := []struct {
		name     string
		filename string
		body     string
		want     string
	}{
		{name: "garbage", filename: "backup.json", body: "not json at all", want: "not a valid Datey portable JSON"},
		{name: "wrong format", filename: "backup.json", body: `{"format":"someone-else","version":1}`, want: "unsupported format"},
		{name: "wrong extension", filename: "backup.txt", body: "{}", want: "Only .json files are supported"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := portableUploadRequest(t, tc.filename, tc.body)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rr.Code)
			}
			if !strings.Contains(rr.Body.String(), tc.want) {
				t.Errorf("body does not contain %q: %s", tc.want, rr.Body.String())
			}
			if people, _ := h.people.List(t.Context()); len(people) != 0 {
				t.Errorf("rejected upload wrote %d people", len(people))
			}
		})
	}
}

func TestPortableImportPreview_RejectsOversize(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPortableRouter(h)

	huge := `{"format":"datey-portable","version":1,"people":[{"name":"` + strings.Repeat("x", 11<<20) + `"}]}`
	req := portableUploadRequest(t, "huge.json", huge)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "too large") && !strings.Contains(rr.Body.String(), "not a usable multipart form") {
		t.Errorf("body lacks a size message: %s", rr.Body.String())
	}
}

func TestPortableImportConfirm_Idempotent(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPortableRouter(h)
	env := samplePortableEnvelope()

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, portableConfirmRequest(t, env))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Import result") {
		t.Errorf("result page missing heading: %s", rr.Body.String())
	}
	people, err := h.people.List(t.Context())
	if err != nil {
		t.Fatalf("list people: %v", err)
	}
	if len(people) != 1 || people[0].Name != "Zoe Example" {
		t.Fatalf("people after import = %#v", people)
	}
	if events, _ := h.events.List(t.Context()); len(events) != 1 {
		t.Errorf("events after import = %d", len(events))
	}

	// A repeat confirm must create nothing.
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, portableConfirmRequest(t, env))
	if rr2.Code != http.StatusOK {
		t.Fatalf("second status = %d, want 200; body: %s", rr2.Code, rr2.Body.String())
	}
	people2, err := h.people.List(t.Context())
	if err != nil {
		t.Fatalf("list people 2: %v", err)
	}
	if len(people2) != 1 {
		t.Errorf("repeat import created people: %d", len(people2))
	}
	if events2, _ := h.events.List(t.Context()); len(events2) != 1 {
		t.Errorf("repeat import created events: %d", len(events2))
	}
}
