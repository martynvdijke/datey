package web

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/age"
	"github.com/datey/datey/internal/tzutil"
)

// trmnlEvent is the JSON shape of a single event in the TRMNL stats feed.
type trmnlEvent struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	Date          string `json:"date"`
	DaysRemaining int    `json:"days_remaining"`
	Relative      string `json:"relative"`
	// YearsSince is set for memorial events: the completed number of years
	// since the remembered date at this occurrence ("N years since").
	YearsSince *int `json:"years_since,omitempty"`
}

// trmnlStats is the full JSON response of GET /api/trmnl/stats.
type trmnlStats struct {
	NextEvent *trmnlEvent  `json:"next_event"`
	Upcoming  []trmnlEvent `json:"upcoming"`
	Stats     trmnlSummary `json:"stats"`
}

// trmnlSummary holds the quick-glance counters for the TRMNL display.
type trmnlSummary struct {
	PeopleCount      int `json:"people_count"`
	EventsNext7Days  int `json:"events_next_7_days"`
	EventsNext30Days int `json:"events_next_30_days"`
}

// trmnlRequestKey extracts the device secret from either the ?key= query
// parameter (legacy URL-based setup) or an Authorization: Bearer header (the
// TRMNL plugin's polling_headers field). Header auth keeps the token out of
// URLs and proxy logs.
func trmnlRequestKey(r *http.Request) string {
	if key := r.URL.Query().Get("key"); key != "" {
		return key
	}
	authz := r.Header.Get("Authorization")
	if len(authz) > len(bearerPrefix) && strings.EqualFold(authz[:len(bearerPrefix)], bearerPrefix) {
		return strings.TrimSpace(authz[len(bearerPrefix):])
	}
	return ""
}

// trmnlAccessOK protects the TRMNL feeds with per-device credentials.
//
// Devices authenticate with either a personal API token (create one per
// device under Settings → API Tokens; rotate/revoke it there) or the legacy
// shared TRMNL_KEY so deployments that already configured a key keep working.
// Any valid, unrevoked, unexpired API token is accepted; the token owner is
// irrelevant because the feeds are read-only. The endpoint is only reachable
// while TRMNL_ENABLED is on.
func (h *Handler) trmnlAccessOK(r *http.Request) bool {
	if !h.cfg.TRMNLEnabled {
		return false
	}
	got := trmnlRequestKey(r)
	if got == "" {
		return false
	}
	if h.cfg.TRMNLKey != "" && subtle.ConstantTimeCompare([]byte(got), []byte(h.cfg.TRMNLKey)) == 1 {
		return true
	}
	if h.apiTokens == nil {
		return false
	}
	if _, err := h.apiTokens.GetByToken(r.Context(), got); err != nil {
		return false
	}
	return true
}

