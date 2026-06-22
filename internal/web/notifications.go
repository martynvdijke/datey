package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/datey/datey/internal/repository"
	"github.com/go-chi/chi/v5"
)

type channelInfo struct {
	Name       string
	Label      string
	Configured bool
}

func (h *Handler) channelInfoList() []channelInfo {
	return []channelInfo{
		{"email", "Email", h.notifReg.IsConfigured("email")},
		{"gotify", "Gotify", h.notifReg.IsConfigured("gotify")},
		{"telegram", "Telegram", h.notifReg.IsConfigured("telegram")},
	}
}

// personOption is a simplified person representation for form dropdowns.
type personOption struct {
	ID   int
	Name string
}

func (h *Handler) personOptions(ctx context.Context) []personOption {
	people, err := h.people.List(ctx)
	if err != nil {
		slog.Error("list people for notification form", "error", err)
		return nil
	}
	opts := make([]personOption, 0, len(people))
	for _, p := range people {
		opts = append(opts, personOption{ID: p.ID, Name: p.Name})
	}
	return opts
}

func (h *Handler) notificationsList(w http.ResponseWriter, r *http.Request) {
	notifications, err := h.oneTimeNots.List(r.Context())
	if err != nil {
		slog.Error("notifications list", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	// Build a person name lookup for notifications that have a person_id.
	// Uses string keys so templates can index with printf "%d" .PersonID.
	personNames := make(map[string]string)
	for _, n := range notifications {
		if n.PersonID != nil {
			key := strconv.Itoa(*n.PersonID)
			if _, ok := personNames[key]; !ok {
				p, err := h.people.Get(r.Context(), *n.PersonID)
				if err == nil && p != nil {
					personNames[key] = p.Name
				}
			}
		}
	}

	h.render(w, r, "notifications.html", map[string]any{
		"Title":         "Datey - One-Time Notifications",
		"Notifications": notifications,
		"PersonNames":   personNames,
	})
}

func (h *Handler) newNotificationForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "notification_form.html", map[string]any{
		"Title":    "Datey - Create Notification",
		"Channels": h.channelInfoList(),
		"People":   h.personOptions(r.Context()),
	})
}

