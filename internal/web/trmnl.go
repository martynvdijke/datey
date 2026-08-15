package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/age"
)

// trmnlEvent is the JSON shape of a single event in the TRMNL stats feed.
type trmnlEvent struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	Date          string `json:"date"`
	DaysRemaining int    `json:"days_remaining"`
	Relative      string `json:"relative"`
}

// trmnlStats is the full JSON response of GET /api/trmnl/stats.
type trmnlStats struct {
	NextEvent *trmnlEvent  `json:"next_event"`
	Upcoming  []trmnlEvent `json:"upcoming"`
	Stats     trmnlSummary `json:"stats"`
}

// trmnlSummary holds the quick-glance counters for the TRMNL display.
type trmnlSummary struct {
	PeopleCount        int `json:"people_count"`
	EventsNext7Days    int `json:"events_next_7_days"`
	EventsNext30Days   int `json:"events_next_30_days"`
	ConfiguredChannels int `json:"configured_channels"`
}

// trmnlStats renders the stats feed for the TRMNL e-ink display plugin.
// Public endpoint: TRMNL devices poll it without authentication.
func (h *Handler) trmnlStats(w http.ResponseWriter, r *http.Request) {
	now := time.Now()

	// The summary counts need a 30-day horizon; the upcoming list/next event
	// use the configured reminder window. Query once with the wider horizon.
	horizon := h.cfg.ReminderDays
	if horizon < 30 {
		horizon = 30
	}
	windowEnd := now.AddDate(0, 0, h.cfg.ReminderDays)

	occurrences, err := h.events.ListUpcomingOccurrences(r.Context(), now, now.AddDate(0, 0, horizon))
	if err != nil {
		slog.Error("trmnl stats: list upcoming", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	upcoming := make([]trmnlEvent, 0, len(occurrences))
	var next *trmnlEvent
	var events7, events30 int
	for _, occ := range occurrences {
		e := occ.Event
		ev := trmnlEventFromEvent(e, occ.Date, now, h.cfg.DateVariant)
		if ev.DaysRemaining <= 7 {
			events7++
		}
		if ev.DaysRemaining <= 30 {
			events30++
		}
		// Events beyond the reminder window still count toward summary stats
		// but are excluded from the display list.
		if occ.Date.After(windowEnd) {
			continue
		}
		upcoming = append(upcoming, ev)
		if next == nil {
			copy := ev
			next = &copy
		}
	}

	allPeople, _ := h.people.List(r.Context())

	channels := h.channelInfoList(r.Context())
	configuredChannels := 0
	for _, ch := range channels {
		if ch.Configured {
			configuredChannels++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(trmnlStats{
		NextEvent: next,
		Upcoming:  upcoming,
		Stats: trmnlSummary{
			PeopleCount:        len(allPeople),
			EventsNext7Days:    events7,
			EventsNext30Days:   events30,
			ConfiguredChannels: configuredChannels,
		},
	}); err != nil {
		slog.Error("trmnl stats: encode", "error", err)
	}
}

// trmnlBirthday is the JSON shape of a single birthday in the TRMNL
// birthdays feed.
type trmnlBirthday struct {
	Name          string `json:"name"`
	Date          string `json:"date"`
	DaysRemaining int    `json:"days_remaining"`
	Relative      string `json:"relative"`
	Age           *int   `json:"age"`
}

// trmnlBirthdaysResponse is the full JSON response of GET /api/trmnl/birthdays.
type trmnlBirthdaysResponse struct {
	NextBirthday *trmnlBirthday  `json:"next_birthday"`
	Birthdays    []trmnlBirthday `json:"birthdays"`
}

// trmnlBirthdays renders the upcoming-birthdays feed for the TRMNL e-ink
// display plugin. Public endpoint: TRMNL devices poll it without
// authentication.
func (h *Handler) trmnlBirthdays(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	windowEnd := now.AddDate(0, 0, h.cfg.ReminderDays)

	occurrences, err := h.events.ListUpcomingOccurrences(r.Context(), now, windowEnd)
	if err != nil {
		slog.Error("trmnl birthdays: list upcoming", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	birthdays := make([]trmnlBirthday, 0, len(occurrences))
	var next *trmnlBirthday
	for _, occ := range occurrences {
		if occ.Event.Type != "birthday" {
			continue
		}
		b := trmnlBirthdayFromEvent(occ.Event, occ.Date, now, h.cfg.DateVariant)
		birthdays = append(birthdays, b)
		if next == nil {
			copy := b
			next = &copy
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(trmnlBirthdaysResponse{
		NextBirthday: next,
		Birthdays:    birthdays,
	}); err != nil {
		slog.Error("trmnl birthdays: encode", "error", err)
	}
}

// trmnlBirthdayFromEvent converts a birthday event into the TRMNL feed
// shape, reusing the stats feed's label/relative logic and adding the age
// the person reaches at the occurrence date (null when the stored date
// carries no usable birth year).
func trmnlBirthdayFromEvent(e *ent.Event, date, now time.Time, variant string) trmnlBirthday {
	ev := trmnlEventFromEvent(e, date, now, variant)

	var ageValue *int
	if n, ok := age.NextAge(e.Date, now); ok {
		ageValue = &n
	}

	return trmnlBirthday{
		Name:          ev.Name,
		Date:          ev.Date,
		DaysRemaining: ev.DaysRemaining,
		Relative:      ev.Relative,
		Age:           ageValue,
	}
}

// trmnlEventFromEvent converts an ent event into the TRMNL feed shape,
// mirroring the dashboard's label and relative-day logic against the
// occurrence date.
func trmnlEventFromEvent(e *ent.Event, date, now time.Time, variant string) trmnlEvent {
	personName := ""
	if p := e.Edges.Person; p != nil {
		personName = p.Name
	} else if c := e.Edges.Contact; c != nil {
		personName = c.Name
	}

	days := int(date.Sub(now).Hours() / 24)

	var relative string
	switch {
	case days <= 0:
		relative = "Today"
	case days == 1:
		relative = "Tomorrow"
	case days <= 7:
		relative = "In " + strconv.Itoa(days) + " days"
	}

	return trmnlEvent{
		Name:          personName,
		Type:          e.Type,
		Date:          shortDate(variant, date),
		DaysRemaining: days,
		Relative:      relative,
	}
}
