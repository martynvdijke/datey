package hijri

import (
	"errors"
	"testing"
	"time"
)

// Reference dates are published tabular-Hijri Gregorian equivalents
// (1 Ramadan 1446 = 2025-03-01, 1 Muharram 1447 = 2025-06-27).
func TestToGregorianGolden(t *testing.T) {
	tests := []struct {
		name      string
		year, mon int
		day       int
		want      string
	}{
		{"1 Ramadan 1446", 2025, 9, 1, "2025-03-01"},
		{"1 Muharram 1447", 2025, 1, 1, "2025-06-27"},
		{"1 Muharram 1448", 2026, 1, 1, "2026-06-17"},
		{"30 Dhu al-Hijjah 1447 (leap)", 2026, 12, 30, "2026-06-16"},
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

func TestTwiceInOneGregorianYear(t *testing.T) {
	// 19 Jumada al-Thani falls on 2024-01-01 and 2024-12-21 (355 days apart;
	// the Hijri year is shorter than the Gregorian one).
	occ, err := OccurrencesInGregorianYear(2024, 6, 19)
	if err != nil {
		t.Fatalf("OccurrencesInGregorianYear: %v", err)
	}
	if len(occ) != 2 {
		t.Fatalf("expected two occurrences, got %v", occ)
	}
	if occ[0].Format("2006-01-02") != "2024-01-01" || occ[1].Format("2006-01-02") != "2024-12-21" {
		t.Errorf("got %s and %s", occ[0].Format("2006-01-02"), occ[1].Format("2006-01-02"))
	}
	// The single-value helper returns the upcoming (earliest) occurrence.
	first, err := ToGregorian(2024, 6, 19)
	if err != nil {
		t.Fatalf("ToGregorian: %v", err)
	}
	if first.Format("2006-01-02") != "2024-01-01" {
		t.Errorf("got %s want 2024-01-01", first.Format("2006-01-02"))
	}
}

func TestZeroOccurrenceInGregorianYear(t *testing.T) {
	// 30 Dhu al-Hijjah only exists in Hijri leap years; 1446 is not one, so
	// the Gregorian year 2025 has no occurrence.
	occ, err := OccurrencesInGregorianYear(2025, 12, 30)
	if err != nil {
		t.Fatalf("OccurrencesInGregorianYear: %v", err)
	}
	if len(occ) != 0 {
		t.Fatalf("expected no occurrence, got %v", occ)
	}
	if _, err := ToGregorian(2025, 12, 30); !errors.Is(err, ErrNoOccurrence) {
		t.Fatalf("expected ErrNoOccurrence, got %v", err)
	}
}

func TestValidMonthDay(t *testing.T) {
	valid := [][2]int{{1, 30}, {2, 29}, {9, 30}, {12, 30}}
	for _, c := range valid {
		if err := ValidMonthDay(c[0], c[1]); err != nil {
			t.Errorf("%d/%d should be valid: %v", c[0], c[1], err)
		}
	}
	invalid := [][2]int{{0, 1}, {13, 1}, {1, 31}, {2, 30}, {4, 30}, {6, 30}}
	for _, c := range invalid {
		if err := ValidMonthDay(c[0], c[1]); err == nil {
			t.Errorf("%d/%d should be invalid", c[0], c[1])
		}
	}
}

func TestOccurrencesUTC(t *testing.T) {
	occ, err := OccurrencesInGregorianYear(2025, 9, 1)
	if err != nil || len(occ) != 1 {
		t.Fatalf("expected one occurrence, got %v err %v", occ, err)
	}
	if occ[0].Location() != time.UTC || occ[0].Hour() != 0 {
		t.Errorf("expected midnight UTC, got %v", occ[0])
	}
}
