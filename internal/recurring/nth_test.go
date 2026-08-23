package recurring

import (
	"testing"
	"time"
)

func TestNthWeekdayOfMonth(t *testing.T) {
	cases := []struct {
		name    string
		year    int
		month   int
		ordinal int
		weekday int
		want    string
		ok      bool
	}{
		{"2nd Sunday May 2026 Mother's Day", 2026, 5, 2, 0, "2026-05-10", true},
		{"4th Thu Nov 2026 Thanksgiving", 2026, 11, 4, 4, "2026-11-26", true},
		{"last Monday Aug 2026", 2026, 8, 5, 1, "2026-08-31", true},
		{"last Sunday Mar 2026", 2026, 3, 5, 0, "2026-03-29", true},
		{"1st Monday Jan 2026", 2026, 1, 1, 1, "2026-01-05", true},
		{"3rd Sunday Jun 2026 Father's Day", 2026, 6, 3, 0, "2026-06-21", true},
		{"leap Feb last Sunday 2024", 2024, 2, 5, 0, "2024-02-25", true},
		{"non-leap Feb 2nd Sunday 2026", 2026, 2, 2, 0, "2026-02-08", true},
		{"last Monday Feb 2021 (4 Mondays)", 2021, 2, 5, 1, "2021-02-22", true},
	}
	for _, c := range cases {
		got, ok := NthWeekdayOfMonth(c.year, c.month, c.ordinal, c.weekday)
		if ok != c.ok {
			t.Errorf("%s: ok=%v want %v", c.name, ok, c.ok)
			continue
		}
		if ok && got.Format("2006-01-02") != c.want {
			t.Errorf("%s: got %s want %s", c.name, got.Format("2006-01-02"), c.want)
		}
		if ok && got.Weekday() != time.Weekday(c.weekday) {
			t.Errorf("%s: weekday mismatch", c.name)
		}
	}
}

func TestNthWeekdayOfMonth_LastVsFour(t *testing.T) {
	// Feb 2021 has 4 Mondays, ordinal 5 (last) should still resolve to 22nd, not skip.
	d, ok := NthWeekdayOfMonth(2021, 2, 5, 1)
	if !ok || d.Day() != 22 {
		t.Errorf("last Monday Feb 2021 expected 22, got %v ok=%v", d, ok)
	}
}

func TestNthWeekdayOfMonth_SkipWithInvalidInputs(t *testing.T) {
	// Invalid ordinal/weekday/month should return ok=false
	if _, ok := NthWeekdayOfMonth(2026, 13, 1, 0); ok {
		t.Error("expected ok=false for month 13")
	}
	if _, ok := NthWeekdayOfMonth(2026, 5, 6, 0); ok {
		t.Error("expected ok=false for ordinal 6")
	}
	if _, ok := NthWeekdayOfMonth(2026, 5, 1, 7); ok {
		t.Error("expected ok=false for weekday 7")
	}
}
