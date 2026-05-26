package repository

import (
	"context"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/recurringrule"
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
	case "nth_weekday":
		return calcNthWeekday(rule.Nth, time.Weekday(rule.Weekday), time.Month(rule.Month), year)
	case "last_weekday":
		return calcLastWeekday(time.Weekday(rule.Weekday), time.Month(rule.Month), year)
	case "fixed":
		return time.Date(year, time.Month(rule.Month), rule.Day, 0, 0, 0, 0, time.UTC)
	default:
		return time.Time{}
	}
}

func calcNthWeekday(nth int, weekday time.Weekday, month time.Month, year int) time.Time {
	firstDay := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	daysUntil := (int(weekday) - int(firstDay.Weekday()) + 7) % 7
	day := 1 + daysUntil + (nth-1)*7
	if day > daysInMonth(month, year) {
		day -= 7
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func calcLastWeekday(weekday time.Weekday, month time.Month, year int) time.Time {
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC)
	daysBack := (int(lastDay.Weekday()) - int(weekday) + 7) % 7
	return lastDay.AddDate(0, 0, -daysBack)
}

func daysInMonth(month time.Month, year int) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func NextOccurrence(date time.Time) time.Time {
	now := time.Now()
	currentYearOccurrence := time.Date(now.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	if currentYearOccurrence.Before(now) || currentYearOccurrence.Equal(now) {
		return time.Date(now.Year()+1, date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	}
	return currentYearOccurrence
}
