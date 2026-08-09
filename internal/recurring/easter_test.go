package recurring

import (
	"testing"
	"time"
)

func TestEasterSunday_KnownDates(t *testing.T) {
	cases := []struct {
		year int
		want string
	}{
		{2000, "2000-04-23"},
		{2024, "2024-03-31"},
		{2026, "2026-04-05"},
		{2038, "2038-04-25"}, // latest possible date
		{2011, "2011-04-24"},
		{2029, "2029-04-01"},
	}

	for _, tc := range cases {
		got := EasterSunday(tc.year).Format("2006-01-02")
		if got != tc.want {
			t.Errorf("EasterSunday(%d): got %s, want %s", tc.year, got, tc.want)
		}
	}
}

func TestDateForType_Offsets(t *testing.T) {
	// 2026: Easter Sunday falls on April 5.
	// Good Friday −2 → Apr 3, Easter Monday +1 → Apr 6,
	// Ash Wednesday −46 → Feb 18, Pentecost +49 → May 24.
	cases := []struct {
		patternType string
		want        string
	}{
		{EasterSundayRule, "2026-04-05"},
		{GoodFridayRule, "2026-04-03"},
		{EasterMondayRule, "2026-04-06"},
		{AshWednesdayRule, "2026-02-18"},
		{PentecostRule, "2026-05-24"},
	}

	for _, tc := range cases {
		got := DateForType(tc.patternType, 2026).Format("2006-01-02")
		if got != tc.want {
			t.Errorf("DateForType(%q, 2026): got %s, want %s", tc.patternType, got, tc.want)
		}
	}
}

func TestDateForType_UnknownType(t *testing.T) {
	if got := DateForType("nth_weekday", 2026); !got.IsZero() {
		t.Errorf("expected zero time for unknown type, got %s", got.Format("2006-01-02"))
	}
}

func TestIsEasterBased(t *testing.T) {
	for _, pt := range []string{EasterSundayRule, GoodFridayRule, EasterMondayRule, AshWednesdayRule, PentecostRule} {
		if !IsEasterBased(pt) {
			t.Errorf("IsEasterBased(%q) = false, want true", pt)
		}
	}
	if IsEasterBased("fixed") || IsEasterBased("nth_weekday") {
		t.Error("IsEasterBased returned true for a fixed-date pattern type")
	}
}

func TestEasterSunday_IsSunday(t *testing.T) {
	for _, year := range []int{2000, 2024, 2026, 2038, 1970, 2100} {
		got := EasterSunday(year)
		if got.Weekday() != time.Sunday {
			t.Errorf("EasterSunday(%d) = %s, want a Sunday", year, got.Format("2006-01-02"))
		}
	}
}
