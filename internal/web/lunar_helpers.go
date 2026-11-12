package web

import (
	"strconv"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/recurring"
)

// lunarOccurrenceForYear returns the Gregorian occurrence for a lunar event.
// It is kept for callers that still name the lunar case explicitly; every
// calendar system goes through the same recurring seam.
func (h *Handler) lunarOccurrenceForYear(e *ent.Event, year int) (time.Time, error) {
	return recurring.OccurrenceForYear(e, year)
}

// formatLunarNotation returns "lunar M/D" or "lunar leap M/D" for display.
func formatLunarNotation(e *ent.Event) string {
	if e.CalendarSystem != "lunar" || e.LunarMonth == nil || e.LunarDay == nil {
		return ""
	}
	if e.LunarLeap {
		return "lunar leap " + strconv.Itoa(*e.LunarMonth) + "/" + strconv.Itoa(*e.LunarDay)
	}
	return "lunar " + strconv.Itoa(*e.LunarMonth) + "/" + strconv.Itoa(*e.LunarDay)
}

// formatCalendarNotation returns a human-readable source-calendar notation for
// a non-Gregorian event: "lunar 8/15", "hebrew 7/1", "hijri 9/1".
func formatCalendarNotation(e *ent.Event) string {
	if e.LunarMonth == nil || e.LunarDay == nil {
		return ""
	}
	switch e.CalendarSystem {
	case "lunar":
		return formatLunarNotation(e)
	case "hebrew":
		month := *e.LunarMonth
		// Adar II is stored as month 12 + leap flag.
		if month == 12 && e.LunarLeap {
			month = 13
		}
		return "hebrew " + strconv.Itoa(month) + "/" + strconv.Itoa(*e.LunarDay)
	case "hijri":
		return "hijri " + strconv.Itoa(*e.LunarMonth) + "/" + strconv.Itoa(*e.LunarDay)
	}
	return ""
}

// nextOccurrenceAfter returns the first occurrence of e on or after from,
// searching the current Gregorian year and the next two (a non-Gregorian
// anniversary can be absent from a year entirely).
func nextOccurrenceAfter(e *ent.Event, from time.Time) (time.Time, bool) {
	fromDay := dateOnly(from, from.Location())
	for y := from.Year(); y <= from.Year()+2; y++ {
		occs, err := recurring.OccurrencesForYear(e, y)
		if err != nil {
			continue
		}
		for _, occ := range occs {
			if !dateOnly(occ, time.UTC).Before(fromDay) {
				return occ, true
			}
		}
	}
	return time.Time{}, false
}

// displayDateForEvent returns the occurrence Gregorian date for display and
// a secondary source-calendar notation if applicable.
//
// For a Hebrew/Hijri date that does not occur in the current Gregorian year
// (e.g. Hijri 30 Dhu al-Hijjah in a non-leap Hijri year) the next year that
// does have the occurrence is used instead of falling back to the stored date.
func displayDateForEvent(e *ent.Event, now time.Time) (primary time.Time, secondary string) {
	nonGregorian := e.CalendarSystem != "" && e.CalendarSystem != "gregorian" && e.LunarMonth != nil && e.LunarDay != nil
	if nonGregorian {
		for y := now.Year(); y <= now.Year()+1; y++ {
			if occ, err := recurring.OccurrenceForYear(e, y); err == nil {
				return occ, formatCalendarNotation(e)
			}
		}
	}
	// Gregorian: occurrence in current year
	if occ, err := recurring.OccurrenceForYear(e, now.Year()); err == nil {
		return occ, ""
	}
	return e.Date, ""
}
