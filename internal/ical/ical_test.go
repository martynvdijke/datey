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
		UID:     "datey-3@example.com",
		Summary: "Anna - birthday",
		Date:    fixedTime(),
		AllDay:  true,
		RRule:   "FREQ=YEARLY",
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

func TestRRuleBoundsAreEmittedVerbatim(t *testing.T) {
	until := Calendar([]Event{{
		UID:    "datey-10@example.com",
		Date:   fixedTime(),
		AllDay: true,
		RRule:  "FREQ=YEARLY;UNTIL=20301231T235959Z",
	}}, fixedTime())
	if !strings.Contains(until, "RRULE:FREQ=YEARLY;UNTIL=20301231T235959Z") {
		t.Errorf("expected UNTIL bound, got:\n%s", until)
	}

	count := Calendar([]Event{{
		UID:    "datey-11@example.com",
		Date:   fixedTime(),
		AllDay: true,
		RRule:  "FREQ=YEARLY;COUNT=10",
	}}, fixedTime())
	if !strings.Contains(count, "RRULE:FREQ=YEARLY;COUNT=10") {
		t.Errorf("expected COUNT bound, got:\n%s", count)
	}
}

func TestRDATEInstances(t *testing.T) {
	out := Calendar([]Event{{
		UID:     "datey-12@example.com",
		Summary: "Anna - rent",
		Date:    time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		AllDay:  true,
		ExtraDates: []time.Time{
			time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
		},
	}}, fixedTime())

	for _, want := range []string{
		"DTSTART;VALUE=DATE:20260131",
		"RDATE;VALUE=DATE:20260228",
		"RDATE;VALUE=DATE:20260331",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "RRULE") {
		t.Errorf("materialized series must not carry an RRULE, got:\n%s", out)
	}
	if strings.Count(out, "RDATE") != 2 {
		t.Errorf("expected exactly 2 RDATE lines, got:\n%s", out)
	}
}

func TestRDATETimedEventUsesFloatingDateTime(t *testing.T) {
	out := Calendar([]Event{{
		UID:         "datey-13@example.com",
		Date:        time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		AllDay:      false,
		StartHour:   9,
		StartMinute: 30,
		ExtraDates:  []time.Time{time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)},
	}}, fixedTime())

	if !strings.Contains(out, "RDATE:20260202T093000") {
		t.Errorf("expected floating DATE-TIME RDATE, got:\n%s", out)
	}
	if strings.Contains(out, "RDATE;VALUE=DATE") {
		t.Errorf("timed event must not emit DATE-valued RDATE, got:\n%s", out)
	}
}

func TestRDATESFolding(t *testing.T) {
	// Many instances still fold to the 75-octet content-line limit.
	ev := Event{UID: "datey-14@example.com", Date: fixedTime(), AllDay: true}
	for i := 0; i < 30; i++ {
		ev.ExtraDates = append(ev.ExtraDates, fixedTime().AddDate(0, 0, i))
	}
	out := Calendar([]Event{ev}, fixedTime())
	for _, line := range strings.Split(out, "\r\n") {
		if len(line) > maxLineOctets {
			t.Errorf("content line exceeds %d octets (%d): %q", maxLineOctets, len(line), line)
		}
	}
}
