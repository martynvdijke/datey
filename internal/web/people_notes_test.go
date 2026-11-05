package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCreatePersonNote_Form(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "Note Person")

	req := withRouteParams(httptest.NewRequest("POST", "/people/"+itoa(p.ID)+"/notes", strings.NewReader("note=Met+at+work&note_date=2025-04-03")), map[string]string{"id": itoa(p.ID)})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.createPersonNote(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "success=Note+added") {
		t.Errorf("expected success redirect, got %q", loc)
	}

	notes, err := h.personNotes.ListByPerson(context.Background(), p.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].Note != "Met at work" {
		t.Errorf("expected note text, got %q", notes[0].Note)
	}
	if got := notes[0].NoteDate.Format("2006-01-02"); got != "2025-04-03" {
		t.Errorf("expected note date 2025-04-03, got %q", got)
	}
}

func TestCreatePersonNote_FormRejectsEmpty(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "Empty Note Person")

	req := withRouteParams(httptest.NewRequest("POST", "/people/"+itoa(p.ID)+"/notes", strings.NewReader("note=")), map[string]string{"id": itoa(p.ID)})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.createPersonNote(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "error=") {
		t.Errorf("expected error redirect, got %q", loc)
	}
	notes, _ := h.personNotes.ListByPerson(context.Background(), p.ID)
	if len(notes) != 0 {
		t.Errorf("expected no notes created")
	}
}

func TestCreatePersonNote_JSON(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "JSON Note Person")

	body := `{"note":"First meeting","note_date":"2025-07-09"}`
	req := withRouteParams(withJSONHeader(httptest.NewRequest("POST", "/people/"+itoa(p.ID)+"/notes", bytes.NewBufferString(body))), map[string]string{"id": itoa(p.ID)})
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.createPersonNote(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 got %d %s", w.Code, w.Body.String())
	}
	var dto PersonNoteDTO
	if err := json.NewDecoder(w.Body).Decode(&dto); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if dto.Note != "First meeting" || dto.NoteDate != "2025-07-09" {
		t.Fatalf("unexpected dto %+v", dto)
	}
	if dto.CreatedAt == "" || dto.UpdatedAt == "" {
		t.Errorf("expected timestamps, got %+v", dto)
	}
}

