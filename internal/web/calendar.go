package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

func (h *Handler) calendarPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "calendar.html", map[string]any{
		"Title": "Datey - Calendar",
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

	var result []calendarEvent
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
	json.NewEncoder(w).Encode(result)
}
