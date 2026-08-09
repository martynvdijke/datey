// Package recurring provides computable recurring-rule dates, such as
// Easter-based holidays resolved via the Gregorian computus.
package recurring

import "time"

// Pattern types for Easter-based recurring rules. These are additive values
// of the recurring-rule `pattern_type` string field; no stored fixed date is
// needed — the type alone determines the date for a given year.
const (
	EasterSundayRule = "easter_sunday" // Easter Sunday
	GoodFridayRule   = "good_friday"   // Easter − 2 days
	EasterMondayRule = "easter_monday" // Easter + 1 day
	AshWednesdayRule = "ash_wednesday" // Easter − 46 days
	PentecostRule    = "pentecost"     // Easter + 49 days
)

// easterOffsets maps an Easter-based pattern type to its day offset from
// Easter Sunday.
var easterOffsets = map[string]int{
	EasterSundayRule: 0,
	GoodFridayRule:   -2,
	EasterMondayRule: 1,
	AshWednesdayRule: -46,
	PentecostRule:    49,
}

// IsEasterBased reports whether patternType is one of the computable
// Easter-based rule types.
func IsEasterBased(patternType string) bool {
	_, ok := easterOffsets[patternType]
	return ok
}

// EasterSunday returns the date of Easter Sunday for the given year using the
// Meeus/Jones/Butcher Gregorian computus algorithm (Western Easter).
func EasterSunday(year int) time.Time {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

// DateForType resolves the date for an Easter-based pattern type in the given
// year. It returns the zero time for unknown types.
func DateForType(patternType string, year int) time.Time {
	offset, ok := easterOffsets[patternType]
	if !ok {
		return time.Time{}
	}
	return EasterSunday(year).AddDate(0, 0, offset)
}
