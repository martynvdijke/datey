package recurring

import (
	"testing"
	"time"

	"github.com/datey/datey/ent"
)

func times(ds ...time.Time) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.Format("2006-01-02")
	}
	return out
}

func fmts(ts []time.Time) []string { return times(ts...) }

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFrequencyOnce(t *testing.T) {
	e := &ent.Event{Type: "trip", Date: date(2026, time.March, 15), Frequency: FrequencyOnce}

	if got := fmts(OccurrencesInForEvent(e, date(2026, time.January, 1), date(2027, time.December, 31))); !equalStrings(got, []string{"2026-03-15"}) {
		t.Fatalf("once window = %v", got)
	}
	if got := OccurrencesInForEvent(e, date(2027, time.January, 1), date(2027, time.December, 31)); len(got) != 0 {
		t.Fatalf("once must not repeat, got %v", got)
	}
	if got := OccurrencesInForEvent(e, date(2025, time.January, 1), date(2025, time.December, 31)); len(got) != 0 {
		t.Fatalf("once must not precede its date, got %v", got)
	}
	if _, err := OccurrenceForYear(e, 2027); err != ErrNoOccurrence {
		t.Fatalf("OccurrenceForYear(once, 2027) err = %v, want ErrNoOccurrence", err)
	}
	if occ, err := OccurrenceForYear(e, 2026); err != nil || occ.Format("2006-01-02") != "2026-03-15" {
		t.Fatalf("OccurrenceForYear(once, 2026) = %v, %v", occ, err)
	}
}

func TestFrequencyAnnualUnchanged(t *testing.T) {
	e := &ent.Event{Type: "birthday", Date: date(1990, time.December, 25), Frequency: FrequencyAnnual}
	if got := fmts(OccurrencesInForEvent(e, date(2026, time.January, 1), date(2027, time.December, 31))); !equalStrings(got, []string{"2026-12-25", "2027-12-25"}) {
		t.Fatalf("annual window = %v", got)
	}
	// Legacy: without bounds the month/day also applies to years before the
	// anchor year (byte-for-byte the historical behaviour).
	if got := fmts(OccurrencesInForEvent(e, date(1985, time.January, 1), date(1985, time.December, 31))); !equalStrings(got, []string{"1985-12-25"}) {
		t.Fatalf("annual pre-anchor = %v", got)
	}
	// Empty frequency means annual.
	legacy := &ent.Event{Type: "birthday", Date: date(1990, time.December, 25)}
	if got := fmts(OccurrencesInForEvent(legacy, date(2026, time.January, 1), date(2026, time.December, 31))); !equalStrings(got, []string{"2026-12-25"}) {
		t.Fatalf("empty frequency = %v", got)
	}
}

func TestFrequencyMonthlyClamping(t *testing.T) {
	e := &ent.Event{Type: "rent", Date: date(2024, time.January, 31), Frequency: FrequencyMonthly}
	got := fmts(OccurrencesInForEvent(e, date(2024, time.January, 1), date(2024, time.December, 31)))
	want := []string{
		"2024-01-31", "2024-02-29", "2024-03-31", "2024-04-30",
		"2024-05-31", "2024-06-30", "2024-07-31", "2024-08-31",
		"2024-09-30", "2024-10-31", "2024-11-30", "2024-12-31",
	}
	if !equalStrings(got, want) {
		t.Fatalf("monthly 2024 = %v, want %v", got, want)
	}

	// Non-leap February clamps to the 28th.
	leap := &ent.Event{Type: "rent", Date: date(2023, time.January, 31), Frequency: FrequencyMonthly}
	got = fmts(OccurrencesInForEvent(leap, date(2023, time.February, 1), date(2023, time.February, 28)))
	if !equalStrings(got, []string{"2023-02-28"}) {
		t.Fatalf("monthly Feb 2023 = %v", got)
	}

	// Window starting mid-series: March only.
	got = fmts(OccurrencesInForEvent(e, date(2024, time.March, 2), date(2024, time.March, 31)))
	if !equalStrings(got, []string{"2024-03-31"}) {
		t.Fatalf("monthly mid-year window = %v", got)
	}
}

