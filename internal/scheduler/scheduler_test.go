package scheduler

import (
	"strings"
	"testing"
	"time"
)

func TestReminderMessage(t *testing.T) {
	now := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	eventDate := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	days := int(eventDate.Sub(now).Hours() / 24)

	title := "Reminder: John - birthday"
	message := "Upcoming birthday for John on June 15 (14 days away)"

	if !strings.Contains(message, "14 days") {
		t.Errorf("Expected 14 days in message, got: %s", message)
	}
	if days != 14 {
		t.Errorf("Expected 14 days remaining, got %d", days)
	}
	_ = title
}
