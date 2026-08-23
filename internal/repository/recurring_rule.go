package repository

import (
	"context"
	"log/slog"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/recurringrule"
	"github.com/datey/datey/internal/recurring"
)

type RecurringRuleRepository struct {
	client *ent.Client
}

func NewRecurringRuleRepository(client *ent.Client) *RecurringRuleRepository {
	return &RecurringRuleRepository{client: client}
}

func (r *RecurringRuleRepository) Create(ctx context.Context, name, patternType string, nth, weekday, month, day int) (*ent.RecurringRule, error) {
	return r.client.RecurringRule.Create().
		SetName(name).
		SetPatternType(patternType).
		SetNth(nth).
		SetWeekday(weekday).
		SetMonth(month).
		SetDay(day).
		SetEnabled(true).
		SetCreatedAt(time.Now()).
		Save(ctx)
}

func (r *RecurringRuleRepository) Get(ctx context.Context, id int) (*ent.RecurringRule, error) {
	return r.client.RecurringRule.Get(ctx, id)
}

func (r *RecurringRuleRepository) List(ctx context.Context) ([]*ent.RecurringRule, error) {
	return r.client.RecurringRule.Query().
		Where(recurringrule.EnabledEQ(true)).
		Order(ent.Asc(recurringrule.FieldName)).
		All(ctx)
}

func (r *RecurringRuleRepository) Update(ctx context.Context, id int, name, patternType string, nth, weekday, month, day int, enabled bool) (*ent.RecurringRule, error) {
	return r.client.RecurringRule.UpdateOneID(id).
		SetName(name).
		SetPatternType(patternType).
		SetNth(nth).
		SetWeekday(weekday).
		SetMonth(month).
		SetDay(day).
		SetEnabled(enabled).
		Save(ctx)
}

func (r *RecurringRuleRepository) Delete(ctx context.Context, id int) error {
	return r.client.RecurringRule.DeleteOneID(id).Exec(ctx)
}

func (r *RecurringRuleRepository) CalculateDate(rule *ent.RecurringRule, year int) time.Time {
	switch rule.PatternType {
	case recurring.NthWeekdayRule:
		if d, ok := recurring.NthWeekdayOfMonth(year, rule.Month, rule.Nth, rule.Weekday); ok {
			return d
		}
		slog.Info("recurring: nth_weekday skipped", "rule", rule.Name, "year", year)
		return time.Time{}
	case "last_weekday":
		return calcLastWeekday(time.Weekday(rule.Weekday), time.Month(rule.Month), year)
	case "fixed":
		return time.Date(year, time.Month(rule.Month), rule.Day, 0, 0, 0, 0, time.UTC)
	default:
		// Easter-based types are computable: no stored date is needed.
		if recurring.IsEasterBased(rule.PatternType) {
			return recurring.DateForType(rule.PatternType, year)
		}
		return time.Time{}
	}
}

func calcLastWeekday(weekday time.Weekday, month time.Month, year int) time.Time {
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC)
	daysBack := (int(lastDay.Weekday()) - int(weekday) + 7) % 7
	return lastDay.AddDate(0, 0, -daysBack)
}

func NextOccurrence(date time.Time) time.Time {
	now := time.Now()
	currentYearOccurrence := time.Date(now.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	if currentYearOccurrence.Before(now) || currentYearOccurrence.Equal(now) {
		return time.Date(now.Year()+1, date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	}
	return currentYearOccurrence
}
