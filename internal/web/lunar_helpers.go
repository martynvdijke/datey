package web

import (
	"strconv"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/recurring"
)

// lunarOccurrenceForYear returns the Gregorian occurrence for a lunar event.
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

// displayDateForEvent returns the occurrence Gregorian date for display and
// a secondary lunar notation if applicable.
func displayDateForEvent(e *ent.Event, now time.Time) (primary time.Time, secondary string) {
	if e.CalendarSystem == "lunar" && e.LunarMonth != nil && e.LunarDay != nil {
		if occ, err := recurring.OccurrenceForYear(e, now.Year()); err == nil {
			return occ, formatLunarNotation(e)
		}
	}
	// Gregorian: occurrence in current year
	if occ, err := recurring.OccurrenceForYear(e, now.Year()); err == nil {
		return occ, ""
	}
	return e.Date, ""
}
