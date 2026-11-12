package recurring

import (
	"time"

	"github.com/datey/datey/ent"
)

// Event frequency values. The stored value is free-form text; anything that is
// not one of these is treated as annual by the expansion functions so a bad
// row degrades to the historical behaviour instead of disappearing.
const (
	FrequencyOnce    = "once"
	FrequencyAnnual  = "annual"
	FrequencyMonthly = "monthly"
	FrequencyWeekly  = "weekly"
	FrequencyEveryN  = "every_n"
)

// Frequencies lists every supported frequency in display order.
var Frequencies = []string{
	FrequencyOnce,
	FrequencyAnnual,
	FrequencyMonthly,
	FrequencyWeekly,
	FrequencyEveryN,
}

// NormalizeFrequency maps a stored frequency to its canonical value. The empty
// string means annual (the column default). The second result is false for an
// unknown value.
func NormalizeFrequency(freq string) (string, bool) {
	switch freq {
	case "":
		return FrequencyAnnual, true
	case FrequencyOnce, FrequencyAnnual, FrequencyMonthly, FrequencyWeekly, FrequencyEveryN:
		return freq, true
	default:
		return FrequencyAnnual, false
	}
}

// dateOnlyUTC truncates t to midnight UTC. Occurrences are always compared as
// calendar days, never as instants.
func dateOnlyUTC(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// addMonthsClamped adds k calendar months to anchor, clamping the day to the
// last day of the target month (Jan 31 + 1 month => Feb 28/29). The result is
// midnight UTC.
func addMonthsClamped(anchor time.Time, k int) time.Time {
	y := anchor.Year()
	m := int(anchor.Month()) - 1 + k
	y += m / 12
	m %= 12
	if m < 0 {
		m += 12
		y--
	}
	month := time.Month(m + 1)
	day := anchor.Day()
	if last := daysInMonthInt(month, y); day > last {
		day = last
	}
	return time.Date(y, month, day, 0, 0, 0, 0, time.UTC)
}

// intervalDays returns the day step for weekly/every_n events.
func intervalDays(e *ent.Event) int {
	freq, _ := NormalizeFrequency(e.Frequency)
	if freq != FrequencyEveryN {
		return 7
	}
	if n := e.Interval; n >= 1 {
		return n
	}
	return 1
}

// gregorianOccurrences expands a gregorian event into every occurrence inside
// the inclusive window [from, to], in ascending order.
//
// Series shapes (the stored date is the anchor / occurrence #1):
//
//	once     the anchor alone
//	annual   the anchor's month/day applied to each year in the window; the
//	         February-29 to February-28 drift is kept. Without end_date or
//	         occurrence_count the legacy behaviour is preserved: the month/day
//	         applies to years before the anchor year too.
//	monthly  the same day-of-month every month from the anchor month, clamped
//	         to the last day of short months (31st => Feb 28/29, Apr 30, ...)
//	weekly   anchor + 7k days
//	every_n  anchor + interval*k days
//
// Bounds (both optional): end_date is the inclusive last calendar day an
// occurrence may fall on; occurrence_count caps the number of occurrences
// counted from the anchor inclusive. The earliest bound wins. When either bound
// is set the series starts at the anchor, so pre-anchor anniversaries are not
// emitted.
func gregorianOccurrences(e *ent.Event, from, to time.Time) []time.Time {
	anchor := dateOnlyUTC(e.Date)
	if anchor.IsZero() {
		return nil
	}
	if to.Before(from) {
		return nil
	}
	// Arithmetic uses calendar days; the window filters keep the caller's raw
	// instants so an occurrence earlier today stays outside a window starting
	// later today (the historical comparison).
	fromDay := dateOnlyUTC(from)
	toDay := dateOnlyUTC(to)
	// A rule event is annual by definition: its date is computed per year from
	// the rule, so a stored frequency of "once" must not suppress it.
	if IsRuleEvent(e) {
		return ruleOccurrences(e, from, to, fromDay, toDay, anchor)
	}
	freq, _ := NormalizeFrequency(e.Frequency)

	endDate, hasEnd := seriesEndDate(e)
	lastK, hasCount := seriesLastIndex(e)

	switch freq {
	case FrequencyOnce:
		if anchor.Before(from) || anchor.After(to) {
			return nil
		}
		if hasEnd && anchor.After(endDate) {
			return nil
		}
		return []time.Time{anchor}

	case FrequencyMonthly:
		return monthlyOccurrences(anchor, from, to, fromDay, endDate, hasEnd, lastK, hasCount)

	case FrequencyWeekly, FrequencyEveryN:
		return stepOccurrences(anchor, from, to, fromDay, intervalDays(e), endDate, hasEnd, lastK, hasCount)

	default: // FrequencyAnnual
		return annualOccurrences(anchor, from, to, fromDay, toDay, endDate, hasEnd, lastK, hasCount)
	}
}

func seriesEndDate(e *ent.Event) (time.Time, bool) {
	if e.EndDate == nil {
		return time.Time{}, false
	}
	return dateOnlyUTC(*e.EndDate), true
}

// seriesLastIndex returns the highest zero-based series index allowed by
// occurrence_count (index 0 is the anchor).
func seriesLastIndex(e *ent.Event) (int, bool) {
	if e.OccurrenceCount == nil || *e.OccurrenceCount < 1 {
		return 0, false
	}
	return *e.OccurrenceCount - 1, true
}

// ruleOccurrences expands a rule event once per year in the window, resolving
// each year with RuleDateForYear. Bounds work like the annual path: end_date is
// the inclusive last day (and the anchor year is its lower bound), while
// occurrence_count counts years from the anchor year (anchor = occurrence #1).
// A year whose rule does not resolve is skipped without consuming a count.
func ruleOccurrences(e *ent.Event, from, to, fromDay, toDay, anchor time.Time) []time.Time {
	endDate, hasEnd := seriesEndDate(e)
	lastK, hasCount := seriesLastIndex(e)

	var out []time.Time
	for y := fromDay.Year(); y <= toDay.Year(); y++ {
		if hasCount || hasEnd {
			if y < anchor.Year() {
				continue
			}
			if hasCount && y-anchor.Year() > lastK {
				break
			}
		}
		occ, ok := RuleDateForYear(e, y)
		if !ok {
			continue
		}
		if occ.Before(from) || occ.After(to) {
			continue
		}
		if hasEnd && occ.After(endDate) {
			break
		}
		out = append(out, occ)
	}
	return out
}

func annualOccurrences(anchor, from, to, fromDay, toDay, endDate time.Time, hasEnd bool, lastK int, hasCount bool) []time.Time {
	var out []time.Time
	for y := fromDay.Year(); y <= toDay.Year(); y++ {
		if hasCount || hasEnd {
			// Bounded series starts at the anchor year.
			if y < anchor.Year() {
				continue
			}
			if hasCount && y-anchor.Year() > lastK {
				break
			}
		}
		occ := occurrenceInYear(anchor, y)
		if occ.Before(from) || occ.After(to) {
			continue
		}
		if hasEnd && occ.After(endDate) {
			break
		}
		out = append(out, occ)
	}
	return out
}

func monthlyOccurrences(anchor, from, to, fromDay, endDate time.Time, hasEnd bool, lastK int, hasCount bool) []time.Time {
	// First month index whose clamped occurrence can reach the window.
	k := (fromDay.Year()-anchor.Year())*12 + int(fromDay.Month()) - int(anchor.Month())
	if k < 0 {
		k = 0
	}
	var out []time.Time
	for ; ; k++ {
		if hasCount && k > lastK {
			break
		}
		occ := addMonthsClamped(anchor, k)
		if occ.After(to) {
			break
		}
		if occ.Before(from) {
			continue
		}
		if hasEnd && occ.After(endDate) {
			break
		}
		out = append(out, occ)
	}
	return out
}

func stepOccurrences(anchor, from, to, fromDay time.Time, step int, endDate time.Time, hasEnd bool, lastK int, hasCount bool) []time.Time {
	if step < 1 {
		step = 1
	}
	// Skip whole steps before the window: ceil(days before window / step).
	diff := int(fromDay.Sub(anchor).Hours() / 24)
	k := 0
	if diff > 0 {
		k = (diff + step - 1) / step
	}
	var out []time.Time
	for ; ; k++ {
		if hasCount && k > lastK {
			break
		}
		occ := anchor.AddDate(0, 0, step*k)
		if occ.After(to) {
			break
		}
		if hasEnd && occ.After(endDate) {
			break
		}
		out = append(out, occ)
	}
	return out
}

// gregorianOccurrencesInYear returns the occurrences of a gregorian event
// inside Gregorian year.
func gregorianOccurrencesInYear(e *ent.Event, year int) []time.Time {
	start := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC)
	return gregorianOccurrences(e, start, end)
}

