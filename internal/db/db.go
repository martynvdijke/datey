package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/migrationlog"
	"github.com/datey/datey/ent/recurringrule"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/vcard"

	_ "github.com/mattn/go-sqlite3"
)

// migrationContactsToPeople is the name recorded in the migration_log table
// when the contacts→people data migration has been applied.
const migrationContactsToPeople = "contacts_to_people"

// migrationBirthdayBackfill is the name recorded in the migration_log table
// when the birthday-event backfill migration has been applied.
const migrationBirthdayBackfill = "birthday_events_backfill"

// migrationDropOneTimeNotifications is the name recorded in the migration_log
// table when the one-time notification tables have been dropped.
const migrationDropOneTimeNotifications = "drop_one_time_notifications"

const migrationCleanVCardNotes = "clean_vcard_notes"

// MigrateContactsToPeople copies data from the contacts table to the people table
// and updates event foreign keys to point to the new person records.
//
// The migration is gated by the migration_log table: once recorded it never
// runs again, replacing the previous fragile "count contacts vs people" heuristic.
func MigrateContactsToPeople(ctx context.Context, client *ent.Client) error {
	// Primary gate: if the migration is already recorded, never run again.
	applied, err := client.MigrationLog.Query().
		Where(migrationlog.NameEQ(migrationContactsToPeople)).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("check migration log: %w", err)
	}
	if applied {
		slog.Info("migration: contacts→people already applied, skipping", "source", "db")
		return nil
	}

	// recordMigrationLog writes the migration_log entry so we never re-run.
	recordMigrationLog := func() error {
		if _, err := client.MigrationLog.Create().
			SetName(migrationContactsToPeople).
			SetAppliedAt(time.Now()).
			Save(ctx); err != nil {
			return fmt.Errorf("record migration log: %w", err)
		}
		return nil
	}

	// Check if there are contacts to migrate.
	count, err := client.Contact.Query().Count(ctx)
	if err != nil {
		return err
	}
	if count == 0 {
		slog.Info("migration: no contacts to migrate, recording", "source", "db")
		return recordMigrationLog()
	}

	// Safety net: if people already exist alongside contacts, assume a prior
	// (pre-log) migration and skip to avoid creating duplicate people.
	peopleCount, err := client.Person.Query().Count(ctx)
	if err != nil {
		return err
	}
	if peopleCount > 0 {
		slog.Warn("migration: people already exist with contacts present, skipping to avoid duplicates", "source", "db", "contacts", count, "people", peopleCount)
		return recordMigrationLog()
	}

	slog.Info("migration: starting contacts → people migration", "source", "db", "contact_count", count)

	// Load all contacts with their events
	contacts, err := client.Contact.Query().WithEvents().All(ctx)
	if err != nil {
		return err
	}

	for _, c := range contacts {
		// Create person record from contact
		p, err := client.Person.Create().
			SetName(c.Name).
			SetNotes(c.Notes).
			SetCreatedAt(c.CreatedAt).
			SetUpdatedAt(c.UpdatedAt).
			Save(ctx)
		if err != nil {
			slog.Error("migration: create person", "source", "db", "contact_id", c.ID, "error", err)
			return err
		}

		// Update events to point to the new person
		for _, e := range c.Edges.Events {
			if err := client.Event.UpdateOneID(e.ID).
				SetPersonID(p.ID).
				Exec(ctx); err != nil {
				slog.Error("migration: update event person_id", "source", "db", "event_id", e.ID, "error", err)
				return err
			}
		}
	}

	// Delete all contacts after successful migration
	deleted, err := client.Contact.Delete().Exec(ctx)
	if err != nil {
		slog.Error("migration: delete contacts", "source", "db", "error", err)
		return err
	}

	if err := recordMigrationLog(); err != nil {
		return err
	}

	slog.Info("migration: completed", "source", "db", "contacts_migrated", count, "contacts_deleted", deleted)
	return nil
}

