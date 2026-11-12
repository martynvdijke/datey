package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/datey/datey/ent/event"
	"github.com/datey/datey/ent/person"
	"github.com/datey/datey/internal/repository"
)

// setupWriteAPIRouter mirrors the production middleware chain over the v1
// write routes: BearerAuth -> APIAuthJSON -> CSRF. APIAuthJSON runs before CSRF
// so anonymous callers get a JSON 401, while an authenticated session write
// without a CSRF token still gets the shared 403.
func setupWriteAPIRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(h.BearerAuth)
		r.Use(h.APIAuthJSON)
		r.Use(h.CSRF)
		r.Post("/api/v1/people", h.apiCreatePerson)
		r.Patch("/api/v1/people/{id}", h.apiUpdatePerson)
		r.Delete("/api/v1/people/{id}", h.apiDeletePerson)
		r.Get("/api/v1/people/{id}/events", h.apiListPersonEvents)
		r.Post("/api/v1/people/{id}/events", h.apiCreateEvent)
		r.Patch("/api/v1/events/{id}", h.apiUpdateEvent)
		r.Delete("/api/v1/events/{id}", h.apiDeleteEvent)
	})
	return r
}

// apiTokenFor creates a user and an API token owned by them.
func apiTokenFor(t *testing.T, h *Handler, username string) (userID int, token string) {
	t.Helper()
	u, _ := createUserWithSession(t, h, username)
	raw, _, err := h.apiTokens.Create(context.Background(), u.ID, "api-test", nil)
	if err != nil {
		t.Fatalf("create api token: %v", err)
	}
	return u.ID, raw
}

