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
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/ical"
)

// recurringFeedHorizonYears is how many years of global recurring rules
// (Mother's Day, ...) are materialized into the global feed.
const recurringFeedHorizonYears = 5

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

// icalFeedGlobal serves every tracked date across all people as iCal events,
// plus enabled global recurring rules materialized over the next five years.
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

	recurring, err := h.recurringFeedEvents(r)
	if err != nil {
		slog.Error("ical: list recurring rules", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}
	feed = append(feed, recurring...)

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
// configured start time / duration (when set) and yearly recurrence.
func (h *Handler) feedEvent(e *ent.Event, host string) ical.Event {
	name := ""
	if p := e.Edges.Person; p != nil {
		name = p.Name
	} else if c := e.Edges.Contact; c != nil {
		name = c.Name
	}

	ev := ical.Event{
		UID:         fmt.Sprintf("datey-event-%d@%s", e.ID, host),
		Summary:     name + " - " + e.Type,
		Description: e.Description,
		Date:        e.Date,
		AllDay:      true,
		RecurYearly: true, // every event recurs annually
	}

	if h.cfg.ICalEventStart != "" {
		if hour, minute, err := config.ParseClockTime(h.cfg.ICalEventStart); err == nil {
			ev.AllDay = false
			ev.StartHour = hour
			ev.StartMinute = minute
			ev.Duration = time.Duration(h.cfg.ICalDurationMinutes) * time.Minute
		}
	}
	return ev
}

// recurringFeedEvents materializes enabled global recurring rules (e.g.
// Mother's Day) for the next five years as all-day events.
func (h *Handler) recurringFeedEvents(r *http.Request) ([]ical.Event, error) {
	rules, err := h.recurringRules.List(r.Context())
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var out []ical.Event
	for _, rule := range rules {
		for y := now.Year(); y < now.Year()+recurringFeedHorizonYears; y++ {
			d := h.recurringRules.CalculateDate(rule, y)
			if d.IsZero() {
				continue
			}
			out = append(out, ical.Event{
				UID:     fmt.Sprintf("datey-rule-%d-%d@%s", rule.ID, y, r.Host),
				Summary: rule.Name,
				Date:    d,
				AllDay:  true,
			})
		}
	}
	return out, nil
}
