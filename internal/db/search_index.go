package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/datey/datey/ent"
)

// searchIndexVersion is bumped whenever the FTS5 schema or the indexed columns
// change. A mismatch makes the next EnsureSearchIndex rebuild the whole index,
// which is the cheap way to migrate derived data on an existing install.
const searchIndexVersion = 1

// Row-id namespaces inside the single FTS5 table (one namespace per source
// table, so ids never collide across entities):
//
//	1_000_000_000  persons
//	2_000_000_000  person_notes
//	3_000_000_000  group_notes
//	4_000_000_000  gift_ideas
//	5_000_000_000  events
//	6_000_000_000  groups
//
// Triggers derive a stable rowid from (namespace, primary key), so replacing a
// document is a cheap delete-by-rowid followed by an insert and an UPDATE can
// never duplicate a row. int32-sized primary keys leave ample headroom.

// searchIndexDDL creates the FTS5 table plus the small metadata table used to
// remember which index version was built.
//
// The table is a regular (not contentless) FTS5 table: storing the UNINDEXED
// columns makes `DELETE ... WHERE entity = ? AND parent_id = ?` possible, which
// the parent-delete triggers rely on to purge child documents. The tokenizer
// folds diacritics so "Muller" finds "Müller".
var searchIndexDDL = []string{
	`CREATE VIRTUAL TABLE IF NOT EXISTS search_index USING fts5(
		entity UNINDEXED,
		entity_id UNINDEXED,
		parent_entity UNINDEXED,
		parent_id UNINDEXED,
		title,
		body,
		tokenize = 'unicode61 remove_diacritics 2'
	)`,
	`CREATE TABLE IF NOT EXISTS search_index_meta (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		version INTEGER NOT NULL
	)`,
}

