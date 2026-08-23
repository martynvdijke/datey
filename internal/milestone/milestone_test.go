package milestone

import (
	"testing"
	"time"
)

func TestIsMilestone_Birthday(t *testing.T) {
	cases := []struct {
		name      string
		birth     time.Time
		occ       time.Time
		want      bool
		wantLabel string
	}{
		{"30th birthday", time.Date(1996, 6, 15, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), true, "30th birthday"},
		{"non-milestone 27", time.Date(1999, 6, 15, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), false, ""},
		{"10th", time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), true, "10th birthday"},
		{"18th", time.Date(2008, 3, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), true, "18th birthday"},
		{"21st", time.Date(2005, 5, 5, 0, 0, 0, 0, time.UTC), time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC), true, "21st birthday"},
		{"100th", time.Date(1926, 6, 15, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), true, "100th birthday"},
		{"yearless no badge", time.Date(1, 6, 15, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), false, ""},
		{"year zero no badge", time.Time{}, time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, label := IsMilestone("birthday", tc.birth, tc.occ)
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			if got && label != tc.wantLabel {
				t.Fatalf("label got %q want %q", label, tc.wantLabel)
			}
		})
	}
}

func TestIsMilestone_LeapDay(t *testing.T) {
	birth := time.Date(1996, 2, 29, 0, 0, 0, 0, time.UTC)
	// 30th birthday occurs Feb 28 2026 in non-leap year
	occ := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	got, label := IsMilestone("birthday", birth, occ)
	if !got {
		t.Fatalf("expected milestone for leap-day 30th, got false")
	}
	if label != "30th birthday" {
		t.Fatalf("label %q want 30th birthday", label)
	}
	// non-milestone leap age 26
	occ2 := time.Date(2022, 2, 28, 0, 0, 0, 0, time.UTC) // 26th
	got, _ = IsMilestone("birthday", birth, occ2)
	if got {
		t.Fatalf("expected no milestone for 26th leap")
	}
}

func TestIsMilestone_Anniversary(t *testing.T) {
	cases := []struct {
		name  string
		start time.Time
		occ   time.Time
		want  bool
		label string
	}{
		{"10th", time.Date(2016, 9, 12, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC), true, "10th anniversary"},
		{"25th silver", time.Date(2001, 9, 12, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC), true, "25th anniversary (Silver)"},
		{"50th golden", time.Date(1976, 9, 12, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC), true, "50th anniversary (Golden)"},
		{"60th diamond", time.Date(1966, 5, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), true, "60th anniversary (Diamond)"},
		{"non-milestone 11", time.Date(2015, 9, 12, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC), false, ""},
		{"yearless", time.Date(1, 9, 12, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC), false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, label := IsMilestone(tc.name, tc.start, tc.occ) // use varied type?
			// Actually force anniversary type
			got2, label2 := IsMilestone("anniversary", tc.start, tc.occ)
			if got2 != tc.want {
				t.Fatalf("got %v want %v label %q", got2, tc.want, label2)
			}
			if got2 && label2 != tc.label {
				t.Fatalf("label got %q want %q (got bool %v)", label2, tc.label, got)
			}
			_ = got
			_ = label
		})
	}
}

func TestIsMilestone_DaysFlag(t *testing.T) {
	start := time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)
	// 10000 days after 1999-01-01 is 2026-05-19
	occ := start.AddDate(0, 0, 10000)
	got, _ := IsMilestone("custom", start, occ)
	if got {
		t.Fatal("without flag should not trigger days milestone")
	}
	got, label := IsMilestoneWithOptions("custom", start, occ, true)
	if !got {
		t.Fatal("with flag should trigger")
	}
	if label != "10000 days" {
		t.Fatalf("label %q", label)
	}
	// non-10000 days
	occ2 := start.AddDate(0, 0, 9999)
	got, _ = IsMilestoneWithOptions("custom", start, occ2, true)
	if got {
		t.Fatal("9999 days should not be milestone")
	}
}
