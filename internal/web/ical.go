package web

import (
	"crypto/subtle"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/age"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/ical"
	recurrence "github.com/datey/datey/internal/recurring"
	"github.com/datey/datey/internal/tzutil"
)

const (
	// scheduleHorizonYears bounds how far ahead monthly, weekly and
	// every-N-days series are materialized into RDATE instances.
	scheduleHorizonYears = 5

	// maxScheduleDates caps the RDATE instances of a single event so a daily
	// or weekly series cannot bloat the feed.
	maxScheduleDates = 250
)

// feedAccessOK reports whether the public iCal feed is enabled and the
// request carries the correct secret key. Failures are indistinguishable
// (404) so the endpoint's existence is not revealed to callers.
func (h *Handler) feedAccessOK(r *http.Request) bool {
	if !h.cfg.ICalEnabled || h.cfg.ICalFeedKey == "" {
		return false
	}
	given := r.URL.Query().Get("key")
	return subtle.ConstantTimeCompare([]byte(given), []byte(h.cfg.ICalFeedKey)) == 1
}

func (h *Handler) writeICal(w http.ResponseWriter, r *http.Request, events []ical.Event) {
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if _, err := io.WriteString(w, ical.Calendar(events, time.Now())); err != nil {
		slog.Error("ical: write feed", "error", err)
	}
}

