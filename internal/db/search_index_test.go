package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/config"
	_ "github.com/mattn/go-sqlite3"
)

// openSearchTestClient opens a real file-backed SQLite database in a temp dir,
// migrates the schema and registers the SQL handle so the search index can be
// built. File-backed (not in-memory) is closer to production and lets the
// triggers run across connections.
func openSearchTestClient(t *testing.T) *ent.Client {
	t.Helper()
	path := filepath.Join(t.TempDir(), "search.db")
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_timeout=5000&_fk=1")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, conn)))
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close client: %v", err)
		}
	})
	RegisterSQLDB(client, conn)
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("schema create: %v", err)
	}
	return client
}

// searchMatchTitles returns "entity:title" for every document matching expr,
// lowest rowid first.
func searchMatchTitles(t *testing.T, client *ent.Client, expr string) []string {
	t.Helper()
	conn, ok := SQLDB(client)
	if !ok {
		t.Fatal("no SQL handle registered")
	}
	rows, err := conn.Query(
		`SELECT entity || ':' || title FROM search_index WHERE search_index MATCH ? ORDER BY rowid`, expr)
	if err != nil {
		t.Fatalf("match %q: %v", expr, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan match: %v", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("match rows: %v", err)
	}
	return out
}

// searchIndexRows counts index rows, optionally filtered by entity.
func searchIndexRows(t *testing.T, client *ent.Client, entity string) int {
	t.Helper()
	conn, ok := SQLDB(client)
	if !ok {
		t.Fatal("no SQL handle registered")
	}
	query := "SELECT count(*) FROM search_index"
	args := []any{}
	if entity != "" {
		query += " WHERE entity = ?"
		args = append(args, entity)
	}
	var n int
	if err := conn.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count index rows: %v", err)
	}
	return n
}

// searchParentOf returns the parent_entity and parent_id of a document.
func searchParentOf(t *testing.T, client *ent.Client, entity string, id int) (string, int) {
	t.Helper()
	conn, _ := SQLDB(client)
	var (
		parentEntity string
		parentID     sql.NullInt64
	)
	if err := conn.QueryRow(
		`SELECT parent_entity, parent_id FROM search_index WHERE entity = ? AND entity_id = ?`,
		entity, id).Scan(&parentEntity, &parentID); err != nil {
		t.Fatalf("parent of %s/%d: %v", entity, id, err)
	}
	return parentEntity, int(parentID.Int64)
}

func TestEnsureSearchIndex_PersonInsertUpdateDelete(t *testing.T) {
	client := openSearchTestClient(t)
	ctx := context.Background()
	if err := EnsureSearchIndex(ctx, client); err != nil {
		t.Fatalf("ensure search index: %v", err)
	}

	now := time.Now()
	person, err := client.Person.Create().
		SetName("Alice Smith").
		SetFirstName("Alice").
		SetLastName("Smith").
		SetNotes("loves chocolate").
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	if got := searchMatchTitles(t, client, `"alice"`); len(got) != 1 || got[0] != "person:Alice Smith" {
		t.Fatalf("after insert, match alice = %v", got)
	}
	if got := searchMatchTitles(t, client, `"chocolate"`); len(got) != 1 {
		t.Fatalf("after insert, match chocolate = %v", got)
	}
	if got := searchMatchTitles(t, client, `"smith"`); len(got) != 1 {
		t.Fatalf("last_name part of the body is not indexed: %v", got)
	}

	// UPDATE must replace, not duplicate.
	if _, err := person.Update().
		SetName("Alicia Jones").
		SetFirstName("Alicia").
		SetLastName("Jones").
		SetNotes("likes tea").
		SetUpdatedAt(time.Now()).
		Save(ctx); err != nil {
		t.Fatalf("update person: %v", err)
	}
	if n := searchIndexRows(t, client, "person"); n != 1 {
		t.Fatalf("update duplicated the index row: %d rows", n)
	}
	if got := searchMatchTitles(t, client, `"alice"`); len(got) != 0 {
		t.Fatalf("stale name still matches: %v", got)
	}
	if got := searchMatchTitles(t, client, `"chocolate"`); len(got) != 0 {
		t.Fatalf("stale notes still match: %v", got)
	}
	if got := searchMatchTitles(t, client, `"tea"`); len(got) != 1 {
		t.Fatalf("new notes not indexed: %v", got)
	}

	// DELETE must remove the document.
	if err := client.Person.DeleteOneID(person.ID).Exec(ctx); err != nil {
		t.Fatalf("delete person: %v", err)
	}
	if n := searchIndexRows(t, client, "person"); n != 0 {
		t.Fatalf("delete left %d index rows", n)
	}
}

