package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/ent/migrationlog"

	_ "github.com/mattn/go-sqlite3"
)

// TestMigrateContactsToPeople_RunsOnce verifies that the migration migrates
// contacts to people on the first run and does NOT re-run after being recorded.
func TestMigrateContactsToPeople_RunsOnce(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:migrate_once_test?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	// Seed a contact with an event.
	c, err := client.Contact.Create().
		SetName("Alice").
		SetNotes("friend").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}
	_, err = client.Event.Create().
		SetType("birthday").
		SetDate(time.Date(2026, time.July, 4, 0, 0, 0, 0, time.UTC)).
		SetDescription("Alice birthday").
		SetCreatedAt(time.Now()).
		SetContact(c).
		Save(ctx)
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	// First run: should migrate.
	if err := MigrateContactsToPeople(ctx, client); err != nil {
		t.Fatalf("first migration run: %v", err)
	}

	// Person should now exist; contact should be deleted.
	peopleCount, err := client.Person.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count people: %v", err)
	}
	if peopleCount != 1 {
		t.Fatalf("expected 1 person after migration, got %d", peopleCount)
	}
	contactCount, err := client.Contact.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count contacts: %v", err)
	}
	if contactCount != 0 {
		t.Fatalf("expected 0 contacts after migration, got %d", contactCount)
	}

	// Migration log should be recorded.
	applied, err := client.MigrationLog.Query().
		Where(migrationlog.NameEQ(migrationContactsToPeople)).
		Exist(ctx)
	if err != nil {
		t.Fatalf("check migration log: %v", err)
	}
	if !applied {
		t.Fatal("expected migration log to be recorded after migration")
	}

	// Second run: should skip (no re-run, no duplicate people).
	if err := MigrateContactsToPeople(ctx, client); err != nil {
		t.Fatalf("second migration run: %v", err)
	}
	peopleCount2, err := client.Person.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count people after 2nd run: %v", err)
	}
	if peopleCount2 != 1 {
		t.Fatalf("expected still 1 person after 2nd run, got %d (migration re-ran)", peopleCount2)
	}
}

// TestMigrateContactsToPeople_NoContacts_RecordsLog verifies that when there
// are no contacts, the migration is still recorded so it never re-runs.
func TestMigrateContactsToPeople_NoContacts_RecordsLog(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:migrate_empty_test?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	if err := MigrateContactsToPeople(ctx, client); err != nil {
		t.Fatalf("migration with no contacts: %v", err)
	}

	applied, err := client.MigrationLog.Query().
		Where(migrationlog.NameEQ(migrationContactsToPeople)).
		Exist(ctx)
	if err != nil {
		t.Fatalf("check migration log: %v", err)
	}
	if !applied {
		t.Fatal("expected migration log to be recorded even when no contacts exist")
	}

	// Adding a contact later should NOT trigger a re-migration (already recorded).
	_, err = client.Contact.Create().
		SetName("Bob").
		SetNotes("").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}
	if err := MigrateContactsToPeople(ctx, client); err != nil {
		t.Fatalf("second migration run: %v", err)
	}
	// The contact should still exist (migration was skipped).
	contactCount, err := client.Contact.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count contacts: %v", err)
	}
	if contactCount != 1 {
		t.Fatalf("expected contact to remain (migration should have skipped), got %d contacts", contactCount)
	}
	// And no people should have been created.
	peopleCount, err := client.Person.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count people: %v", err)
	}
	if peopleCount != 0 {
		t.Fatalf("expected 0 people (migration skipped), got %d", peopleCount)
	}
}

// TestMigrateContactsToPeople_PeopleExist_SkipsWithoutDuplicating verifies the
// safety net: if people already exist alongside contacts (pre-log deployment),
// the migration records and skips instead of creating duplicates.
func TestMigrateContactsToPeople_PeopleExist_SkipsWithoutDuplicating(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:migrate_dup_test?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	// Pre-existing person (e.g. created via UI) and a leftover contact.
	_, err := client.Person.Create().
		SetName("Existing Person").
		SetNotes("").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	_, err = client.Contact.Create().
		SetName("Leftover Contact").
		SetNotes("").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}

	if err := MigrateContactsToPeople(ctx, client); err != nil {
		t.Fatalf("migration with existing people: %v", err)
	}

	// Should still have exactly 1 person (no duplicate).
	peopleCount, err := client.Person.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count people: %v", err)
	}
	if peopleCount != 1 {
		t.Fatalf("expected 1 person (no duplicate), got %d", peopleCount)
	}

	// Migration should be recorded.
	applied, err := client.MigrationLog.Query().
		Where(migrationlog.NameEQ(migrationContactsToPeople)).
		Exist(ctx)
	if err != nil {
		t.Fatalf("check migration log: %v", err)
	}
	if !applied {
		t.Fatal("expected migration log to be recorded when skipping due to existing people")
	}
}

// TestMigrateBirthdayEvents_CreatesMissing verifies that persons with a
// parseable BDAY in vcard_data and no birthday event get one created.
func TestMigrateBirthdayEvents_CreatesMissing(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:bday_backfill_create_test?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	_, err := client.Person.Create().
		SetName("Dana").
		SetNotes("").
		SetVcardData("BEGIN:VCARD\nVERSION:4.0\nFN:Dana\nBDAY:19951120\nEND:VCARD\n").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create person: %v", err)
	}

	if err := MigrateBirthdayEvents(ctx, client); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	events, err := client.Event.Query().All(ctx)
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 birthday event, got %d", len(events))
	}
	if events[0].Type != "birthday" {
		t.Fatalf("expected type birthday, got %q", events[0].Type)
	}
	if want := time.Date(1995, time.November, 20, 0, 0, 0, 0, time.UTC); !events[0].Date.Equal(want) {
		t.Fatalf("expected date %v, got %v", want, events[0].Date)
	}
	if events[0].Description != "Imported from vCard" {
		t.Fatalf("expected description 'Imported from vCard', got %q", events[0].Description)
	}
}