// icalFeedGlobal serves every tracked date across all people as iCal events.
// Rule-based calendar events travel with the regular event list: they are real
// events now, so the feed must not emit them a second time.
func (h *Handler) icalFeedGlobal(w http.ResponseWriter, r *http.Request) {
	if !h.feedAccessOK(r) {
		h.renderError(w, r, http.StatusNotFound)
		return
	}

	events, err := h.events.List(r.Context())
	if err != nil {
		slog.Error("ical: list events", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	feed := make([]ical.Event, 0, len(events))
	for _, e := range events {
		feed = append(feed, h.feedEvent(e, r.Host))
	}

	h.writeICal(w, r, feed)
}

// icalFeedPerson serves the dates belonging to a single person.
func (h *Handler) icalFeedPerson(w http.ResponseWriter, r *http.Request) {
	if !h.feedAccessOK(r) {
		h.renderError(w, r, http.StatusNotFound)
		return
	}

	personID, err := strconv.Atoi(r.PathValue("personID"))
	if err != nil {
		h.renderError(w, r, http.StatusNotFound)
		return
	}

	if _, err := h.people.Get(r.Context(), personID); err != nil {
		h.renderError(w, r, http.StatusNotFound)
		return
	}

	events, err := h.events.ListByPerson(r.Context(), personID)
	if err != nil {
		slog.Error("ical: list person events", "error", err, "person_id", personID)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	feed := make([]ical.Event, 0, len(events))
	for _, e := range events {
		feed = append(feed, h.feedEvent(e, r.Host))
	}

	h.writeICal(w, r, feed)
}

// feedEvent converts a stored event into an iCal feed event, applying the
// configured start time / duration (when set) and the event's real recurrence.
// For lunar, Hebrew and Hijri events the Gregorian occurrence for the current
// year is used as the DATE value (feeds emit Gregorian only).
func (h *Handler) feedEvent(e *ent.Event, host string) ical.Event {
	name := eventOwnerName(e)
	// Rule-based and other ownerless events are named by their description.
	if name == "" {
		name = e.Description
	}
	date := e.Date
	if e.CalendarSystem != "" && e.CalendarSystem != "gregorian" && e.LunarMonth != nil && e.LunarDay != nil {
		// Resolve the current year in the owner's zone so a lunar event that
		// crosses New Year for them is not pinned to the server's year.
		year := tzutil.Today(time.Now(), occurrenceLocation(e)).Year()
		for y := year; y <= year+1; y++ {
			if occ, err := h.lunarOccurrenceForYear(e, y); err == nil {
				date = occ
				break
			}
		}
	}
	description := e.Description
	if e.Type == "memorial" && e.Date.Year() > 1 {
		if years, ok := age.YearsSince(e.Date, date); ok {
			unit := "years"
			if years == 1 {
				unit = "year"
			}
			line := fmt.Sprintf("%d %s since", years, unit)
			if description == "" {
				description = line
			} else {
				description += "\n" + line
			}
		}
	}
	ev := ical.Event{
		UID:         fmt.Sprintf("datey-event-%d@%s", e.ID, host),
		Summary:     name + " - " + e.Type,
		Description: description,
		Date:        date,
		AllDay:      true,
	}

	if h.cfg.ICalEventStart != "" {
		if hour, minute, err := config.ParseClockTime(h.cfg.ICalEventStart); err == nil {
			ev.AllDay = false
			ev.StartHour = hour
			ev.StartMinute = minute
			ev.Duration = time.Duration(h.cfg.ICalDurationMinutes) * time.Minute
		}
	}

	applySchedule(&ev, e)
	return ev
}

// applySchedule maps the event's stored recurrence onto the VEVENT.
//
// A rule event (nth weekday of a month, or an Easter-based holiday) is annual
// by definition: its DTSTART is the rule's date in the anchor year. Nth-weekday
// rules map onto RRULE with BYMONTH/BYDAY; Easter-based dates move every year,
// so they are materialized as RDATE instances exactly like a schedule the RFC
// cannot express.
//
// A once event is a single VEVENT with no rule. An annual Gregorian series maps
// onto RRULE:FREQ=YEARLY, carrying the stored end_date / occurrence_count bound
// (the earlier bound wins, matching internal/recurring). Monthly, weekly and
// every-N-days series are materialized as RDATE instances: RFC 5545 expansion
// diverges from the engine (the engine clamps the 31st to month end, RFC skips
// short months), so a rule would publish dates the app does not show.
//
// Non-Gregorian calendars keep the previous behaviour: a yearly rule on the
// Gregorian occurrence converted for the feed.
func applySchedule(ev *ical.Event, e *ent.Event) {
	if recurrence.IsRuleEvent(e) {
		applyRuleSchedule(ev, e)
		return
	}
	freq, _ := recurrence.NormalizeFrequency(e.Frequency)
	if freq == recurrence.FrequencyOnce {
		return
	}
	if e.CalendarSystem != "" && e.CalendarSystem != "gregorian" {
		ev.RRule = "FREQ=YEARLY"
		return
	}
	if freq == recurrence.FrequencyAnnual {
		ev.RRule = annualRule(e, ev.AllDay)
		return
	}
	ev.ExtraDates = scheduleDates(e, ev.Date)
}

// weekdayCodes are the RFC 5545 BYDAY weekday abbreviations, indexed Sunday=0.
var weekdayCodes = [7]string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}

// applyRuleSchedule publishes a rule event. The rule is annual no matter what
// frequency is stored; the bound helpers work on the anchor year just like the
// annual path.
func applyRuleSchedule(ev *ical.Event, e *ent.Event) {
	anchorYear := e.Date.Year()
	if anchorYear <= 1 {
		anchorYear = time.Now().Year()
	}
	if d, ok := recurrence.RuleDateForYear(e, anchorYear); ok {
		ev.Date = d
	}

	if recurrence.NormalizeRuleKind(e.RuleKind) == recurrence.NthWeekdayRule {
		ordinal := 5
		if e.RuleOrdinal != nil && *e.RuleOrdinal >= 1 && *e.RuleOrdinal <= 5 {
			ordinal = *e.RuleOrdinal
		}
		weekday := 0
		if e.RuleWeekday != nil && *e.RuleWeekday >= 0 && *e.RuleWeekday <= 6 {
			weekday = *e.RuleWeekday
		}
		base := fmt.Sprintf("FREQ=YEARLY;BYMONTH=%d;BYDAY=%s%s",
			int(e.Date.Month()), byDayOrdinal(ordinal), weekdayCodes[weekday])
		ev.RRule = annualRuleBase(e, ev.AllDay, base)
		return
	}

	// Easter-based kinds: the date moves every year, so publish the computed
	// occurrences as RDATE instances.
	ev.ExtraDates = scheduleDates(e, ev.Date)
}

// byDayOrdinal renders the BYDAY ordinal prefix: the fifth occurrence of a
// weekday is the last one in the month, which RFC 5545 spells -1.
func byDayOrdinal(ordinal int) string {
	if ordinal >= 5 {
		return "-1"
	}
	return strconv.Itoa(ordinal)
}

// annualRule renders the RRULE of an annual Gregorian series. When both bounds
// are stored the earlier one is kept, mirroring the engine's rule that the
// earliest bound wins.
func annualRule(e *ent.Event, allDay bool) string {
	return annualRuleBase(e, allDay, "FREQ=YEARLY")
}

// annualRuleBase renders an annual RRULE on top of base (for example
// "FREQ=YEARLY;BYMONTH=5;BYDAY=2SU"), carrying the earlier of end_date and
// occurrence_count as COUNT / UNTIL.
func annualRuleBase(e *ent.Event, allDay bool, base string) string {
	hasCount := e.OccurrenceCount != nil && *e.OccurrenceCount >= 1
	hasEnd := e.EndDate != nil
	if hasCount && hasEnd {
		// Keep whichever bound ends the series first.
		if last, ok := nthAnnualOccurrence(e, *e.OccurrenceCount); ok && last.Before(feedDay(*e.EndDate)) {
			hasEnd = false
		} else {
			hasCount = false
		}
	}
	switch {
	case hasCount:
		return base + ";COUNT=" + strconv.Itoa(*e.OccurrenceCount)
	case hasEnd:
		// RFC 5545 requires UNTIL to carry the same value type as DTSTART.
		if allDay {
			return base + ";UNTIL=" + feedDay(*e.EndDate).Format("20060102")
		}
		return base + ";UNTIL=" + feedDay(*e.EndDate).Format("20060102") + "T235959Z"
	default:
		return base
	}
}

// nthAnnualOccurrence returns the nth (1-based, anchor inclusive) occurrence of
// an annual series, which is the last allowed instance under occurrence_count.
func nthAnnualOccurrence(e *ent.Event, n int) (time.Time, bool) {
	occs, err := recurrence.OccurrencesForYear(e, e.Date.Year()+n-1)
	if err != nil || len(occs) == 0 {
		return time.Time{}, false
	}
	return occs[len(occs)-1], true
}

// scheduleDates materializes the real occurrences of a monthly, weekly or
// every-N-days series from its anchor through the earlier of end_date and
// anchor+5 years, capped at maxScheduleDates. The anchor itself is skipped: it
// is already the VEVENT's DTSTART.
func scheduleDates(e *ent.Event, dtstart time.Time) []time.Time {
	from := feedDay(e.Date)
	to := from.AddDate(scheduleHorizonYears, 0, 0)
	if e.EndDate != nil {
		if end := feedDay(*e.EndDate); end.Before(to) {
			to = end
		}
	}
	occs := recurrence.OccurrencesInForEvent(e, from, to)
	out := make([]time.Time, 0, len(occs))
	for _, occ := range occs {
		if len(out) >= maxScheduleDates {
			break
		}
		if occ.Equal(feedDay(dtstart)) {
			continue
		}
		out = append(out, occ)
	}
	return out
}

// feedDay truncates t to midnight UTC: feed occurrences are calendar days.
func feedDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