// searchIndexTriggers keep the index in sync with every write path — ent
// repositories, CSV/vCard imports, CardDAV/Google sync and cascade deletes —
// without a single Go call site. They are dropped and recreated on every
// startup so a schema migration that recreates a table cannot leave stale
// trigger definitions behind.
//
// Column names are the SQLite names from ent/migrate/schema.go; a typo here
// fails at runtime, not at compile time, so they are verified by
// search_index_test.go.
var searchIndexTriggers = []string{
	// persons
	`CREATE TRIGGER search_persons_ai AFTER INSERT ON persons BEGIN
		INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
		VALUES (
			1000000000 + NEW.id, 'person', NEW.id, '', NULL, NEW.name,
			trim(coalesce(NEW.first_name,'') || ' ' || coalesce(NEW.middle_name,'') || ' ' ||
			     coalesce(NEW.last_name,'') || ' ' || coalesce(NEW.notes,''))
		);
	END`,
	`CREATE TRIGGER search_persons_au AFTER UPDATE ON persons BEGIN
		DELETE FROM search_index WHERE rowid = 1000000000 + OLD.id;
		INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
		VALUES (
			1000000000 + NEW.id, 'person', NEW.id, '', NULL, NEW.name,
			trim(coalesce(NEW.first_name,'') || ' ' || coalesce(NEW.middle_name,'') || ' ' ||
			     coalesce(NEW.last_name,'') || ' ' || coalesce(NEW.notes,''))
		);
	END`,
	// Deleting a person also purges its child documents. Child rows normally
	// disappear through their own delete triggers; this is the safety net for
	// paths that remove the parent without touching the children.
	`CREATE TRIGGER search_persons_ad AFTER DELETE ON persons BEGIN
		DELETE FROM search_index WHERE rowid = 1000000000 + OLD.id;
		DELETE FROM search_index WHERE entity = 'person_note' AND parent_id = OLD.id;
		DELETE FROM search_index WHERE entity = 'gift_idea' AND parent_id = OLD.id;
		DELETE FROM search_index WHERE entity = 'event' AND parent_entity = 'person' AND parent_id = OLD.id;
	END`,

	// person_notes
	`CREATE TRIGGER search_person_notes_ai AFTER INSERT ON person_notes BEGIN
		INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
		VALUES (2000000000 + NEW.id, 'person_note', NEW.id, 'person', NEW.person_timeline, '', NEW.note);
	END`,
	`CREATE TRIGGER search_person_notes_au AFTER UPDATE ON person_notes BEGIN
		DELETE FROM search_index WHERE rowid = 2000000000 + OLD.id;
		INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
		VALUES (2000000000 + NEW.id, 'person_note', NEW.id, 'person', NEW.person_timeline, '', NEW.note);
	END`,
	`CREATE TRIGGER search_person_notes_ad AFTER DELETE ON person_notes BEGIN
		DELETE FROM search_index WHERE rowid = 2000000000 + OLD.id;
	END`,

	// group_notes
	`CREATE TRIGGER search_group_notes_ai AFTER INSERT ON group_notes BEGIN
		INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
		VALUES (3000000000 + NEW.id, 'group_note', NEW.id, 'group', NEW.group_notes, '', NEW.note);
	END`,
	`CREATE TRIGGER search_group_notes_au AFTER UPDATE ON group_notes BEGIN
		DELETE FROM search_index WHERE rowid = 3000000000 + OLD.id;
		INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
		VALUES (3000000000 + NEW.id, 'group_note', NEW.id, 'group', NEW.group_notes, '', NEW.note);
	END`,
	`CREATE TRIGGER search_group_notes_ad AFTER DELETE ON group_notes BEGIN
		DELETE FROM search_index WHERE rowid = 3000000000 + OLD.id;
	END`,

	// gift_ideas
	`CREATE TRIGGER search_gift_ideas_ai AFTER INSERT ON gift_ideas BEGIN
		INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
		VALUES (4000000000 + NEW.id, 'gift_idea', NEW.id, 'person', NEW.person_gift_ideas,
			NEW.title, coalesce(NEW.notes,''));
	END`,
	`CREATE TRIGGER search_gift_ideas_au AFTER UPDATE ON gift_ideas BEGIN
		DELETE FROM search_index WHERE rowid = 4000000000 + OLD.id;
		INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
		VALUES (4000000000 + NEW.id, 'gift_idea', NEW.id, 'person', NEW.person_gift_ideas,
			NEW.title, coalesce(NEW.notes,''));
	END`,
	`CREATE TRIGGER search_gift_ideas_ad AFTER DELETE ON gift_ideas BEGIN
		DELETE FROM search_index WHERE rowid = 4000000000 + OLD.id;
	END`,

	// events
	`CREATE TRIGGER search_events_ai AFTER INSERT ON events BEGIN
		INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
		VALUES (
			5000000000 + NEW.id, 'event', NEW.id,
			CASE WHEN NEW.person_events IS NOT NULL THEN 'person'
			     WHEN NEW.group_events IS NOT NULL THEN 'group'
			     WHEN NEW.contact_events IS NOT NULL THEN 'contact'
			     ELSE '' END,
			coalesce(NEW.person_events, NEW.group_events, NEW.contact_events),
			NEW.type,
			trim(coalesce(NEW.description,'') || ' ' || coalesce(NEW.notes,''))
		);
	END`,
	`CREATE TRIGGER search_events_au AFTER UPDATE ON events BEGIN
		DELETE FROM search_index WHERE rowid = 5000000000 + OLD.id;
		INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
		VALUES (
			5000000000 + NEW.id, 'event', NEW.id,
			CASE WHEN NEW.person_events IS NOT NULL THEN 'person'
			     WHEN NEW.group_events IS NOT NULL THEN 'group'
			     WHEN NEW.contact_events IS NOT NULL THEN 'contact'
			     ELSE '' END,
			coalesce(NEW.person_events, NEW.group_events, NEW.contact_events),
			NEW.type,
			trim(coalesce(NEW.description,'') || ' ' || coalesce(NEW.notes,''))
		);
	END`,
	`CREATE TRIGGER search_events_ad AFTER DELETE ON events BEGIN
		DELETE FROM search_index WHERE rowid = 5000000000 + OLD.id;
	END`,

	// groups
	`CREATE TRIGGER search_groups_ai AFTER INSERT ON groups BEGIN
		INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
		VALUES (6000000000 + NEW.id, 'group', NEW.id, '', NULL, NEW.name, coalesce(NEW.description,''));
	END`,
	`CREATE TRIGGER search_groups_au AFTER UPDATE ON groups BEGIN
		DELETE FROM search_index WHERE rowid = 6000000000 + OLD.id;
		INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
		VALUES (6000000000 + NEW.id, 'group', NEW.id, '', NULL, NEW.name, coalesce(NEW.description,''));
	END`,
	`CREATE TRIGGER search_groups_ad AFTER DELETE ON groups BEGIN
		DELETE FROM search_index WHERE rowid = 6000000000 + OLD.id;
		DELETE FROM search_index WHERE entity = 'group_note' AND parent_id = OLD.id;
		DELETE FROM search_index WHERE entity = 'event' AND parent_entity = 'group' AND parent_id = OLD.id;
	END`,
}