func TestFrequencyWeeklyWindowArithmetic(t *testing.T) {
	e := &ent.Event{Type: "class", Date: date(2026, time.January, 1), Frequency: FrequencyWeekly}
	// Anchor is a Thursday; Jan 15 and Jan 22 are Thursdays.
	got := fmts(OccurrencesInForEvent(e, date(2026, time.January, 10), date(2026, time.January, 25)))
	if !equalStrings(got, []string{"2026-01-15", "2026-01-22"}) {
		t.Fatalf("weekly window = %v", got)
	}
	// A window far into the series must not walk from the anchor day.
	got = fmts(OccurrencesInForEvent(e, date(2036, time.March, 1), date(2036, time.March, 15)))
	if len(got) == 0 {
		t.Fatalf("weekly far window empty")
	}
	for _, s := range got {
		if s != "2036-03-06" && s != "2036-03-13" {
			t.Fatalf("weekly far window = %v", got)
		}
	}
}

func TestFrequencyEveryN(t *testing.T) {
	e := &ent.Event{Type: "water", Date: date(2026, time.January, 1), Frequency: FrequencyEveryN, Interval: 10}
	got := fmts(OccurrencesInForEvent(e, date(2026, time.January, 15), date(2026, time.February, 15)))
	if !equalStrings(got, []string{"2026-01-21", "2026-01-31", "2026-02-10"}) {
		t.Fatalf("every_n = %v", got)
	}
	// interval 1 is daily.
	daily := &ent.Event{Type: "water", Date: date(2026, time.January, 1), Frequency: FrequencyEveryN, Interval: 1}
	if got := OccurrencesInForEvent(daily, date(2026, time.January, 1), date(2026, time.January, 10)); len(got) != 10 {
		t.Fatalf("daily count = %d, want 10", len(got))
	}
	// Missing/invalid interval falls back to 1 rather than dropping the series.
	broken := &ent.Event{Type: "water", Date: date(2026, time.January, 1), Frequency: FrequencyEveryN}
	if got := OccurrencesInForEvent(broken, date(2026, time.January, 1), date(2026, time.January, 3)); len(got) != 3 {
		t.Fatalf("interval fallback count = %d, want 3", len(got))
	}
}

func TestEndDateInclusive(t *testing.T) {
	end := date(2026, time.January, 15)
	e := &ent.Event{Type: "class", Date: date(2026, time.January, 1), Frequency: FrequencyWeekly, EndDate: &end}
	got := fmts(OccurrencesInForEvent(e, date(2026, time.January, 1), date(2026, time.December, 31)))
	if !equalStrings(got, []string{"2026-01-01", "2026-01-08", "2026-01-15"}) {
		t.Fatalf("end_date inclusive = %v", got)
	}
	if got := fmts(OccurrencesInForEvent(e, date(2027, time.January, 1), date(2027, time.December, 31))); len(got) != 0 {
		t.Fatalf("end_date past must stop series, got %v", got)
	}
}

func TestOccurrenceCountInclusive(t *testing.T) {
	count := 3
	e := &ent.Event{Type: "rent", Date: date(2026, time.January, 31), Frequency: FrequencyMonthly, OccurrenceCount: &count}
	got := fmts(OccurrencesInForEvent(e, date(2026, time.January, 1), date(2026, time.December, 31)))
	if !equalStrings(got, []string{"2026-01-31", "2026-02-28", "2026-03-31"}) {
		t.Fatalf("occurrence_count = %v", got)
	}

	two := 2
	annual := &ent.Event{Type: "birthday", Date: date(2020, time.June, 1), Frequency: FrequencyAnnual, OccurrenceCount: &two}
	got = fmts(OccurrencesInForEvent(annual, date(2019, time.January, 1), date(2026, time.December, 31)))
	if !equalStrings(got, []string{"2020-06-01", "2021-06-01"}) {
		t.Fatalf("annual occurrence_count = %v", got)
	}
	// One occurrence only (the anchor).
	one := 1
	single := &ent.Event{Type: "trip", Date: date(2026, time.May, 5), Frequency: FrequencyWeekly, OccurrenceCount: &one}
	got = fmts(OccurrencesInForEvent(single, date(2026, time.January, 1), date(2026, time.December, 31)))
	if !equalStrings(got, []string{"2026-05-05"}) {
		t.Fatalf("count=1 = %v", got)
	}
}

