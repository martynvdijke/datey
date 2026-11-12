package recurring

import (
	"testing"
	"time"

	"github.com/datey/datey/ent"
)

func ruleEvent(kind string, ordinal, weekday *int, date time.Time) *ent.Event {
	return &ent.Event{
		Type:           "holiday",
		Date:           date,
		CalendarSystem: "gregorian",
		Frequency:      FrequencyAnnual,
		RuleKind:       kind,
		RuleOrdinal:    ordinal,
		RuleWeekday:    weekday,
	}
}

func intPtr(v int) *int { return &v }

func TestNormalizeRuleKind(t *testing.T) {
	cases := map[string]string{
		"":                "",
		"  ":              "",
		"fixed":           "",
		"nth_weekday":     NthWeekdayRule,
		"last_weekday":    NthWeekdayRule,
		"LAST_WEEKDAY":    NthWeekdayRule,
		"easter_sunday":   EasterSundayRule,
		"good_friday":     GoodFridayRule,
		"easter_monday":   EasterMondayRule,
		"ash_wednesday":   AshWednesdayRule,
		"pentecost":       PentecostRule,
		"some_new_kind":   "",
		"nth_weekday_alt": "",
	}
	for in, want := range cases {
		if got := NormalizeRuleKind(in); got != want {
			t.Errorf("NormalizeRuleKind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsRuleEvent(t *testing.T) {
	if IsRuleEvent(nil) {
		t.Error("nil event is not a rule event")
	}
	if IsRuleEvent(&ent.Event{RuleKind: "  "}) {
		t.Error("blank rule kind is not a rule event")
	}
	if !IsRuleEvent(&ent.Event{RuleKind: EasterSundayRule}) {
		t.Error("easter event must be a rule event")
	}
}

func TestRuleDateForYear_NthWeekdaySecondSundayOfMay(t *testing.T) {
	e := ruleEvent(NthWeekdayRule, intPtr(2), intPtr(0), time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC))
	want := map[int]string{
		2024: "2024-05-12",
		2025: "2025-05-11",
		2026: "2026-05-10",
		2027: "2027-05-09",
	}
	for year, w := range want {
		got, ok := RuleDateForYear(e, year)
		if !ok {
			t.Fatalf("%d: expected a resolvable date", year)
		}
		if got.Format("2006-01-02") != w {
			t.Errorf("%d: got %s, want %s", year, got.Format("2006-01-02"), w)
		}
	}
}

func TestRuleDateForYear_LastSundayOfMay(t *testing.T) {
	for _, kind := range []string{NthWeekdayRule, RuleLastWeekday} {
		e := ruleEvent(kind, intPtr(5), intPtr(0), time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC))
		want := map[int]string{
			2024: "2024-05-26",
			2025: "2025-05-25",
			2026: "2026-05-31",
			2027: "2027-05-30",
		}
		for year, w := range want {
			got, ok := RuleDateForYear(e, year)
			if !ok {
				t.Fatalf("%s %d: expected a resolvable date", kind, year)
			}
			if got.Format("2006-01-02") != w {
				t.Errorf("%s %d: got %s, want %s", kind, year, got.Format("2006-01-02"), w)
			}
		}
	}
}

func TestRuleDateForYear_EasterBasedKinds(t *testing.T) {
	cases := []struct {
		kind string
		want map[int]string
	}{
		{EasterSundayRule, map[int]string{2024: "2024-03-31", 2025: "2025-04-20", 2026: "2026-04-05"}},
		{GoodFridayRule, map[int]string{2024: "2024-03-29", 2025: "2025-04-18", 2026: "2026-04-03"}},
		{EasterMondayRule, map[int]string{2024: "2024-04-01", 2025: "2025-04-21", 2026: "2026-04-06"}},
		{AshWednesdayRule, map[int]string{2024: "2024-02-14", 2025: "2025-03-05", 2026: "2026-02-18"}},
		{PentecostRule, map[int]string{2024: "2024-05-19", 2025: "2025-06-08", 2026: "2026-05-24"}},
	}
	for _, tc := range cases {
		e := ruleEvent(tc.kind, nil, nil, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		for year, w := range tc.want {
			got, ok := RuleDateForYear(e, year)
			if !ok {
				t.Fatalf("%s %d: expected a resolvable date", tc.kind, year)
			}
			if got.Format("2006-01-02") != w {
				t.Errorf("%s %d: got %s, want %s", tc.kind, year, got.Format("2006-01-02"), w)
			}
		}
	}
}

func TestRuleDateForYear_PlainStoredDate(t *testing.T) {
	e := ruleEvent("", nil, nil, time.Date(1990, 8, 15, 0, 0, 0, 0, time.UTC))
	got, ok := RuleDateForYear(e, 2026)
	if !ok {
		t.Fatal("expected a resolvable date")
	}
	if got.Format("2006-01-02") != "2026-08-15" {
		t.Errorf("got %s, want 2026-08-15", got.Format("2006-01-02"))
	}
}

func TestRuleDateForYear_DefaultsAndFailures(t *testing.T) {
	// nil ordinal/weekday default to last Sunday of the stored month.
	e := ruleEvent(NthWeekdayRule, nil, nil, time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC))
	got, ok := RuleDateForYear(e, 2026)
	if !ok || got.Format("2006-01-02") != "2026-05-31" {
		t.Errorf("default ordinal/weekday: got %v %v, want 2026-05-31", got, ok)
	}

	unknown := ruleEvent("not_a_rule", nil, nil, time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC))
	if _, ok := RuleDateForYear(unknown, 2026); ok {
		t.Error("unknown rule kind must not resolve")
	}

	// February 29 only exists in a leap year.
	leap := ruleEvent("", nil, nil, time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC))
	if _, ok := RuleDateForYear(leap, 2025); ok {
		t.Error("Feb 29 must not resolve in a non-leap year")
	}
	if got, ok := RuleDateForYear(leap, 2028); !ok || got.Format("2006-01-02") != "2028-02-29" {
		t.Errorf("Feb 29 leap year: got %v %v", got, ok)
	}
}

func TestRuleEventDate(t *testing.T) {
	got, ok := RuleEventDate(NthWeekdayRule, 2, 0, 5, 0, 2026)
	if !ok || got.Format("2006-01-02") != "2026-05-10" {
		t.Fatalf("got %v %v, want 2026-05-10", got, ok)
	}
	// ordinal 0 falls back to last.
	got, ok = RuleEventDate(NthWeekdayRule, 0, 0, 5, 0, 2026)
	if !ok || got.Format("2006-01-02") != "2026-05-31" {
		t.Fatalf("ordinal 0: got %v %v, want 2026-05-31", got, ok)
	}
	if _, ok := RuleEventDate(NthWeekdayRule, 2, 0, 13, 0, 2026); ok {
		t.Error("month 13 must not resolve")
	}
	if _, ok := RuleEventDate("bogus", 0, 0, 5, 1, 2026); ok {
		t.Error("unknown kind must not resolve")
	}
}
