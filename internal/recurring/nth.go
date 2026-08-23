package recurring

import (
	"log/slog"
	"time"
)

const NthWeekdayRule = "nth_weekday"

// NthWeekdayOfMonth returns the date of the nth occurrence of weekday in
// month/year. Ordinal 1-4 means the nth occurrence, ordinal 5 means "last"
// occurrence (the final weekday of the month regardless of count). Month is
// 1-12, weekday 0-6 (Sunday=0). If the nth literal occurrence does not exist
// (e.g. 5th weekday treated literally, or nth>count) it returns ok=false. For
// ordinal 5 as "last" it always succeeds.
func NthWeekdayOfMonth(year, month, ordinal, weekday int) (time.Time, bool) {
	if month < 1 || month > 12 || ordinal < 1 || ordinal > 5 || weekday < 0 || weekday > 6 {
		return time.Time{}, false
	}
	m := time.Month(month)
	wd := time.Weekday(weekday)

	// ordinal 5 is "last" — find last occurrence of weekday in month.
	if ordinal == 5 {
		// last day of month
		lastDay := time.Date(year, m+1, 0, 0, 0, 0, 0, time.UTC)
		daysBack := (int(lastDay.Weekday()) - int(wd) + 7) % 7
		return lastDay.AddDate(0, 0, -daysBack), true
	}

	firstDay := time.Date(year, m, 1, 0, 0, 0, 0, time.UTC)
	daysUntil := (int(wd) - int(firstDay.Weekday()) + 7) % 7
	day := 1 + daysUntil + (ordinal-1)*7
	if day > daysInMonthInt(m, year) {
		slog.Info("nth_weekday: nonexistent ordinal, skipping year", "year", year, "month", month, "ordinal", ordinal, "weekday", weekday)
		return time.Time{}, false
	}
	return time.Date(year, m, day, 0, 0, 0, 0, time.UTC), true
}

func daysInMonthInt(month time.Month, year int) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
