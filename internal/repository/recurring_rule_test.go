package repository

import (
	"testing"
	"time"
)

func TestCalcNthWeekday_MothersDay2026(t *testing.T) {
	date := calcNthWeekday(2, time.Sunday, time.May, 2026)
	want := time.Date(2026, time.May, 10, 0, 0, 0, 0, time.UTC)
	if !date.Equal(want) {
		t.Errorf("Mother's Day 2026: got %s, want %s", date.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func TestCalcNthWeekday_FathersDay2026(t *testing.T) {
	date := calcNthWeekday(3, time.Sunday, time.June, 2026)
	want := time.Date(2026, time.June, 21, 0, 0, 0, 0, time.UTC)
	if !date.Equal(want) {
		t.Errorf("Father's Day 2026: got %s, want %s", date.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func TestCalcNthWeekday_FirstMonday(t *testing.T) {
	date := calcNthWeekday(1, time.Monday, time.January, 2026)
	want := time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC)
	if !date.Equal(want) {
		t.Errorf("First Monday Jan 2026: got %s, want %s", date.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func TestCalcNthWeekday_FourthFriday(t *testing.T) {
	date := calcNthWeekday(4, time.Friday, time.November, 2026)
	if date.Month() != time.November || date.Weekday() != time.Friday {
		t.Errorf("Fourth Friday Nov 2026: got %s", date.Format("2006-01-02"))
	}
}

func TestCalcLastWeekday_LastSunday(t *testing.T) {
	date := calcLastWeekday(time.Sunday, time.March, 2026)
	want := time.Date(2026, time.March, 29, 0, 0, 0, 0, time.UTC)
	if !date.Equal(want) {
		t.Errorf("Last Sunday March 2026: got %s, want %s", date.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func TestCalcLastWeekday_LastFriday(t *testing.T) {
	date := calcLastWeekday(time.Friday, time.December, 2026)
	want := time.Date(2026, time.December, 25, 0, 0, 0, 0, time.UTC)
	if !date.Equal(want) {
		t.Errorf("Last Friday Dec 2026: got %s, want %s", date.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func TestCalcFixedDate(t *testing.T) {
	date := calcDateFixed(time.January, 1, 2026)
	want := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	if !date.Equal(want) {
		t.Errorf("New Year 2026: got %s, want %s", date.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func TestNextOccurrence_FutureThisYear(t *testing.T) {
	now := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	birthday := time.Date(1990, time.December, 25, 0, 0, 0, 0, time.UTC)
	next := nextOccurrence(birthday, now)
	want := time.Date(2026, time.December, 25, 0, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("Next occurrence: got %s, want %s", next.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func TestNextOccurrence_PastThisYear(t *testing.T) {
	now := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	birthday := time.Date(1990, time.February, 14, 0, 0, 0, 0, time.UTC)
	next := nextOccurrence(birthday, now)
	want := time.Date(2027, time.February, 14, 0, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("Next occurrence (past): got %s, want %s", next.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func calcDateFixed(month time.Month, day, year int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func nextOccurrence(date, now time.Time) time.Time {
	currentYear := time.Date(now.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	if currentYear.Before(now) || currentYear.Equal(now) {
		return time.Date(now.Year()+1, date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	}
	return currentYear
}
