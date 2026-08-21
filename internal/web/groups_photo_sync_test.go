package web

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/immich"
	"github.com/datey/datey/internal/photos"
	"github.com/go-chi/chi/v5"
)

// withRouteParams attaches chi URL params to a request, as the router would.
func withRouteParams(r *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func seedPerson(t *testing.T, h *Handler, name string) *ent.Person {
	t.Helper()
	p, err := h.people.Create(context.Background(), name, "", "")
	if err != nil {
		t.Fatalf("seed person: %v", err)
	}
	return p
}

func seedGroup(t *testing.T, h *Handler, name string) *ent.Group {
	t.Helper()
	g, err := h.groups.Create(context.Background(), name, "")
	if err != nil {
		t.Fatalf("seed group: %v", err)
	}
	return g
}

func TestUploadPersonPhoto(t *testing.T) {
	h := newTestWebHandler(t)
	h.cfg.DataDir = t.TempDir()
	h.photoStore = photos.NewStore(h.cfg.DataDir)
	p := seedPerson(t, h, "Photo Person")

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	part, _ := mw.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="photo"; filename="me.png"`},
		"Content-Type":        {"image/png"},
	})
	part.Write([]byte("fake-png-bytes"))
	mw.Close()

	req := httptest.NewRequest("POST", "/people/1/photo/upload", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = withRouteParams(req, map[string]string{"id": itoa(p.ID)})
	w := httptest.NewRecorder()

	h.uploadPersonPhoto(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "success=Photo+uploaded") {
		t.Errorf("expected success redirect, got %q", loc)
	}

	got, err := h.people.Get(context.Background(), p.ID)
	if err != nil {
		t.Fatalf("get person: %v", err)
	}
	if got.PhotoPath == nil || *got.PhotoPath == "" {
		t.Fatal("expected photo_path recorded")
	}
	if got.PhotoSource == nil || *got.PhotoSource != "upload" {
		t.Errorf("expected photo_source upload, got %v", got.PhotoSource)
	}
	if got.PhotoContentType == nil || *got.PhotoContentType != "image/png" {
		t.Errorf("expected content type image/png, got %v", got.PhotoContentType)
	}
	if _, _, err := h.photoStore.Open(*got.PhotoPath); err != nil {
		t.Errorf("stored file unreadable: %v", err)
	}
}

func TestUploadPersonPhoto_RejectsNonImage(t *testing.T) {
	h := newTestWebHandler(t)
	h.cfg.DataDir = t.TempDir()
	h.photoStore = photos.NewStore(h.cfg.DataDir)
	p := seedPerson(t, h, "Picky Person")

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("photo", "notes.txt")
	fw.Write([]byte("plain text"))
	mw.Close()

	req := httptest.NewRequest("POST", "/people/1/photo/upload", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = withRouteParams(req, map[string]string{"id": itoa(p.ID)})
	w := httptest.NewRecorder()

	h.uploadPersonPhoto(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "error=") {
		t.Errorf("expected error redirect, got %q", loc)
	}
	got, _ := h.people.Get(context.Background(), p.ID)
	if got.PhotoPath != nil {
		t.Error("expected no photo state after rejected upload")
	}
}

func TestRemovePersonPhoto(t *testing.T) {
	h := newTestWebHandler(t)
	h.cfg.DataDir = t.TempDir()
	h.photoStore = photos.NewStore(h.cfg.DataDir)
	p := seedPerson(t, h, "Removable")

	rel, err := h.photoStore.Save(p.ID, "image/png", []byte("data"))
	if err != nil {
		t.Fatalf("save photo: %v", err)
	}
	if _, err := h.people.SetPhotoState(context.Background(), p.ID, rel, "image/png", "upload"); err != nil {
		t.Fatalf("set photo state: %v", err)
	}

	req := withRouteParams(httptest.NewRequest("POST", "/people/1/photo/remove", nil), map[string]string{"id": itoa(p.ID)})
	w := httptest.NewRecorder()
	h.removePersonPhoto(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "success=Photo+removed") {
		t.Errorf("expected success redirect, got %q", loc)
	}
	got, _ := h.people.Get(context.Background(), p.ID)
	if got.PhotoPath != nil || got.PhotoSource != nil {
		t.Error("expected photo state cleared")
	}
	if _, _, err := h.photoStore.Open(rel); err == nil {
		t.Error("expected stored file deleted")
	}
}

func TestViewGroup_MembersAndCandidates(t *testing.T) {
	h := newTestWebHandler(t)
	g := seedGroup(t, h, "Viewers")
	member := seedPerson(t, h, "Member One")
	outsider := seedPerson(t, h, "Outsider")
	_ = outsider
	if err := h.groups.AddPerson(context.Background(), g.ID, member.ID); err != nil {
		t.Fatalf("add member: %v", err)
	}

	req := withRouteParams(httptest.NewRequest("GET", "/groups/1", nil).WithContext(withUserContext(context.Background())), map[string]string{"id": itoa(g.ID)})
	w := httptest.NewRecorder()
	h.viewGroup(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	html := w.Body.String()
	if !strings.Contains(html, "Member One") {
		t.Error("expected member rendered")
	}
	if !strings.Contains(html, "Outsider") {
		t.Error("expected candidate rendered")
	}
}

func TestViewGroup_NotFound(t *testing.T) {
	h := newTestWebHandler(t)
	req := withRouteParams(httptest.NewRequest("GET", "/groups/999", nil).WithContext(withUserContext(context.Background())), map[string]string{"id": "999"})
	w := httptest.NewRecorder()
	h.viewGroup(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGroupMembershipAddRemove(t *testing.T) {
	h := newTestWebHandler(t)
	g := seedGroup(t, h, "Memberees")
	p := seedPerson(t, h, "Joiner")

	// Add
	req := withRouteParams(
		httptest.NewRequest("POST", "/groups/1/members/add", strings.NewReader("person_id="+itoa(p.ID))),
		map[string]string{"id": itoa(g.ID)})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.addGroupMember(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("add: expected 303, got %d", w.Code)
	}
	members, _ := h.groups.ListPeopleInGroup(context.Background(), g.ID)
	if len(members) != 1 {
		t.Fatalf("expected 1 member after add, got %d", len(members))
	}

	// Remove
	req = withRouteParams(httptest.NewRequest("POST", "/groups/1/members/2/remove", nil),
		map[string]string{"id": itoa(g.ID), "personID": itoa(p.ID)})
	w = httptest.NewRecorder()
	h.removeGroupMember(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("remove: expected 303, got %d", w.Code)
	}
	members, _ = h.groups.ListPeopleInGroup(context.Background(), g.ID)
	if len(members) != 0 {
		t.Errorf("expected 0 members after remove, got %d", len(members))
	}
}

func TestCreateGroupNote(t *testing.T) {
	h := newTestWebHandler(t)
	g := seedGroup(t, h, "Noted")

	// Missing note text redirects with error.
	req := withRouteParams(
		httptest.NewRequest("POST", "/groups/1/notes", strings.NewReader("note=")),
		map[string]string{"id": itoa(g.ID)})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.createGroupNote(w, req)
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "error=") {
		t.Errorf("expected error redirect for empty note, got %q", loc)
	}

	// Valid note with explicit date.
	form := "note=Trip+planned&note_date=2026-04-01"
	req = withRouteParams(
		httptest.NewRequest("POST", "/groups/1/notes", strings.NewReader(form)),
		map[string]string{"id": itoa(g.ID)})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	h.createGroupNote(w, req)
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "success=Note+added") {
		t.Errorf("expected success redirect, got %q", loc)
	}
	notes, err := h.groupNotes.ListByGroup(context.Background(), g.ID)
	if err != nil || len(notes) != 1 {
		t.Fatalf("expected 1 stored note, got %d (err=%v)", len(notes), err)
	}
	wantDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	if !notes[0].NoteDate.Equal(wantDate) {
		t.Errorf("expected note date %v, got %v", wantDate, notes[0].NoteDate)
	}

	// Delete
	req = withRouteParams(httptest.NewRequest("POST", "/groups/1/notes/1/delete", nil),
		map[string]string{"id": itoa(g.ID), "noteID": itoa(notes[0].ID)})
	w = httptest.NewRecorder()
	h.deleteGroupNote(w, req)
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "success=Note+deleted") {
		t.Errorf("expected delete success redirect, got %q", loc)
	}
	notes, _ = h.groupNotes.ListByGroup(context.Background(), g.ID)
	if len(notes) != 0 {
		t.Errorf("expected note deleted, got %d", len(notes))
	}
}

// newImmichTestHandler wires a handler to a fake Immich API server.
func newImmichTestHandler(t *testing.T, peopleJSON string, thumbnailBody string, thumbnailCT string) (*Handler, *httptest.Server) {
	t.Helper()
	h := newTestWebHandler(t)
	h.cfg.DataDir = t.TempDir()
	h.photoStore = photos.NewStore(h.cfg.DataDir)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/people":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(peopleJSON))
		case strings.HasSuffix(r.URL.Path, "/thumbnail"):
			w.Header().Set("Content-Type", thumbnailCT)
			_, _ = w.Write([]byte(thumbnailBody))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	h.immich = immich.New(srv.URL, "test-key")
	return h, srv
}

func TestImmichBulkSync_Disabled(t *testing.T) {
	h := newTestWebHandler(t)
	h.immich = immich.New("", "") // not configured

	w := httptest.NewRecorder()
	h.immichBulkSync(w, httptest.NewRequest("POST", "/settings/immich/sync", nil))

	if got := w.Header().Get("HX-Trigger"); !strings.Contains(got, "not configured") {
		t.Errorf("expected toast header about missing config, got %q", got)
	}
	var payload struct {
		ShowToast struct {
			Type string `json:"type"`
		} `json:"show-toast"`
	}
	if err := json.Unmarshal([]byte(w.Header().Get("HX-Trigger")), &payload); err != nil {
		t.Fatalf("toast payload: %v", err)
	}
	if payload.ShowToast.Type != "error" {
		t.Errorf("expected error toast, got %q", payload.ShowToast.Type)
	}
}

func TestImmichBulkSync_HappyPath(t *testing.T) {
	peopleJSON := `[
		{"id":"imm-1","name":"Matched Person"},
		{"id":"imm-2","name":"Disabled Person"},
		{"id":"imm-3","name":"Ghost"}
	]`
	h, _ := newImmichTestHandler(t, peopleJSON, "thumbbytes", "image/png")

	matched := seedPerson(t, h, "Matched Person")
	disabled := seedPerson(t, h, "Disabled Person")
	disabled, _ = h.people.SetImmichPhoto(context.Background(), disabled.ID, nil, true)
	unmatched := seedPerson(t, h, "Nobody Home")

	w := httptest.NewRecorder()
	h.immichBulkSync(w, httptest.NewRequest("POST", "/settings/immich/sync", nil))

	body := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(body, "Matched Person") {
		t.Error("expected matched person in result")
	}
	if !strings.Contains(body, "no match") {
		t.Error("expected unmatched reason in result")
	}

	got, _ := h.people.Get(context.Background(), matched.ID)
	if got.PhotoSource == nil || *got.PhotoSource != "immich" {
		t.Errorf("expected imported photo source immich, got %v", got.PhotoSource)
	}
	if _, _, err := h.photoStore.Open(*got.PhotoPath); err != nil {
		t.Errorf("imported file unreadable: %v", err)
	}

	gotUnmatched, _ := h.people.Get(context.Background(), unmatched.ID)
	if gotUnmatched.PhotoPath != nil {
		t.Error("unmatched person must not get a photo")
	}

	gotDisabled, _ := h.people.Get(context.Background(), disabled.ID)
	if gotDisabled.PhotoPath != nil {
		t.Error("disabled person must not get a photo")
	}
	if !strings.Contains(body, "disabled") {
		t.Error("expected disabled reason listed")
	}
}

func TestImmichBulkSync_UploadWinsOverImport(t *testing.T) {
	peopleJSON := `[{"id":"imm-1","name":"Uploader"}]`
	h, _ := newImmichTestHandler(t, peopleJSON, "should-not-be-used", "image/png")

	p := seedPerson(t, h, "Uploader")
	rel, err := h.photoStore.Save(p.ID, "image/jpeg", []byte("uploaded-original"))
	if err != nil {
		t.Fatalf("seed upload: %v", err)
	}
	if _, err := h.people.SetPhotoState(context.Background(), p.ID, rel, "image/jpeg", "upload"); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	w := httptest.NewRecorder()
	h.immichBulkSync(w, httptest.NewRequest("POST", "/settings/immich/sync", nil))

	body := w.Body.String()
	if !strings.Contains(body, "uploaded photo kept") {
		t.Errorf("expected skip notice, body: %s", body)
	}
	got, _ := h.people.Get(context.Background(), p.ID)
	if got.PhotoSource == nil || *got.PhotoSource != "upload" {
		t.Errorf("upload source must be preserved, got %v", got.PhotoSource)
	}
	rc, _, err := h.photoStore.Open(*got.PhotoPath)
	if err != nil {
		t.Fatalf("open photo: %v", err)
	}
	defer rc.Close()
	buf := new(bytes.Buffer)
	buf.ReadFrom(rc)
	if buf.String() != "uploaded-original" {
		t.Errorf("uploaded bytes overwritten: got %q", buf.String())
	}
}

func TestListPeople_GroupUnionFilter(t *testing.T) {
	h := newTestWebHandler(t)
	gA := seedGroup(t, h, "Alpha")
	gB := seedGroup(t, h, "Beta")
	inA := seedPerson(t, h, "In Alpha")
	inB := seedPerson(t, h, "In Beta")
	neither := seedPerson(t, h, "In Neither")
	_ = neither
	_ = gB
	if err := h.groups.AddPerson(context.Background(), gA.ID, inA.ID); err != nil {
		t.Fatal(err)
	}
	if err := h.groups.AddPerson(context.Background(), gB.ID, inB.ID); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/people?group="+itoa(gA.ID)+","+itoa(gB.ID), nil)
	w := httptest.NewRecorder()
	h.listPeople(w, req)

	html := w.Body.String()
	if !strings.Contains(html, "In Alpha") || !strings.Contains(html, "In Beta") {
		t.Error("expected union of both groups' members")
	}
	if strings.Contains(html, "In Neither") {
		t.Error("non-member must be excluded from group filter")
	}
}

func TestListPeople_GroupPrefixSearch(t *testing.T) {
	h := newTestWebHandler(t)
	g := seedGroup(t, h, "Family")
	inG := seedPerson(t, h, "Kin Member")
	outside := seedPerson(t, h, "Stranger")
	_ = outside
	if err := h.groups.AddPerson(context.Background(), g.ID, inG.ID); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/people?q=group%3AFamily", nil)
	w := httptest.NewRecorder()
	h.listPeople(w, req)

	html := w.Body.String()
	if !strings.Contains(html, "Kin Member") {
		t.Error("expected group member for group: prefix search")
	}
	if strings.Contains(html, "Stranger") {
		t.Error("non-member must be excluded from group: search")
	}
}