// triggerNames lists the triggers in searchIndexTriggers so they can be
// dropped before being recreated.
var triggerNames = []string{
	"search_persons_ai", "search_persons_au", "search_persons_ad",
	"search_person_notes_ai", "search_person_notes_au", "search_person_notes_ad",
	"search_group_notes_ai", "search_group_notes_au", "search_group_notes_ad",
	"search_gift_ideas_ai", "search_gift_ideas_au", "search_gift_ideas_ad",
	"search_events_ai", "search_events_au", "search_events_ad",
	"search_groups_ai", "search_groups_au", "search_groups_ad",
}

// indexSource describes how one source table maps onto search_index.
type indexSource struct {
	// insertAll copies every source row; insertMissing adds only rows the
	// index does not have yet, and pruneOrphans drops index rows whose source
	// (or parent) no longer exists.
	insertAll     string
	insertMissing string
	pruneOrphans  string
}

var searchIndexSources = []indexSource{
	{
		insertAll: `INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
			SELECT 1000000000 + id, 'person', id, '', NULL, name,
			       trim(coalesce(first_name,'') || ' ' || coalesce(middle_name,'') || ' ' ||
			            coalesce(last_name,'') || ' ' || coalesce(notes,''))
			FROM persons`,
		insertMissing: `INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
			SELECT 1000000000 + id, 'person', id, '', NULL, name,
			       trim(coalesce(first_name,'') || ' ' || coalesce(middle_name,'') || ' ' ||
			            coalesce(last_name,'') || ' ' || coalesce(notes,''))
			FROM persons
			WHERE id NOT IN (SELECT entity_id FROM search_index WHERE entity = 'person')`,
		pruneOrphans: `DELETE FROM search_index
			WHERE entity = 'person' AND entity_id NOT IN (SELECT id FROM persons)`,
	},
	{
		insertAll: `INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
			SELECT 2000000000 + id, 'person_note', id, 'person', person_timeline, '', note
			FROM person_notes`,
		insertMissing: `INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
			SELECT 2000000000 + id, 'person_note', id, 'person', person_timeline, '', note
			FROM person_notes
			WHERE id NOT IN (SELECT entity_id FROM search_index WHERE entity = 'person_note')`,
		pruneOrphans: `DELETE FROM search_index
			WHERE entity = 'person_note'
			  AND (entity_id NOT IN (SELECT id FROM person_notes)
			       OR parent_id NOT IN (SELECT id FROM persons))`,
	},
	{
		insertAll: `INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
			SELECT 3000000000 + id, 'group_note', id, 'group', group_notes, '', note
			FROM group_notes`,
		insertMissing: `INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
			SELECT 3000000000 + id, 'group_note', id, 'group', group_notes, '', note
			FROM group_notes
			WHERE id NOT IN (SELECT entity_id FROM search_index WHERE entity = 'group_note')`,
		pruneOrphans: `DELETE FROM search_index
			WHERE entity = 'group_note'
			  AND (entity_id NOT IN (SELECT id FROM group_notes)
			       OR parent_id NOT IN (SELECT id FROM groups))`,
	},
	{
		insertAll: `INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
			SELECT 4000000000 + id, 'gift_idea', id, 'person', person_gift_ideas, title, coalesce(notes,'')
			FROM gift_ideas`,
		insertMissing: `INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
			SELECT 4000000000 + id, 'gift_idea', id, 'person', person_gift_ideas, title, coalesce(notes,'')
			FROM gift_ideas
			WHERE id NOT IN (SELECT entity_id FROM search_index WHERE entity = 'gift_idea')`,
		pruneOrphans: `DELETE FROM search_index
			WHERE entity = 'gift_idea'
			  AND (entity_id NOT IN (SELECT id FROM gift_ideas)
			       OR parent_id NOT IN (SELECT id FROM persons))`,
	},
	{
		insertAll: `INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
			SELECT 5000000000 + id, 'event', id,
			       CASE WHEN person_events IS NOT NULL THEN 'person'
			            WHEN group_events IS NOT NULL THEN 'group'
			            WHEN contact_events IS NOT NULL THEN 'contact'
			            ELSE '' END,
			       coalesce(person_events, group_events, contact_events),
			       type,
			       trim(coalesce(description,'') || ' ' || coalesce(notes,''))
			FROM events`,
		insertMissing: `INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
			SELECT 5000000000 + id, 'event', id,
			       CASE WHEN person_events IS NOT NULL THEN 'person'
			            WHEN group_events IS NOT NULL THEN 'group'
			            WHEN contact_events IS NOT NULL THEN 'contact'
			            ELSE '' END,
			       coalesce(person_events, group_events, contact_events),
			       type,
			       trim(coalesce(description,'') || ' ' || coalesce(notes,''))
			FROM events
			WHERE id NOT IN (SELECT entity_id FROM search_index WHERE entity = 'event')`,
		pruneOrphans: `DELETE FROM search_index
			WHERE entity = 'event'
			  AND (entity_id NOT IN (SELECT id FROM events)
			       OR (parent_entity = 'person' AND parent_id NOT IN (SELECT id FROM persons))
			       OR (parent_entity = 'group' AND parent_id NOT IN (SELECT id FROM groups)))`,
	},
	{
		insertAll: `INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
			SELECT 6000000000 + id, 'group', id, '', NULL, name, coalesce(description,'')
			FROM groups`,
		insertMissing: `INSERT INTO search_index(rowid, entity, entity_id, parent_entity, parent_id, title, body)
			SELECT 6000000000 + id, 'group', id, '', NULL, name, coalesce(description,'')
			FROM groups
			WHERE id NOT IN (SELECT entity_id FROM search_index WHERE entity = 'group')`,
		pruneOrphans: `DELETE FROM search_index
			WHERE entity = 'group' AND entity_id NOT IN (SELECT id FROM groups)`,
	},
}

