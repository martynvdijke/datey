package milestone

import (
	"fmt"
	"time"
)

// IsMilestone reports whether the occurrence on occurrenceDate derived from
// eventDate of the given eventType is a cultural milestone.
// Yearless events (year <=1) return false. Future-dated events (negative age) also return false.
// Label is e.g. "30th birthday" or "25th anniversary (Silver)".
func IsMilestone(eventType string, eventDate, occurrenceDate time.Time) (bool, string) {
	return IsMilestoneWithOptions(eventType, eventDate, occurrenceDate, false)
}

// IsMilestoneWithOptions is like IsMilestone but includes optional 10_000 day check when includeDays is true.
func IsMilestoneWithOptions(eventType string, eventDate, occurrenceDate time.Time, includeDays bool) (bool, string) {
	if eventDate.Year() <= 1 {
		return false, ""
	}
	if occurrenceDate.Before(eventDate) {
		return false, ""
	}
	switch eventType {
	case "birthday":
		years := occurrenceDate.Year() - eventDate.Year()
		if years < 0 {
			return false, ""
		}
		if isBirthdayMilestone(years) {
			return true, fmt.Sprintf("%s birthday", ordinal(years))
		}
		if includeDays {
			if ok, label := checkDaysMilestone(eventDate, occurrenceDate); ok {
				return true, label
			}
		}
		return false, ""
	case "anniversary", "wedding":
		years := occurrenceDate.Year() - eventDate.Year()
		if years <= 0 {
			return false, ""
		}
		if isAnniversaryMilestone(years) {
			lbl := fmt.Sprintf("%s anniversary", ordinal(years))
			switch years {
			case 25:
				lbl += " (Silver)"
			case 50:
				lbl += " (Golden)"
			case 60:
				lbl += " (Diamond)"
			}
			return true, lbl
		}
		if includeDays {
			if ok, label := checkDaysMilestone(eventDate, occurrenceDate); ok {
				return true, label
			}
		}
		return false, ""
	default:
		if includeDays {
			if ok, label := checkDaysMilestone(eventDate, occurrenceDate); ok {
				return true, label
			}
		}
		return false, ""
	}
}

func isBirthdayMilestone(age int) bool {
	switch age {
	case 10, 18, 20, 21, 30, 40, 50, 60, 70, 80, 90, 100:
		return true
	}
	return false
}

func isAnniversaryMilestone(years int) bool {
	switch years {
	case 10, 25, 50, 60:
		return true
	}
	return false
}

func checkDaysMilestone(eventDate, occurrenceDate time.Time) (bool, string) {
	// Normalize to date only midnight UTC
	a := time.Date(eventDate.Year(), eventDate.Month(), eventDate.Day(), 0, 0, 0, 0, time.UTC)
	b := time.Date(occurrenceDate.Year(), occurrenceDate.Month(), occurrenceDate.Day(), 0, 0, 0, 0, time.UTC)
	days := int(b.Sub(a).Hours() / 24)
	if days <= 0 {
		return false, ""
	}
	if days%10000 == 0 {
		return true, fmt.Sprintf("%d days", days)
	}
	return false, ""
}

func ordinal(n int) string {
	if n%100 >= 11 && n%100 <= 13 {
		return fmt.Sprintf("%dth", n)
	}
	switch n % 10 {
	case 1:
		return fmt.Sprintf("%dst", n)
	case 2:
		return fmt.Sprintf("%dnd", n)
	case 3:
		return fmt.Sprintf("%drd", n)
	}
	return fmt.Sprintf("%dth", n)
}
