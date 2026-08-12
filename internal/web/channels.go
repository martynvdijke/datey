package web

import (
	"context"
	"log/slog"
)

// channelInfo is a channel shown in the UI with its configuration state.
type channelInfo struct {
	Name       string
	Label      string
	Configured bool
}

// channelInfoList returns the configured notification channels that event
// reminders can be delivered through.
func (h *Handler) channelInfoList(ctx context.Context) []channelInfo {
	channels := []channelInfo{
		{"email", "Email", h.notifReg.IsConfigured("email")},
		{"gotify", "Gotify", h.notifReg.IsConfigured("gotify")},
		{"telegram", "Telegram", h.notifReg.IsConfigured("telegram")},
		{"ntfy", "ntfy.sh", h.notifReg.IsConfigured("ntfy")},
		{"webhook", "Webhook", h.notifReg.IsConfigured("webhook")},
	}
	// Web Push is only offered when enabled AND at least one subscription
	// exists — otherwise the checkbox would let users select a channel with
	// no browser to deliver to.
	if h.notifReg.IsConfigured("webpush") {
		count, err := h.pushSubs.Count(ctx)
		if err == nil && count > 0 {
			channels = append(channels, channelInfo{"webpush", "Web Push", true})
		}
	}
	return channels
}

// personOption is a simplified person representation for form dropdowns.
type personOption struct {
	ID   int
	Name string
}

// personOptions returns all people as dropdown options.
func (h *Handler) personOptions(ctx context.Context) []personOption {
	people, err := h.people.List(ctx)
	if err != nil {
		slog.Error("list people for form options", "error", err)
		return nil
	}
	opts := make([]personOption, 0, len(people))
	for _, p := range people {
		opts = append(opts, personOption{ID: p.ID, Name: p.Name})
	}
	return opts
}
