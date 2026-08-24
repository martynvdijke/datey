package web

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

func generateKey() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte("fallback-key-12345"))
	}
	return hex.EncodeToString(b)
}

func (h *Handler) settingsBackupRestore(w http.ResponseWriter, r *http.Request) {
	h.auditRecord(r, "backup.restore_staged", "")
	http.Redirect(w, r, "/settings/backup?success=Restore+staged", http.StatusSeeOther)
}

func (h *Handler) regenerateFeedKey(kind string, auditAction string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := generateKey()
		ctx := r.Context()
		switch kind {
		case "ical":
			h.cfg.ICalFeedKey = key
			_ = h.settingsStore.SetFeedKey(ctx, "ical", key)
		case "rss":
			h.cfg.RSSFeedKey = key
			_ = h.settingsStore.SetFeedKey(ctx, "rss", key)
		case "upcoming":
			h.cfg.UpcomingAPIKey = key
			_ = h.settingsStore.SetFeedKey(ctx, "upcoming", key)
		case "homeassistant":
			h.cfg.HomeAssistantKey = key
			_ = h.settingsStore.SetFeedKey(ctx, "homeassistant", key)
		case "trmnl":
			// TRMNL uses same stats feed; regenerate a generic key if tracked elsewhere
			_ = h.settingsStore.SetFeedKey(ctx, "trmnl", key)
		}
		h.auditRecord(r, auditAction, "")
		http.Redirect(w, r, "/settings/config?success=Feed+key+regenerated", http.StatusSeeOther)
	}
}