// applyNonGregorianBounds filters the raw occurrences of one Gregorian year for
// a lunar/hebrew/hijri event by occurrence_count and end_date. Counting walks
// the earlier years from the anchor year, so it stays correct when a target
// year holds two occurrences or none.
func applyNonGregorianBounds(e *ent.Event, year int, occs []time.Time) []time.Time {
	endDate, hasEnd := seriesEndDate(e)
	_, hasCount := seriesLastIndex(e)
	if !hasEnd && !hasCount {
		return occs
	}
	anchorYear := dateOnlyUTC(e.Date).Year()

	if hasEnd {
		filtered := make([]time.Time, 0, len(occs))
		for _, occ := range occs {
			if !occ.After(endDate) {
				filtered = append(filtered, occ)
			}
		}
		occs = filtered
	}
	if !hasCount {
		return occs
	}
	if year < anchorYear {
		return nil
	}
	count := *e.OccurrenceCount
	for y := anchorYear; y < year; y++ {
		prev, err := rawNonGregorianOccurrencesForYear(e, y)
		if err != nil {
			continue
		}
		for _, occ := range prev {
			if hasEnd && occ.After(endDate) {
				continue
			}
			count--
		}
	}
	if count <= 0 {
		return nil
	}
	if len(occs) > count {
		occs = occs[:count]
	}
	return occs
}
