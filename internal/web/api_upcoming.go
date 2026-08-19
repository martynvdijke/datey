package web

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// upcomingEvent is one entry of GET /api/upcoming.
type upcomingEvent struct {
	ID            int    `json:"id"`
	Person        string `json:"person"`
	Type          string `json:"type"`
	Date          string `json:"date"`
	DaysRemaining int    `json:"days_remaining"`
}

// upcomingAPIAccessOK mirrors the iCal/RSS feed key semantics: the API is
// only reachable when enabled with a configured key and a matching ?key=.
func (h *Handler) upcomingAPIAccessOK(r *http.Request) bool {
	if !h.cfg.UpcomingAPIEnabled || h.cfg.UpcomingAPIKey == "" {
		return false
	}
	got := r.URL.Query().Get("key")
	return subtle.ConstantTimeCompare([]byte(got), []byte(h.cfg.UpcomingAPIKey)) == 1
}

// upcomingAPI serves GET /api/upcoming: a JSON array of upcoming events sorted
// by date ascending. The horizon defaults to the configured reminder window
// and is capped at 365 days.
func (h *Handler) upcomingAPI(w http.ResponseWriter, r *http.Request) {
	if !h.upcomingAPIAccessOK(r) {
		h.renderError(w, r, http.StatusNotFound)
		return
	}

	days := h.cfg.ReminderDays
	if raw := r.URL.Query().Get("days"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			h.renderError(w, r, http.StatusBadRequest)
			return
		}
		days = n
	}
	if days > 365 {
		days = 365
	}

	now := time.Now()
	occurrences, err := h.events.ListUpcomingOccurrences(r.Context(), todayStartUTC(), now.AddDate(0, 0, days))
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	out := make([]upcomingEvent, 0, len(occurrences))
	for _, occ := range occurrences {
		e := occ.Event
		person := ""
		if p := e.Edges.Person; p != nil {
			person = p.Name
		} else if c := e.Edges.Contact; c != nil {
			person = c.Name
		}
		out = append(out, upcomingEvent{
			ID:            e.ID,
			Person:        person,
			Type:          e.Type,
			Date:          occ.Date.Format("2006-01-02"),
			DaysRemaining: int(occ.Date.Sub(now).Hours() / 24),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		slog.Error("upcoming api: encode response", "error", err)
	}
}