// ErrSearchIndexUnavailable reports that the FTS5 index could not be built or
// queried. Callers should degrade to a "search unavailable" state instead of
// failing the request.
var ErrSearchIndexUnavailable = errors.New("search index unavailable")

// sqlDBHandles maps an ent client to the *sql.DB it was built on. ent does not
// expose the underlying handle, and the FTS5 index needs raw SQL; db.Init
// registers the handle when it creates the client, and tests that build a
// client themselves must call RegisterSQLDB too.
var sqlDBHandles sync.Map // map[*ent.Client]*sql.DB

// RegisterSQLDB associates conn with client so raw-SQL features (the full-text
// search index) can reach the database.
func RegisterSQLDB(client *ent.Client, conn *sql.DB) {
	if client == nil || conn == nil {
		return
	}
	sqlDBHandles.Store(client, conn)
}

// SQLDB returns the *sql.DB registered for client, if any.
func SQLDB(client *ent.Client) (*sql.DB, bool) {
	if client == nil {
		return nil, false
	}
	v, ok := sqlDBHandles.Load(client)
	if !ok {
		return nil, false
	}
	conn, ok := v.(*sql.DB)
	return conn, ok
}

// clientDB returns the SQL handle registered for client.
func clientDB(client *ent.Client) (*sql.DB, error) {
	if client == nil {
		return nil, fmt.Errorf("%w: nil ent client", ErrSearchIndexUnavailable)
	}
	conn, ok := SQLDB(client)
	if !ok {
		return nil, fmt.Errorf("%w: no SQL handle registered for ent client", ErrSearchIndexUnavailable)
	}
	return conn, nil
}

