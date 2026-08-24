package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/datey/datey/internal/db"
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/repository"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) settings(w http.ResponseWriter, r *http.Request) {
	type channelStatus struct {
		Name       string
		Configured bool
	}

	channels := []channelStatus{
		{Name: "email", Configured: h.notifReg.IsConfigured("email")},
		{Name: "gotify", Configured: h.notifReg.IsConfigured("gotify")},
		{Name: "telegram", Configured: h.notifReg.IsConfigured("telegram")},
		{Name: "ntfy", Configured: h.notifReg.IsConfigured("ntfy")},
		{Name: "webhook", Configured: h.notifReg.IsConfigured("webhook")},
		{Name: "discord", Configured: h.notifReg.IsConfigured("discord")},
		{Name: "slack", Configured: h.notifReg.IsConfigured("slack")},
		{Name: "matrix", Configured: h.notifReg.IsConfigured("matrix")},
		{Name: "webpush", Configured: h.notifReg.IsConfigured("webpush")},
	}

	// CardDAV section: on a failed POST, re-render with the submitted values
	// (except the password, which is never rendered) and per-field errors.
	errs, _ := r.Context().Value(configFormErrorsKey{}).(map[string]string)
	submitted := url.Values{}
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		submitted = r.PostForm
	}

	carddav := carddavView{
		Enabled:      h.cfg.CarddavEnabled,
		URL:          h.cfg.CarddavURL,
		Username:     h.cfg.CarddavUsername,
		HasPassword:  h.cfg.CarddavPassword != "",
		DeletePolicy: h.cfg.CarddavDeletePolicy,
		Errors:       errs,
	}
	if submitted != nil {
		if v, ok := submitted["CARDDAV_ENABLED"]; ok && len(v) > 0 {
			carddav.Enabled = v[0] == "on"
		}
		if v := submitted.Get("CARDDAV_URL"); v != "" {
			carddav.URL = v
		}
		if v := submitted.Get("CARDDAV_USERNAME"); v != "" {
			carddav.Username = v
		}
		if v := submitted.Get("CARDDAV_DELETE_POLICY"); v != "" {
			carddav.DeletePolicy = v
		}
	}

	locale := ""
	if u := UserFromContext(r.Context()); u != nil && u.Locale != nil {
		locale = *u.Locale
	}
	h.render(w, r, "settings.html", map[string]any{
		"Title":         "Datey - Settings",
		"SettingsTab":   "notifications",
		"Channels":      channels,
		"Carddav":       carddav,
		"CurrentLocale": locale,
	})
}

func (h *Handler) settingsEinkToggle(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Accept optional "enabled" query param to explicitly set state.
	// When absent, toggle the current state (backward compat).
	enabledStr := r.URL.Query().Get("enabled")
	var newVal bool
	if enabledStr != "" {
		newVal = enabledStr == "true"
		if err := h.users.SetEinkMode(r.Context(), u.ID, newVal); err != nil {
			slog.Error("eink set mode", "error", err)
			http.Error(w, "failed to set e-ink mode", http.StatusInternalServerError)
			return
		}
	} else {
		var err error
		newVal, err = h.users.UpdateEinkMode(r.Context(), u.ID)
		if err != nil {
			slog.Error("eink toggle", "error", err)
			http.Error(w, "failed to toggle e-ink mode", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]bool{"eink_mode": newVal}); err != nil {
		slog.Error("encode response", "error", err)
	}
}

func (h *Handler) settingsLogs(w http.ResponseWriter, r *http.Request) {
	levelFilter := r.URL.Query().Get("level")
	sourceFilter := r.URL.Query().Get("source")

	pageStr := r.URL.Query().Get("page")
	page := 0
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}
	limit := 100
	offset := page * limit

	entries, total := h.logStore.Query(levelFilter, sourceFilter, offset, limit)
	currentLevel := logstore.LevelName(h.logStore.Level())

	h.render(w, r, "settings.html", map[string]any{
		"Title":        "Datey - Settings",
		"SettingsTab":  "logs",
		"Entries":      entries,
		"Total":        total,
		"Page":         page,
		"Limit":        limit,
		"LevelFilter":  levelFilter,
		"SourceFilter": sourceFilter,
		"CurrentLevel": currentLevel,
	})
}

func (h *Handler) settingsBackup(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "settings.html", map[string]any{
		"Title":               "Datey - Settings",
		"SettingsTab":         "backup",
		"BackupDir":           h.cfg.BackupDir,
		"BackupRetentionDays": h.cfg.BackupRetentionDays,
	})
}

