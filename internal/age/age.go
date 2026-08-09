// Package age computes a person's age from their birthday date.
//
// The birth year is derived from the birthday event's date at render time.
// Dates without a usable year (Go's zero time / year 0001, or a future-dated
// manual entry that carries no real birth year) report HasAge=false so the UI
// can omit the age text.
package age

import "time"

// Info carries the derived ages for a single birthday date.
type Info struct {
	// Current is the person's age as of `now` (valid when HasAge).
	Current int
	// Next is the age they turn at their next birthday occurrence.
	Next int
	// HasAge is false when no usable birth year can be derived.
	HasAge bool
}

// InfoFor returns the age information for someone born on birthDate as of now.
func InfoFor(birthDate, now time.Time) Info {
	current, ok := AgeAt(birthDate, now)
	if !ok {
		return Info{}
	}
	return Info{Current: current, Next: current + 1, HasAge: true}
}

// AgeAt returns the age of someone born on birthDate as of now, and whether a
// usable birth year exists.
//
// A birth date with year <= 1 (Go's zero time / year 0001, i.e. a date-only
// entry) yields ok=false. A "birth" dated in the future (negative age, e.g. a
// manual birthday entry carrying the upcoming occurrence year instead of a
// birth year) also yields ok=false so the UI never shows nonsense ages.
//
// Leap-day rule: Feb 29 birthdays are treated as Feb 28 in non-leap years for
// the "has the birthday occurred yet" comparison.
func AgeAt(birthDate, now time.Time) (int, bool) {
	if birthDate.Year() <= 1 {
		return 0, false
	}

	years := now.Year() - birthDate.Year()
	if !birthdayOccurred(birthDate, now) {
		years--
	}
	if years < 0 {
		return 0, false
	}
	return years, true
}

// NextAge returns the age the person turns at their next birthday occurrence,
// and whether a usable birth year exists. This is always Current+1: when the
// birthday has not occurred yet this year they turn N+1 on it; when it already
// passed, the next occurrence is next year and they turn N+1 then.
func NextAge(birthDate, now time.Time) (int, bool) {
	current, ok := AgeAt(birthDate, now)
	if !ok {
		return 0, false
	}
	return current + 1, true
}

// birthdayOccurred reports whether the birthday has occurred (or is today) in
// the year of `now`. A Feb 29 birthday is treated as Feb 28 when the year of
// `now` is not a leap year.
func birthdayOccurred(birthDate, now time.Time) bool {
	m, d := effectiveMonthDay(birthDate, now.Year())
	return now.Month() > m || (now.Month() == m && now.Day() >= d)
}

// effectiveMonthDay returns the month/day to compare against in the given
// year, applying the leap-day rule (Feb 29 → Feb 28 in non-leap years).
func effectiveMonthDay(birthDate time.Time, year int) (time.Month, int) {
	m, d := birthDate.Month(), birthDate.Day()
	if m == time.February && d == 29 && !isLeapYear(year) {
		return time.February, 28
	}
	return m, d
}

func isLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}
