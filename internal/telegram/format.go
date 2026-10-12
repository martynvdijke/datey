package telegram

import (
	"fmt"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/milestone"
)

// ownerName resolves the owner display name for an event.
func ownerName(e *ent.Event) string {
	if e == nil {
		return "Unknown"
	}
	if e.Edges.Person != nil && e.Edges.Person.Name != "" {
		return e.Edges.Person.Name
	}
	if e.Edges.Group != nil && e.Edges.Group.Name != "" {
		return e.Edges.Group.Name
	}
	if e.Edges.Contact != nil && e.Edges.Contact.Name != "" {
		return e.Edges.Contact.Name
	}
	return "Unknown"
}

// relativeLabel returns "Today", "Tomorrow", or "In N days" relative to todayStart.
func relativeLabel(occDate, todayStart time.Time) string {
	// Compare date-only in UTC.
	occDay := time.Date(occDate.Year(), occDate.Month(), occDate.Day(), 0, 0, 0, 0, time.UTC)
	todayDay := time.Date(todayStart.Year(), todayStart.Month(), todayStart.Day(), 0, 0, 0, 0, time.UTC)
	diff := int(occDay.Sub(todayDay).Hours() / 24)
	switch diff {
	case 0:
		return "Today"
	case 1:
		return "Tomorrow"
	default:
		if diff < 0 {
			return fmt.Sprintf("%d days ago", -diff)
		}
		return fmt.Sprintf("In %d days", diff)
	}
}

func formatDate(d time.Time) string {
	return d.Format("2006-01-02")
}

// formatDetail builds the owner-independent part of a line:
// {type} · {relative} ({date}) plus age and milestone annotations.
func formatDetail(e *ent.Event, occDate, todayStart time.Time) string {
	rel := relativeLabel(occDate, todayStart)
	dateStr := formatDate(occDate)
	s := fmt.Sprintf("%s · %s (%s)", e.Type, rel, dateStr)

	// Age for birthdays (skip future-dated birth dates).
	if e.Type == "birthday" && e.Date.Year() > 1 {
		years := occDate.Year() - e.Date.Year()
		if years >= 0 && !occDate.Before(e.Date) {
			s += fmt.Sprintf(" (turns %d)", years)
		}
	}

	// Milestone label.
	if ok, label := milestone.IsMilestone(e.Type, e.Date, occDate); ok && label != "" {
		s += fmt.Sprintf(" 🎉 %s", label)
	}

	return s
}

// formatLine builds one line: • {owner} — {detail}.
func formatLine(e *ent.Event, occDate, todayStart time.Time) string {
	return fmt.Sprintf("• %s — %s", ownerName(e), formatDetail(e, occDate, todayStart))
}
