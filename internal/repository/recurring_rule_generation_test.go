package repository

import "testing"

func TestCalculateDate_NthWeekdayGeneration(t *testing.T) {
	repo := newTestRecurringRuleRepo(t)
	// Mother's Day 2nd Sunday May
	rule, _ := repo.Create(t.Context(), "Mother's Day", "nth_weekday", 2, 0, 5, 0)
	d := repo.CalculateDate(rule, 2026)
	if d.Format("2006-01-02") != "2026-05-10" {
		t.Errorf("Mother's Day 2026 got %s", d.Format("2006-01-02"))
	}
	// Last Monday Aug
	rule2, _ := repo.Create(t.Context(), "Last Mon Aug", "nth_weekday", 5, 1, 8, 0)
	d2 := repo.CalculateDate(rule2, 2026)
	if d2.Format("2006-01-02") != "2026-08-31" {
		t.Errorf("Last Monday Aug 2026 got %s", d2.Format("2006-01-02"))
	}
	// Fixed and Easter unaffected
	rule3, _ := repo.Create(t.Context(), "New Year", "fixed", 0, 0, 1, 1)
	if got := repo.CalculateDate(rule3, 2026).Format("2006-01-02"); got != "2026-01-01" {
		t.Errorf("fixed got %s", got)
	}
}
