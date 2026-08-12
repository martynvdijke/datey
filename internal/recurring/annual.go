package recurring

import "time"

// annualEventTypes are event types whose month/day recurs every year. These
// match the set the iCal feed renders with RRULE:FREQ=YEARLY.
var annualEventTypes = map[string]bool{
	"birthday":    true,
	"anniversary": true,
	"wedding":     true,
	"holiday":     true,
}

// IsAnnualType reports whether an event type recurs annually.
func IsAnnualType(eventType string) bool {
	return annualEventTypes[eventType]
}

// isLeapYear reports whether y is a leap year (Gregorian rules).
func isLeapYear(y int) bool {
	return y%4 == 0 && (y%100 != 0 || y%400 == 0)
}

// occurrenceInYear applies the month/day of date to the given year, drifting
// February 29 to February 28 in non-leap years. The result is midnight UTC.
func occurrenceInYear(date time.Time, year int) time.Time {
	month, day := date.Month(), date.Day()
	if month == time.February && day == 29 && !isLeapYear(year) {
		day = 28
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// OccurrencesIn returns the annual occurrences of date whose month/day falls
// inside the inclusive range [from, to]. The stored date's year is ignored —
// the month/day is applied to each year the range spans, so a historical
// birthday (e.g. 1990-05-12) yields its current-year occurrence. February 29
// drifts to February 28 in non-leap years.
//
// At most two occurrences are returned (the range spans at most one extra
// calendar year past its start year), sorted ascending, at midnight UTC.
func OccurrencesIn(date time.Time, from, to time.Time) []time.Time {
	if date.IsZero() {
		return nil
	}

	var out []time.Time
	for y := from.Year(); y <= to.Year(); y++ {
		occ := occurrenceInYear(date, y)
		if occ.Before(from) || occ.After(to) {
			continue
		}
		out = append(out, occ)
	}
	return out
}
