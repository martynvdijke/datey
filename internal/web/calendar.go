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
	events, err := h.events.ListUpcoming(r.Context(), now, end)
	if err != nil {
		slog.Error("calendar page: list upcoming", "error", err)
		events = nil // degrade gracefully — noscript list will be empty
	}

	type upcomingEvent struct {
		Name string
		Date string
		Type string
	}
	var upcoming []upcomingEvent
	for _, e := range events {
		name := ""
		if p := e.Edges.Person; p != nil {
			name = p.Name
		} else if c := e.Edges.Contact; c != nil {
			name = c.Name
		}
		upcoming = append(upcoming, upcomingEvent{
			Name: name,
			Date: e.Date.Format("Jan 2, 2006"),
			Type: e.Type,
		})
	}

	h.render(w, r, "calendar.html", map[string]any{
		"Title":          "Datey - Calendar",
		"UpcomingEvents": upcoming,
	})
}

func (h *Handler) calendarEvents(w http.ResponseWriter, r *http.Request) {
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	start, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		// Default to start of current month
		now := time.Now()
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}

	end, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		// Default to end of next month
		now := time.Now()
		end = time.Date(now.Year(), now.Month()+2, 0, 0, 0, 0, 0, time.UTC)
	}

	events, err := h.events.ListInRange(r.Context(), start, end)
	if err != nil {
		slog.Error("calendar events: list", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Color map by event type
	colorMap := map[string]string{
		"birthday":    "#0d6efd",
		"anniversary": "#198754",
		"wedding":     "#ffc107",
		"holiday":     "#dc3545",
		"meeting":     "#6f42c1",
		"custom":      "#6c757d",
	}

	type calendarEvent struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Start       string `json:"start"`
		AllDay      bool   `json:"allDay"`
		Background  string `json:"backgroundColor"`
		Border      string `json:"borderColor"`
		TextColor   string `json:"textColor"`
		Description string `json:"description"`
		Type        string `json:"type"`
	}

	result := make([]calendarEvent, 0)
	for _, e := range events {
		contactName := ""
		if contact := e.Edges.Contact; contact != nil {
			contactName = contact.Name
		}
		color, ok := colorMap[e.Type]
		if !ok {
			color = "#6c757d"
		}
		title := contactName
		if e.Type != "" {
			title = contactName + " - " + e.Type
		}
		result = append(result, calendarEvent{
			ID:          fmt.Sprintf("%d", e.ID),
			Title:       title,
			Start:       e.Date.Format("2006-01-02"),
			AllDay:      true,
			Background:  color,
			Border:      color,
			TextColor:   "#ffffff",
			Description: e.Description,
			Type:        e.Type,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Error("calendar: encode events", "error", err)
	}
}
