package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/internal/repository"
)

// notificationChannelOrder lists the per-user channels in the order their
// cards are rendered. The save handler iterates the same list, so a card is
// always persisted under the field names the template emitted.
var notificationChannelOrder = []string{
	"email", "gotify", "telegram", "ntfy", "pushover",
	"webhook", "discord", "slack", "matrix", "webpush",
	"fcm", "newitems",
}

// notificationChannelWithoutTarget reports whether the channel is a plain
// on/off switch: those have no destination field and always persist (even with
// an empty target) so that unchecking the box is stored rather than skipped.
func notificationChannelWithoutTarget(channel string) bool {
	return channel == "fcm" || channel == "newitems"
}

// channelHealthTimeLayout is the server-side timestamp format used on the
// channel cards. It matches the settings pages' existing date style.
const channelHealthTimeLayout = "2006-01-02 15:04"

// channelCard is the view model behind one per-user channel card. Health
// fields are zero when the channel has never been attempted.
type channelCard struct {
	Channel             string
	Target              string
	Enabled             bool
	HasHealth           bool
	LastSuccess         string
	LastFailure         string
	ConsecutiveFailures int
	LastError           string
	Failed              bool

	// NoTarget marks switch-only channels (Android app, new item alerts):
	// the template renders a checkbox without a destination field.
	NoTarget  bool
	TitleKey  string // i18n key overriding the channel name as the card title
	HelpKey   string // i18n key for the card's helper text
	EnableKey string // i18n key overriding the generic enable label
}

func (h *Handler) settingsNotifications(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	repo := repository.NewUserNotificationChannelRepository(h.client)
	channels, _ := repo.MapByUser(r.Context(), u.ID)
	healthRows, err := repository.NewChannelHealthRepository(h.client).ListAll(r.Context())
	if err != nil {
		// Health is diagnostic only: a read failure must not hide the form.
		healthRows = nil
	}
	groups, _ := h.groups.List(r.Context())
	fcmConfigured := h.cfg.FCMEnabled && h.cfg.FCMProjectID != "" && h.cfg.FCMServiceAccountJSON != ""
	h.render(w, r, "settings_notifications.html", map[string]any{
		"Title":                     "Datey - Notification Settings",
		"SettingsTab":               "notifications-personal",
		"ChannelCards":              buildChannelCards(channels, repository.IndexChannelHealth(healthRows)),
		"Groups":                    groups,
		"NotificationScopeMode":     string(u.NotificationScopeMode),
		"NotificationScopeGroupIds": u.NotificationScopeGroupIds,
		"FCMConfigured":             fcmConfigured,
	})
}

// buildChannelCards joins the user's channel configuration with the batched
// health index. It performs no queries: the caller loads every health row with
// a single ListAll and passes the index in.
func buildChannelCards(channels map[string]*ent.UserNotificationChannel, health map[string]*ent.ChannelHealth) []channelCard {
	cards := make([]channelCard, 0, len(notificationChannelOrder))
	for _, name := range notificationChannelOrder {
		card := channelCard{Channel: name}
		switch name {
		case "fcm":
			card.NoTarget = true
			card.TitleKey = "notifications.fcm_title"
			card.HelpKey = "notifications.fcm_help"
			card.EnableKey = "notifications.fcm_enable"
		case "newitems":
			card.NoTarget = true
			card.TitleKey = "notifications.newitems_title"
			card.HelpKey = "notifications.newitems_help"
			card.EnableKey = "notifications.newitems_enable"
			// A missing row means "enabled": new item alerts are opt-out,
			// matching the dispatcher's default.
			card.Enabled = true
		}
		target := ""
		if ch, ok := channels[name]; ok {
			card.Target = ch.Target
			card.Enabled = ch.Enabled
			target = ch.Target
		}
		row, ok := health[repository.ChannelHealthKey(name, target)]
		if !ok || row == nil {
			cards = append(cards, card)
			continue
		}
		card.HasHealth = true
		card.LastSuccess = formatHealthTime(row.LastSuccessAt)
		card.LastFailure = formatHealthTime(row.LastFailureAt)
		card.ConsecutiveFailures = row.ConsecutiveFailures
		card.Failed = row.ConsecutiveFailures > 0
		if card.Failed {
			card.LastError = row.LastError
		}
		cards = append(cards, card)
	}
	return cards
}

// formatHealthTime renders a nullable timestamp, or "" when absent.
func formatHealthTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Local().Format(channelHealthTimeLayout)
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
	for _, ct := range notificationChannelOrder {
		target := strings.TrimSpace(r.FormValue("target_" + ct))
		enabled := r.FormValue("enabled_"+ct) == "on"
		// Switch-only channels have no target field: persist them even when
		// unchecked, otherwise turning the channel off would never be saved.
		if target == "" && !enabled && !notificationChannelWithoutTarget(ct) {
			continue
		}
		// ValidateTarget accepts an empty target, which is the only value the
		// switch-only channels ever submit.
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
