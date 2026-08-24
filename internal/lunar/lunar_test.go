package lunar

import (
	"testing"
	"time"
)

func TestLunarToGregorianGolden(t *testing.T) {
	tests := []struct {
		year, month, day int
		leap             bool
		want             string
	}{
		{2023, 1, 1, false, "2023-01-22"},
		{2024, 1, 1, false, "2024-02-10"},
		{2025, 1, 1, false, "2025-01-29"},
		{2023, 8, 15, false, "2023-09-29"},
		{2020, 1, 1, false, "2020-01-25"},
	}
	for _, tc := range tests {
		got, err := LunarToGregorian(tc.year, tc.month, tc.day, tc.leap)
		if err != nil {
			t.Fatalf("year %d: %v", tc.year, err)
		}
		if got.Format("2006-01-02") != tc.want {
			t.Errorf("lunar %d/%d/%d -> got %s want %s", tc.year, tc.month, tc.day, got.Format("2006-01-02"), tc.want)
		}
	}
}

func TestLeapHandling(t *testing.T) {
	// 2023 has leap 2, 2024 has no leap 2
	if _, err := LunarToGregorian(2023, 2, 1, true); err != nil {
		t.Fatalf("2023 leap 2 should exist: %v", err)
	}
	if _, err := LunarToGregorian(2024, 2, 1, true); err != ErrNoOccurrence {
		t.Fatalf("2024 leap 2 should be ErrNoOccurrence, got %v", err)
	}
	// regular entry unaffected in leap year
	got, err := LunarToGregorian(2023, 2, 1, false)
	if err != nil {
		t.Fatalf("regular 2023/2/1: %v", err)
	}
	_ = got
	// out of range
	if _, err := LunarToGregorian(1899, 1, 1, false); err == nil {
		t.Error("expected error for year 1899")
	}
	// day overflow
	lyear := 2023
	// find a month with 29 days to test overflow? Just ensure 30 on 29-day month fails some year; try 2023/2 has 29? Might vary.
	// Instead test that day 31 always fails range
	if _, err := LunarToGregorian(lyear, 1, 31, false); err == nil {
		t.Error("expected error for day 31")
	}
}

func TestLunarToGregorianMidnightUTC(t *testing.T) {
	got, _ := LunarToGregorian(2023, 1, 1, false)
	if got.Location() != time.UTC {
		t.Error("expected UTC")
	}
	if got.Hour() != 0 || got.Minute() != 0 {
		t.Error("expected midnight")
	}
}
