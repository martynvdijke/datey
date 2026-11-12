package search

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/db"
	_ "github.com/mattn/go-sqlite3"
)

func TestBuildMatchExpr(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"single_term", "alice", `"alice"*`},
		{"two_terms", "alice smith", `"alice" "smith"*`},
		{"extra_whitespace", "  alice   smith  ", `"alice" "smith"*`},
		{"empty", "", ""},
		{"whitespace_only", "   \t\n ", ""},
		{"punctuation_only", "!!! ??? ***", ""},
		{"embedded_quotes", `he said "hi"`, `"he" "said" "hi"*`},
		{"operators", "AND OR NOT", `"AND" "OR" "NOT"*`},
		{"near", "NEAR(foo bar)", `"NEAR" "foo" "bar"*`},
		{"prefix_star", "foo* bar", `"foo" "bar"*`},
		{"leading_dash", "-alice", `"alice"*`},
		{"column_filter", "title:alice", `"title" "alice"*`},
		{"caret", "^alice", `"alice"*`},
		{"parentheses", "(alice)", `"alice"*`},
		{"unicode", "Zoë Müller", `"Zoë" "Müller"*`},
		{"apostrophe", "O'Brien", `"O" "Brien"*`},
		{"accents", "café", `"café"*`},
		{"backslash", `alice\ smith`, `"alice" "smith"*`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BuildMatchExpr(tc.query); got != tc.want {
				t.Fatalf("BuildMatchExpr(%q) = %q, want %q", tc.query, got, tc.want)
			}
		})
	}
}

func TestBuildMatchExpr_TermCap(t *testing.T) {
	expr := BuildMatchExpr("one two three four five six seven eight nine ten")
	want := `"one" "two" "three" "four" "five" "six" "seven" "eight"*`
	if expr != want {
		t.Fatalf("term cap: got %q, want %q", expr, want)
	}
	if n := strings.Count(expr, `"`); n != MaxTerms*2 {
		t.Fatalf("expected %d quotes for %d terms, got %d", MaxTerms*2, MaxTerms, n)
	}
}

func TestBuildMatchExpr_LengthCap(t *testing.T) {
	// 500 characters of one word must not reach FTS5 untruncated.
	long := strings.Repeat("a", MaxQueryRunes+100)
	expr := BuildMatchExpr(long)
	if len(expr) == 0 {
		t.Fatal("expected an expression")
	}
	if len(expr) > maxTermRunes+3 { // quotes + trailing star
		t.Fatalf("term not capped: length %d", len(expr))
	}
}

// newIndexedDB builds a migrated database with the real search index in place.
func newIndexedDB(t *testing.T) (*ent.Client, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "search.db")
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
	return client, conn
}

func TestSearcher_PrefixGroupingAndURL(t *testing.T) {
	client, conn := newIndexedDB(t)
	ctx := context.Background()
	if err := db.EnsureSearchIndex(ctx, client); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	now := time.Now()
	alice, err := client.Person.Create().
		SetName("Alice Smith").
		SetNotes("celebrates a birthday in July").
		SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	groups, err := New(conn).Search(ctx, "birt")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(groups) != 1 || groups[0].Entity != EntityPerson || len(groups[0].Results) != 1 {
		t.Fatalf("unexpected groups: %+v", groups)
	}
	res := groups[0].Results[0]
	if res.ID != alice.ID {
		t.Errorf("id = %d, want %d", res.ID, alice.ID)
	}
	if res.URL != "/people/"+itoa(alice.ID) {
		t.Errorf("url = %q", res.URL)
	}
	if !strings.Contains(string(res.Snippet), "<mark>birthday</mark>") {
		t.Errorf("snippet not highlighted: %q", res.Snippet)
	}

	// The title column is searched too.
	groups, err = New(conn).Search(ctx, "alice")
	if err != nil {
		t.Fatalf("search title: %v", err)
	}
	if len(groups) != 1 || groups[0].Count() != 1 {
		t.Fatalf("title search groups: %+v", groups)
	}
}

