package web

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/datey/datey/internal/immich"
	"github.com/datey/datey/internal/photos"
)

func TestImmichSyncResult_BothSections(t *testing.T) {
	h2, _ := newImmichTestHandler(t, `[{"id":"imm-1","name":"Matched One"},{"id":"imm-2","name":"Skipped One"}]`, "thumbbytes", "image/png")
	matched := seedPerson(t, h2, "Matched One")
	skipped := seedPerson(t, h2, "Skipped One")
	rel2, _ := h2.photoStore.Save(skipped.ID, "image/jpeg", []byte("orig"))
	_, _ = h2.people.SetPhotoState(context.Background(), skipped.ID, rel2, "image/jpeg", "upload")
	unmatched := seedPerson(t, h2, "Unmatched Person")
	disabledP := seedPerson(t, h2, "Disabled Person")
	disabledP, _ = h2.people.SetImmichPhoto(context.Background(), disabledP.ID, nil, true)

	w := httptest.NewRecorder()
	h2.immichBulkSync(w, httptest.NewRequest("POST", "/settings/immich/sync", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "4 people checked") {
		t.Errorf("expected total 4 in summary, got: %s", body[:500])
	}
	if !strings.Contains(body, "Matched (2)") {
		t.Errorf("expected Matched (2), got body: %s", body)
	}
	if !strings.Contains(body, "Unmatched (2)") {
		t.Errorf("expected Unmatched (2), got body: %s", body)
	}
	for _, p := range []struct {
		id   int
		name string
	}{{matched.ID, "Matched One"}, {skipped.ID, "Skipped One"}, {unmatched.ID, "Unmatched Person"}, {disabledP.ID, "Disabled Person"}} {
		link := `<a href="/people/` + itoa(p.id) + `">` + p.name + `</a>`
		if !strings.Contains(body, link) {
			t.Errorf("expected link %q in body", link)
		}
	}
	if !strings.Contains(body, "Imported") {
		t.Errorf("expected Imported badge")
	}
	if !strings.Contains(body, "uploaded photo kept") {
		t.Errorf("expected skipped reason 'uploaded photo kept'")
	}
	if !strings.Contains(body, "no match") {
		t.Errorf("expected 'no match' reason for unmatched")
	}
	if !strings.Contains(body, "disabled") {
		t.Errorf("expected 'disabled' reason")
	}
	if !strings.Contains(body, "Set override") {
		t.Errorf("expected Set override link in unmatched section")
	}
	if !strings.Contains(body, "Matched") || !strings.Contains(body, "Unmatched") {
		t.Errorf("both sections must be present")
	}
	if !strings.Contains(body, "1 imported") {
		t.Errorf("expected '1 imported' in summary")
	}
	if !strings.Contains(body, "1 skipped") {
		t.Errorf("expected '1 skipped' in summary")
	}
}

func TestImmichSyncResult_FailedBadge(t *testing.T) {
	peopleJSON := `[{"id":"imm-1","name":"Fail Person"}]`
	h, _ := newImmichTestHandler(t, peopleJSON, "not-an-image", "text/plain")
	seedPerson(t, h, "Fail Person")
	w := httptest.NewRecorder()
	h.immichBulkSync(w, httptest.NewRequest("POST", "/settings/immich/sync", nil))
	body := w.Body.String()
	if !strings.Contains(body, "Failed:") {
		t.Errorf("expected Failed badge, got: %s", body)
	}
	if !strings.Contains(body, `badge-danger`) {
		t.Errorf("expected badge-danger for failed")
	}
	if !strings.Contains(body, "Matched (1)") {
		t.Errorf("expected Matched (1)")
	}
}

func TestImmichSyncResult_TemplateDirect(t *testing.T) {
	h := newTestWebHandler(t)
	tmpl, ok := h.templates["immich_sync_result.html"]
	if !ok {
		t.Fatalf("template not loaded")
	}
	result := syncResult{
		Total: 3,
		Matched: []syncMatchedRow{
			{PersonID: 1, Name: "Alice", ImmichID: "imm-1", ImmichName: "Alice", Imported: true},
			{PersonID: 2, Name: "Bob", ImmichID: "imm-2", ImmichName: "Bob", Skipped: "uploaded photo kept"},
		},
		Unmatched: []syncUnmatchedRow{
			{PersonID: 3, Name: "Charlie", Reason: "no match"},
		},
		ImportedN:  1,
		SkippedN:   1,
		FailedN:    0,
		UnmatchedN: 1,
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "immich_sync_result.html", map[string]any{"Result": result}); err != nil {
		t.Fatalf("execute template: %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, "Matched (2)") {
		t.Errorf("expected Matched (2), got %s", body)
	}
	if !strings.Contains(body, "Unmatched (1)") {
		t.Errorf("expected Unmatched (1)")
	}
	if !strings.Contains(body, `<a href="/people/1">Alice</a>`) {
		t.Errorf("expected link for Alice")
	}
	if !strings.Contains(body, `<a href="/people/3">Charlie</a>`) {
		t.Errorf("expected link for Charlie")
	}
	if !strings.Contains(body, "Imported") {
		t.Errorf("expected Imported badge")
	}
	if !strings.Contains(body, "uploaded photo kept") {
		t.Errorf("expected skipped badge")
	}
	if !strings.Contains(body, "no match") {
		t.Errorf("expected no match reason")
	}
}

func TestSettings_ImmichSyncButton_DisabledWhenNotConfigured(t *testing.T) {
	h := newTestWebHandler(t)
	h.immich = immich.New("", "")
	h.cfg.ImmichURL = ""
	h.cfg.ImmichAPIKey = ""
	h.cfg.DataDir = t.TempDir()
	h.photoStore = photos.NewStore(h.cfg.DataDir)
	router := setupConfigRouter(h)
	req := httptest.NewRequest("GET", "/settings/config", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Sync with Immich") {
		t.Fatalf("Sync button not found in settings config page")
	}
	idx := strings.Index(body, `hx-post="/settings/immich/sync"`)
	if idx == -1 {
		t.Fatalf("immich sync button hx-post not found")
	}
	// find button tag surrounding idx
	start := strings.LastIndex(body[:idx], "<button")
	if start == -1 {
		t.Fatalf("button start not found")
	}
	end := strings.Index(body[start:], ">")
	if end == -1 {
		t.Fatalf("button end not found")
	}
	buttonTag := body[start : start+end+1]
	if !strings.Contains(buttonTag, " disabled") {
		t.Errorf("expected Sync button to have disabled attribute when Immich not configured, tag: %s", buttonTag)
	}
	if !strings.Contains(body, "Configure Immich URL and API key first") {
		t.Errorf("expected help text 'Configure Immich URL and API key first' when disabled")
	}
	// extract segment around button for help text check
	lo := idx - 500
	if lo < 0 {
		lo = 0
	}
	hi := idx + 500
	if hi > len(body) {
		hi = len(body)
	}
	segment := body[lo:hi]
	if strings.Contains(segment, "Matches every person") {
		t.Errorf("should not show enabled help text when disabled")
	}
}

func TestSettings_ImmichSyncButton_EnabledWhenConfigured(t *testing.T) {
	h := newTestWebHandler(t)
	h.cfg.ImmichURL = "https://photos.example.com"
	h.cfg.ImmichAPIKey = "secret-key"
	h.immich = immich.New(h.cfg.ImmichURL, h.cfg.ImmichAPIKey)
	h.cfg.DataDir = t.TempDir()
	h.photoStore = photos.NewStore(h.cfg.DataDir)
	router := setupConfigRouter(h)
	req := httptest.NewRequest("GET", "/settings/config", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Sync with Immich") {
		t.Fatalf("Sync button not found")
	}
	idx := strings.Index(body, `hx-post="/settings/immich/sync"`)
	if idx == -1 {
		t.Fatalf("immich sync button hx-post not found")
	}
	start := strings.LastIndex(body[:idx], "<button")
	if start == -1 {
		t.Fatalf("button start not found")
	}
	end := strings.Index(body[start:], ">")
	if end == -1 {
		t.Fatalf("button end not found")
	}
	buttonTag := body[start : start+end+1]
	if strings.Contains(buttonTag, " disabled") {
		t.Errorf("expected Sync button to be enabled (no disabled attr) when Immich configured, tag: %s", buttonTag)
	}
	if !strings.Contains(body, "Matches every person against Immich") {
		t.Errorf("expected enabled help text when configured")
	}
	lo := idx - 500
	if lo < 0 {
		lo = 0
	}
	hi := idx + 500
	if hi > len(body) {
		hi = len(body)
	}
	segment := body[lo:hi]
	if strings.Contains(segment, "Configure Immich URL and API key first") {
		t.Errorf("should not show disabled help text when enabled")
	}
}

var _ = http.StatusOK
