package age

import (
	"testing"
	"time"
)

func mustDate(t *testing.T, year int, month time.Month, day int) time.Time {
	t.Helper()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func TestAgeAt_UpcomingBirthday(t *testing.T) {
	// Born 1996-08-12; on 2026-08-09 the birthday has not occurred yet → 29.
	got, ok := AgeAt(mustDate(t, 1996, 8, 12), mustDate(t, 2026, 8, 9))
	if !ok || got != 29 {
		t.Errorf("AgeAt = %d, %v; want 29, true", got, ok)
	}
}

func TestAgeAt_PastBirthday(t *testing.T) {
	// Born 1996-08-12; on 2026-08-13 the birthday has passed → 30.
	got, ok := AgeAt(mustDate(t, 1996, 8, 12), mustDate(t, 2026, 8, 13))
	if !ok || got != 30 {
		t.Errorf("AgeAt = %d, %v; want 30, true", got, ok)
	}
}

func TestAgeAt_OnBirthday(t *testing.T) {
	// Born 1996-08-12; on 2026-08-12 the birthday is today → 30.
	got, ok := AgeAt(mustDate(t, 1996, 8, 12), mustDate(t, 2026, 8, 12))
	if !ok || got != 30 {
		t.Errorf("AgeAt = %d, %v; want 30, true", got, ok)
	}
}

func TestAgeAt_YearBoundary(t *testing.T) {
	// Born 2000-12-31.
	if got, ok := AgeAt(mustDate(t, 2000, 12, 31), mustDate(t, 2026, 1, 1)); !ok || got != 25 {
		t.Errorf("after New Year: AgeAt = %d, %v; want 25, true", got, ok)
	}
	if got, ok := AgeAt(mustDate(t, 2000, 12, 31), mustDate(t, 2026, 12, 31)); !ok || got != 26 {
		t.Errorf("on birthday: AgeAt = %d, %v; want 26, true", got, ok)
	}
	if got, ok := AgeAt(mustDate(t, 2000, 12, 31), mustDate(t, 2027, 1, 1)); !ok || got != 26 {
		t.Errorf("next year: AgeAt = %d, %v; want 26, true", got, ok)
	}
}

func TestAgeAt_LeapDayBirthday(t *testing.T) {
	// Born 2000-02-29 (a leap year). In non-leap 2026, Feb 28 is the
	// comparison date, so the birthday has occurred by 2026-02-28 → 26.
	if got, ok := AgeAt(mustDate(t, 2000, 2, 29), mustDate(t, 2026, 2, 27)); !ok || got != 25 {
		t.Errorf("before Feb 28: AgeAt = %d, %v; want 25, true", got, ok)
	}
	if got, ok := AgeAt(mustDate(t, 2000, 2, 29), mustDate(t, 2026, 2, 28)); !ok || got != 26 {
		t.Errorf("on Feb 28 (non-leap): AgeAt = %d, %v; want 26, true", got, ok)
	}
	if got, ok := AgeAt(mustDate(t, 2000, 2, 29), mustDate(t, 2026, 3, 1)); !ok || got != 26 {
		t.Errorf("after Feb 28: AgeAt = %d, %v; want 26, true", got, ok)
	}

	// In a leap year (2024), the birthday is Feb 29 itself.
	if got, ok := AgeAt(mustDate(t, 2000, 2, 29), mustDate(t, 2024, 2, 28)); !ok || got != 23 {
		t.Errorf("leap year before Feb 29: AgeAt = %d, %v; want 23, true", got, ok)
	}
	if got, ok := AgeAt(mustDate(t, 2000, 2, 29), mustDate(t, 2024, 2, 29)); !ok || got != 24 {
		t.Errorf("leap year on Feb 29: AgeAt = %d, %v; want 24, true", got, ok)
	}
}

func TestAgeAt_UnknownYear(t *testing.T) {
	// Go's zero time (year 1) represents a date-only entry → no age.
	zero := time.Time{}
	if got, ok := AgeAt(zero, mustDate(t, 2026, 8, 9)); ok {
		t.Errorf("AgeAt(zero) = %d, %v; want 0, false", got, ok)
	}

	// Explicit year 0001 (date-only) → no age.
	if got, ok := AgeAt(mustDate(t, 1, 8, 12), mustDate(t, 2026, 8, 9)); ok {
		t.Errorf("AgeAt(year 1) = %d, %v; want 0, false", got, ok)
	}
}

func TestAgeAt_FutureDatedEntry(t *testing.T) {
	// A manual birthday entry with the upcoming occurrence year instead of a
	// birth year (e.g. 2026-12-25 while it is 2026-08-09) must not render a
	// nonsense age.
	if got, ok := AgeAt(mustDate(t, 2026, 12, 25), mustDate(t, 2026, 8, 9)); ok {
		t.Errorf("AgeAt(future entry) = %d, %v; want 0, false", got, ok)
	}
}

func TestNextAge(t *testing.T) {
	// Born 1996-08-12. Before the 2026 birthday: turns 30 on the upcoming one.
	if got, ok := NextAge(mustDate(t, 1996, 8, 12), mustDate(t, 2026, 8, 9)); !ok || got != 30 {
		t.Errorf("NextAge before birthday = %d, %v; want 30, true", got, ok)
	}
	// After the 2026 birthday: turns 31 on the next one (2027).
	if got, ok := NextAge(mustDate(t, 1996, 8, 12), mustDate(t, 2026, 8, 13)); !ok || got != 31 {
		t.Errorf("NextAge after birthday = %d, %v; want 31, true", got, ok)
	}
	// Unknown year → no age.
	if got, ok := NextAge(time.Time{}, mustDate(t, 2026, 8, 9)); ok {
		t.Errorf("NextAge(zero) = %d, %v; want 0, false", got, ok)
	}
}

func TestInfoFor(t *testing.T) {
	info := InfoFor(mustDate(t, 1996, 8, 12), mustDate(t, 2026, 8, 9))
	if !info.HasAge || info.Current != 29 || info.Next != 30 {
		t.Errorf("InfoFor = %+v; want HasAge=true Current=29 Next=30", info)
	}

	noAge := InfoFor(time.Time{}, mustDate(t, 2026, 8, 9))
	if noAge.HasAge {
		t.Errorf("InfoFor(zero) = %+v; want HasAge=false", noAge)
	}
}