// doWriteAPI performs a JSON request against the write API.
func doWriteAPI(t *testing.T, router chi.Router, method, path, bearer string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	switch v := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(v)
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = strings.NewReader(string(raw))
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// decodeWriteBody decodes a JSON response body into T.
func decodeWriteBody[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	return out
}

type apiFieldErrorBody struct {
	Error   string            `json:"error"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields"`
}

// ── People ───────────────────────────────────────────────────────────────

func TestAPICreatePerson_HappyPath(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupWriteAPIRouter(h)
	_, token := apiTokenFor(t, h, "creator")

	group, err := h.groups.Create(context.Background(), "Family", "")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	w := doWriteAPI(t, router, "POST", "/api/v1/people", token, map[string]any{
		"first_name":  "Ada",
		"middle_name": "Augusta",
		"last_name":   "Lovelace",
		"notes":       "Mathematician",
		"groups":      []int{group.ID},
		"tags":        []string{"friend"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	dto := decodeWriteBody[PersonWriteDTO](t, w)
	if dto.Name != "Ada Augusta Lovelace" {
		t.Errorf("name = %q, want %q", dto.Name, "Ada Augusta Lovelace")
	}
	if dto.Notes != "Mathematician" {
		t.Errorf("notes = %q", dto.Notes)
	}
	if !dto.NotifyBirthdays {
		t.Error("notify_birthdays should default to true")
	}
	if len(dto.Groups) != 1 || dto.Groups[0].ID != group.ID {
		t.Errorf("groups = %+v", dto.Groups)
	}
	if len(dto.Tags) != 1 || dto.Tags[0] != "friend" {
		t.Errorf("tags = %+v", dto.Tags)
	}

	stored, err := h.client.Person.Get(context.Background(), dto.ID)
	if err != nil {
		t.Fatalf("load person: %v", err)
	}
	if stored.FirstName == nil || *stored.FirstName != "Ada" {
		t.Errorf("first_name not persisted: %+v", stored.FirstName)
	}
}

func TestAPICreatePerson_ValidationFailure(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupWriteAPIRouter(h)
	_, token := apiTokenFor(t, h, "validator")

	w := doWriteAPI(t, router, "POST", "/api/v1/people", token, map[string]any{"notes": "no name"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	body := decodeWriteBody[apiFieldErrorBody](t, w)
	if body.Error != "validation" {
		t.Errorf("error = %q, want validation", body.Error)
	}
	if body.Fields["name"] != "required" {
		t.Errorf("fields = %+v", body.Fields)
	}
}

func TestAPICreatePerson_InvalidJSON(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupWriteAPIRouter(h)
	_, token := apiTokenFor(t, h, "badjson")

	w := doWriteAPI(t, router, "POST", "/api/v1/people", token, "{not json")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if body := decodeWriteBody[apiFieldErrorBody](t, w); body.Error != "bad_request" {
		t.Errorf("error = %q, want bad_request", body.Error)
	}
}

func TestAPICreatePerson_DuplicateName409(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupWriteAPIRouter(h)
	_, token := apiTokenFor(t, h, "duplicator")

	payload := map[string]any{"name": "Same Name"}
	if w := doWriteAPI(t, router, "POST", "/api/v1/people", token, payload); w.Code != http.StatusCreated {
		t.Fatalf("first create = %d: %s", w.Code, w.Body.String())
	}
	w := doWriteAPI(t, router, "POST", "/api/v1/people", token, payload)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if body := decodeWriteBody[apiFieldErrorBody](t, w); body.Error != "conflict" {
		t.Errorf("error = %q, want conflict", body.Error)
	}
	if strings.Contains(w.Body.String(), "UNIQUE") {
		t.Errorf("response leaked the database constraint: %s", w.Body.String())
	}
}

func TestAPIUpdatePerson_PartialKeepsOtherFields(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupWriteAPIRouter(h)
	_, token := apiTokenFor(t, h, "editor")

	p, err := h.people.CreateStructured(context.Background(), "Ann Lee", "Ann", "", "Lee", "original", "")
	if err != nil {
		t.Fatalf("seed person: %v", err)
	}

	// Notes-only patch must not clear the structured name.
	w := doWriteAPI(t, router, "PATCH", "/api/v1/people/"+itoa(p.ID), token, map[string]any{"notes": "updated"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	dto := decodeWriteBody[PersonWriteDTO](t, w)
	if dto.Name != "Ann Lee" || dto.Notes != "updated" {
		t.Errorf("after notes patch: name=%q notes=%q", dto.Name, dto.Notes)
	}

	// Changing one structured part recomputes the display name.
	w = doWriteAPI(t, router, "PATCH", "/api/v1/people/"+itoa(p.ID), token, map[string]any{"first_name": "Anne"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	dto = decodeWriteBody[PersonWriteDTO](t, w)
	if dto.Name != "Anne Lee" {
		t.Errorf("name = %q, want Anne Lee", dto.Name)
	}

	// Clearing a part drops it from the display name.
	w = doWriteAPI(t, router, "PATCH", "/api/v1/people/"+itoa(p.ID), token, map[string]any{"last_name": ""})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if dto = decodeWriteBody[PersonWriteDTO](t, w); dto.Name != "Anne" {
		t.Errorf("name = %q, want Anne", dto.Name)
	}

	// The legacy display name alone still renames a person without parts.
	w = doWriteAPI(t, router, "PATCH", "/api/v1/people/"+itoa(p.ID), token, map[string]any{"name": "Rename"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if dto = decodeWriteBody[PersonWriteDTO](t, w); dto.Name != "Rename" {
		t.Errorf("name = %q, want Rename", dto.Name)
	}
}

func TestAPIUpdatePerson_ValidationFailure(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupWriteAPIRouter(h)
	_, token := apiTokenFor(t, h, "badpatch")

	p, err := h.people.Create(context.Background(), "Keep Me", "", "")
	if err != nil {
		t.Fatalf("seed person: %v", err)
	}
	// Explicit empty name is not allowed.
	w := doWriteAPI(t, router, "PATCH", "/api/v1/people/"+itoa(p.ID), token, map[string]any{"notes": "x", "first_name": "", "name": ""})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if body := decodeWriteBody[apiFieldErrorBody](t, w); body.Fields["name"] != "required" {
		t.Errorf("fields = %+v", body.Fields)
	}
}

func TestAPIUpdatePerson_NotFound(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupWriteAPIRouter(h)
	_, token := apiTokenFor(t, h, "missing")

	w := doWriteAPI(t, router, "PATCH", "/api/v1/people/9999", token, map[string]any{"notes": "x"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPIUpdatePerson_DuplicateName409(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupWriteAPIRouter(h)
	_, token := apiTokenFor(t, h, "clash")

	if _, err := h.people.Create(context.Background(), "Taken Name", "", ""); err != nil {
		t.Fatalf("seed first: %v", err)
	}
	other, err := h.people.Create(context.Background(), "Other Name", "", "")
	if err != nil {
		t.Fatalf("seed second: %v", err)
	}

	w := doWriteAPI(t, router, "PATCH", "/api/v1/people/"+itoa(other.ID), token, map[string]any{"name": "Taken Name"})
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPIDeletePerson(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupWriteAPIRouter(h)
	_, token := apiTokenFor(t, h, "deleter")

	p, err := h.people.Create(context.Background(), "Delete Me", "", "")
	if err != nil {
		t.Fatalf("seed person: %v", err)
	}

	w := doWriteAPI(t, router, "DELETE", "/api/v1/people/"+itoa(p.ID), token, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if n, err := h.client.Person.Query().Where(person.IDEQ(p.ID)).Count(context.Background()); err != nil || n != 0 {
		t.Fatalf("person still present (n=%d, err=%v)", n, err)
	}

	// A second delete reports 404 instead of a 500.
	w = doWriteAPI(t, router, "DELETE", "/api/v1/people/"+itoa(p.ID), token, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// ── Events ───────────────────────────────────────────────────────────────

func TestAPICreateEvent_HappyPath(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupWriteAPIRouter(h)
	_, token := apiTokenFor(t, h, "eventcreator")

	personID := newTestPerson(t, h, "Event Owner")
	w := doWriteAPI(t, router, "POST", "/api/v1/people/"+itoa(personID)+"/events", token, map[string]any{
		"type":        "birthday",
		"date":        "1990-05-12",
		"description": "Born in May",
		"notes":       "private note",
		"frequency":   "annual",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	dto := decodeWriteBody[EventWriteDTO](t, w)
	if dto.PersonID == nil || *dto.PersonID != personID {
		t.Errorf("person_id = %v, want %d", dto.PersonID, personID)
	}
	if dto.Date != "1990-05-12" || dto.CalendarSystem != "gregorian" || dto.Frequency != "annual" {
		t.Errorf("unexpected event payload: %+v", dto)
	}
	if dto.Notes != "private note" {
		t.Errorf("notes = %q", dto.Notes)
	}

	stored, err := h.events.Get(context.Background(), dto.ID)
	if err != nil {
		t.Fatalf("load event: %v", err)
	}
	if stored.Interval != 1 || stored.Frequency != "annual" || stored.Notes != "private note" {
		t.Errorf("stored schedule = %q/%d notes=%q", stored.Frequency, stored.Interval, stored.Notes)
	}
}

func TestAPICreateEvent_ValidationFailure(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupWriteAPIRouter(h)
	_, token := apiTokenFor(t, h, "eventvalidator")
	personID := newTestPerson(t, h, "Validation Owner")

	cases := []struct {
		name     string
		payload  map[string]any
		wantKeys []string
	}{
		{
			name:     "missing type and bad frequency",
			payload:  map[string]any{"date": "2026-07-04", "frequency": "fortnightly"},
			wantKeys: []string{"type", "frequency"},
		},
		{
			name:     "invalid date",
			payload:  map[string]any{"type": "custom", "date": "not-a-date"},
			wantKeys: []string{"date"},
		},
		{
			name:     "non-gregorian non-annual",
			payload:  map[string]any{"type": "birthday", "date": "2026-07-04", "calendar_system": "lunar", "lunar_month": 8, "lunar_day": 15, "frequency": "monthly"},
			wantKeys: []string{"frequency"},
		},
		{
			name:     "end date before start",
			payload:  map[string]any{"type": "custom", "date": "2026-07-04", "frequency": "weekly", "end_date": "2026-07-03"},
			wantKeys: []string{"end_date"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doWriteAPI(t, router, "POST", "/api/v1/people/"+itoa(personID)+"/events", token, tc.payload)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
			body := decodeWriteBody[apiFieldErrorBody](t, w)
			for _, key := range tc.wantKeys {
				if _, ok := body.Fields[key]; !ok {
					t.Errorf("missing field error %q in %+v", key, body.Fields)
				}
			}
			if strings.Contains(w.Body.String(), "ent:") {
				t.Errorf("response leaked an internal error: %s", w.Body.String())
			}
		})
	}
}

func TestAPICreateEvent_PersonNotFound(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupWriteAPIRouter(h)
	_, token := apiTokenFor(t, h, "noowner")

	w := doWriteAPI(t, router, "POST", "/api/v1/people/4242/events", token, map[string]any{"type": "birthday", "date": "2026-07-04"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPIListPersonEvents_IncludesSchedule(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupWriteAPIRouter(h)
	_, token := apiTokenFor(t, h, "eventlister")

	personID := newTestPerson(t, h, "List Owner")
	end := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	five := 5
	sched := repository.NewEventSchedule("every_n", 14, &end, &five)
	if _, err := h.events.CreateForPersonWithSchedule(context.Background(), personID, "custom", time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC), "desc", "gregorian", nil, nil, false, sched); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	newTestEvent(t, h, personID, "birthday", time.Date(1990, 5, 12, 0, 0, 0, 0, time.UTC))

	w := doWriteAPI(t, router, "GET", "/api/v1/people/"+itoa(personID)+"/events", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		PersonID int             `json:"person_id"`
		Events   []EventWriteDTO `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.PersonID != personID || len(body.Events) != 2 {
		t.Fatalf("body = %+v", body)
	}
	for _, e := range body.Events {
		if e.Type == "custom" {
			if e.Frequency != "every_n" || e.Interval != 14 {
				t.Errorf("schedule = %q/%d", e.Frequency, e.Interval)
			}
			if e.EndDate == nil || *e.EndDate != "2026-12-31" {
				t.Errorf("end_date = %v", e.EndDate)
			}
			if e.OccurrenceCount == nil || *e.OccurrenceCount != 5 {
				t.Errorf("occurrence_count = %v", e.OccurrenceCount)
			}
		}
	}

	w = doWriteAPI(t, router, "GET", "/api/v1/people/9999/events", token, nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("missing person: expected 404, got %d", w.Code)
	}
}

func TestAPIUpdateEvent_Partial(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupWriteAPIRouter(h)
	_, token := apiTokenFor(t, h, "eventeditor")

	personID := newTestPerson(t, h, "Patch Owner")
	created, err := h.events.CreateForPerson(context.Background(), personID, "custom", time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC), "before")
	if err != nil {
		t.Fatalf("seed event: %v", err)
	}

	w := doWriteAPI(t, router, "PATCH", "/api/v1/events/"+itoa(created.ID), token, map[string]any{
		"frequency":        "every_n",
		"interval":         2,
		"end_date":         "2026-12-31",
		"occurrence_count": 4,
		"description":      "after",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	dto := decodeWriteBody[EventWriteDTO](t, w)
	if dto.Frequency != "every_n" || dto.Interval != 2 || dto.Description != "after" {
		t.Errorf("payload = %+v", dto)
	}
	if dto.Date != "2026-07-04" {
		t.Errorf("date changed unexpectedly: %q", dto.Date)
	}

	// An empty end_date clears the bound; occurrence_count 0 clears the count.
	w = doWriteAPI(t, router, "PATCH", "/api/v1/events/"+itoa(created.ID), token, map[string]any{"end_date": "", "occurrence_count": 0})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	dto = decodeWriteBody[EventWriteDTO](t, w)
	if dto.EndDate != nil || dto.OccurrenceCount != nil {
		t.Errorf("bounds not cleared: end=%v count=%v", dto.EndDate, dto.OccurrenceCount)
	}

	w = doWriteAPI(t, router, "PATCH", "/api/v1/events/9999", token, map[string]any{"description": "x"})
	if w.Code != http.StatusNotFound {
		t.Errorf("missing event: expected 404, got %d", w.Code)
	}
}

func TestAPIDeleteEvent(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupWriteAPIRouter(h)
	_, token := apiTokenFor(t, h, "eventdeleter")

	personID := newTestPerson(t, h, "Delete Event Owner")
	created, err := h.events.CreateForPerson(context.Background(), personID, "birthday", time.Date(1990, 5, 12, 0, 0, 0, 0, time.UTC), "")
	if err != nil {
		t.Fatalf("seed event: %v", err)
	}

	w := doWriteAPI(t, router, "DELETE", "/api/v1/events/"+itoa(created.ID), token, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if n, err := h.client.Event.Query().Where(event.IDEQ(created.ID)).Count(context.Background()); err != nil || n != 0 {
		t.Fatalf("event still present (n=%d, err=%v)", n, err)
	}
	w = doWriteAPI(t, router, "DELETE", "/api/v1/events/"+itoa(created.ID), token, nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("second delete: expected 404, got %d", w.Code)
	}
}

// ── Auth ─────────────────────────────────────────────────────────────────

func TestAPIWrite_Unauthorized(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupWriteAPIRouter(h)
	personID := newTestPerson(t, h, "Auth Owner")
	created, err := h.events.CreateForPerson(context.Background(), personID, "birthday", time.Date(1990, 5, 12, 0, 0, 0, 0, time.UTC), "")
	if err != nil {
		t.Fatalf("seed event: %v", err)
	}

	cases := []struct{ method, path string }{
		{"POST", "/api/v1/people"},
		{"PATCH", "/api/v1/people/" + itoa(personID)},
		{"DELETE", "/api/v1/people/" + itoa(personID)},
		{"GET", "/api/v1/people/" + itoa(personID) + "/events"},
		{"POST", "/api/v1/people/" + itoa(personID) + "/events"},
		{"PATCH", "/api/v1/events/" + itoa(created.ID)},
		{"DELETE", "/api/v1/events/" + itoa(created.ID)},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			w := doWriteAPI(t, router, tc.method, tc.path, "", map[string]any{"name": "Nope", "type": "birthday", "date": "2026-07-04"})
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
			}
			if body := decodeWriteBody[apiFieldErrorBody](t, w); body.Error != "unauthorized" {
				t.Errorf("error = %q, want unauthorized", body.Error)
			}
		})
	}

	// Nothing was modified by the rejected calls.
	if n, _ := h.client.Person.Query().Count(context.Background()); n != 1 {
		t.Errorf("person count = %d, want 1", n)
	}
}

func TestAPIWrite_InvalidBearerRejected(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupWriteAPIRouter(h)

	w := doWriteAPI(t, router, "POST", "/api/v1/people", "deadbeef", map[string]any{"name": "Nope"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPIWrite_SessionAuthWorks(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupWriteAPIRouter(h)
	_, sessionToken := createUserWithSession(t, h, "sessionuser")

	personID := newTestPerson(t, h, "Session Owner")

	req := httptest.NewRequest("GET", "/api/v1/people/"+itoa(personID)+"/events", nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cookie", "session="+sessionToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ── Round trip ───────────────────────────────────────────────────────────

// TestAPIWriteRoundTrip_MonthlyEventAppearsInUpcoming walks the whole feature:
// create a person and a monthly event over the API, then confirm the occurrence
// is visible to ListUpcomingOccurrences (the dashboard/feed source).
func TestAPIWriteRoundTrip_MonthlyEventAppearsInUpcoming(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupWriteAPIRouter(h)
	_, token := apiTokenFor(t, h, "roundtrip")

	w := doWriteAPI(t, router, "POST", "/api/v1/people", token, map[string]any{
		"first_name": "Round",
		"last_name":  "Trip",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create person: %d: %s", w.Code, w.Body.String())
	}
	p := decodeWriteBody[PersonWriteDTO](t, w)

	today := todayStartUTC()
	dateStr := today.Format("2006-01-02")
	w = doWriteAPI(t, router, "POST", "/api/v1/people/"+itoa(p.ID)+"/events", token, map[string]any{
		"type":        "anniversary",
		"date":        dateStr,
		"description": "monthly round trip",
		"frequency":   "monthly",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create event: %d: %s", w.Code, w.Body.String())
	}
	ev := decodeWriteBody[EventWriteDTO](t, w)

	occurrences, err := h.events.ListUpcomingOccurrences(context.Background(), today, today)
	if err != nil {
		t.Fatalf("list upcoming: %v", err)
	}
	if len(occurrences) != 1 {
		t.Fatalf("got %d occurrences, want 1", len(occurrences))
	}
	occ := occurrences[0]
	if occ.Event.ID != ev.ID {
		t.Errorf("occurrence event = %d, want %d", occ.Event.ID, ev.ID)
	}
	if !occ.Date.Equal(today) {
		t.Errorf("occurrence date = %v, want %v", occ.Date, today)
	}
	if occ.Event.Frequency != "monthly" {
		t.Errorf("frequency = %q, want monthly", occ.Event.Frequency)
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────
