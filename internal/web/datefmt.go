package web

import "time"

// datefmt renders user-facing dates according to the configured date variant.
//
// The variant is one of:
//   - "european": day-first ("25 Dec", "25 Dec 2025")
//   - "us":       month-first ("Dec 25", "Dec 25, 2025")
//
// An unknown variant falls back to european. Machine-readable feeds (iCal,
// RSS, Home Assistant, upcoming API) keep ISO 8601 and must not use these
// helpers.

// dateLayouts holds the display layouts for each variant: short (no year),
// long (with year), and weekday (with weekday name).
var dateLayouts = map[string]struct{ short, long, weekday string }{
	"european": {"2 Jan", "2 Jan 2006", "Monday 2 January"},
	"us":       {"Jan 2", "Jan 2, 2006", "Monday, January 2"},
}

// layoutsFor returns the layouts for variant, defaulting to european when the
// variant is unknown or empty.
func layoutsFor(variant string) (short, long, weekday string) {
	l, ok := dateLayouts[variant]
	if !ok {
		l = dateLayouts["european"]
	}
	return l.short, l.long, l.weekday
}

// shortDate formats t without a year ("25 Dec" or "Dec 25").
func shortDate(variant string, t time.Time) string {
	short, _, _ := layoutsFor(variant)
	return t.Format(short)
}

// longDate formats t with a year ("25 Dec 2025" or "Dec 25, 2025").
func longDate(variant string, t time.Time) string {
	_, long, _ := layoutsFor(variant)
	return t.Format(long)
}

// weekdayDate formats t with the weekday name ("Monday 2 January" or
// "Monday, January 2").
func weekdayDate(variant string, t time.Time) string {
	_, _, weekday := layoutsFor(variant)
	return t.Format(weekday)
}
