package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/datey/datey/internal/db"
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/notifier"
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
	}

	h.render(w, r, "settings.html", map[string]any{
		"Title":       "Datey - Settings",
		"SettingsTab": "notifications",
		"Channels":    channels,
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
	json.NewEncoder(w).Encode(map[string]bool{"eink_mode": newVal})
}

func (h *Handler) settingsConfig(w http.ResponseWriter, r *http.Request) {
	type configItem struct {
		Key   string
		Value string
	}

	cfg := h.cfg
	items := []configItem{
		{"Port", fmt.Sprintf("%d", cfg.Port)},
		{"DataDir", cfg.DataDir},
		{"SchedulerHour", fmt.Sprintf("%d", cfg.SchedulerHour)},
		{"ReminderDays", fmt.Sprintf("%d", cfg.ReminderDays)},
		{"LogLevel", cfg.LogLevel},
		{"LogBufferSize", fmt.Sprintf("%d", cfg.LogBufferSize)},
		{"OTLPEndpoint", maskValue(cfg.OTLPEndpoint)},
		{"SMTPHost", cfg.SMTPHost},
		{"SMTPPort", fmt.Sprintf("%d", cfg.SMTPPort)},
		{"SMTPUser", cfg.SMTPUser},
		{"SMTPPass", maskValue(cfg.SMTPPass)},
		{"SMTPTLS", fmt.Sprintf("%v", cfg.SMTPTLS)},
		{"NotifyEmail", maskEmail(cfg.NotifyEmail)},
		{"GotifyURL", cfg.GotifyURL},
		{"GotifyToken", maskValue(cfg.GotifyToken)},
		{"TelegramBotToken", maskValue(cfg.TelegramBotToken)},
		{"TelegramChatID", cfg.TelegramChatID},
		{"UmamiURL", cfg.UmamiURL},
		{"UmamiWebsiteID", cfg.UmamiWebsiteID},
		{"BackupDir", cfg.BackupDir},
		{"BackupRetentionDays", fmt.Sprintf("%d", cfg.BackupRetentionDays)},
		{"EinkMode", fmt.Sprintf("%v", cfg.EinkMode)},
	}

	h.render(w, r, "settings.html", map[string]any{
		"Title":       "Datey - Settings",
		"SettingsTab": "config",
		"ConfigItems": items,
	})
}

func maskValue(s string) string {
	if s == "" {
		return "\u2014"
	}
	return "****"
}

// maskEmail partially masks an email address, showing only the first character
// and domain, e.g. "j***@example.com". Returns the original string if it
// doesn't contain '@'.
func maskEmail(s string) string {
	if s == "" {
		return "\u2014"
	}
	at := strings.Index(s, "@")
	if at < 1 {
		return maskValue(s)
	}
	return s[:1] + "***" + s[at:]
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
		w.Write([]byte(`<div class="alert alert-danger">Backup failed: ` + err.Error() + `</div>`))
		return
	}
	slog.Info("manual backup completed", "dir", h.cfg.BackupDir)
	w.Write([]byte(`<div class="alert alert-success">Backup completed successfully!</div>`))
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

	var err error
	switch channel {
	case "email":
		n := notifier.NewEmailNotifier(h.cfg)
		err = n.Send(r.Context(), title, message)
	case "gotify":
		n := notifier.NewGotifyNotifier(h.cfg)
		err = n.Send(r.Context(), title, message)
	case "telegram":
		n := notifier.NewTelegramNotifier(h.cfg)
		err = n.Send(r.Context(), title, message)
	default:
		slog.Warn("test notification: unknown channel", "source", "settings", "channel", channel)
		http.Error(w, "unknown channel", http.StatusBadRequest)
		return
	}

	if err != nil {
		slog.Error("test notification failed", "source", "settings", "channel", channel, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Info("test notification sent", "source", "settings", "channel", channel)
	w.Write([]byte("✅ Test sent!"))
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
	json.NewEncoder(w).Encode(map[string]any{
		"level": req.Level,
	})
}