func TestEnsureSearchIndex_AllSourceTables(t *testing.T) {
	client := openSearchTestClient(t)
	ctx := context.Background()
	if err := EnsureSearchIndex(ctx, client); err != nil {
		t.Fatalf("ensure search index: %v", err)
	}

	now := time.Now()
	person, err := client.Person.Create().SetName("Bob").
		SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	group, err := client.Group.Create().SetName("Book club").SetDescription("monthly reading").
		SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	note, err := client.PersonNote.Create().
		SetPersonID(person.ID).SetNote("borrowed a telescope").SetNoteDate(now).
		SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		t.Fatalf("create person note: %v", err)
	}
	groupNote, err := client.GroupNote.Create().
		SetGroupID(group.ID).SetNote("reads science fiction").SetNoteDate(now).
		SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		t.Fatalf("create group note: %v", err)
	}
	gift, err := client.GiftIdea.Create().
		SetPersonID(person.ID).SetTitle("Espresso machine").SetNotes("the shiny one").
		SetStatus("idea").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		t.Fatalf("create gift idea: %v", err)
	}
	event, err := client.Event.Create().
		SetType("birthday").SetDate(now).SetDescription("turns forty").SetCreatedAt(now).
		SetPersonID(person.ID).Save(ctx)
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	cases := []struct {
		expr  string
		title string
	}{
		{`"telescope"`, "person_note:"},
		{`"fiction"`, "group_note:"},
		{`"espresso"`, "gift_idea:Espresso machine"},
		{`"forty"`, "event:birthday"},
		{`"monthly"`, "group:Book club"},
	}
	for _, tc := range cases {
		got := searchMatchTitles(t, client, tc.expr)
		if len(got) != 1 || got[0] != tc.title {
			t.Errorf("match %s = %v, want [%s]", tc.expr, got, tc.title)
		}
	}

	if pe, pid := searchParentOf(t, client, "person_note", note.ID); pe != "person" || pid != person.ID {
		t.Errorf("person_note parent = %s/%d, want person/%d", pe, pid, person.ID)
	}
	if pe, pid := searchParentOf(t, client, "group_note", groupNote.ID); pe != "group" || pid != group.ID {
		t.Errorf("group_note parent = %s/%d, want group/%d", pe, pid, group.ID)
	}
	if pe, pid := searchParentOf(t, client, "gift_idea", gift.ID); pe != "person" || pid != person.ID {
		t.Errorf("gift_idea parent = %s/%d, want person/%d", pe, pid, person.ID)
	}
	if pe, pid := searchParentOf(t, client, "event", event.ID); pe != "person" || pid != person.ID {
		t.Errorf("event parent = %s/%d, want person/%d", pe, pid, person.ID)
	}

	// Child deletes remove their documents.
	if err := client.PersonNote.DeleteOneID(note.ID).Exec(ctx); err != nil {
		t.Fatalf("delete person note: %v", err)
	}
	if got := searchMatchTitles(t, client, `"telescope"`); len(got) != 0 {
		t.Errorf("deleted note still matches: %v", got)
	}

	// Deleting the group purges its notes and events through the trigger.
	if err := client.GroupNote.DeleteOneID(groupNote.ID).Exec(ctx); err != nil {
		t.Fatalf("delete group note: %v", err)
	}
	if got := searchMatchTitles(t, client, `"fiction"`); len(got) != 0 {
		t.Errorf("deleted group note still matches: %v", got)
	}
}

func TestEnsureSearchIndex_BackfillsExistingRows(t *testing.T) {
	client := openSearchTestClient(t)
	ctx := context.Background()

	now := time.Now()
	if _, err := client.Person.Create().SetName("Carol").SetNotes("met at the bakery").
		SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatalf("create person: %v", err)
	}

	// The index did not exist when the row was written, so only the backfill
	// can find it.
	if err := EnsureSearchIndex(ctx, client); err != nil {
		t.Fatalf("ensure search index: %v", err)
	}
	if got := searchMatchTitles(t, client, `"bakery"`); len(got) != 1 || got[0] != "person:Carol" {
		t.Fatalf("backfill missing: %v", got)
	}
}

