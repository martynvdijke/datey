// Package hijri converts tabular Islamic (Hijri) month/day anniversaries to
// Gregorian dates.
//
// The conversion is defined per *Gregorian* year: given a Hijri month/day, it
// returns the Gregorian date(s) on which that anniversary falls inside the
// requested Gregorian year.
//
// A tabular Hijri year is 354 or 355 days, about 11 days shorter than a
// Gregorian year. Consequently the same Hijri month/day can occur twice in one
// Gregorian year (when two consecutive anniversaries fall inside the same
// 365/366-day window) and, for day 30 of a 29-day month, not at all in some
// Gregorian years. OccurrencesInGregorianYear returns a slice so callers see
// both occurrences instead of silently dropping one; ToGregorian returns the
// earliest occurrence in the year, which is the upcoming anniversary.
package hijri

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/hablullah/go-hijri"
)

// ErrNoOccurrence is returned when the requested Hijri date has no Gregorian
// occurrence inside the target Gregorian year.
var ErrNoOccurrence = errors.New("hijri: no occurrence in Gregorian year")

// ToGregorian returns the first Gregorian occurrence inside gYear for the
// Hijri month/day, or ErrNoOccurrence when the date does not occur in that
// Gregorian year.
func ToGregorian(gYear, month, day int) (time.Time, error) {
	occ, err := OccurrencesInGregorianYear(gYear, month, day)
	if err != nil {
		return time.Time{}, err
	}
	if len(occ) == 0 {
		return time.Time{}, ErrNoOccurrence
	}
	return occ[0], nil
}

// OccurrencesInGregorianYear returns every Gregorian date inside gYear on
// which the Hijri month/day anniversary falls, sorted ascending.
//
// Existence is verified by round-tripping the candidate Gregorian date back to
// the tabular Hijri calendar: a day 30 that does not exist in that Hijri year
// (e.g. 30 Dhu al-Hijjah in a non-leap year) overflows into the next month and
// the round-trip rejects it, so it is treated as no occurrence instead of
// being reported on a wrong day. Duplicate dates are removed so the same
// anniversary is never notified twice.
func OccurrencesInGregorianYear(gYear, month, day int) ([]time.Time, error) {
	if err := ValidMonthDay(month, day); err != nil {
		return nil, err
	}

	// Tabular Hijri year at the start of gYear: (gYear-622) * 33/32.
	base := (gYear-622)*33/32 + 1
	out := make([]time.Time, 0, 2)
	for hy := base - 2; hy <= base+2; hy++ {
		candidate := hijri.HijriDate{
			Day:     int64(day),
			Month:   int64(month),
			Year:    int64(hy),
			Pattern: hijri.Default,
		}.ToGregorian()
		gt := time.Date(candidate.Year(), candidate.Month(), candidate.Day(), 0, 0, 0, 0, time.UTC)
		if gt.Year() != gYear {
			continue
		}
		// Verify the day actually exists in that Hijri year.
		back, err := hijri.CreateHijriDate(gt, hijri.Default)
		if err != nil || back.Month != int64(month) || back.Day != int64(day) {
			continue
		}
		if len(out) > 0 && out[len(out)-1].Equal(gt) {
			continue
		}
		out = append(out, gt)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out, nil
}

// ValidMonthDay reports whether month/day is a usable tabular Hijri date.
// Odd months have 30 days, even months 29; month 12 has 30 days in leap years.
func ValidMonthDay(month, day int) error {
	if month < 1 || month > 12 {
		return fmt.Errorf("hijri: month %d out of range 1..12", month)
	}
	if day < 1 || day > 30 {
		return fmt.Errorf("hijri: day %d out of range 1..30", day)
	}
	if month%2 == 0 && month != 12 && day > 29 {
		return fmt.Errorf("hijri: day %d does not exist in month %d", day, month)
	}
	return nil
}

// DaysInMonth returns the length of the Hijri month in the given Hijri year
// under the tabular (default) leap-year pattern.
func DaysInMonth(year, month int) int {
	if month == 12 && isLeapYear(year) {
		return 30
	}
	return 29 + month%2
}

// isLeapYear mirrors the Default pattern of github.com/hablullah/go-hijri
// (leap years are 2, 5, 7, 10, 13, 16, 18, 21, 24, 26 and 29 of the 30-year
// cycle). It is used only for UI guidance; occurrence generation verifies
// existence by round-trip.
func isLeapYear(year int) bool {
	switch year % 30 {
	case 2, 5, 7, 10, 13, 16, 18, 21, 24, 26, 29:
		return true
	}
	return false
}
