package db

import (
	"context"
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
