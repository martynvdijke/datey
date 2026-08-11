// Package ics parses RFC 5545 iCalendar files into the subset of event
// data Datey can import (summary, start date, all-day flag, yearly recurrence).
package ics

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	ical "github.com/arran4/golang-ical"
)

// MaxFileSize is the maximum accepted size of an .ics upload in bytes.
const MaxFileSize = 10 << 20 // 10 MB

// Event is a single VEVENT extracted from a calendar file.
type Event struct {
	// Summary is the VEVENT SUMMARY property ("" if absent).
	Summary string
	// Start is the event start. For all-day events only the date part is
	// meaningful (midnight in server-local time); timed events keep their
	// wall-clock time.
	Start time.Time
	// AllDay is true when DTSTART was a date-only (VALUE=DATE) value.
	AllDay bool
	// RecurYearly is true when the event carries a FREQ=YEARLY recurrence rule.
	RecurYearly bool
}

// Parse reads an .ics stream and returns the importable VEVENTs. VEVENTs
// without a usable DTSTART are skipped. An error is returned for malformed
// files and for input exceeding MaxFileSize.
func Parse(r io.Reader) ([]Event, error) {
	// Bound the input up front so oversized files are rejected before any
	// parsing work happens.
	data, err := io.ReadAll(io.LimitReader(r, MaxFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("read calendar: %w", err)
	}
	if len(data) > MaxFileSize {
		return nil, errors.New("file exceeds maximum size of 10 MB")
	}

	calendar, err := ical.ParseCalendar(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse calendar: %w", err)
	}

	var events []Event
	for _, vevent := range calendar.Events() {
		ev, ok := extractEvent(vevent)
		if !ok {
			continue
		}
		events = append(events, ev)
	}
	return events, nil
}

// extractEvent converts one VEVENT, reporting ok=false when it has no usable
// DTSTART. All-day vs timed is determined by the DTSTART VALUE=DATE parameter
// (GetAllDayStartAt alone is not reliable: it returns the date part of any
// value rather than failing on timed values).
func extractEvent(vevent *ical.VEvent) (Event, bool) {
	var ev Event

	if prop := vevent.GetProperty(ical.ComponentPropertySummary); prop != nil {
		ev.Summary = strings.TrimSpace(prop.Value)
	}

	startProp := vevent.GetProperty(ical.ComponentPropertyDtStart)
	if startProp == nil {
		return Event{}, false
	}
	allDay := false
	if vals, ok := startProp.ICalParameters["VALUE"]; ok && len(vals) > 0 && strings.EqualFold(vals[0], "DATE") {
		allDay = true
	}

	var start time.Time
	var err error
	if allDay {
		start, err = vevent.GetAllDayStartAt()
	} else {
		start, err = vevent.GetStartAt()
	}
	if err != nil {
		return Event{}, false
	}

	ev.AllDay = allDay
	if allDay {
		// Date-only values are midnight in some location; normalize to UTC
		// so downstream code only ever relies on the calendar date.
		start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	}
	ev.Start = start

	if rrules, err := vevent.GetRRules(); err == nil {
		for _, rr := range rrules {
			if rr.Freq == ical.FrequencyYearly {
				ev.RecurYearly = true
				break
			}
		}
	}

	return ev, true
}