func (h *Handler) createNotification(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	message := r.FormValue("message")
	scheduledAtStr := r.FormValue("scheduled_at")
	personIDStr := r.FormValue("person_id")
	eventType := r.FormValue("event_type")

	errors := make(map[string]string)

	if message == "" {
		errors["message"] = "Message is required"
	}

	if scheduledAtStr == "" {
		errors["scheduled_at"] = "Scheduled date and time is required"
	}

	if len(errors) > 0 {
		h.render(w, r, "notification_form.html", map[string]any{
			"Title":    "Datey - Create Notification",
			"Errors":   errors,
			"FormData": map[string]string{
				"Message":     message,
				"ScheduledAt": scheduledAtStr,
				"PersonID":    personIDStr,
				"EventType":   eventType,
			},
			"Channels": h.channelInfoList(),
			"People":   h.personOptions(r.Context()),
		})
		return
	}

	scheduledAt, err := time.ParseInLocation("2006-01-02T15:04", scheduledAtStr, time.Local)
	if err != nil {
		errors["scheduled_at"] = "Invalid date/time format"
		h.render(w, r, "notification_form.html", map[string]any{
			"Title":    "Datey - Create Notification",
			"Errors":   errors,
			"FormData": map[string]string{
				"Message":     message,
				"ScheduledAt": scheduledAtStr,
				"PersonID":    personIDStr,
				"EventType":   eventType,
			},
			"Channels": h.channelInfoList(),
			"People":   h.personOptions(r.Context()),
		})
		return
	}

	if scheduledAt.Before(time.Now()) {
		errors["scheduled_at"] = "Scheduled time must be in the future"
		h.render(w, r, "notification_form.html", map[string]any{
			"Title":  "Datey - Create Notification",
			"Errors": errors,
			"FormData": map[string]string{
				"Message":     message,
				"ScheduledAt": scheduledAtStr,
				"PersonID":    personIDStr,
				"EventType":   eventType,
			},
			"Channels": h.channelInfoList(),
			"People":   h.personOptions(r.Context()),
		})
		return
	}

	// Parse selected channel targets (default to all configured if none selected)
	channels := r.Form["channels"]
	if len(channels) == 0 {
		for _, name := range []string{"email", "gotify", "telegram"} {
			if h.notifReg.IsConfigured(name) {
				channels = append(channels, name)
			}
		}
	}

	if len(channels) == 0 {
		errors["channels"] = "At least one notification channel must be selected"
		h.render(w, r, "notification_form.html", map[string]any{
			"Title":  "Datey - Create Notification",
			"Errors": errors,
			"FormData": map[string]string{
				"Message":     message,
				"ScheduledAt": scheduledAtStr,
				"PersonID":    personIDStr,
				"EventType":   eventType,
			},
			"Channels": h.channelInfoList(),
			"People":   h.personOptions(r.Context()),
		})
		return
	}

	opts := &repository.CreateNotificationOptions{}
	if pid, err := strconv.Atoi(personIDStr); err == nil && pid > 0 {
		opts.PersonID = &pid
	}
	if eventType != "" {
		opts.EventType = eventType
	}

	_, err = h.oneTimeNots.Create(r.Context(), message, scheduledAt, channels, opts)
	if err != nil {
		slog.Error("create notification", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	toastHeader(w, "Notification created", "success")
	http.Redirect(w, r, "/notifications", http.StatusSeeOther)
}

func (h *Handler) deleteNotification(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := h.oneTimeNots.Delete(r.Context(), id); err != nil {
		slog.Error("delete notification", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	toastHeader(w, "Notification deleted", "success")
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

type apiDelivery struct {
	ID           int        `json:"id"`
	Channel      string     `json:"channel"`
	Status       string     `json:"status"`
	SentAt       *time.Time `json:"sent_at,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
}

func (h *Handler) apiNotifications(w http.ResponseWriter, r *http.Request) {
	notifications, err := h.oneTimeNots.List(r.Context())
	if err != nil {
		slog.Error("api notifications list", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	type apiNotification struct {
		ID          int           `json:"id"`
		Message     string        `json:"message"`
		ScheduledAt time.Time     `json:"scheduled_at"`
		Status      string        `json:"status"`
		CreatedAt   time.Time     `json:"created_at"`
		SentAt      *time.Time    `json:"sent_at,omitempty"`
		PersonID    *int          `json:"person_id,omitempty"`
		EventType   string        `json:"event_type,omitempty"`
		Deliveries  []apiDelivery `json:"deliveries"`
	}

	result := make([]apiNotification, len(notifications))
	for i, n := range notifications {
		deliveries := make([]apiDelivery, 0, len(n.Edges.Deliveries))
		for _, d := range n.Edges.Deliveries {
			deliveries = append(deliveries, apiDelivery{
				ID:           d.ID,
				Channel:      d.Channel,
				Status:       d.Status,
				SentAt:       d.SentAt,
				ErrorMessage: d.ErrorMessage,
			})
		}
		notif := apiNotification{
			ID:          n.ID,
			Message:     n.Message,
			ScheduledAt: n.ScheduledAt,
			Status:      n.Status,
			CreatedAt:   n.CreatedAt,
			SentAt:      n.SentAt,
			EventType:   n.EventType,
			Deliveries:  deliveries,
		}
		if n.PersonID != nil {
			notif.PersonID = n.PersonID
		}
		result[i] = notif
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Error("notifications: encode response", "error", err)
	}
}

// testNotificationNow sends a notification immediately via the specified channel.
// This is the handler for the "Send Test" button on the notification form.
func (h *Handler) testNotificationNow(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	message := r.FormValue("message")
	channel := r.FormValue("channel")

	if message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	if channel == "" {
		http.Error(w, "channel is required", http.StatusBadRequest)
		return
	}

	if !h.notifReg.IsConfigured(channel) {
		http.Error(w, "channel not configured", http.StatusBadRequest)
		return
	}

	title := "Datey Test Notification"
	if err := h.notifReg.Send(r.Context(), channel, title, message); err != nil {
		slog.Error("test notification now failed", "channel", channel, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte(`<div class="alert alert-danger py-2 mb-0">Failed to send test notification. Check server logs for details.</div>`)); err != nil {
			slog.Error("write response", "error", err)
		}
		return
	}

	slog.Info("test notification now sent", "channel", channel)
	if _, err := w.Write([]byte(`<div class="alert alert-success py-2 mb-0">Test sent!</div>`)); err != nil {
		slog.Error("write response", "error", err)
	}
}
