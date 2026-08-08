// Package ical renders RFC 5545 iCalendar documents for the public iCal feed.
//
// The generator is intentionally minimal: it emits a VCALENDAR with one
// VEVENT per event, supports all-day and timed events, optional yearly
// recurrence, and RFC 5545 text escaping and line folding. No external
// dependency is required.
package ical

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// maxLineOctets is the RFC 5545 content-line limit, excluding the CRLF.
	maxLineOctets = 75

	prodid = "-//Datey//Datey iCal Feed//EN"
)

// Event describes a single VEVENT to render.
type Event struct {
	// UID is a stable, globally unique identifier for the event (e.g.
	// "datey-42@example.com"). It must not change between requests so that
	// calendar clients do not treat refetched events as new ones.
	UID string

	// Summary is the human-readable title (the TEXT property value).
	Summary string

	// Description is the optional event description; empty means omitted.
	Description string

	// Date is the calendar date (and, for timed events, the reference day)
	// of the event.
	Date time.Time

	// AllDay renders the event as a DATE-only value with no time and no
	// DTEND. When false, StartHour/StartMinute/Duration are used.
	AllDay bool

	// StartHour/StartMinute are the floating local start time used when
	// AllDay is false.
	StartHour   int
	StartMinute int

	// Duration is the event length used to compute DTEND when timed.
	Duration time.Duration

	// RecurYearly emits RRULE:FREQ=YEARLY so annual dates (birthdays,
	// anniversaries, ...) repeat every year in subscribed calendars.
	RecurYearly bool
}

// Calendar renders a complete VCALENDAR document containing the given events.
// dtstamp is the document creation time, rendered in UTC.
func Calendar(events []Event, dtstamp time.Time) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:" + prodid + "\r\n")
	b.WriteString("CALSCALE:GREGORIAN\r\n")
	for _, e := range events {
		writeVEVENT(&b, e, dtstamp)
	}
	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

func writeVEVENT(b *strings.Builder, e Event, dtstamp time.Time) {
	b.WriteString("BEGIN:VEVENT\r\n")
	writeProperty(b, "UID", e.UID)
	writeProperty(b, "DTSTAMP", dtstamp.UTC().Format("20060102T150405Z"))
	if e.AllDay {
		writeProperty(b, "DTSTART;VALUE=DATE", e.Date.Format("20060102"))
	} else {
		start := time.Date(e.Date.Year(), e.Date.Month(), e.Date.Day(),
			e.StartHour, e.StartMinute, 0, 0, e.Date.Location())
		end := start.Add(e.Duration)
		writeProperty(b, "DTSTART", start.Format("20060102T150405"))
		writeProperty(b, "DTEND", end.Format("20060102T150405"))
	}
	writeProperty(b, "SUMMARY", escapeText(e.Summary))
	if e.Description != "" {
		writeProperty(b, "DESCRIPTION", escapeText(e.Description))
	}
	if e.RecurYearly {
		writeProperty(b, "RRULE", "FREQ=YEARLY")
	}
	b.WriteString("END:VEVENT\r\n")
}

// writeProperty writes a single content line, folding it onto continuation
// lines when it exceeds the RFC 5545 75-octet limit.
func writeProperty(b *strings.Builder, name, value string) {
	segs := fold(name + ":" + value)
	b.WriteString(segs[0])
	b.WriteString("\r\n")
	for _, s := range segs[1:] {
		b.WriteString(" ")
		b.WriteString(s)
		b.WriteString("\r\n")
	}
}

// fold breaks a content line into segments of at most 75 octets. The first
// segment carries the full budget; continuation segments are limited to 74
// octets because the caller prefixes them with a single space (which counts
// toward the 75-octet limit). Segments never split a UTF-8 rune.
func fold(line string) []string {
	if line == "" {
		return []string{""}
	}
	var segs []string
	max := maxLineOctets
	for len(line) > 0 {
		if len(line) <= max {
			segs = append(segs, line)
			break
		}
		cut := max
		for cut > 0 && !utf8.RuneStart(line[cut]) {
			cut--
		}
		if cut == 0 {
			cut = max // no rune boundary found within budget; hard cut
		}
		segs = append(segs, line[:cut])
		line = line[cut:]
		max = maxLineOctets - 1
	}
	return segs
}

// escapeText escapes a TEXT property value per RFC 5545 §3.3.11: backslash,
// semicolon, and comma are backslash-escaped, and line breaks are encoded as
// the two characters "\n".
func escapeText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case ';':
			b.WriteString(`\;`)
		case ',':
			b.WriteString(`\,`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			// Line breaks are encoded via '\n'; ignore bare CR.
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
