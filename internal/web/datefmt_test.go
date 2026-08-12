package web

import (
	"testing"
	"time"
)

func TestShortDate(t *testing.T) {
	ts := time.Date(2025, time.December, 25, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		variant string
		want    string
	}{
		{"european", "25 Dec"},
		{"us", "Dec 25"},
		{"", "25 Dec"},       // empty falls back to european
		{"french", "25 Dec"}, // unknown falls back to european
	}
	for _, tc := range cases {
		if got := shortDate(tc.variant, ts); got != tc.want {
			t.Errorf("shortDate(%q) = %q, want %q", tc.variant, got, tc.want)
		}
	}
}

func TestLongDate(t *testing.T) {
	ts := time.Date(2025, time.December, 25, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		variant string
		want    string
	}{
		{"european", "25 Dec 2025"},
		{"us", "Dec 25, 2025"},
		{"", "25 Dec 2025"},
	}
	for _, tc := range cases {
		if got := longDate(tc.variant, ts); got != tc.want {
			t.Errorf("longDate(%q) = %q, want %q", tc.variant, got, tc.want)
		}
	}
}

func TestWeekdayDate(t *testing.T) {
	ts := time.Date(2025, time.December, 25, 0, 0, 0, 0, time.UTC) // a Thursday
	cases := []struct {
		variant string
		want    string
	}{
		{"european", "Thursday 25 December"},
		{"us", "Thursday, December 25"},
		{"", "Thursday 25 December"},
	}
	for _, tc := range cases {
		if got := weekdayDate(tc.variant, ts); got != tc.want {
			t.Errorf("weekdayDate(%q) = %q, want %q", tc.variant, got, tc.want)
		}
	}
}
