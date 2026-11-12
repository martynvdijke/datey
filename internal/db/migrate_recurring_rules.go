package db

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/migrationlog"
	"github.com/datey/datey/internal/recurring"
)

// migrationRecurringRulesToEvents records that legacy RecurringRule rows have
// been folded into annual events.
const migrationRecurringRulesToEvents = "recurring_rules_to_events"

// MigrateRecurringRulesToEvents folds every enabled RecurringRule into an
// annual event so the scheduler can remind about it, instead of the rule only
// feeding the iCal export.
//
// The migration is idempotent two ways: a migration_log row short-circuits
// later runs, and each converted rule is disabled so a partially applied run
// leaves nothing enabled to convert. Rules the operator had already disabled
// are left untouched — converting them would re-enable reminders the operator
// switched off.
//
// Rule month is taken from the rule itself and stored in the event date so the
// date resolution in internal/recurring has everything it needs.
func MigrateRecurringRulesToEvents(ctx context.Context, client *ent.Client) error {
	applied, err := client.MigrationLog.Query().
		Where(migrationlog.NameEQ(migrationRecurringRulesToEvents)).
		Exist(ctx)
	if err != nil {
		// Pre-schema or partially created database: let the next boot retry.
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	if applied {
		return nil
	}

	rules, err := client.RecurringRule.Query().All(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}

	year := time.Now().Year()
	converted := 0

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		kind, ordinal, weekday := ruleEventKind(rule)
		date := ruleEventDate(rule, kind, ordinal, weekday, year)

		create := client.Event.Create().
			SetType("holiday").
			SetDate(date).
			SetDescription(rule.Name).
			SetCalendarSystem("gregorian").
			SetFrequency(recurring.FrequencyAnnual).
			SetCreatedAt(time.Now()).
			SetRuleKind(kind)
		if ordinal > 0 {
			create = create.SetRuleOrdinal(ordinal)
		}
		if kind == recurring.NthWeekdayRule {
			create = create.SetRuleWeekday(weekday)
		}

		if _, err := create.Save(ctx); err != nil {
			slog.Warn("migrate recurring rule: create event",
				"source", "db", "rule", rule.Name, "error", err)
			continue
		}
		if _, err := client.RecurringRule.UpdateOneID(rule.ID).SetEnabled(false).Save(ctx); err != nil {
			slog.Warn("migrate recurring rule: disable rule",
				"source", "db", "rule", rule.Name, "error", err)
			continue
		}
		converted++
	}

	if _, err := client.MigrationLog.Create().
		SetName(migrationRecurringRulesToEvents).
		SetAppliedAt(time.Now()).
		Save(ctx); err != nil {
		return err
	}

	if converted > 0 {
		slog.Info("migrated recurring rules to events", "source", "db", "converted", converted)
	}
	return nil
}

// ruleEventKind maps a legacy rule onto the event rule kind, normalising
// "last_weekday" to nth_weekday with ordinal 5 (last) and "fixed" to no rule
// kind at all, because a fixed date is already fully described by the event
// date.
func ruleEventKind(rule *ent.RecurringRule) (kind string, ordinal, weekday int) {
	switch rule.PatternType {
	case recurring.NthWeekdayRule:
		return recurring.NthWeekdayRule, rule.Nth, rule.Weekday
	case "last_weekday":
		return recurring.NthWeekdayRule, 5, rule.Weekday
	case "fixed":
		return "", 0, 0
	default:
		if recurring.IsEasterBased(rule.PatternType) {
			return rule.PatternType, 0, 0
		}
		return "", 0, 0
	}
}

// ruleEventDate resolves the rule's date in the given year, falling back to
// January 1 when the rule cannot be resolved so the event still has a date.
func ruleEventDate(rule *ent.RecurringRule, kind string, ordinal, weekday, year int) time.Time {
	switch {
	case kind == recurring.NthWeekdayRule:
		if d, ok := recurring.NthWeekdayOfMonth(year, rule.Month, ordinal, weekday); ok {
			return d
		}
	case recurring.IsEasterBased(kind):
		if d := recurring.DateForType(kind, year); !d.IsZero() {
			return d
		}
	case kind == "" && rule.Month >= 1 && rule.Month <= 12 && rule.Day >= 1 && rule.Day <= 31:
		return time.Date(year, time.Month(rule.Month), rule.Day, 0, 0, 0, 0, time.UTC)
	}
	return time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
}
