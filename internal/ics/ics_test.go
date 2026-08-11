package ics

import (
	"strings"
	"testing"
	"time"
)

const calendarHead = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Datey//Test//EN\r\n"

func parseOne(t *testing.T, body string) []Event {
	t.Helper()
	events, err := Parse(strings.NewReader(calendarHead + body + "END:VCALENDAR\r\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return events
}

func TestParse_AllDayEvent(t *testing.T) {
	events := parseOne(t, "BEGIN:VEVENT\r\nSUMMARY:Birthday party\r\nDTSTART;VALUE=DATE:20260812\r\nEND:VEVENT\r\n")
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	ev := events[0]
	if !ev.AllDay {
		t.Errorf("want AllDay=true, got false")
	}
	if ev.Start.Year() != 2026 || ev.Start.Month() != time.August || ev.Start.Day() != 12 {
		t.Errorf("want start 2026-08-12, got %v", ev.Start)
	}
	if ev.Summary != "Birthday party" {
		t.Errorf("want summary %q, got %q", "Birthday party", ev.Summary)
	}
	if ev.RecurYearly {
		t.Errorf("want RecurYearly=false for non-recurring event")
	}
}

func TestParse_TimedEvent(t *testing.T) {
	events := parseOne(t, "BEGIN:VEVENT\r\nSUMMARY:Doctor visit\r\nDTSTART:20260812T093000Z\r\nEND:VEVENT\r\n")
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.AllDay {
		t.Errorf("want AllDay=false for timed event")
	}
	want := time.Date(2026, time.August, 12, 9, 30, 0, 0, time.UTC)
	if !ev.Start.Equal(want) {
		t.Errorf("want start %v, got %v", want, ev.Start)
	}
}

func TestParse_YearlyRecurrence(t *testing.T) {
	events := parseOne(t, "BEGIN:VEVENT\r\nSUMMARY:Anniversary\r\nDTSTART;VALUE=DATE:20150620\r\nRRULE:FREQ=YEARLY;INTERVAL=1\r\nEND:VEVENT\r\n")
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if !events[0].RecurYearly {
		t.Errorf("want RecurYearly=true for FREQ=YEARLY")
	}
	if !events[0].AllDay {
		t.Errorf("want AllDay=true for date-only DTSTART")
	}
}

func TestParse_NonYearlyRecurrence(t *testing.T) {
	events := parseOne(t, "BEGIN:VEVENT\r\nSUMMARY:Weekly standup\r\nDTSTART;VALUE=DATE:20260810\r\nRRULE:FREQ=WEEKLY;BYDAY=MO\r\nEND:VEVENT\r\n")
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if events[0].RecurYearly {
		t.Errorf("want RecurYearly=false for FREQ=WEEKLY")
	}
}

func TestParse_FoldedLines(t *testing.T) {
	// RFC 5545 line folding: a long line is continued with CRLF + one space.
	summary := "This is a very long event summary that will definitely need to be folded across multiple physical lines"
	var folded strings.Builder
	folded.WriteString("BEGIN:VEVENT\r\n")
	line := "SUMMARY:" + summary
	for len(line) > 75 {
		folded.WriteString(line[:75] + "\r\n ")
		line = line[75:]
	}
	folded.WriteString(line + "\r\n")
	folded.WriteString("DTSTART;VALUE=DATE:20260812\r\nEND:VEVENT\r\n")

	events := parseOne(t, folded.String())
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if events[0].Summary != summary {
		t.Errorf("folded summary mismatch:\nwant %q\ngot  %q", summary, events[0].Summary)
	}
}

func TestParse_SkipsEventsWithoutStart(t *testing.T) {
	events := parseOne(t, "BEGIN:VEVENT\r\nSUMMARY:No date here\r\nEND:VEVENT\r\n")
	if len(events) != 0 {
		t.Errorf("want 0 events, got %d", len(events))
	}
}

func TestParse_InvalidInput(t *testing.T) {
	if _, err := Parse(strings.NewReader("this is not an iCalendar file at all")); err == nil {
		t.Errorf("want error for invalid input, got nil")
	}
}

func TestParse_EmptySummary(t *testing.T) {
	events := parseOne(t, "BEGIN:VEVENT\r\nDTSTART;VALUE=DATE:20260812\r\nEND:VEVENT\r\n")
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if events[0].Summary != "" {
		t.Errorf("want empty summary, got %q", events[0].Summary)
	}
}

func TestParse_OversizedInput(t *testing.T) {
	// One byte over the limit must be rejected before parsing.
	big := strings.NewReader(strings.Repeat("x", MaxFileSize+1))
	if _, err := Parse(big); err == nil {
		t.Errorf("want error for oversized input, got nil")
	}
}

func TestParse_MultipleEvents(t *testing.T) {
	body := "BEGIN:VEVENT\r\nSUMMARY:First\r\nDTSTART;VALUE=DATE:20260801\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nSUMMARY:Second\r\nDTSTART;VALUE=DATE:20260901\r\nEND:VEVENT\r\n"
	events := parseOne(t, body)
	if len(events) != 2 {
		t.Fatalf("want 2 events, got %d", len(events))
	}
	if events[0].Summary != "First" || events[1].Summary != "Second" {
		t.Errorf("unexpected summaries: %q, %q", events[0].Summary, events[1].Summary)
	}
}
