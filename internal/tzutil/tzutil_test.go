package tzutil

import (
	"strings"
	"testing"
	"time"
)

func TestLocationNeverNilAndFallsBack(t *testing.T) {
	if got := Location(""); got != time.Local {
		t.Errorf("Location(\"\") = %v, want time.Local", got)
	}
	if got := Location("Local"); got != time.Local {
		t.Errorf("Location(\"Local\") = %v, want time.Local", got)
	}
	if got := Location("Not/AZone"); got != time.Local {
		t.Errorf("Location(unknown) = %v, want time.Local fallback", got)
	}
	if got := Location("Europe/Berlin"); got == nil || got.String() != "Europe/Berlin" {
		t.Errorf("Location(Europe/Berlin) = %v, want Berlin", got)
	}
	if got := Location("UTC"); got == nil || got.String() != "UTC" {
		t.Errorf("Location(UTC) = %v, want UTC", got)
	}
}

func TestValid(t *testing.T) {
	for _, name := range []string{"", "Local", "UTC", "Europe/Berlin", "America/Argentina/Buenos_Aires"} {
		if !Valid(name) {
			t.Errorf("Valid(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"Mars/Olympus", "europe/berlin", "Berlin", "Europe/Berl in"} {
		if Valid(name) {
			t.Errorf("Valid(%q) = true, want false", name)
		}
	}
}

func TestValidateErrorMentionsName(t *testing.T) {
	if err := Validate("Europe/Berlin"); err != nil {
		t.Fatalf("Validate(valid) = %v", err)
	}
	err := Validate("Mars/Olympus")
	if err == nil {
		t.Fatal("Validate(invalid) = nil, want error")
	}
	if !strings.Contains(err.Error(), "Mars/Olympus") {
		t.Errorf("error %q does not name the rejected zone", err)
	}
}

func TestTodayUsesZoneCalendarDate(t *testing.T) {
	// 2026-01-01 23:30 UTC is already 2026-01-02 in Auckland (+13) while it is
	// still 2026-01-01 in Los Angeles (-8) and in UTC.
	now := time.Date(2026, time.January, 1, 23, 30, 0, 0, time.UTC)

	auckland := Location("Pacific/Auckland")
	if got := Today(now, auckland); got.Day() != 2 || got.Month() != time.January {
		t.Errorf("Today(Auckland) = %v, want Jan 2", got)
	}
	la := Location("America/Los_Angeles")
	if got := Today(now, la); got.Day() != 1 {
		t.Errorf("Today(LA) = %v, want Jan 1", got)
	}
	if got := Today(now, time.UTC); got.Day() != 1 {
		t.Errorf("Today(UTC) = %v, want Jan 1", got)
	}
	// The normalized result is midnight UTC carrying the zone's date, so date
	// arithmetic never sees a 23/25-hour day.
	if got := Today(now, auckland); got.Hour() != 0 || got.Location() != time.UTC {
		t.Errorf("Today should be midnight UTC, got %v", got)
	}
}

func TestDaysBetweenIsDSTSafe(t *testing.T) {
	berlin := Location("Europe/Berlin")
	// 2026-03-29 is the spring-forward day in Berlin: the local day is 23h.
	before := time.Date(2026, time.March, 28, 0, 0, 0, 0, berlin)
	after := time.Date(2026, time.March, 30, 0, 0, 0, 0, berlin)
	if got := DaysBetween(before, after); got != 2 {
		t.Errorf("DaysBetween across DST = %d, want 2", got)
	}
	// Autumn: 2026-10-25 is the fall-back day (25h).
	before = time.Date(2026, time.October, 24, 0, 0, 0, 0, berlin)
	after = time.Date(2026, time.October, 26, 0, 0, 0, 0, berlin)
	if got := DaysBetween(before, after); got != 2 {
		t.Errorf("DaysBetween across fall-back = %d, want 2", got)
	}
	// Same day, different clock times, is zero.
	a := time.Date(2026, time.March, 28, 1, 0, 0, 0, berlin)
	b := time.Date(2026, time.March, 28, 23, 0, 0, 0, berlin)
	if got := DaysBetween(a, b); got != 0 {
		t.Errorf("DaysBetween(same day) = %d, want 0", got)
	}
	// Negative direction.
	if got := DaysBetween(after, before); got != -2 {
		t.Errorf("DaysBetween(reverse) = %d, want -2", got)
	}
}

func TestDaysUntil(t *testing.T) {
	now := time.Date(2026, time.January, 1, 23, 30, 0, 0, time.UTC)
	event := time.Date(2026, time.January, 3, 0, 0, 0, 0, time.UTC)

	// In Auckland it is already Jan 2, so the event is one day away.
	if got := DaysUntil(event, Location("Pacific/Auckland"), now); got != 1 {
		t.Errorf("DaysUntil(Auckland) = %d, want 1", got)
	}
	// In UTC it is still Jan 1, so the event is two days away.
	if got := DaysUntil(event, time.UTC, now); got != 2 {
		t.Errorf("DaysUntil(UTC) = %d, want 2", got)
	}
	// A past date is negative.
	past := time.Date(2025, time.December, 30, 0, 0, 0, 0, time.UTC)
	if got := DaysUntil(past, time.UTC, now); got != -2 {
		t.Errorf("DaysUntil(past) = %d, want -2", got)
	}
}

func TestResolveOrder(t *testing.T) {
	t.Cleanup(func() { SetDefault("") })

	// No person zone, no default: server local.
	if got := Resolve(""); got != time.Local {
		t.Errorf("Resolve(\"\") = %v, want time.Local", got)
	}
	// Instance default applies when the person has none.
	SetDefault("Asia/Tokyo")
	if got := Resolve(""); got.String() != "Asia/Tokyo" {
		t.Errorf("Resolve(\"\") with default = %v, want Asia/Tokyo", got)
	}
	if DefaultName() != "Asia/Tokyo" {
		t.Errorf("DefaultName() = %q, want Asia/Tokyo", DefaultName())
	}
	// The person's own zone wins over the default.
	if got := Resolve("Europe/Berlin"); got.String() != "Europe/Berlin" {
		t.Errorf("Resolve(person) = %v, want Europe/Berlin", got)
	}
	// An invalid stored value falls back to the default rather than panicking.
	if got := Resolve("Bogus/Zone"); got.String() != "Asia/Tokyo" {
		t.Errorf("Resolve(invalid) = %v, want default Asia/Tokyo", got)
	}
	// An invalid default is ignored.
	SetDefault("Bogus/Zone")
	if DefaultName() != "Asia/Tokyo" {
		t.Errorf("SetDefault(invalid) changed the default to %q", DefaultName())
	}
}

func TestResolveWith(t *testing.T) {
	if got := ResolveWith("Europe/Berlin", "Asia/Tokyo"); got.String() != "Europe/Berlin" {
		t.Errorf("ResolveWith(person) = %v, want Europe/Berlin", got)
	}
	if got := ResolveWith("", "Asia/Tokyo"); got.String() != "Asia/Tokyo" {
		t.Errorf("ResolveWith(global) = %v, want Asia/Tokyo", got)
	}
	if got := ResolveWith("", ""); got != time.Local {
		t.Errorf("ResolveWith(empty) = %v, want time.Local", got)
	}
}

func TestFormatDate(t *testing.T) {
	// 2026-01-01 23:30 UTC is Jan 2 in Auckland.
	now := time.Date(2026, time.January, 1, 23, 30, 0, 0, time.UTC)
	if got := FormatDate(now, Location("Pacific/Auckland"), "2006-01-02"); got != "2026-01-02" {
		t.Errorf("FormatDate(Auckland) = %q, want 2026-01-02", got)
	}
	if got := FormatDate(now, time.UTC, "2006-01-02"); got != "2026-01-01" {
		t.Errorf("FormatDate(UTC) = %q, want 2026-01-01", got)
	}
}

func TestCommonZoneGroups(t *testing.T) {
	groups := CommonZoneGroups()
	if len(groups) == 0 {
		t.Fatal("no zone groups")
	}
	if groups[0].Region != "UTC" {
		t.Errorf("first group = %q, want UTC", groups[0].Region)
	}
	found := map[string]bool{}
	for _, g := range groups {
		for _, z := range g.Zones {
			if !Valid(z) {
				t.Errorf("curated zone %q is not loadable", z)
			}
			found[z] = true
		}
	}
	for _, want := range []string{"UTC", "Europe/Berlin", "America/New_York", "Asia/Tokyo", "Australia/Sydney"} {
		if !found[want] {
			t.Errorf("curated list is missing %q", want)
		}
	}
}