// TestMigrateBirthdayEvents_SkipsExisting verifies that persons who already
// have a birthday event are not backfilled again.
func TestMigrateBirthdayEvents_SkipsExisting(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:bday_backfill_existing_test?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	p, err := client.Person.Create().
		SetName("Dana").
		SetNotes("").
		SetVcardData("BEGIN:VCARD\nVERSION:4.0\nFN:Dana\nBDAY:19951120\nEND:VCARD\n").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	if _, err := client.Event.Create().
		SetType("birthday").
		SetDate(time.Date(1995, time.November, 20, 0, 0, 0, 0, time.UTC)).
		SetCreatedAt(time.Now()).
		SetPerson(p).
		Save(ctx); err != nil {
		t.Fatalf("create existing birthday event: %v", err)
	}

	if err := MigrateBirthdayEvents(ctx, client); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	count, err := client.Event.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 event (no duplicate), got %d", count)
	}
}

// TestMigrateBirthdayEvents_NoOpOnRerun verifies the migration_log gate: the
// second run does nothing even for persons created after the first run.
func TestMigrateBirthdayEvents_NoOpOnRerun(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:bday_backfill_rerun_test?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	if err := MigrateBirthdayEvents(ctx, client); err != nil {
		t.Fatalf("first backfill run: %v", err)
	}

	// Person with parseable BDAY created after the migration was recorded.
	if _, err := client.Person.Create().
		SetName("Dana").
		SetNotes("").
		SetVcardData("BEGIN:VCARD\nVERSION:4.0\nFN:Dana\nBDAY:19951120\nEND:VCARD\n").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx); err != nil {
		t.Fatalf("create person: %v", err)
	}

	if err := MigrateBirthdayEvents(ctx, client); err != nil {
		t.Fatalf("second backfill run: %v", err)
	}

	count, err := client.Event.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 events on re-run, got %d", count)
	}
}

// TestMigrateBirthdayEvents_SkipsUnparseable verifies that persons without
// vcard_data or with an unparseable BDAY are skipped.
func TestMigrateBirthdayEvents_SkipsUnparseable(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:bday_backfill_unparseable_test?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	for _, name := range []string{"NoData", "NoBDAY", "BadBDAY"} {
		create := client.Person.Create().
			SetName(name).
			SetNotes("").
			SetCreatedAt(time.Now()).
			SetUpdatedAt(time.Now())
		switch name {
		case "NoBDAY":
			create.SetVcardData("BEGIN:VCARD\nVERSION:4.0\nFN:NoBDAY\nEND:VCARD\n")
		case "BadBDAY":
			create.SetVcardData("BEGIN:VCARD\nVERSION:4.0\nFN:BadBDAY\nBDAY:--1399\nEND:VCARD\n")
		}
		if _, err := create.Save(ctx); err != nil {
			t.Fatalf("create person %s: %v", name, err)
		}
	}

	if err := MigrateBirthdayEvents(ctx, client); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	count, err := client.Event.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 events for unparseable persons, got %d", count)
	}

	applied, err := client.MigrationLog.Query().
		Where(migrationlog.NameEQ(migrationBirthdayBackfill)).
		Exist(ctx)
	if err != nil {
		t.Fatalf("check migration log: %v", err)
	}
	if !applied {
		t.Fatal("expected migration log to be recorded after backfill")
	}
}

// TestDropOneTimeNotificationTables verifies the migration drops both tables,
// records the migration_log entry, and is idempotent on subsequent runs.
func TestDropOneTimeNotificationTables(t *testing.T) {
	dsn := "file:migrate_drop_test?mode=memory&cache=shared&_fk=1"
	client := enttest.Open(t, "sqlite3", dsn)
	defer client.Close()
	ctx := context.Background()

	// Seed the tables that the feature used to own (ent no longer knows them,
	// so create them with raw DDL on a dedicated connection).
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open raw connection: %v", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "CREATE TABLE onetimenotifications (id integer primary key autoincrement, message text)"); err != nil {
		t.Fatalf("create onetimenotifications: %v", err)
	}
	if _, err := conn.ExecContext(ctx, "CREATE TABLE notification_deliveries (id integer primary key autoincrement, channel text)"); err != nil {
		t.Fatalf("create notification_deliveries: %v", err)
	}
	if _, err := conn.ExecContext(ctx, "INSERT INTO onetimenotifications (message) VALUES ('pending')"); err != nil {
		t.Fatalf("seed row: %v", err)
	}

	// First run: should drop both tables and record the log entry.
	if err := DropOneTimeNotificationTables(ctx, client, dsn); err != nil {
		t.Fatalf("first migration run: %v", err)
	}

	for _, table := range []string{"onetimenotifications", "notification_deliveries"} {
		var name string
		err := conn.QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != sql.ErrNoRows {
			t.Errorf("expected table %s to be dropped, got err=%v", table, err)
		}
	}

	applied, err := client.MigrationLog.Query().
		Where(migrationlog.NameEQ(migrationDropOneTimeNotifications)).
		Exist(ctx)
	if err != nil {
		t.Fatalf("check migration log: %v", err)
	}
	if !applied {
		t.Fatal("expected migration log to be recorded after drop")
	}

	// Second run: should skip (gated by migration_log).
	if err := DropOneTimeNotificationTables(ctx, client, dsn); err != nil {
		t.Fatalf("second migration run: %v", err)
	}
}
