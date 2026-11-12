package web

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/db"
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/notifier"
	"github.com/go-chi/chi/v5"
	_ "github.com/mattn/go-sqlite3"
)

// newSearchTestHandler builds a handler over a fresh file-backed database with
// the SQL handle registered, so the FTS5 index can be created when a test asks
// for it. File-backed (not the shared in-memory DSN used by newTestWebHandler)
// keeps the search tests independent of each other.
func newSearchTestHandler(t *testing.T, name string) (*Handler, *ent.Client) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name+".db")
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_timeout=5000&_fk=1")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, conn)))
	t.Cleanup(func() { _ = client.Close() })
	db.RegisterSQLDB(client, conn)
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("schema create: %v", err)
	}
	cfg := &config.Config{ReminderDays: 7, SchedulerCatchup: true}
	return NewHandler(cfg, client, notifier.NewRegistry(), logstore.NewStore(100)), client
}

func setupSearchRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Get("/search", h.handleSearch)
	return r
}

func getSearch(t *testing.T, h *Handler, target string, htmx bool) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	rr := httptest.NewRecorder()
	setupSearchRouter(h).ServeHTTP(rr, req)
	return rr.Code, rr.Body.String()
}

// seedSearchData creates a person with a note and a gift idea.
func seedSearchData(t *testing.T, h *Handler, client *ent.Client) (*ent.Person, *ent.PersonNote) {
	t.Helper()
	ctx := context.Background()
	now := time.Now()
	person, err := client.Person.Create().SetName("Alice Smith").
		SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	note, err := client.PersonNote.Create().
		SetPersonID(person.ID).SetNote("Alice loves chocolate cake").SetNoteDate(now).
		SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if _, err := client.GiftIdea.Create().
		SetPersonID(person.ID).SetTitle("Chocolate box").SetNotes("dark, 70%").
		SetStatus("idea").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatalf("create gift idea: %v", err)
	}
	_ = h
	return person, note
}

func TestSearch_RendersGroupedResults(t *testing.T) {
	h, client := newSearchTestHandler(t, "search_grouped")
	ctx := context.Background()
	if err := db.EnsureSearchIndex(ctx, client); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	person, _ := seedSearchData(t, h, client)

	code, body := getSearch(t, h, "/search?q=chocolate", false)
	if code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", code, body)
	}
	mustContain := []string{
		`id="search-group-person_note"`,
		`id="search-group-gift_idea"`,
		"Notes",
		"Gift ideas",
		"Alice Smith",
		"<mark>Chocolate</mark>",
		"/people/" + itoa(person.ID),
		"2 result(s) for",
		"Datey - Search",
	}
	for _, want := range mustContain {
		if !strings.Contains(body, want) {
			t.Errorf("page is missing %q", want)
		}
	}
	// Without a person match there must be no People section.
	if strings.Contains(body, `id="search-group-person"`) {
		t.Error("unexpected people section for a notes/gift query")
	}
}

func TestSearch_EmptyResultState(t *testing.T) {
	h, client := newSearchTestHandler(t, "search_empty")
	ctx := context.Background()
	if err := db.EnsureSearchIndex(ctx, client); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	seedSearchData(t, h, client)

	code, body := getSearch(t, h, "/search?q=zzzznotfound", false)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if !strings.Contains(body, "No matches found.") {
		t.Error("empty state missing")
	}
	if !strings.Contains(body, "0 result(s) for") {
		t.Error("zero-result summary missing")
	}
}

func TestSearch_EmptyQueryShowsHint(t *testing.T) {
	h, client := newSearchTestHandler(t, "search_hint")
	if err := db.EnsureSearchIndex(context.Background(), client); err != nil {
		t.Fatalf("ensure index: %v", err)
	}

	code, body := getSearch(t, h, "/search", false)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if !strings.Contains(body, "Search across people, notes, gift ideas, events and groups.") {
		t.Error("hint missing for an empty query")
	}
	if strings.Contains(body, "result(s) for") {
		t.Error("a summary should not be shown without a query")
	}
}

func TestSearch_HTMXReturnsContentOnly(t *testing.T) {
	h, client := newSearchTestHandler(t, "search_htmx")
	if err := db.EnsureSearchIndex(context.Background(), client); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	seedSearchData(t, h, client)

	code, body := getSearch(t, h, "/search?q=chocolate", true)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if strings.Contains(body, "<!DOCTYPE html>") || strings.Contains(body, "<html") {
		t.Error("HTMX response must not be a full document")
	}
	if !strings.Contains(body, `id="search-group-person_note"`) {
		t.Error("HTMX response is missing the results fragment")
	}
}

func TestSearch_DegradesWhenIndexMissing(t *testing.T) {
	// No EnsureSearchIndex call: the FTS5 table does not exist.
	h, _ := newSearchTestHandler(t, "search_missing")

	code, body := getSearch(t, h, "/search?q=alice", false)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (degrade, not fail)", code)
	}
	if !strings.Contains(body, "Full-text search is unavailable right now.") {
		t.Errorf("unavailable state missing: %s", body)
	}
	if !strings.Contains(body, `id="search-unavailable"`) {
		t.Error("unavailable alert element missing")
	}
}

func TestSearch_EscapesQueryAndStoredMarkup(t *testing.T) {
	h, client := newSearchTestHandler(t, "search_escape")
	ctx := context.Background()
	if err := db.EnsureSearchIndex(ctx, client); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	now := time.Now()
	if _, err := client.Person.Create().
		SetName("Mallory").
		SetNotes(`<script>alert(1)</script> alert notes`).
		SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatalf("create person: %v", err)
	}

	code, body := getSearch(t, h, "/search?q=alert", false)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Error("stored markup was rendered unescaped")
	}
	if !strings.Contains(body, "&lt;script&gt;") || !strings.Contains(body, "&lt;/script&gt;") {
		t.Error("expected escaped stored markup in the snippet")
	}

	// A crafted query must never be reflected as markup.
	code, body = getSearch(t, h, `/search?q=%3Cscript%3Ealert(2)%3C%2Fscript%3E`, false)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if strings.Contains(body, "<script>alert(2)</script>") {
		t.Error("query reflected unescaped")
	}
}
