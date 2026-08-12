package recurring

import (
	"testing"
	"time"
)

func TestIsAnnualType(t *testing.T) {
	cases := []struct {
		eventType string
		want      bool
	}{
		{"birthday", true},
		{"anniversary", true},
		{"wedding", true},
		{"holiday", true},
		{"meeting", false},
		{"custom", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsAnnualType(tc.eventType); got != tc.want {
			t.Errorf("IsAnnualType(%q): got %v, want %v", tc.eventType, got, tc.want)
		}
	}
}

func TestOccurrencesIn_AnnualHistoricalDate(t *testing.T) {
	// Historical birthday 1990-05-12 must yield the current-year occurrence
	// when that occurrence falls inside the window.
	birth := time.Date(1990, 5, 12, 0, 0, 0, 0, time.UTC)
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)

	got := OccurrencesIn(birth, from, to)
	if len(got) != 1 {
		t.Fatalf("got %d occurrences, want 1", len(got))
	}
	if want := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC); !got[0].Equal(want) {
		t.Errorf("got %v, want %v", got[0], want)
	}
}

func TestOccurrencesIn_YearBoundary(t *testing.T) {
	// Window crosses Dec→Jan; occurrence applies to the next calendar year.
	birth := time.Date(1990, 1, 5, 0, 0, 0, 0, time.UTC)
	from := time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC)
	to := time.Date(2027, 1, 10, 0, 0, 0, 0, time.UTC)

	got := OccurrencesIn(birth, from, to)
	if len(got) != 1 {
		t.Fatalf("got %d occurrences, want 1", len(got))
	}
	if want := time.Date(2027, 1, 5, 0, 0, 0, 0, time.UTC); !got[0].Equal(want) {
		t.Errorf("got %v, want %v", got[0], want)
	}
}

func TestOccurrencesIn_LeapDay(t *testing.T) {
	// Feb 29 drifts to Feb 28 in non-leap years, stays Feb 29 in leap years.
	birth := time.Date(1992, 2, 29, 0, 0, 0, 0, time.UTC)

	// 2026 is not a leap year.
	got := OccurrencesIn(birth, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	if len(got) != 1 {
		t.Fatalf("non-leap: got %d occurrences, want 1", len(got))
	}
	if want := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC); !got[0].Equal(want) {
		t.Errorf("non-leap: got %v, want %v", got[0], want)
	}

	// 2028 is a leap year.
	got = OccurrencesIn(birth, time.Date(2028, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2028, 3, 1, 0, 0, 0, 0, time.UTC))
	if len(got) != 1 {
		t.Fatalf("leap: got %d occurrences, want 1", len(got))
	}
	if want := time.Date(2028, 2, 29, 0, 0, 0, 0, time.UTC); !got[0].Equal(want) {
		t.Errorf("leap: got %v, want %v", got[0], want)
	}
}

func TestOccurrencesIn_NoOccurrence(t *testing.T) {
	// No occurrence of the annual date inside the window.
	birth := time.Date(1990, 5, 12, 0, 0, 0, 0, time.UTC)
	got := OccurrencesIn(birth, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	if len(got) != 0 {
		t.Fatalf("got %d occurrences, want 0", len(got))
	}
}

func TestOccurrencesIn_TwoOccurrencesAcrossYears(t *testing.T) {
	// A window spanning two calendar years (2026-12-25 → 2028-01-01) that
	// contains the date in each year returns both occurrences, ascending.
	birth := time.Date(1990, 12, 27, 0, 0, 0, 0, time.UTC)
	got := OccurrencesIn(birth, time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC), time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC))
	if len(got) != 2 {
		t.Fatalf("got %d occurrences, want 2", len(got))
	}
	want := []time.Time{
		time.Date(2026, 12, 27, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 12, 27, 0, 0, 0, 0, time.UTC),
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Errorf("occurrence %d: got %v, want %v", i, got[i], want[i])
		}
	}
}
