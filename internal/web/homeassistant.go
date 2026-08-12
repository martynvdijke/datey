package web

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/datey/datey/ent"
)

// homeAssistantEvent is one entry of the upcoming array in the Home Assistant
// stats feed. The "days" key maps onto json_attributes in the sensor YAML.
type homeAssistantEvent struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Date string `json:"date"`
	Days int    `json:"days"`
}

// homeAssistantStats is the flat JSON document served at
// GET /api/homeassistant/stats, shaped for a Home Assistant RESTful sensor
// (value_template + json_attributes).
type homeAssistantStats struct {
	NextEvent     string                `json:"next_event"`
	NextEventDate string                `json:"next_event_date"`
	NextEventDays int                   `json:"next_event_days"`
	EventsIn7Days int                   `json:"events_in_7_days"`
	EventsIn30Days int                  `json:"events_in_30_days"`
	Upcoming      []homeAssistantEvent `json:"upcoming"`
}

// homeAssistantAccessOK mirrors the iCal/RSS/API feed key semantics: the
// endpoint is only reachable when enabled with a configured key and a
// matching ?key= query parameter.
func (h *Handler) homeAssistantAccessOK(r *http.Request) bool {
	if !h.cfg.HomeAssistantEnabled || h.cfg.HomeAssistantKey == "" {
		return false
	}
	got := r.URL.Query().Get("key")
	return subtle.ConstantTimeCompare([]byte(got), []byte(h.cfg.HomeAssistantKey)) == 1
}

// homeAssistantStats serves GET /api/homeassistant/stats for the Home
// Assistant RESTful sensor plugin.
func (h *Handler) homeAssistantStats(w http.ResponseWriter, r *http.Request) {
	if !h.homeAssistantAccessOK(r) {
		h.renderError(w, r, http.StatusNotFound)
		return
	}

	now := time.Now()

	// The 7/30-day counts need a 30-day horizon; the next event and the
	// upcoming list use the configured reminder window. Query once with the
	// wider horizon, mirroring the TRMNL stats feed.
	horizon := h.cfg.ReminderDays
	if horizon < 30 {
		horizon = 30
	}
	windowEnd := now.AddDate(0, 0, h.cfg.ReminderDays)

	occurrences, err := h.events.ListUpcomingOccurrences(r.Context(), now, now.AddDate(0, 0, horizon))
	if err != nil {
		slog.Error("home assistant stats: list upcoming", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	upcoming := make([]homeAssistantEvent, 0, len(occurrences))
	var next *homeAssistantEvent
	nextDate := ""
	nextDays := 0
	var events7, events30 int
	for _, occ := range occurrences {
		e := occ.Event
		ev := homeAssistantEventFromEvent(e, occ.Date, now)
		if ev.Days <= 7 {
			events7++
		}
		if ev.Days <= 30 {
			events30++
		}
		// Events beyond the reminder window still count toward the summary
		// counts but are excluded from the display list.
		if occ.Date.After(windowEnd) {
			continue
		}
		upcoming = append(upcoming, ev)
		if next == nil {
			copy := ev
			next = &copy
			nextDate = occ.Date.Format("2006-01-02")
			nextDays = ev.Days
		}
	}

	// next_event is a single human-readable string ("Dana's birthday") so a
	// HA sensor can use it directly as an attribute; it stays empty when no
	// events are upcoming.
	nextEvent := ""
	if next != nil {
		nextEvent = next.Name
		if nextEvent == "" {
			nextEvent = titleCase(next.Type)
		} else {
			nextEvent = nextEvent + "'s " + titleCase(next.Type)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	if err := json.NewEncoder(w).Encode(homeAssistantStats{
		NextEvent:      nextEvent,
		NextEventDate:  nextDate,
		NextEventDays:  nextDays,
		EventsIn7Days:  events7,
		EventsIn30Days: events30,
		Upcoming:       upcoming,
	}); err != nil {
		slog.Error("home assistant stats: encode", "error", err)
	}
}

// homeAssistantEventFromEvent converts an ent event into the Home Assistant
// feed shape against the occurrence date.
func homeAssistantEventFromEvent(e *ent.Event, date, now time.Time) homeAssistantEvent {
	personName := ""
	if p := e.Edges.Person; p != nil {
		personName = p.Name
	} else if c := e.Edges.Contact; c != nil {
		personName = c.Name
	}

	return homeAssistantEvent{
		Name: personName,
		Type: e.Type,
		Date: date.Format("2006-01-02"),
		Days: int(date.Sub(now).Hours() / 24),
	}
}
