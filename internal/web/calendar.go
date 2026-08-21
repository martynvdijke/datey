package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

func (h *Handler) calendarPage(w http.ResponseWriter, r *http.Request) {
	// Fetch upcoming events for the <noscript> fallback (next 30 days).
	now := time.Now()
	end := now.AddDate(0, 0, 30)
	occurrences, err := h.events.ListUpcomingOccurrences(r.Context(), todayStartUTC(), end)
	if err != nil {
		slog.Error("calendar page: list upcoming", "error", err)
		occurrences = nil // degrade gracefully — noscript list will be empty
	}

	type upcomingEvent struct {
		Name string
		Date string
		Type string
	}
	var upcoming []upcomingEvent
	for _, occ := range occurrences {
		e := occ.Event
		name := eventOwnerName(e)
		upcoming = append(upcoming, upcomingEvent{
			Name: name,
			Date: longDate(h.cfg.DateVariant, occ.Date),
			Type: e.Type,
		})
	}

	h.render(w, r, "calendar.html", map[string]any{
		"Title":          "Datey - Calendar",
		"UpcomingEvents": upcoming,
		"People":         h.personOptions(r.Context()),
	})
}

func (h *Handler) calendarEvents(w http.ResponseWriter, r *http.Request) {
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	// FullCalendar sends RFC3339 timestamps with the browser's timezone
	// offset (e.g. 2026-07-26T00:00:00+02:00); bare dates are also accepted.
	// The wall-clock date is used as midnight UTC so the day boundaries match
	// how event occurrences are computed (midnight UTC).
	start, ok := parseCalendarDate(startStr)
	if !ok {
		// Default to start of current month
		now := time.Now()
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}

	end, ok := parseCalendarDate(endStr)
	if !ok {
		// Default to end of next month
		now := time.Now()
		end = time.Date(now.Year(), now.Month()+2, 0, 0, 0, 0, 0, time.UTC)
	}

	// ListUpcomingOccurrences expands every event to its annual occurrence
	// date in the range, so historical dates render on every year they fall
	// due.
	occurrences, err := h.events.ListUpcomingOccurrences(r.Context(), start, end)
	if err != nil {
		slog.Error("calendar events: list", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	type calendarEvent struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Start       string   `json:"start"`
		AllDay      bool     `json:"allDay"`
		ClassNames  []string `json:"className"`
		Description string   `json:"description"`
		Notes       string   `json:"notes"`
		Type        string   `json:"type"`
	}

	result := make([]calendarEvent, 0, len(occurrences))
	for _, occ := range occurrences {
		e := occ.Event
		name := eventOwnerName(e)
		title := name
		if e.Type != "" {
			if title != "" {
				title = title + " - " + e.Type
			} else {
				title = e.Type
			}
		}
		result = append(result, calendarEvent{
			ID:          fmt.Sprintf("%d", e.ID),
			Title:       title,
			Start:       occ.Date.Format("2006-01-02"),
			AllDay:      true,
			ClassNames:  []string{"event-" + e.Type},
			Description: e.Description,
			Notes:       e.Notes,
			Type:        e.Type,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Error("calendar: encode events", "error", err)
	}
}

// parseCalendarDate parses FullCalendar range parameters, which are sent as
// RFC3339 timestamps with the browser's timezone offset (e.g.
// 2026-07-26T00:00:00+02:00); bare YYYY-MM-DD dates are also accepted. The
// wall-clock date is returned as midnight UTC so the day boundaries match how
// event occurrences are computed (midnight UTC). The bool result is false
// when the string cannot be parsed.
func parseCalendarDate(s string) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), true
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), true
	}
	return time.Time{}, false
}