// trmnlStats renders the stats feed for the TRMNL e-ink display plugin.
func (h *Handler) trmnlStats(w http.ResponseWriter, r *http.Request) {
	if !h.trmnlAccessOK(r) {
		h.renderError(w, r, http.StatusNotFound)
		return
	}
	now := time.Now()

	// The summary counts need a 30-day horizon; the upcoming list/next event
	// use the configured reminder window. Query once with the wider horizon.
	horizon := h.cfg.ReminderDays
	if horizon < 30 {
		horizon = 30
	}

	occurrences, err := h.events.ListUpcomingOccurrences(r.Context(), todayStartUTC().AddDate(0, 0, -1), now.AddDate(0, 0, horizon+1))
	if err != nil {
		slog.Error("trmnl stats: list upcoming", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	upcoming := make([]trmnlEvent, 0, len(occurrences))
	var next *trmnlEvent
	var events7, events30 int
	reminderDays := h.cfg.ReminderDays
	for _, occ := range occurrences {
		e := occ.Event
		loc := occurrenceLocation(e)
		days := tzutil.DaysUntil(occ.Date, loc, now)
		// Padded query bounds: drop dates that are already past or beyond the
		// horizon once counted in the owner's own zone.
		if days < 0 || days > horizon {
			continue
		}
		ev := trmnlEventFromEvent(e, occ.Date, now, h.cfg.DateVariant, loc)
		if ev.DaysRemaining <= 7 {
			events7++
		}
		if ev.DaysRemaining <= 30 {
			events30++
		}
		// Events beyond the reminder window still count toward summary stats
		// but are excluded from the display list.
		if days > reminderDays {
			continue
		}
		upcoming = append(upcoming, ev)
		if next == nil {
			copy := ev
			next = &copy
		}
	}

	// Always show at least the next upcoming birthday: when no birthday falls
	// inside the reminder window, pin the next birthday — however far out —
	// to the display list so the feed never hides it.
	hasBirthday := false
	for _, ev := range upcoming {
		if ev.Type == "birthday" {
			hasBirthday = true
			break
		}
	}
	if !hasBirthday {
		if nb, err := h.events.NextBirthdayOccurrence(r.Context(), now); err != nil {
			slog.Error("trmnl stats: next birthday", "error", err)
		} else if nb != nil {
			ev := trmnlEventFromEvent(nb.Event, nb.Date, now, h.cfg.DateVariant, occurrenceLocation(nb.Event))
			upcoming = append(upcoming, ev)
			if next == nil {
				copy := ev
				next = &copy
			}
		}
	}

	allPeople, _ := h.people.List(r.Context())

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(trmnlStats{
		NextEvent: next,
		Upcoming:  upcoming,
		Stats: trmnlSummary{
			PeopleCount:      len(allPeople),
			EventsNext7Days:  events7,
			EventsNext30Days: events30,
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
// display plugin.
func (h *Handler) trmnlBirthdays(w http.ResponseWriter, r *http.Request) {
	if !h.trmnlAccessOK(r) {
		h.renderError(w, r, http.StatusNotFound)
		return
	}
	now := time.Now()
	reminderDays := h.cfg.ReminderDays

	occurrences, err := h.events.ListUpcomingOccurrences(r.Context(), todayStartUTC().AddDate(0, 0, -1), now.AddDate(0, 0, reminderDays+1))
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
		loc := occurrenceLocation(occ.Event)
		days := tzutil.DaysUntil(occ.Date, loc, now)
		if days < 0 || days > reminderDays {
			continue
		}
		b := trmnlBirthdayFromEvent(occ.Event, occ.Date, now, h.cfg.DateVariant, loc)
		birthdays = append(birthdays, b)
		if next == nil {
			copy := b
			next = &copy
		}
	}

	// Always show at least the next upcoming birthday: when none falls
	// inside the reminder window, fall back to the next birthday — however
	// far out — so the feed never reports "No upcoming birthdays".
	if next == nil {
		if nb, err := h.events.NextBirthdayOccurrence(r.Context(), now); err != nil {
			slog.Error("trmnl birthdays: next birthday", "error", err)
		} else if nb != nil {
			b := trmnlBirthdayFromEvent(nb.Event, nb.Date, now, h.cfg.DateVariant, occurrenceLocation(nb.Event))
			birthdays = append(birthdays, b)
			next = &b
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
func trmnlBirthdayFromEvent(e *ent.Event, date, now time.Time, variant string, loc *time.Location) trmnlBirthday {
	ev := trmnlEventFromEvent(e, date, now, variant, loc)

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
func trmnlEventFromEvent(e *ent.Event, date, now time.Time, variant string, loc *time.Location) trmnlEvent {
	personName := eventOwnerName(e)

	days := tzutil.DaysUntil(date, loc, now)

	var relative string
	switch {
	case days <= 0:
		relative = "Today"
	case days == 1:
		relative = "Tomorrow"
	case days <= 7:
		relative = "In " + strconv.Itoa(days) + " days"
	}

	var yearsSince *int
	if e.Type == "memorial" && e.Date.Year() > 1 {
		if n, ok := age.YearsSince(e.Date, date); ok {
			yearsSince = &n
		}
	}

	return trmnlEvent{
		Name:          personName,
		Type:          e.Type,
		Date:          shortDate(variant, date),
		DaysRemaining: days,
		Relative:      relative,
		YearsSince:    yearsSince,
	}
}