func TestSearcher_ChildEntitiesLinkToOwner(t *testing.T) {
	client, conn := newIndexedDB(t)
	ctx := context.Background()
	if err := db.EnsureSearchIndex(ctx, client); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	now := time.Now()
	alice, err := client.Person.Create().SetName("Alice Smith").
		SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	if _, err := client.PersonNote.Create().
		SetPersonID(alice.ID).SetNote("chocolate cake recipe").SetNoteDate(now).
		SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatalf("create note: %v", err)
	}
	if _, err := client.GiftIdea.Create().
		SetPersonID(alice.ID).SetTitle("Train set").SetNotes("wooden and painted").
		SetStatus("idea").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatalf("create gift: %v", err)
	}

	groups, err := New(conn).Search(ctx, "chocolate")
	if err != nil {
		t.Fatalf("search note: %v", err)
	}
	if len(groups) != 1 || groups[0].Entity != EntityPersonNote {
		t.Fatalf("groups = %+v", groups)
	}
	note := groups[0].Results[0]
	if note.URL != "/people/"+itoa(alice.ID) {
		t.Errorf("note url = %q, want person page", note.URL)
	}
	if note.ParentTitle != "Alice Smith" {
		t.Errorf("note parent title = %q", note.ParentTitle)
	}

	groups, err = New(conn).Search(ctx, "wooden")
	if err != nil {
		t.Fatalf("search gift: %v", err)
	}
	if len(groups) != 1 || groups[0].Entity != EntityGiftIdea {
		t.Fatalf("groups = %+v", groups)
	}
	gift := groups[0].Results[0]
	if gift.Title != "Train set" || gift.ParentTitle != "Alice Smith" {
		t.Errorf("gift title/parent = %q / %q", gift.Title, gift.ParentTitle)
	}
	if gift.URL != "/people/"+itoa(alice.ID) {
		t.Errorf("gift url = %q", gift.URL)
	}
}

func TestSearcher_Diacritics(t *testing.T) {
	client, conn := newIndexedDB(t)
	ctx := context.Background()
	if err := db.EnsureSearchIndex(ctx, client); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	now := time.Now()
	if _, err := client.Person.Create().SetName("Zoë Müller").
		SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatalf("create person: %v", err)
	}
	groups, err := New(conn).Search(ctx, "muller")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(groups) != 1 || groups[0].Count() != 1 {
		t.Fatalf("diacritic folding failed: %+v", groups)
	}
}

func TestSearcher_EmptyQuery(t *testing.T) {
	_, conn := newIndexedDB(t)
	groups, err := New(conn).Search(context.Background(), "   ")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if groups != nil {
		t.Fatalf("expected no groups, got %+v", groups)
	}
}

func TestSearcher_MissingIndex(t *testing.T) {
	// Schema migrated, index never created.
	_, conn := newIndexedDB(t)
	_, err := New(conn).Search(context.Background(), "alice")
	if !errors.Is(err, ErrIndexUnavailable) {
		t.Fatalf("err = %v, want ErrIndexUnavailable", err)
	}
}

func TestSearcher_ResultLimit(t *testing.T) {
	client, conn := newIndexedDB(t)
	ctx := context.Background()
	if err := db.EnsureSearchIndex(ctx, client); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	now := time.Now()
	for i := 0; i < DefaultLimit+15; i++ {
		if _, err := client.Person.Create().
			SetName("Person Number " + itoa(i)).
			SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
			t.Fatalf("create person %d: %v", i, err)
		}
	}
	groups, err := New(conn).Search(ctx, "person")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	total := 0
	for _, g := range groups {
		total += g.Count()
	}
	if total != DefaultLimit {
		t.Fatalf("got %d results, want the cap %d", total, DefaultLimit)
	}

	// A custom limit is honoured.
	s := New(conn)
	s.Limit = 5
	groups, err = s.Search(ctx, "person")
	if err != nil {
		t.Fatalf("limited search: %v", err)
	}
	total = 0
	for _, g := range groups {
		total += g.Count()
	}
	if total != 5 {
		t.Fatalf("got %d results, want 5", total)
	}
}

func TestSearcher_EscapesStoredHTML(t *testing.T) {
	client, conn := newIndexedDB(t)
	ctx := context.Background()
	if err := db.EnsureSearchIndex(ctx, client); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	now := time.Now()
	if _, err := client.Person.Create().
		SetName("Mallory").
		SetNotes(`<script>alert("x")</script> script kiddie`).
		SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatalf("create person: %v", err)
	}
	groups, err := New(conn).Search(ctx, "kiddie")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(groups) != 1 || groups[0].Count() != 1 {
		t.Fatalf("groups = %+v", groups)
	}
	snippet := string(groups[0].Results[0].Snippet)
	if strings.Contains(snippet, "<script>") {
		t.Fatalf("snippet contains live markup: %q", snippet)
	}
	if !strings.Contains(snippet, "&lt;script&gt;") {
		t.Fatalf("snippet not escaped: %q", snippet)
	}
}

// itoa avoids importing strconv for one call.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
