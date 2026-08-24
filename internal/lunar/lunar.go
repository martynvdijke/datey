package lunar

import (
	"errors"
	"fmt"
	"time"

	"github.com/6tail/lunar-go/calendar"
)

// ErrNoOccurrence is returned when a leap-marked lunar date has no
// occurrence in the requested Gregorian year (the year lacks that leap month).
var ErrNoOccurrence = errors.New("lunar: no occurrence in this year (leap month absent)")

// LunarToGregorian converts a lunar month/day to the Gregorian date for the
// given Gregorian year.
//
//   - month 1..12, day 1..30
//   - leap true means the leap-month variant (e.g. leap 4 =闰四月)
//   - For leap==true the function returns ErrNoOccurrence when the target year
//     does not contain that leap month.
//   - Returns a midnight UTC time on the converted Gregorian date.
//   - Supported year range is approximately 1900–2100; out-of-range years
//     return an error.
func LunarToGregorian(year, month, day int, leap bool) (time.Time, error) {
	if month < 1 || month > 12 {
		return time.Time{}, fmt.Errorf("lunar: month %d out of range 1..12", month)
	}
	if day < 1 || day > 30 {
		return time.Time{}, fmt.Errorf("lunar: day %d out of range 1..30", day)
	}
	if year < 1900 || year > 2100 {
		return time.Time{}, fmt.Errorf("lunar: year %d out of supported range 1900..2100", year)
	}

	monthKey := month
	if leap {
		monthKey = -month
	}

	// Verify month exists in this lunar year (leap months only exist some years).
	ly := calendar.NewLunarYear(year)
	if ly.GetMonth(monthKey) == nil {
		if leap {
			return time.Time{}, ErrNoOccurrence
		}
		return time.Time{}, fmt.Errorf("lunar: month %d not found for year %d", month, year)
	}

	// Validate day count for that specific month.
	lm := ly.GetMonth(monthKey)
	if day > lm.GetDayCount() {
		return time.Time{}, fmt.Errorf("lunar: day %d exceeds %d days in lunar %d/%d (leap=%v)", day, lm.GetDayCount(), year, month, leap)
	}

	// calendar.NewLunar panics on invalid inputs; we have validated.
	l := calendar.NewLunar(year, monthKey, day, 0, 0, 0)
	s := l.GetSolar()
	return time.Date(s.GetYear(), time.Month(s.GetMonth()), s.GetDay(), 0, 0, 0, 0, time.UTC), nil
}

// IsLeapMonth reports whether the given lunar year contains a leap month
// with the given month number (1..12).
func IsLeapMonth(year, month int) bool {
	ly := calendar.NewLunarYear(year)
	return ly.GetMonth(-month) != nil
}
