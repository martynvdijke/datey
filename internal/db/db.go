package db

import (
	"context"
	"log/slog"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/recurringrule"
	"github.com/datey/datey/internal/config"

	_ "github.com/mattn/go-sqlite3"
)

// MigrateContactsToPeople copies data from the contacts table to the people table
// and updates event foreign keys to point to the new person records.
func MigrateContactsToPeople(ctx context.Context, client *ent.Client) error {
	// Check if migration is needed by counting contacts
	count, err := client.Contact.Query().Count(ctx)
	if err != nil {
		return err
	}
	if count == 0 {
		slog.Info("migration: no contacts to migrate", "source", "db")
		return nil
	}

	// Check if people already exist (migration already run)
	peopleCount, err := client.Person.Query().Count(ctx)
	if err != nil {
		return err
	}
	if peopleCount > 0 {
		slog.Info("migration: people already exist, skipping", "source", "db", "people_count", peopleCount)
		return nil
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

	slog.Info("migration: completed", "source", "db", "contacts_migrated", count, "contacts_deleted", deleted)
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

		q.Save(ctx)
	}
}