func TestBothBoundsEarliestWins(t *testing.T) {
	count := 10
	end := date(2026, time.January, 20)
	e := &ent.Event{Type: "class", Date: date(2026, time.January, 1), Frequency: FrequencyWeekly, OccurrenceCount: &count, EndDate: &end}
	got := fmts(OccurrencesInForEvent(e, date(2026, time.January, 1), date(2026, time.December, 31)))
	if !equalStrings(got, []string{"2026-01-01", "2026-01-08", "2026-01-15"}) {
		t.Fatalf("earliest bound (end_date) = %v", got)
	}

	farEnd := date(2027, time.December, 31)
	e.EndDate = &farEnd
	got = fmts(OccurrencesInForEvent(e, date(2026, time.January, 1), date(2027, time.December, 31)))
	wantTen := []string{
		"2026-01-01", "2026-01-08", "2026-01-15", "2026-01-22", "2026-01-29",
		"2026-02-05", "2026-02-12", "2026-02-19", "2026-02-26", "2026-03-05",
	}
	if !equalStrings(got, wantTen) {
		t.Fatalf("earliest bound (count) = %v", got)
	}
}

func TestOccurrencesForYearRespectsFrequency(t *testing.T) {
	weekly := &ent.Event{Type: "class", Date: date(2026, time.January, 1), Frequency: FrequencyWeekly}
	occs, err := OccurrencesForYear(weekly, 2026)
	if err != nil {
		t.Fatalf("OccurrencesForYear: %v", err)
	}
	if len(occs) != 52 && len(occs) != 53 { // Jan 1 2026 is a Thursday.
		t.Fatalf("weekly occurrences in 2026 = %d", len(occs))
	}
	once := &ent.Event{Type: "trip", Date: date(2026, time.March, 15), Frequency: FrequencyOnce}
	if occs, _ := OccurrencesForYear(once, 2026); len(occs) != 1 {
		t.Fatalf("once in anchor year = %d", len(occs))
	}
	if occs, _ := OccurrencesForYear(once, 2027); len(occs) != 0 {
		t.Fatalf("once in later year = %d", len(occs))
	}
	// A zero-value event (no frequency column set) still expands annually.
	zero := &ent.Event{Type: "birthday", Date: date(1990, time.September, 9)}
	if occs, _ := OccurrencesForYear(zero, 2026); len(occs) != 1 || occs[0].Format("2006-01-02") != "2026-09-09" {
		t.Fatalf("zero frequency expansion = %v", occs)
	}
}

func TestNonGregorianFrequencyIgnoredForExpansion(t *testing.T) {
	// The handler rejects non-annual frequencies on non-Gregorian events; the
	// engine defensively keeps the annual conversion rather than dropping them.
	e := &ent.Event{
		Type:           "birthday",
		Date:           date(1990, time.January, 1),
		CalendarSystem: "lunar",
		LunarMonth:     ptr(1),
		LunarDay:       ptr(1),
		Frequency:      FrequencyWeekly,
	}
	got, err := OccurrenceForYear(e, 2023)
	if err != nil || got.Format("2006-01-02") != "2023-01-22" {
		t.Fatalf("non-gregorian annual fallback = %v, %v", got, err)
	}
}

func TestNonGregorianBounds(t *testing.T) {
	end := date(2021, time.December, 31)
	count := 2
	e := &ent.Event{
		Type:            "birthday",
		Date:            date(1990, time.January, 1),
		CalendarSystem:  "lunar",
		LunarMonth:      ptr(1),
		LunarDay:        ptr(1),
		EndDate:         &end,
		OccurrenceCount: &count,
	}
	if occs, err := OccurrencesForYear(e, 2022); err != nil || len(occs) != 0 {
		t.Fatalf("bounded lunar past end = %v, %v", occs, err)
	}
	if occs, err := OccurrencesForYear(e, 1991); err != nil || len(occs) != 1 {
		t.Fatalf("bounded lunar count #2 = %v, %v", occs, err)
	}
	if occs, err := OccurrencesForYear(e, 1992); err != nil || len(occs) != 0 {
		t.Fatalf("bounded lunar count exhausted = %v, %v", occs, err)
	}
}
