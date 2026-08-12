package web

import (
	"testing"
	"time"
)

func TestCalendarDayDeltaIgnoresTimeAndDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	from := dateOnly(time.Date(2025, 3, 8, 23, 30, 0, 0, loc), loc)
	to := dateOnly(time.Date(2025, 3, 9, 0, 15, 0, 0, loc), loc)
	if got := calendarDayDelta(from, to); got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
}