// EnsureSearchIndex creates the FTS5 table, installs the sync triggers and
// brings the index in line with the data.
//
// It is safe to call on every startup: the DDL is idempotent, triggers are
// replaced, and repeat runs only add missing rows and drop orphans. A version
// mismatch (upgrade) triggers a full rebuild. Any failure is returned so the
// caller can log it and run without full-text search.
func EnsureSearchIndex(ctx context.Context, client *ent.Client) error {
	conn, err := clientDB(client)
	if err != nil {
		return err
	}

	for _, stmt := range searchIndexDDL {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%w: create index: %v", ErrSearchIndexUnavailable, err)
		}
	}
	for _, name := range triggerNames {
		if _, err := conn.ExecContext(ctx, "DROP TRIGGER IF EXISTS "+name); err != nil {
			return fmt.Errorf("%w: drop trigger %s: %v", ErrSearchIndexUnavailable, name, err)
		}
	}
	for _, stmt := range searchIndexTriggers {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%w: create trigger: %v", ErrSearchIndexUnavailable, err)
		}
	}

	var version int
	err = conn.QueryRowContext(ctx, "SELECT version FROM search_index_meta WHERE id = 1").Scan(&version)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return RebuildSearchIndex(ctx, client)
	case err != nil:
		return fmt.Errorf("%w: read index version: %v", ErrSearchIndexUnavailable, err)
	case version != searchIndexVersion:
		slog.Info("search index: schema version changed, rebuilding",
			"source", "db", "from", version, "to", searchIndexVersion)
		return RebuildSearchIndex(ctx, client)
	}

	for _, src := range searchIndexSources {
		if _, err := conn.ExecContext(ctx, src.pruneOrphans); err != nil {
			return fmt.Errorf("%w: prune index: %v", ErrSearchIndexUnavailable, err)
		}
		if _, err := conn.ExecContext(ctx, src.insertMissing); err != nil {
			return fmt.Errorf("%w: backfill index: %v", ErrSearchIndexUnavailable, err)
		}
	}
	return nil
}

// RebuildSearchIndex throws the derived index away and repopulates it from
// scratch in a single transaction. It is exposed for maintenance tooling; the
// startup path only calls it when the index version changes.
func RebuildSearchIndex(ctx context.Context, client *ent.Client) error {
	conn, err := clientDB(client)
	if err != nil {
		return err
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%w: begin rebuild: %v", ErrSearchIndexUnavailable, err)
	}
	defer func() {
		if rerr := tx.Rollback(); rerr != nil && !errors.Is(rerr, sql.ErrTxDone) {
			slog.Warn("search index: rollback rebuild", "source", "db", "error", rerr)
		}
	}()

	if _, err := tx.ExecContext(ctx, "DELETE FROM search_index"); err != nil {
		return fmt.Errorf("%w: clear index: %v", ErrSearchIndexUnavailable, err)
	}
	for _, src := range searchIndexSources {
		if _, err := tx.ExecContext(ctx, src.insertAll); err != nil {
			return fmt.Errorf("%w: rebuild index: %v", ErrSearchIndexUnavailable, err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO search_index_meta(id, version) VALUES (1, ?)
		 ON CONFLICT(id) DO UPDATE SET version = excluded.version`, searchIndexVersion); err != nil {
		return fmt.Errorf("%w: record index version: %v", ErrSearchIndexUnavailable, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: commit rebuild: %v", ErrSearchIndexUnavailable, err)
	}
	return nil
}
