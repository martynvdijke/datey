package recurring

import (
	"errors"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/lunar"
)

// ErrNoOccurrence is returned when a lunar event has no Gregorian occurrence
// in the target year (leap-month absent).
var ErrNoOccurrence = errors.New("recurring: no occurrence in year")

// OccurrenceForYear returns the Gregorian date for event e in Gregorian year.
// For gregorian (default) events it applies month/day passthrough (Feb 29->28).
// For lunar events it converts via internal/lunar; leap-marked events with no
// leap month in the target year return ErrNoOccurrence.
func OccurrenceForYear(e *ent.Event, year int) (time.Time, error) {
	cs := e.CalendarSystem
	if cs == "" {
		cs = "gregorian"
	}
	if cs == "lunar" {
		if e.LunarMonth == nil || e.LunarDay == nil {
			return time.Time{}, errors.New("recurring: lunar event missing month/day")
		}
		gt, err := lunar.LunarToGregorian(year, *e.LunarMonth, *e.LunarDay, e.LunarLeap)
		if err != nil {
			if errors.Is(err, lunar.ErrNoOccurrence) {
				return time.Time{}, ErrNoOccurrence
			}
			return time.Time{}, err
		}
		return gt, nil
	}
	// gregorian
	return occurrenceInYear(e.Date, year), nil
}

// OccurrencesInForEvent expands event e to occurrences in [from,to] using
// OccurrenceForYear. This is the lunar-aware replacement for
// OccurrencesIn(time.Time,...).
func OccurrencesInForEvent(e *ent.Event, from, to time.Time) []time.Time {
	var out []time.Time
	for y := from.Year(); y <= to.Year(); y++ {
		occ, err := OccurrenceForYear(e, y)
		if err != nil {
			if errors.Is(err, ErrNoOccurrence) {
				continue
			}
			continue
		}
		if occ.Before(from) || occ.After(to) {
			continue
		}
		out = append(out, occ)
	}
	return out
}
