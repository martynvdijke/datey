package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

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

func (h *Handler) notificationsList(w http.ResponseWriter, r *http.Request) {
	notifications, err := h.oneTimeNots.List(r.Context())
	if err != nil {
		slog.Error("notifications list", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	h.render(w, r, "notifications.html", map[string]any{
		"Title":         "Datey - One-Time Notifications",
		"Notifications": notifications,
	})
}

func (h *Handler) newNotificationForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "notification_form.html", map[string]any{
		"Title":    "Datey - Create Notification",
		"Channels": h.channelInfoList(),
	})
}

func (h *Handler) createNotification(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	message := r.FormValue("message")
	scheduledAtStr := r.FormValue("scheduled_at")

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
			},
			"Channels": h.channelInfoList(),
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
			},
			"Channels": h.channelInfoList(),
		})
		return
	}

	if scheduledAt.Before(time.Now()) {
		errors["scheduled_at"] = "Scheduled time must be in the future"
		h.render(w, r, "notification_form.html", map[string]any{
			"Title":    "Datey - Create Notification",
			"Errors":   errors,
			"FormData": map[string]string{
				"Message":     message,
				"ScheduledAt": scheduledAtStr,
			},
			"Channels": h.channelInfoList(),
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
			},
			"Channels": h.channelInfoList(),
		})
		return
	}

	_, err = h.oneTimeNots.Create(r.Context(), message, scheduledAt, channels)
	if err != nil {
		slog.Error("create notification", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

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

func (h *Handler) apiNotifications(w http.ResponseWriter, r *http.Request) {
	notifications, err := h.oneTimeNots.List(r.Context())
	if err != nil {
		slog.Error("api notifications list", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	type apiNotification struct {
		ID          int        `json:"id"`
		Message     string     `json:"message"`
		ScheduledAt time.Time  `json:"scheduled_at"`
		Status      string     `json:"status"`
		CreatedAt   time.Time  `json:"created_at"`
		SentAt      *time.Time `json:"sent_at,omitempty"`
	}

	result := make([]apiNotification, len(notifications))
	for i, n := range notifications {
		result[i] = apiNotification{
			ID:          n.ID,
			Message:     n.Message,
			ScheduledAt: n.ScheduledAt,
			Status:      n.Status,
			CreatedAt:   n.CreatedAt,
			SentAt:      n.SentAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
