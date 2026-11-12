// Package hebrew converts Hebrew (Jewish) month/day anniversaries to
// Gregorian dates.
//
// The conversion is defined per *Gregorian* year, matching the rest of the
// recurring package: given a Hebrew month/day, it returns the Gregorian
// date(s) on which that anniversary falls inside the requested Gregorian year.
//
// A Hebrew year is 353–385 days long, so unlike the Gregorian calendar a
// Hebrew month/day is not guaranteed to fall exactly once in every Gregorian
// year (it can occasionally fall twice, or — for day 30 of a 29-day month —
// not at all). OccurrencesInGregorianYear therefore returns a slice; most
// callers will see exactly one element.
package hebrew

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/hebcal/hdate"
)

// ErrNoOccurrence is returned when the requested Hebrew date has no Gregorian
// occurrence inside the target Gregorian year.
var ErrNoOccurrence = errors.New("hebrew: no occurrence in Gregorian year")

// ToGregorian returns the first Gregorian occurrence inside gYear for the
// Hebrew month/day, or ErrNoOccurrence when the date does not occur in that
// Gregorian year.
//
// month is the conventional Hebrew month number 1..13:
//
//	1 Nisan, 2 Iyyar, 3 Sivan, 4 Tamuz, 5 Av, 6 Elul,
//	7 Tishrei, 8 Cheshvan, 9 Kislev, 10 Tevet, 11 Shvat,
//	12 Adar (or Adar I in a leap year), 13 Adar II.
//
// Month 13 exists only in Hebrew leap years; in a non-leap year it falls back
// to Adar (month 12) as described on OccurrencesInGregorianYear.
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
// which the Hebrew month/day anniversary falls, sorted ascending.
//
// Fallback rules (the conventions from "Calendrical Calculations" and hebcal;
// see hdate.GetBirthdayOrAnniversary/GetYahrzeit):
//
//   - Adar II (month 13) is observed in the last month of the target Hebrew
//     year: Adar II in a leap year, plain Adar in a non-leap year. Adar II
//     day 29 therefore becomes Adar 29 in a non-leap year.
//   - Adar I day 30 (only valid in a leap year) has no counterpart in a
//     non-leap year; it is observed on 1 Nisan of that year, i.e. the first
//     day of the following month, rather than being skipped.
//   - Cheshvan 30 in a short (29-day Cheshvan) year becomes 1 Kislev, and
//     Kislev 30 in a short Kislev year becomes 1 Tevet.
//
// The Hebrew year boundary is handled by trying the Hebrew years that overlap
// the Gregorian year: a Gregorian January date belongs to the Hebrew year that
// began in the previous September/October.
func OccurrencesInGregorianYear(gYear, month, day int) ([]time.Time, error) {
	hm, err := normalizeMonth(month)
	if err != nil {
		return nil, err
	}
	if err := ValidMonthDay(month, day); err != nil {
		return nil, err
	}

	// Hebrew year at the start of gYear is roughly gYear + 3760/3761.
	// Probe a small window and keep only dates that land in gYear.
	base := gYear + 3761
	seen := make(map[int64]struct{}, 2)
	out := make([]time.Time, 0, 2)
	for hy := base - 2; hy <= base+2; hy++ {
		hd, ok := anniversary(hy, hm, month, day)
		if !ok {
			continue
		}
		gt := hd.Gregorian()
		if gt.Year() != gYear {
			continue
		}
		abs := hd.Abs()
		if _, dup := seen[abs]; dup {
			continue
		}
		seen[abs] = struct{}{}
		out = append(out, gt)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out, nil
}

// ValidMonthDay reports whether month/day is a usable Hebrew date. Day counts
// follow the longest form of each month so that, for example, Adar I 30 can be
// entered even though it only exists in leap years (the conversion applies the
// documented fallback in other years).
func ValidMonthDay(month, day int) error {
	if month < 1 || month > 13 {
		return fmt.Errorf("hebrew: month %d out of range 1..13", month)
	}
	if day < 1 || day > 30 {
		return fmt.Errorf("hebrew: day %d out of range 1..30", day)
	}
	switch month {
	case 2, 4, 6, 10, 13: // Iyyar, Tamuz, Elul, Tevet, Adar II always 29
		if day > 29 {
			return fmt.Errorf("hebrew: day %d does not exist in month %d", day, month)
		}
	}
	return nil
}

// IsLeapYear reports whether the Hebrew year is a leap (13-month) year.
func IsLeapYear(year int) bool { return hdate.IsLeapYear(year) }

func normalizeMonth(month int) (hdate.HMonth, error) {
	if month < 1 || month > 13 {
		return 0, fmt.Errorf("hebrew: month %d out of range 1..13", month)
	}
	return hdate.HMonth(month), nil
}

// anniversary builds the Hebrew date for the requested month/day in hyear,
// applying the fallback rules documented on OccurrencesInGregorianYear.
func anniversary(hyear int, hm hdate.HMonth, inputMonth, day int) (hdate.HDate, bool) {
	month := hm
	// Adar II (month 13) maps to the last month of the target year: Adar II
	// in a leap year, Adar in a non-leap year. hdate.New also maps Adar2 to
	// Adar1 in non-leap years, but doing it explicitly keeps the rule visible.
	if inputMonth == 13 {
		month = hdate.HMonth(hdate.MonthsInYear(hyear))
	}
	// Adar I 30 in a non-leap year → 1 Nisan.
	if month == hdate.Adar1 && day == 30 && !hdate.IsLeapYear(hyear) {
		month, day = hdate.Nisan, 1
	}
	// Cheshvan/Kislev 30 in years where those months have 29 days.
	if month == hdate.Cheshvan && day == 30 && !hdate.LongCheshvan(hyear) {
		month, day = hdate.Kislev, 1
	}
	if month == hdate.Kislev && day == 30 && hdate.DaysInMonth(hdate.Kislev, hyear) < 30 {
		month, day = hdate.Tevet, 1
	}
	if day < 1 || day > hdate.DaysInMonth(month, hyear) {
		return hdate.HDate{}, false
	}
	return hdate.New(hyear, month, day), true
}
