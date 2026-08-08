package ical

import (
	"strings"
	"testing"
	"time"
)

func fixedTime() time.Time {
	return time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
}

func TestCalendarStructure(t *testing.T) {
	out := Calendar(nil, fixedTime())
	if !strings.HasPrefix(out, "BEGIN:VCALENDAR\r\n") {
		t.Errorf("expected document to start with BEGIN:VCALENDAR, got %q", out)
	}
	if !strings.HasSuffix(out, "END:VCALENDAR\r\n") {
		t.Errorf("expected document to end with END:VCALENDAR, got %q", out)
	}
	for _, want := range []string{"VERSION:2.0", "PRODID:-//Datey//Datey iCal Feed//EN"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output", want)
		}
	}
}

func TestAllDayEvent(t *testing.T) {
	out := Calendar([]Event{{
		UID:     "datey-1@example.com",
		Summary: "Anna - birthday",
		Date:    fixedTime(),
		AllDay:  true,
	}}, fixedTime())

	if !strings.Contains(out, "DTSTART;VALUE=DATE:20260815") {
		t.Errorf("expected all-day DTSTART;VALUE=DATE:20260815, got:\n%s", out)
	}
	if strings.Contains(out, "DTEND") {
		t.Errorf("all-day event must not emit DTEND, got:\n%s", out)
	}
	if strings.Contains(out, "RRULE") {
		t.Errorf("non-recurring event must not emit RRULE, got:\n%s", out)
	}
	if !strings.Contains(out, "SUMMARY:Anna - birthday") {
		t.Errorf("expected summary, got:\n%s", out)
	}
	if !strings.Contains(out, "UID:datey-1@example.com") {
		t.Errorf("expected UID, got:\n%s", out)
	}
}

func TestTimedEvent(t *testing.T) {
	out := Calendar([]Event{{
		UID:         "datey-2@example.com",
		Summary:     "Anna - birthday",
		Date:        fixedTime(),
		AllDay:      false,
		StartHour:   9,
		StartMinute: 0,
		Duration:    60 * time.Minute,
	}}, fixedTime())

	if !strings.Contains(out, "DTSTART:20260815T090000") {
		t.Errorf("expected timed DTSTART:20260815T090000, got:\n%s", out)
	}
	if !strings.Contains(out, "DTEND:20260815T100000") {
		t.Errorf("expected DTEND:20260815T100000 (start+duration), got:\n%s", out)
	}
	if strings.Contains(out, "VALUE=DATE") {
		t.Errorf("timed event must not use VALUE=DATE, got:\n%s", out)
	}
}

func TestYearlyRecurrence(t *testing.T) {
	recurring := Calendar([]Event{{
		UID:         "datey-3@example.com",
		Summary:     "Anna - birthday",
		Date:        fixedTime(),
		AllDay:      true,
		RecurYearly: true,
	}}, fixedTime())
	if !strings.Contains(recurring, "RRULE:FREQ=YEARLY") {
		t.Errorf("expected RRULE:FREQ=YEARLY, got:\n%s", recurring)
	}

	oneOff := Calendar([]Event{{
		UID:     "datey-4@example.com",
		Summary: "Anna - meeting",
		Date:    fixedTime(),
		AllDay:  true,
	}}, fixedTime())
	if strings.Contains(oneOff, "RRULE") {
		t.Errorf("one-off event must not emit RRULE, got:\n%s", oneOff)
	}
}

func TestDescriptionIncluded(t *testing.T) {
	out := Calendar([]Event{{
		UID:         "datey-5@example.com",
		Summary:     "Anna - birthday",
		Description: "Her 40th",
		Date:        fixedTime(),
		AllDay:      true,
	}}, fixedTime())
	if !strings.Contains(out, "DESCRIPTION:Her 40th") {
		t.Errorf("expected DESCRIPTION, got:\n%s", out)
	}

	noDesc := Calendar([]Event{{
		UID:     "datey-6@example.com",
		Summary: "Anna - birthday",
		Date:    fixedTime(),
		AllDay:  true,
	}}, fixedTime())
	if strings.Contains(noDesc, "DESCRIPTION") {
		t.Errorf("empty description must be omitted, got:\n%s", noDesc)
	}
}

func TestTextEscaping(t *testing.T) {
	out := Calendar([]Event{{
		UID:         "datey-7@example.com",
		Summary:     `O'Brien, Jr. ; VIP`,
		Description: "Line one\nLine two",
		Date:        fixedTime(),
		AllDay:      true,
	}}, fixedTime())
	if !strings.Contains(out, `SUMMARY:O'Brien\, Jr. \; VIP`) {
		t.Errorf("expected escaped summary, got:\n%s", out)
	}
	if !strings.Contains(out, "DESCRIPTION:Line one\\nLine two") {
		t.Errorf("expected newline escaped as \\n, got:\n%s", out)
	}
}

func TestLineFolding(t *testing.T) {
	longSummary := strings.Repeat("x", 200)
	out := Calendar([]Event{{
		UID:     "datey-8@example.com",
		Summary: longSummary,
		Date:    fixedTime(),
		AllDay:  true,
	}}, fixedTime())

	// Every folded line (up to the CRLF) must be at most 75 octets.
	for _, line := range strings.Split(out, "\r\n") {
		if line == "" {
			continue
		}
		if len(line) > maxLineOctets {
			t.Errorf("content line exceeds %d octets (%d): %q", maxLineOctets, len(line), line)
		}
	}
	// The first fold segment is exactly 75 octets including the property name.
	if !strings.Contains(out, "SUMMARY:"+strings.Repeat("x", 67)+"\r\n x") {
		t.Errorf("expected first fold segment of 75 octets followed by a continuation, got:\n%s", out)
	}
}

func TestDTSTAMPIsUTC(t *testing.T) {
	out := Calendar([]Event{{
		UID:     "datey-9@example.com",
		Summary: "Anna - birthday",
		Date:    fixedTime(),
		AllDay:  true,
	}}, time.Date(2026, 8, 8, 12, 30, 45, 0, time.FixedZone("UTC+2", 2*3600)))
	if !strings.Contains(out, "DTSTAMP:20260808T103045Z") {
		t.Errorf("expected UTC DTSTAMP with Z suffix, got:\n%s", out)
	}
}

func TestMultipleEvents(t *testing.T) {
	out := Calendar([]Event{
		{UID: "datey-a@example.com", Summary: "A", Date: fixedTime(), AllDay: true},
		{UID: "datey-b@example.com", Summary: "B", Date: fixedTime(), AllDay: true},
	}, fixedTime())
	if strings.Count(out, "BEGIN:VEVENT") != 2 || strings.Count(out, "END:VEVENT") != 2 {
		t.Errorf("expected exactly 2 VEVENT blocks, got:\n%s", out)
	}
}

func TestFoldUnicodeBoundary(t *testing.T) {
	// A rune that spans the fold boundary must not be split mid-rune.
	s := strings.Repeat("é", 40) // 2 octets per rune → 80 octets
	segs := fold("SUMMARY:" + s)
	for _, seg := range segs {
		if len(seg) > maxLineOctets {
			t.Errorf("segment exceeds %d octets: %d (%q)", maxLineOctets, len(seg), seg)
		}
	}
	joined := strings.Join(segs, "")
	if joined != "SUMMARY:"+s {
		t.Errorf("fold changed content:\n got %q\nwant %q", joined, "SUMMARY:"+s)
	}
}
