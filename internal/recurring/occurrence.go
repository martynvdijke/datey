package recurring

import (
	"errors"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/hebrew"
	"github.com/datey/datey/internal/hijri"
	"github.com/datey/datey/internal/lunar"
)

// ErrNoOccurrence is returned when a non-Gregorian event has no Gregorian
// occurrence in the target year (absent leap month, or a date that does not
// exist in that calendar year).
var ErrNoOccurrence = errors.New("recurring: no occurrence in year")

// OccurrenceForYear returns the first Gregorian date for event e in Gregorian
// year. For gregorian events it applies the event's frequency (see
// gregorianOccurrences); a once event only has an occurrence in its anchor
// year. For lunar events it converts via internal/lunar. For hebrew and hijri
// events it converts via internal/hebrew and internal/hijri.
//
// A Hebrew or Hijri month/day can occasionally occur twice in one Gregorian
// year. This single-value helper returns the earliest occurrence (the upcoming
// anniversary); callers that need both must use OccurrencesForYear, which is
// what OccurrencesInForEvent does so feeds never miss an occurrence.
func OccurrenceForYear(e *ent.Event, year int) (time.Time, error) {
	occ, err := OccurrencesForYear(e, year)
	if err != nil {
		return time.Time{}, err
	}
	if len(occ) == 0 {
		return time.Time{}, ErrNoOccurrence
	}
	return occ[0], nil
}

// OccurrencesForYear returns every Gregorian occurrence of event e inside
// Gregorian year, in ascending order.
//
// Gregorian events honour frequency, interval, end_date and occurrence_count.
// Non-Gregorian calendar systems support annual semantics only (a non-annual
// frequency combined with them is rejected by the web handler); end_date and
// occurrence_count are still applied to their annual occurrences.
//
// Hebrew events encode Adar II as month 12 together with the lunar_leap flag
// (the ent schema caps lunar_month at 12). hijri month/day use the same
// lunar_month/lunar_day columns.
func OccurrencesForYear(e *ent.Event, year int) ([]time.Time, error) {
	cs := calendarSystem(e)
	if cs == "gregorian" {
		return gregorianOccurrencesInYear(e, year), nil
	}
	occs, err := rawNonGregorianOccurrencesForYear(e, year)
	if err != nil {
		return nil, err
	}
	return applyNonGregorianBounds(e, year, occs), nil
}

// rawNonGregorianOccurrencesForYear converts the event's source-calendar
// month/day into its Gregorian occurrences in Gregorian year, without applying
// frequency bounds.
func rawNonGregorianOccurrencesForYear(e *ent.Event, year int) ([]time.Time, error) {
	switch calendarSystem(e) {
	case "lunar":
		if e.LunarMonth == nil || e.LunarDay == nil {
			return nil, errors.New("recurring: lunar event missing month/day")
		}
		gt, err := lunar.LunarToGregorian(year, *e.LunarMonth, *e.LunarDay, e.LunarLeap)
		if err != nil {
			if errors.Is(err, lunar.ErrNoOccurrence) {
				return nil, ErrNoOccurrence
			}
			return nil, err
		}
		return []time.Time{gt}, nil
	case "hebrew":
		if e.LunarMonth == nil || e.LunarDay == nil {
			return nil, errors.New("recurring: hebrew event missing month/day")
		}
		month := *e.LunarMonth
		// Adar II is stored as month 12 + leap flag; expose it as month 13.
		if month == 12 && e.LunarLeap {
			month = 13
		}
		occ, err := hebrew.OccurrencesInGregorianYear(year, month, *e.LunarDay)
		if err != nil {
			if errors.Is(err, hebrew.ErrNoOccurrence) {
				return nil, ErrNoOccurrence
			}
			return nil, err
		}
		if len(occ) == 0 {
			return nil, ErrNoOccurrence
		}
		return occ, nil
	case "hijri":
		if e.LunarMonth == nil || e.LunarDay == nil {
			return nil, errors.New("recurring: hijri event missing month/day")
		}
		occ, err := hijri.OccurrencesInGregorianYear(year, *e.LunarMonth, *e.LunarDay)
		if err != nil {
			if errors.Is(err, hijri.ErrNoOccurrence) {
				return nil, ErrNoOccurrence
			}
			return nil, err
		}
		if len(occ) == 0 {
			return nil, ErrNoOccurrence
		}
		return occ, nil
	}
	return nil, ErrNoOccurrence
}

func calendarSystem(e *ent.Event) string {
	if e.CalendarSystem == "" {
		return "gregorian"
	}
	return e.CalendarSystem
}

// OccurrencesInForEvent expands event e to occurrences in [from,to]. Gregorian
// events are expanded directly with window arithmetic (no year-by-year walk
// from the epoch); non-Gregorian events go through OccurrencesForYear, so a
// Hijri/Hebrew event that occurs twice in one Gregorian year yields two entries
// and a year without an occurrence is skipped.
func OccurrencesInForEvent(e *ent.Event, from, to time.Time) []time.Time {
	if calendarSystem(e) == "gregorian" {
		return gregorianOccurrences(e, from, to)
	}
	var out []time.Time
	for y := from.Year(); y <= to.Year(); y++ {
		occs, err := OccurrencesForYear(e, y)
		if err != nil {
			continue
		}
		for _, occ := range occs {
			if occ.Before(from) || occ.After(to) {
				continue
			}
			out = append(out, occ)
		}
	}
	return out
}
