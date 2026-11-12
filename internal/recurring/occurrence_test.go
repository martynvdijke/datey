package recurring

import (
	"errors"
	"testing"
	"time"

	"github.com/datey/datey/ent"
)

func ptr(n int) *int { return &n }

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestOccurrenceForYearGregorianRegression(t *testing.T) {
	e := &ent.Event{Type: "birthday", Date: date(1990, time.December, 25)}
	got, err := OccurrenceForYear(e, 2026)
	if err != nil || got.Format("2006-01-02") != "2026-12-25" {
		t.Fatalf("got %v err %v", got, err)
	}
	leap := &ent.Event{Type: "birthday", Date: date(2000, time.February, 29)}
	got, err = OccurrenceForYear(leap, 2026)
	if err != nil || got.Format("2006-01-02") != "2026-02-28" {
		t.Fatalf("leap fallback got %v err %v", got, err)
	}
}

func TestOccurrenceForYearLunarRegression(t *testing.T) {
	e := &ent.Event{
		Type:           "birthday",
		Date:           date(1990, time.January, 1),
		CalendarSystem: "lunar",
		LunarMonth:     ptr(1),
		LunarDay:       ptr(1),
	}
	// Existing lunar semantics: the target year is the lunar year.
	got, err := OccurrenceForYear(e, 2023)
	if err != nil || got.Format("2006-01-02") != "2023-01-22" {
		t.Fatalf("got %v err %v", got, err)
	}
}

func TestOccurrenceForYearHebrew(t *testing.T) {
	roshHashanah := &ent.Event{
		Type:           "birthday",
		Date:           date(1990, time.September, 23),
		CalendarSystem: "hebrew",
		LunarMonth:     ptr(7),
		LunarDay:       ptr(1),
	}
	got, err := OccurrenceForYear(roshHashanah, 2025)
	if err != nil || got.Format("2006-01-02") != "2025-09-23" {
		t.Fatalf("got %v err %v", got, err)
	}

	// 10 Tevet 5785 falls on 2025-01-10: a Gregorian January date in the
	// Hebrew year that began in autumn 2024.
	tevet := &ent.Event{
		Type:           "memorial",
		Date:           date(2020, time.January, 10),
		CalendarSystem: "hebrew",
		LunarMonth:     ptr(10),
		LunarDay:       ptr(10),
	}
	occ, err := OccurrenceForYear(tevet, 2025)
	if err != nil || occ.Format("2006-01-02") != "2025-01-10" {
		t.Fatalf("got %v err %v", occ, err)
	}
	// daysUntil across the civil-year boundary from 2025-01-01.
	if days := int(occ.Sub(date(2025, time.January, 1)).Hours() / 24); days != 9 {
		t.Errorf("daysUntil = %d, want 9", days)
	}

	// Adar II is stored as month 12 + leap flag (schema caps month at 12).
	adarII := &ent.Event{
		Type:           "birthday",
		Date:           date(1990, time.March, 24),
		CalendarSystem: "hebrew",
		LunarMonth:     ptr(12),
		LunarDay:       ptr(14),
		LunarLeap:      true,
	}
	got, err = OccurrenceForYear(adarII, 2024)
	if err != nil || got.Format("2006-01-02") != "2024-03-24" {
		t.Fatalf("Adar II got %v err %v", got, err)
	}
}

func TestOccurrenceForYearHijriDoubleAndMissing(t *testing.T) {
	e := &ent.Event{
		Type:           "custom",
		Date:           date(1990, time.June, 19),
		CalendarSystem: "hijri",
		LunarMonth:     ptr(6),
		LunarDay:       ptr(19),
	}
	// 19 Jumada al-Thani occurs twice in 2024.
	occs, err := OccurrencesForYear(e, 2024)
	if err != nil {
		t.Fatalf("OccurrencesForYear: %v", err)
	}
	if len(occs) != 2 || occs[0].Format("2006-01-02") != "2024-01-01" || occs[1].Format("2006-01-02") != "2024-12-21" {
		t.Fatalf("got %v", occs)
	}
	if first, _ := OccurrenceForYear(e, 2024); first.Format("2006-01-02") != "2024-01-01" {
		t.Errorf("OccurrenceForYear = %s, want earliest", first.Format("2006-01-02"))
	}
	inRange := OccurrencesInForEvent(e, date(2024, time.January, 1), date(2024, time.December, 31))
	if len(inRange) != 2 {
		t.Errorf("OccurrencesInForEvent returned %d, want 2", len(inRange))
	}

	// 30 Dhu al-Hijjah does not exist in the non-leap Hijri year 1446, so
	// the Gregorian year 2025 has no occurrence.
	missing := &ent.Event{
		Type:           "custom",
		Date:           date(1990, time.June, 16),
		CalendarSystem: "hijri",
		LunarMonth:     ptr(12),
		LunarDay:       ptr(30),
	}
	if _, err := OccurrenceForYear(missing, 2025); !errors.Is(err, ErrNoOccurrence) {
		t.Fatalf("expected ErrNoOccurrence, got %v", err)
	}
	if got := OccurrencesInForEvent(missing, date(2025, time.January, 1), date(2025, time.December, 31)); len(got) != 0 {
		t.Errorf("expected no occurrences, got %v", got)
	}
}

func TestMemorialYearsSinceUsesOccurrenceYear(t *testing.T) {
	e := &ent.Event{Type: "memorial", Date: date(2010, time.May, 1)}
	occ, err := OccurrenceForYear(e, 2026)
	if err != nil {
		t.Fatalf("OccurrenceForYear: %v", err)
	}
	if occ.Format("2006-01-02") != "2026-05-01" {
		t.Fatalf("got %s", occ.Format("2006-01-02"))
	}
}