// MigrateBirthdayEvents backfills a birthday event for every person that has a
// parseable BDAY in their stored vCard data but no birthday event yet.
//
// This lets people imported via vCard (whose birth date was only stored as raw
// vcard_data) participate in annual birthday notifications after the upgrade.
// The migration is gated by the migration_log table: it runs exactly once.
func MigrateBirthdayEvents(ctx context.Context, client *ent.Client) error {
	// Primary gate: if the migration is already recorded, never run again.
	applied, err := client.MigrationLog.Query().
		Where(migrationlog.NameEQ(migrationBirthdayBackfill)).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("check migration log: %w", err)
	}
	if applied {
		slog.Info("migration: birthday backfill already applied, skipping", "source", "db")
		return nil
	}

	// recordMigrationLog writes the migration_log entry so we never re-run.
	recordMigrationLog := func() error {
		if _, err := client.MigrationLog.Create().
			SetName(migrationBirthdayBackfill).
			SetAppliedAt(time.Now()).
			Save(ctx); err != nil {
			return fmt.Errorf("record migration log: %w", err)
		}
		return nil
	}

	persons, err := client.Person.Query().WithEvents().All(ctx)
	if err != nil {
		return fmt.Errorf("load persons: %w", err)
	}

	// If there is nothing to process, still record the log so we never scan again.
	if len(persons) == 0 {
		slog.Info("migration: no persons to backfill, recording", "source", "db")
		return recordMigrationLog()
	}

	slog.Info("migration: starting birthday event backfill", "source", "db", "persons", len(persons))

	var created, skipped int
	for _, p := range persons {
		// Skip persons that already have a birthday event so we never
		// double-notify.
		hasBirthday := false
		for _, e := range p.Edges.Events {
			if e.Type == "birthday" {
				hasBirthday = true
				break
			}
		}
		if hasBirthday {
			skipped++
			continue
		}

		if p.VcardData == "" {
			skipped++
			continue
		}

		contacts, err := vcard.Parse(strings.NewReader(p.VcardData))
		if err != nil || len(contacts) == 0 || contacts[0].Birthday == nil {
			skipped++
			continue
		}

		if _, err := client.Event.Create().
			SetType("birthday").
			SetDate(*contacts[0].Birthday).
			SetDescription("Imported from vCard").
			SetCreatedAt(time.Now()).
			SetPersonID(p.ID).
			Save(ctx); err != nil {
			slog.Error("migration: create birthday event", "source", "db", "person_id", p.ID, "error", err)
			return err
		}
		created++
	}

	if err := recordMigrationLog(); err != nil {
		return err
	}

	slog.Info("migration: birthday backfill completed", "source", "db", "processed", len(persons), "created", created, "skipped", skipped)
	return nil
}

// CleanVCardNotes removes provider bookkeeping that older imports appended to
// the visible notes field. The original card remains in vcard_data for export.
func CleanVCardNotes(ctx context.Context, client *ent.Client) error {
	applied, err := client.MigrationLog.Query().Where(migrationlog.NameEQ(migrationCleanVCardNotes)).Exist(ctx)
	if err != nil {
		return fmt.Errorf("check migration log: %w", err)
	}
	if applied {
		return nil
	}
	people, err := client.Person.Query().All(ctx)
	if err != nil {
		return fmt.Errorf("load people for note cleanup: %w", err)
	}
	cleaned := 0
	for _, p := range people {
		if p.VcardData == "" {
			continue
		}
		contacts, parseErr := vcard.Parse(strings.NewReader(p.VcardData))
		if parseErr != nil || len(contacts) == 0 {
			continue
		}
		if strings.Contains(p.Notes, "UID:") || strings.Contains(p.Notes, "SOURCE:") || strings.Contains(p.Notes, "PRODID:") || strings.Contains(p.Notes, "REV:") {
			if _, err := client.Person.UpdateOneID(p.ID).SetNotes(contacts[0].Notes).SetUpdatedAt(time.Now()).Save(ctx); err != nil {
				return err
			}
			cleaned++
		}
	}
	if _, err := client.MigrationLog.Create().SetName(migrationCleanVCardNotes).SetAppliedAt(time.Now()).Save(ctx); err != nil {
		return err
	}
	slog.Info("migration: cleaned vCard notes", "source", "db", "cleaned", cleaned)
	return nil
}