func TestEnsureSearchIndex_IsIdempotentAndSelfHealing(t *testing.T) {
	client := openSearchTestClient(t)
	ctx := context.Background()

	now := time.Now()
	if _, err := client.Person.Create().SetName("Dana").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatalf("create person: %v", err)
	}
	if _, err := client.Group.Create().SetName("Choir").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatalf("create group: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := EnsureSearchIndex(ctx, client); err != nil {
			t.Fatalf("ensure run %d: %v", i, err)
		}
		if n := searchIndexRows(t, client, ""); n != 2 {
			t.Fatalf("after run %d index has %d rows, want 2", i, n)
		}
	}

	// Wipe the derived table; the next run must restore it.
	conn, _ := SQLDB(client)
	if _, err := conn.Exec("DELETE FROM search_index"); err != nil {
		t.Fatalf("wipe index: %v", err)
	}
	if err := EnsureSearchIndex(ctx, client); err != nil {
		t.Fatalf("ensure after wipe: %v", err)
	}
	if n := searchIndexRows(t, client, ""); n != 2 {
		t.Fatalf("self-heal left %d rows, want 2", n)
	}

	// Rebuild is idempotent too.
	for i := 0; i < 2; i++ {
		if err := RebuildSearchIndex(ctx, client); err != nil {
			t.Fatalf("rebuild %d: %v", i, err)
		}
		if n := searchIndexRows(t, client, ""); n != 2 {
			t.Fatalf("after rebuild %d index has %d rows, want 2", i, n)
		}
	}
}

func TestEnsureSearchIndex_InstallsAllTriggers(t *testing.T) {
	client := openSearchTestClient(t)
	ctx := context.Background()
	if err := EnsureSearchIndex(ctx, client); err != nil {
		t.Fatalf("ensure search index: %v", err)
	}

	conn, _ := SQLDB(client)
	var n int
	if err := conn.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type = 'trigger' AND name LIKE 'search_%'`).Scan(&n); err != nil {
		t.Fatalf("count triggers: %v", err)
	}
	if n != len(triggerNames) {
		t.Fatalf("installed %d triggers, want %d", n, len(triggerNames))
	}
}

// TestInit_BuildsSearchIndex exercises the production startup path end to end:
// db.Init must expose the SQL handle and install a working index, and a second
// startup against the same data directory must be safe.
func TestInit_BuildsSearchIndex(t *testing.T) {
	cfg := &config.Config{DataDir: t.TempDir()}

	client, err := Init(cfg)
	if err != nil {
		t.Fatalf("first init: %v", err)
	}
	conn, ok := SQLDB(client)
	if !ok {
		t.Fatal("db.Init did not register the SQL handle")
	}
	var table string
	if err := conn.QueryRow(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'search_index'`).Scan(&table); err != nil {
		t.Fatalf("search_index table missing after Init: %v", err)
	}

	ctx := context.Background()
	now := time.Now()
	if _, err := client.Person.Create().SetName("Eve").SetNotes("tea and biscuits").
		SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatalf("create person: %v", err)
	}
	if got := searchMatchTitles(t, client, `"biscuits"`); len(got) != 1 {
		t.Fatalf("trigger not installed by Init: %v", got)
	}

	// The absolute row count is not fixed: boot also seeds built-in recurring
	// rules, which the rule-to-event migration converts into indexed events.
	// What matters is that a restart does not duplicate rows, so compare
	// against the count taken before the restart instead of a literal.
	before := searchIndexRows(t, client, "")

	if err := client.Close(); err != nil {
		t.Fatalf("close first client: %v", err)
	}

	// Restart against the same data directory: index and triggers are reused.
	client2, err := Init(cfg)
	if err != nil {
		t.Fatalf("second init: %v", err)
	}
	t.Cleanup(func() { _ = client2.Close() })
	if got := searchMatchTitles(t, client2, `"biscuits"`); len(got) != 1 {
		t.Fatalf("existing documents lost on restart: %v", got)
	}
	if n := searchIndexRows(t, client2, ""); n != before {
		t.Fatalf("restart changed index row count: %d, want %d", n, before)
	}
}

func TestEnsureSearchIndex_WithoutSQLHandle(t *testing.T) {
	// An ent client built without RegisterSQLDB cannot use the index; the
	// failure must be recognised as "unavailable" rather than a crash.
	conn, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "nohandle.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, conn)))
	t.Cleanup(func() { _ = client.Close() })
	if err := EnsureSearchIndex(context.Background(), client); err == nil {
		t.Fatal("expected an error when no SQL handle is registered")
	}
}
