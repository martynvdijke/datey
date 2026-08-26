package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestExportSingleVCard_IncludesCategories(t *testing.T) {
	h := newTestWebHandler(t)
	personID := newTestPerson(t, h, "Cate Person")
	grp, err := h.groups.Create(t.Context(), "Family", "")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := h.groups.AddPerson(t.Context(), grp.ID, personID); err != nil {
		t.Fatalf("add person: %v", err)
	}

	r := chi.NewRouter()
	r.Get("/people/{id}/vcard", h.handleExportSingleVCard)

	req := httptest.NewRequest(http.MethodGet, "/people/"+itoa(personID)+"/vcard", nil)
	req = req.WithContext(withUserContext(req.Context()))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "CATEGORIES:Family") {
		t.Errorf("expected CATEGORIES:Family in export, got %q", body)
	}
}

func TestImportVCard_RestoresGroupMembership(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupVCardImportRouter(h)

	vcf := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:ImportCat Person\r\nCATEGORIES:Family,Friends\r\nEND:VCARD\r\n"
	req, err := vcardUploadRequest("/people/import", map[string]string{"cat.vcf": vcf}, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rr.Code, rr.Body.String())
	}
	people, _ := h.people.List(t.Context())
	if len(people) != 1 {
		t.Fatalf("expected 1 person, got %d", len(people))
	}
	groups, err := h.groups.ListByPerson(t.Context(), people[0].ID)
	if err != nil {
		t.Fatalf("ListByPerson: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %v", len(groups), groups)
	}
}

func TestGroupExport_IncludesNotesAndEvents(t *testing.T) {
	h := newTestWebHandler(t)
	grp, err := h.groups.Create(t.Context(), "ExportGroup", "")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if _, err := h.groupNotes.Create(t.Context(), grp.ID, "Lake note", time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("create note: %v", err)
	}
	if _, err := h.events.CreateForGroup(t.Context(), grp.ID, "anniversary", time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), "Reunion"); err != nil {
		t.Fatalf("create event: %v", err)
	}

	r := chi.NewRouter()
	r.Get("/groups/{id}/export", h.handleExportSingleGroup)
	req := httptest.NewRequest(http.MethodGet, "/groups/"+itoa(grp.ID)+"/export", nil)
	req = req.WithContext(withUserContext(req.Context()))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rr.Code, rr.Body.String())
	}
	var ge GroupExport
	if err := json.NewDecoder(rr.Body).Decode(&ge); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(ge.Notes) != 1 || ge.Notes[0].Note != "Lake note" {
		t.Errorf("notes = %v, want 1 with Lake note", ge.Notes)
	}
	if len(ge.Events) != 1 || ge.Events[0].Type != "anniversary" {
		t.Errorf("events = %v, want 1 anniversary", ge.Events)
	}
}