func TestPersonDetailJSON_IncludesNotesTimeline(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "Timeline Person")

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := h.personNotes.Create(context.Background(), p.ID, "older", base); err != nil {
		t.Fatalf("create note: %v", err)
	}
	if _, err := h.personNotes.Create(context.Background(), p.ID, "newer", base.AddDate(0, 0, 10)); err != nil {
		t.Fatalf("create note: %v", err)
	}

	req := withJSONHeader(withRouteParams(httptest.NewRequest("GET", "/people/"+itoa(p.ID), nil).WithContext(withUserContext(context.Background())), map[string]string{"id": itoa(p.ID)}))
	w := httptest.NewRecorder()
	h.viewPerson(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("code %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Notes         string          `json:"notes"`
		NotesTimeline []PersonNoteDTO `json:"notes_timeline"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.NotesTimeline == nil {
		t.Fatal("expected notes_timeline array, got null")
	}
	if len(resp.NotesTimeline) != 2 {
		t.Fatalf("expected 2 timeline notes, got %d", len(resp.NotesTimeline))
	}
	if resp.NotesTimeline[0].Note != "older" || resp.NotesTimeline[1].Note != "newer" {
		t.Errorf("expected oldest-first ordering, got [%s %s]", resp.NotesTimeline[0].Note, resp.NotesTimeline[1].Note)
	}
	if resp.NotesTimeline[0].NoteDate != "2025-01-01" {
		t.Errorf("expected formatted note_date, got %q", resp.NotesTimeline[0].NoteDate)
	}
}

func TestPersonDetailJSON_EmptyNotesTimelineIsArray(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "No Notes Person")

	req := withJSONHeader(withRouteParams(httptest.NewRequest("GET", "/people/"+itoa(p.ID), nil).WithContext(withUserContext(context.Background())), map[string]string{"id": itoa(p.ID)}))
	w := httptest.NewRecorder()
	h.viewPerson(w, req)

	var resp map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := string(resp["notes_timeline"]); got != "[]" {
		t.Errorf("expected empty array, got %s", got)
	}
	if _, ok := resp["notes"]; !ok {
		t.Error("expected legacy notes field to remain present")
	}
}

func TestDeletePersonNote(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "Delete Note Person")
	n, err := h.personNotes.Create(context.Background(), p.ID, "to delete", time.Now())
	if err != nil {
		t.Fatalf("create note: %v", err)
	}

	req := withRouteParams(httptest.NewRequest("POST", "/people/"+itoa(p.ID)+"/notes/"+itoa(n.ID)+"/delete", nil), map[string]string{"id": itoa(p.ID), "noteID": itoa(n.ID)})
	w := httptest.NewRecorder()
	h.deletePersonNote(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "success=Note+deleted") {
		t.Errorf("expected success redirect, got %q", loc)
	}
	notes, _ := h.personNotes.ListByPerson(context.Background(), p.ID)
	if len(notes) != 0 {
		t.Errorf("expected note deleted")
	}
}

func TestDeletePersonNote_JSON(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "JSON Delete Note Person")
	n, err := h.personNotes.Create(context.Background(), p.ID, "to delete", time.Now())
	if err != nil {
		t.Fatalf("create note: %v", err)
	}

	req := withRouteParams(withJSONHeader(httptest.NewRequest("POST", "/people/"+itoa(p.ID)+"/notes/"+itoa(n.ID)+"/delete", nil)), map[string]string{"id": itoa(p.ID), "noteID": itoa(n.ID)})
	w := httptest.NewRecorder()
	h.deletePersonNote(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 got %d %s", w.Code, w.Body.String())
	}
	if w.Body.Len() != 0 {
		t.Errorf("expected empty body, got %q", w.Body.String())
	}
}

func TestDeletePersonNote_HTMX(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "HTMX Delete Note Person")
	n, err := h.personNotes.Create(context.Background(), p.ID, "to delete", time.Now())
	if err != nil {
		t.Fatalf("create note: %v", err)
	}

	req := withRouteParams(httptest.NewRequest("POST", "/people/"+itoa(p.ID)+"/notes/"+itoa(n.ID)+"/delete", nil), map[string]string{"id": itoa(p.ID), "noteID": itoa(n.ID)})
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	h.deletePersonNote(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("HX-Refresh"); got != "true" {
		t.Errorf("expected HX-Refresh true, got %q", got)
	}
	notes, _ := h.personNotes.ListByPerson(context.Background(), p.ID)
	if len(notes) != 0 {
		t.Errorf("expected note deleted")
	}
}

func TestCreatePersonNote_FormRejectsWhitespace(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "Whitespace Note Person")

	req := withRouteParams(httptest.NewRequest("POST", "/people/"+itoa(p.ID)+"/notes", strings.NewReader("note=+++")), map[string]string{"id": itoa(p.ID)})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.createPersonNote(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "error=") {
		t.Errorf("expected error redirect, got %q", loc)
	}
	notes, _ := h.personNotes.ListByPerson(context.Background(), p.ID)
	if len(notes) != 0 {
		t.Errorf("expected no notes created")
	}
}

func TestCreatePersonNote_JSONRejectsWhitespace(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "JSON Whitespace Note Person")

	body := `{"note":"   "}`
	req := withRouteParams(withJSONHeader(httptest.NewRequest("POST", "/people/"+itoa(p.ID)+"/notes", bytes.NewBufferString(body))), map[string]string{"id": itoa(p.ID)})
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.createPersonNote(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d %s", w.Code, w.Body.String())
	}
	notes, _ := h.personNotes.ListByPerson(context.Background(), p.ID)
	if len(notes) != 0 {
		t.Errorf("expected no notes created")
	}
}

func TestCreatePersonNote_JSONRejectsInvalidDate(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "Invalid Date Note Person")

	body := `{"note":"valid note","note_date":"not-a-date"}`
	req := withRouteParams(withJSONHeader(httptest.NewRequest("POST", "/people/"+itoa(p.ID)+"/notes", bytes.NewBufferString(body))), map[string]string{"id": itoa(p.ID)})
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.createPersonNote(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d %s", w.Code, w.Body.String())
	}
	notes, _ := h.personNotes.ListByPerson(context.Background(), p.ID)
	if len(notes) != 0 {
		t.Errorf("expected no notes created")
	}
}

func TestPersonDetailTemplate_RendersNotesCard(t *testing.T) {
	h := newTestWebHandler(t)
	p := seedPerson(t, h, "Render Note Person")
	if _, err := h.personNotes.Create(context.Background(), p.ID, "RenderedNote", time.Now()); err != nil {
		t.Fatalf("create note: %v", err)
	}

	req := withRouteParams(httptest.NewRequest("GET", "/people/"+itoa(p.ID), nil).WithContext(withUserContext(context.Background())), map[string]string{"id": itoa(p.ID)})
	w := httptest.NewRecorder()
	h.viewPerson(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "person-notes-heading") {
		t.Error("expected notes card")
	}
	if !strings.Contains(body, "RenderedNote") {
		t.Error("expected note text rendered")
	}
	if !strings.Contains(body, "Add Note") {
		t.Error("expected add note button")
	}
}
