package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/internal/repository"
)

func (h *Handler) settingsNotifications(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	repo := repository.NewUserNotificationChannelRepository(h.client)
	channels, _ := repo.ListByUser(r.Context(), u.ID)
	groups, _ := h.groups.List(r.Context())
	h.render(w, r, "settings.html", map[string]any{
		"Title":                     "Datey - Notification Settings",
		"SettingsTab":               "notifications-personal",
		"PersonalChannels":          channels,
		"Groups":                    groups,
		"NotificationScopeMode":     u.NotificationScopeMode,
		"NotificationScopeGroupIds": u.NotificationScopeGroupIds,
	})
}

func (h *Handler) settingsNotificationsSave(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	repo := repository.NewUserNotificationChannelRepository(h.client)
	for _, ct := range []string{"email", "gotify", "telegram", "ntfy", "webhook", "discord", "slack", "matrix", "webpush"} {
		target := strings.TrimSpace(r.FormValue("target_" + ct))
		enabled := r.FormValue("enabled_"+ct) == "on"
		if target == "" && !enabled {
			continue
		}
		if err := repository.ValidateTarget(ct, target); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if _, err := repo.Upsert(r.Context(), u.ID, ct, target, enabled); err != nil {
			http.Error(w, "failed to save", http.StatusInternalServerError)
			return
		}
	}
	mode := r.FormValue("notification_scope_mode")
	if mode != "all" && mode != "selected" {
		mode = "all"
	}
	groupIDs := r.FormValue("notification_scope_group_ids")
	// validate group IDs are ints
	if mode == "selected" && groupIDs != "" {
		for _, p := range strings.Split(groupIDs, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if _, err := strconv.Atoi(p); err != nil {
				http.Error(w, "invalid group id", http.StatusBadRequest)
				return
			}
		}
	} else {
		groupIDs = ""
	}
	_ = h.client.User.UpdateOneID(u.ID).SetNotificationScopeMode(user.NotificationScopeMode(mode)).SetNotificationScopeGroupIds(groupIDs).Exec(r.Context())
	http.Redirect(w, r, "/settings/notifications", http.StatusSeeOther)
}

// helper to avoid import cycle; ent expects enum type, use string via raw update
