package recurring

import (
	"testing"
	"time"

	"github.com/datey/datey/ent"
)

// ruleEventForTest builds a gregorian rule event anchored in anchor year.
func ruleEventForTest(kind string, ordinal, weekday *int, date time.Time, freq string) *ent.Event {
	return &ent.Event{
		Type:           "holiday",
		Date:           date,
		CalendarSystem: "gregorian",
		Frequency:      freq,
		RuleKind:       kind,
		RuleOrdinal:    ordinal,
		RuleWeekday:    weekday,
	}
}

func TestRuleEventExpansion_OnceFrequencyStillRecurs(t *testing.T) {
	e := ruleEventForTest(NthWeekdayRule, ptr(2), ptr(0),
		date(2026, time.May, 10), FrequencyOnce)

	got := fmts(OccurrencesInForEvent(e, date(2026, time.January, 1), date(2028, time.December, 31)))
	want := []string{"2026-05-10", "2027-05-09", "2028-05-14"}
	if !equalStrings(got, want) {
		t.Fatalf("rule event expansion = %v, want %v", got, want)
	}
	if occs, _ := OccurrencesForYear(e, 2027); len(occs) != 1 || occs[0].Format("2006-01-02") != "2027-05-09" {
		t.Fatalf("rule event in non-anchor year = %v", occs)
	}
}

func TestRuleEventExpansion_Easter(t *testing.T) {
	e := ruleEventForTest(EasterSundayRule, nil, nil,
		date(2026, time.April, 5), FrequencyOnce)

	got := fmts(OccurrencesInForEvent(e, date(2025, time.January, 1), date(2027, time.December, 31)))
	want := []string{"2025-04-20", "2026-04-05", "2027-03-28"}
	if !equalStrings(got, want) {
		t.Fatalf("easter expansion = %v, want %v", got, want)
	}
}

func TestRuleEventExpansion_WindowInclusive(t *testing.T) {
	e := ruleEventForTest(NthWeekdayRule, ptr(2), ptr(0),
		date(2026, time.May, 10), FrequencyAnnual)
	if got := fmts(OccurrencesInForEvent(e, date(2026, time.May, 10), date(2026, time.May, 10))); !equalStrings(got, []string{"2026-05-10"}) {
		t.Fatalf("single-day window = %v", got)
	}
	if got := fmts(OccurrencesInForEvent(e, date(2026, time.May, 11), date(2026, time.May, 11))); len(got) != 0 {
		t.Fatalf("window without occurrence = %v", got)
	}
}

func TestRuleEventExpansion_EndDateInclusive(t *testing.T) {
	end := date(2027, time.May, 9)
	e := ruleEventForTest(NthWeekdayRule, ptr(2), ptr(0),
		date(2026, time.May, 10), FrequencyAnnual)
	e.EndDate = &end

	got := fmts(OccurrencesInForEvent(e, date(2026, time.January, 1), date(2029, time.December, 31)))
	want := []string{"2026-05-10", "2027-05-09"}
	if !equalStrings(got, want) {
		t.Fatalf("rule end_date = %v, want %v", got, want)
	}
}

func TestRuleEventExpansion_OccurrenceCountFromAnchorYear(t *testing.T) {
	count := 2
	e := ruleEventForTest(NthWeekdayRule, ptr(5), ptr(0),
		date(2026, time.May, 31), FrequencyAnnual)
	e.OccurrenceCount = &count

	// Legacy windows before the anchor year are suppressed by the bound.
	got := fmts(OccurrencesInForEvent(e, date(2024, time.January, 1), date(2029, time.December, 31)))
	want := []string{"2026-05-31", "2027-05-30"}
	if !equalStrings(got, want) {
		t.Fatalf("rule occurrence_count = %v, want %v", got, want)
	}
}

func TestRuleEventExpansion_UnknownKindYieldsNothing(t *testing.T) {
	e := ruleEventForTest("mystery", nil, nil, date(2026, time.May, 10), FrequencyAnnual)
	if got := OccurrencesInForEvent(e, date(2026, time.January, 1), date(2026, time.December, 31)); len(got) != 0 {
		t.Fatalf("unknown rule kind = %v", got)
	}
}