// DropOneTimeNotificationTables removes the tables that belonged to the removed
// one-time notification feature: onetimenotifications and notification_deliveries.
//
// ent schema migration never drops unused tables, so this runs explicit DROP
// statements on a dedicated connection. The migration is gated by the
// migration_log table: it runs exactly once. Any pending one-time notifications
// are intentionally lost.
func DropOneTimeNotificationTables(ctx context.Context, client *ent.Client, dbPath string) error {
	// Primary gate: if the migration is already recorded, never run again.
	applied, err := client.MigrationLog.Query().
		Where(migrationlog.NameEQ(migrationDropOneTimeNotifications)).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("check migration log: %w", err)
	}
	if applied {
		slog.Info("migration: one-time notification tables already dropped, skipping", "source", "db")
		return nil
	}

	// recordMigrationLog writes the migration_log entry so we never re-run.
	recordMigrationLog := func() error {
		if _, err := client.MigrationLog.Create().
			SetName(migrationDropOneTimeNotifications).
			SetAppliedAt(time.Now()).
			Save(ctx); err != nil {
			return fmt.Errorf("record migration log: %w", err)
		}
		return nil
	}

	// Open a dedicated connection so we can execute DDL that ent does not expose.
	// The path may already carry query parameters (e.g. in tests), so append
	// pragmas with the correct separator.
	separator := "?"
	if strings.Contains(dbPath, "?") {
		separator = "&"
	}
	conn, err := sql.Open("sqlite3", dbPath+separator+"_journal_mode=WAL&_timeout=5000")
	if err != nil {
		return fmt.Errorf("open database for migration: %w", err)
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			slog.Warn("migration: close database connection", "source", "db", "error", cerr)
		}
	}()

	tables := []string{"onetimenotifications", "notification_deliveries"}
	for _, table := range tables {
		if _, err := conn.ExecContext(ctx, "DROP TABLE IF EXISTS "+table); err != nil {
			return fmt.Errorf("drop table %s: %w", table, err)
		}
	}

	if err := recordMigrationLog(); err != nil {
		return err
	}

	slog.Info("migration: dropped one-time notification tables", "source", "db", "tables", tables)
	return nil
}

func Init(cfg *config.Config) (*ent.Client, error) {
	dbPath := cfg.DataDir + "/datey.db"
	client, err := ent.Open("sqlite3", dbPath+"?_journal_mode=WAL&_timeout=5000&_fk=1")
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	if err := client.Schema.Create(ctx); err != nil {
		return nil, err
	}

	seedBuiltInRules(ctx, client)

	return client, nil
}

func seedBuiltInRules(ctx context.Context, client *ent.Client) {
	rules := []struct {
		Name        string
		PatternType string
		Nth         int
		Weekday     int
		Month       int
		Day         int
	}{
		{Name: "Mother's Day", PatternType: "nth_weekday", Nth: 2, Weekday: 0, Month: 5},
		{Name: "Father's Day", PatternType: "nth_weekday", Nth: 3, Weekday: 0, Month: 6},
		{Name: "New Year's Day", PatternType: "fixed", Month: 1, Day: 1},
		// Easter-based rules are computable: no fixed date is stored; the
		// pattern type alone resolves the date for each year.
		{Name: "Easter Sunday", PatternType: "easter_sunday"},
		{Name: "Good Friday", PatternType: "good_friday"},
		{Name: "Easter Monday", PatternType: "easter_monday"},
		{Name: "Ash Wednesday", PatternType: "ash_wednesday"},
		{Name: "Pentecost", PatternType: "pentecost"},
	}

	for _, r := range rules {
		exists, _ := client.RecurringRule.Query().Where(
			recurringrule.NameEQ(r.Name),
		).Exist(ctx)
		if exists {
			continue
		}

		q := client.RecurringRule.Create().
			SetName(r.Name).
			SetPatternType(r.PatternType).
			SetNth(r.Nth).
			SetWeekday(r.Weekday).
			SetMonth(r.Month).
			SetDay(r.Day).
			SetCreatedAt(time.Now())

		if _, err := q.Save(ctx); err != nil {
			slog.Warn("seed built-in rule", "name", r.Name, "error", err)
		}
	}
}
