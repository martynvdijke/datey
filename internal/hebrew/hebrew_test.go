package hebrew

import (
	"errors"
	"testing"
	"time"
)

// Reference dates are published Hebrew-calendar dates (hebcal / "Calendrical
// Calculations"), not values produced by this implementation.
func TestToGregorianGolden(t *testing.T) {
	tests := []struct {
		name      string
		year, mon int
		day       int
		want      string
	}{
		{"Rosh Hashanah 5785 (1 Tishrei)", 2024, 7, 1, "2024-10-03"},
		{"Rosh Hashanah 5786", 2025, 7, 1, "2025-09-23"},
		{"Pesach 5785 (15 Nisan)", 2025, 1, 15, "2025-04-13"},
		{"Pesach 5784", 2024, 1, 15, "2024-04-23"},
		{"Purim 5784 (14 Adar II)", 2024, 13, 14, "2024-03-24"},
		{"30 Adar I 5784 (leap year)", 2024, 12, 30, "2024-03-10"},
		{"30 Adar I fallback in 5785", 2025, 12, 30, "2025-03-30"},
		{"10 Tevet 5785 crosses civil new year", 2025, 10, 10, "2025-01-10"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ToGregorian(tc.year, tc.mon, tc.day)
			if err != nil {
				t.Fatalf("ToGregorian: %v", err)
			}
			if got.Format("2006-01-02") != tc.want {
				t.Errorf("got %s want %s", got.Format("2006-01-02"), tc.want)
			}
		})
	}
}

func TestHebrewYearBoundaryNoOccurrence(t *testing.T) {
	// 10 Tevet 5784 fell on 2023-12-22 and 10 Tevet 5785 on 2025-01-10, so
	// the Gregorian year 2024 contains no occurrence. This is the Hebrew-year
	// boundary case: a Gregorian January date belongs to the previous Hebrew
	// year.
	if _, err := ToGregorian(2024, 10, 10); !errors.Is(err, ErrNoOccurrence) {
		t.Fatalf("expected ErrNoOccurrence for 10 Tevet in 2024, got %v", err)
	}
}

func TestAdarIIFallbackNonLeapYear(t *testing.T) {
	// 14 Adar II in a non-leap Hebrew year is observed in Adar (month 12),
	// i.e. Purim 5785 on 2025-03-14.
	got, err := ToGregorian(2025, 13, 14)
	if err != nil {
		t.Fatalf("ToGregorian: %v", err)
	}
	if got.Format("2006-01-02") != "2025-03-14" {
		t.Errorf("got %s want 2025-03-14", got.Format("2006-01-02"))
	}
}

func TestValidMonthDay(t *testing.T) {
	valid := [][2]int{{1, 30}, {7, 1}, {12, 30}, {13, 29}, {10, 10}}
	for _, c := range valid {
		if err := ValidMonthDay(c[0], c[1]); err != nil {
			t.Errorf("%d/%d should be valid: %v", c[0], c[1], err)
		}
	}
	invalid := [][2]int{{0, 1}, {14, 1}, {1, 31}, {13, 30}, {2, 30}, {4, 30}, {6, 30}, {10, 30}}
	for _, c := range invalid {
		if err := ValidMonthDay(c[0], c[1]); err == nil {
			t.Errorf("%d/%d should be invalid", c[0], c[1])
		}
	}
}

func TestIsLeapYear(t *testing.T) {
	if !IsLeapYear(5784) {
		t.Error("5784 should be a leap year")
	}
	if IsLeapYear(5785) {
		t.Error("5785 should not be a leap year")
	}
}

func TestOccurrencesUTC(t *testing.T) {
	occ, err := OccurrencesInGregorianYear(2025, 1, 15)
	if err != nil || len(occ) != 1 {
		t.Fatalf("expected one occurrence, got %v err %v", occ, err)
	}
	if occ[0].Location() != time.UTC || occ[0].Hour() != 0 {
		t.Errorf("expected midnight UTC, got %v", occ[0])
	}
}
