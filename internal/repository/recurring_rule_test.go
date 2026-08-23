package repository

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/internal/recurring"
	_ "github.com/mattn/go-sqlite3"
)

func TestCalcNthWeekday_MothersDay2026(t *testing.T) {
	d, _ := recurring.NthWeekdayOfMonth(2026, 5, 2, 0)
	date := d
	want := time.Date(2026, time.May, 10, 0, 0, 0, 0, time.UTC)
	if !date.Equal(want) {
		t.Errorf("Mother's Day 2026: got %s, want %s", date.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func TestCalcNthWeekday_FathersDay2026(t *testing.T) {
	d, _ := recurring.NthWeekdayOfMonth(2026, 6, 3, 0)
	date := d
	want := time.Date(2026, time.June, 21, 0, 0, 0, 0, time.UTC)
	if !date.Equal(want) {
		t.Errorf("Father's Day 2026: got %s, want %s", date.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func TestCalcNthWeekday_FirstMonday(t *testing.T) {
	d, _ := recurring.NthWeekdayOfMonth(2026, 1, 1, 1)
	date := d
	want := time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC)
	if !date.Equal(want) {
		t.Errorf("First Monday Jan 2026: got %s, want %s", date.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func TestCalcNthWeekday_FourthFriday(t *testing.T) {
	d, _ := recurring.NthWeekdayOfMonth(2026, 11, 4, 5)
	date := d
	if date.Month() != time.November || date.Weekday() != time.Friday {
		t.Errorf("Fourth Friday Nov 2026: got %s", date.Format("2006-01-02"))
	}
}

func TestCalcLastWeekday_LastSunday(t *testing.T) {
	date := calcLastWeekday(time.Sunday, time.March, 2026)
	want := time.Date(2026, time.March, 29, 0, 0, 0, 0, time.UTC)
	if !date.Equal(want) {
		t.Errorf("Last Sunday March 2026: got %s, want %s", date.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func TestCalcLastWeekday_LastFriday(t *testing.T) {
	date := calcLastWeekday(time.Friday, time.December, 2026)
	want := time.Date(2026, time.December, 25, 0, 0, 0, 0, time.UTC)
	if !date.Equal(want) {
		t.Errorf("Last Friday Dec 2026: got %s, want %s", date.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func TestCalcFixedDate(t *testing.T) {
	date := calcDateFixed(time.January, 1, 2026)
	want := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	if !date.Equal(want) {
		t.Errorf("New Year 2026: got %s, want %s", date.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func TestNextOccurrence_FutureThisYear(t *testing.T) {
	now := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	birthday := time.Date(1990, time.December, 25, 0, 0, 0, 0, time.UTC)
	next := nextOccurrence(birthday, now)
	want := time.Date(2026, time.December, 25, 0, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("Next occurrence: got %s, want %s", next.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func TestNextOccurrence_PastThisYear(t *testing.T) {
	now := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	birthday := time.Date(1990, time.February, 14, 0, 0, 0, 0, time.UTC)
	next := nextOccurrence(birthday, now)
	want := time.Date(2027, time.February, 14, 0, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("Next occurrence (past): got %s, want %s", next.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func calcDateFixed(month time.Month, day, year int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func nextOccurrence(date, now time.Time) time.Time {
	currentYear := time.Date(now.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	if currentYear.Before(now) || currentYear.Equal(now) {
		return time.Date(now.Year()+1, date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	}
	return currentYear
}

func newTestRecurringRuleRepo(t *testing.T) *RecurringRuleRepository {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_recurring_rule_repo?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return NewRecurringRuleRepository(client)
}

func TestCalculateDate_EasterRules_2026(t *testing.T) {
	repo := newTestRecurringRuleRepo(t)

	cases := []struct {
		patternType string
		want        string
	}{
		{recurring.EasterSundayRule, "2026-04-05"},
		{recurring.GoodFridayRule, "2026-04-03"},
		{recurring.EasterMondayRule, "2026-04-06"},
		{recurring.AshWednesdayRule, "2026-02-18"},
		{recurring.PentecostRule, "2026-05-24"},
	}

	for _, tc := range cases {
		rule, err := repo.Create(context.Background(), tc.patternType, tc.patternType, 0, 0, 0, 0)
		if err != nil {
			t.Fatalf("create rule %q: %v", tc.patternType, err)
		}
		got := repo.CalculateDate(rule, 2026).Format("2006-01-02")
		if got != tc.want {
			t.Errorf("CalculateDate(%q, 2026): got %s, want %s", tc.patternType, got, tc.want)
		}
	}
}

func TestCalculateDate_EasterRules_2038(t *testing.T) {
	repo := newTestRecurringRuleRepo(t)

	rule, err := repo.Create(context.Background(), "Easter Sunday", recurring.EasterSundayRule, 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	got := repo.CalculateDate(rule, 2038).Format("2006-01-02")
	if want := "2038-04-25"; got != want {
		t.Errorf("CalculateDate(easter, 2038): got %s, want %s", got, want)
	}
}