func (h *Handler) settingsBackupRun(w http.ResponseWriter, r *http.Request) {
	dbPath := h.cfg.DataDir + "/datey.db"
	if err := db.Backup(dbPath, h.cfg.BackupDir, h.cfg.BackupRetentionDays); err != nil {
		slog.Error("manual backup failed", "error", err)
		if _, err := w.Write([]byte(`<div class="alert alert-danger">Backup failed. Check server logs for details.</div>`)); err != nil {
			slog.Error("write response", "error", err)
		}
		return
	}
	slog.Info("manual backup completed", "dir", h.cfg.BackupDir)
	h.auditRecord(r, "backup.run", h.cfg.BackupDir)
	if _, err := w.Write([]byte(`<div class="alert alert-success">Backup completed successfully!</div>`)); err != nil {
		slog.Error("write response", "error", err)
	}
}

func (h *Handler) oldLogsRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/settings/logs", http.StatusMovedPermanently)
}

func (h *Handler) testNotification(w http.ResponseWriter, r *http.Request) {
	channel := chi.URLParam(r, "channel")
	if !h.notifReg.IsConfigured(channel) {
		slog.Warn("test notification: channel not configured", "source", "settings", "channel", channel)
		http.Error(w, "channel not configured", http.StatusBadRequest)
		return
	}
	title := "Datey Test Notification"
	message := fmt.Sprintf("This is a test notification sent at %s", time.Now().Format(time.RFC3339))
	// Resolve per-user target if exists
	target := ""
	if u := UserFromContext(r.Context()); u != nil {
		if repo := repository.NewUserNotificationChannelRepository(h.client); repo != nil {
			if m, err2 := repo.MapByUser(r.Context(), u.ID); err2 == nil {
				if ch, ok := m[channel]; ok && ch.Enabled && ch.Target != "" {
					target = ch.Target
				}
			}
		}
	}
	var err error
	switch channel {
	case "email":
		n := notifier.NewEmailNotifier(h.cfg)
		err = n.SendTo(r.Context(), title, message, target)
	case "gotify":
		n := notifier.NewGotifyNotifier(h.cfg)
		err = n.SendTo(r.Context(), title, message, target)
	case "telegram":
		n := notifier.NewTelegramNotifier(h.cfg)
		err = n.SendTo(r.Context(), title, message, target)
	case "ntfy":
		n := notifier.NewNtfyNotifier(h.cfg)
		err = n.SendTo(r.Context(), title, message, target)
	case "webhook":
		n := notifier.NewWebhookNotifier(h.cfg)
		err = n.SendTo(r.Context(), title, message, target)
	case "discord":
		n := notifier.NewDiscordNotifier(h.cfg)
		err = n.SendTo(r.Context(), title, message, target)
	case "slack":
		n := notifier.NewSlackNotifier(h.cfg)
		err = n.SendTo(r.Context(), title, message, target)
	case "matrix":
		n := notifier.NewMatrixNotifier(h.cfg)
		err = n.SendTo(r.Context(), title, message, target)
	case "webpush":
		n := notifier.NewWebPushNotifier(h.cfg, h.pushSubs)
		err = n.SendTo(r.Context(), title, message, target)
	default:
		slog.Warn("test notification: unknown channel", "source", "settings", "channel", channel)
		http.Error(w, "unknown channel", http.StatusBadRequest)
		return
	}

	if err != nil {
		slog.Error("test notification failed", "source", "settings", "channel", channel, "error", err)
		http.Error(w, "Failed to send test notification", http.StatusInternalServerError)
		return
	}

	slog.Info("test notification sent", "source", "settings", "channel", channel)
	if _, err := w.Write([]byte("✅ Test sent!")); err != nil {
		slog.Error("write response", "error", err)
	}
}

func (h *Handler) setLogLevel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Level string `json:"level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	level, ok := logstore.ParseLogLevel(req.Level)
	if !ok {
		http.Error(w, "invalid level, use: debug, info, warn, error", http.StatusBadRequest)
		return
	}

	prev := logstore.LevelName(h.logStore.Level())
	h.logStore.SetLevel(level)

	slog.Info("log level changed", "from", prev, "to", req.Level)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"level": req.Level,
	}); err != nil {
		slog.Error("encode response", "error", err)
	}
}
