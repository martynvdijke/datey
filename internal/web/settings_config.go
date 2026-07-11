package web

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/settings"
)

// configField describes a single editable setting for the admin config form.
type configField struct {
	Name            string // form field name (matches the env var)
	Label           string
	Value           string // current value as a string for rendering
	Type            string // text, number, checkbox, select, readonly
	Help            string
	RestartRequired bool
	ReadOnly        bool
	Secret          bool
	Error           string
	Options         []string // for select
	Checked         bool     // for checkbox
	Selected        string   // current selected option value (for select)
}

// configGroup bundles related fields under a heading in the form.
type configGroup struct {
	Title  string
	Fields []configField
}

// settingsConfig renders the admin Configuration tab as an editable form.
// On a validation error from a prior POST, the submitted values and
// per-field errors are re-rendered so the admin can correct them.
func (h *Handler) settingsConfig(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfg

	submitted := url.Values{}
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		submitted = r.PostForm
	}
	errs, _ := r.Context().Value(configFormErrorsKey{}).(map[string]string)

	h.render(w, r, "settings.html", map[string]any{
		"Title":        "Datey - Settings",
		"SettingsTab":  "config",
		"ConfigGroups": buildConfigGroups(cfg, submitted, errs),
	})
}

// settingsConfigSave handles POST /settings/config. It validates, persists to
// the app_config row, applies hot-reloadable fields to the in-memory config,
// and re-renders the form with errors on validation failure.
func (h *Handler) settingsConfigSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	errs, err := h.settingsStore.ApplyForm(r.Context(), h.cfg, r.PostForm)
	if err != nil && err != settings.ErrInvalid {
		slog.Error("save app_config", "error", err)
		http.Error(w, "failed to save settings", http.StatusInternalServerError)
		return
	}
	if len(errs) > 0 {
		ctx := contextWithFormErrors(r.Context(), errs)
		req := r.WithContext(ctx)
		req.Method = http.MethodPost
		h.settingsConfig(w, req)
		return
	}

	// Hot-reload the live log level so the ring buffer / slog handler follow
	// the DB-stored value immediately.
	if level, ok := logstore.ParseLogLevel(h.cfg.LogLevel); ok {
		h.logStore.SetLevel(level)
	}

	toastHeader(w, "Settings saved. Restart-required fields apply on next restart.", "success")
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

// contextWithFormErrors / configFormErrorsKey let the POST handler pass
// per-field errors into the GET re-render without a redirect.
type configFormErrorsKey struct{}

func contextWithFormErrors(ctx context.Context, errs map[string]string) context.Context {
	return context.WithValue(ctx, configFormErrorsKey{}, errs)
}

// buildConfigGroups constructs the grouped form descriptor from the live cfg,
// overlaying submitted form values (on a POST re-render) and per-field errors.
func buildConfigGroups(cfg *config.Config, submitted url.Values, errs map[string]string) []configGroup {
	val := func(envKey, current string) string {
		if submitted != nil {
			if v, ok := submitted[envKey]; ok && len(v) > 0 {
				return v[0]
			}
		}
		return current
	}
	checked := func(envKey string, current bool) bool {
		if submitted != nil {
			if _, ok := submitted[envKey]; ok {
				return true
			}
			return false
		}
		return current
	}
	errFor := func(key string) string {
		if errs == nil {
			return ""
		}
		return errs[key]
	}

	dataDir := configField{
		Name:    "DATA_DIR",
		Label:   "Data Directory",
		Value:   cfg.DataDir,
		Type:    "readonly",
		Help:    "Set via the DATA_DIR environment variable. Determines where the SQLite database lives and cannot be changed at runtime.",
		ReadOnly: true,
	}

	general := configGroup{Title: "General", Fields: []configField{
		{Name: "PORT", Label: "Server Port", Value: val("PORT", strconv.Itoa(cfg.Port)), Type: "number", RestartRequired: true, Help: "Requires restart to apply.", Error: errFor("PORT")},
		dataDir,
		{Name: "SCHEDULER_HOUR", Label: "Scheduler Hour (0-23)", Value: val("SCHEDULER_HOUR", strconv.Itoa(cfg.SchedulerHour)), Type: "number", RestartRequired: true, Help: "Daily reminder run hour. Requires restart to apply.", Error: errFor("SCHEDULER_HOUR")},
		{Name: "REMINDER_DAYS", Label: "Reminder Window (1-365 days)", Value: val("REMINDER_DAYS", strconv.Itoa(cfg.ReminderDays)), Type: "number", Error: errFor("REMINDER_DAYS")},
		{Name: "LOG_LEVEL", Label: "Log Level", Value: val("LOG_LEVEL", cfg.LogLevel), Type: "select", Options: []string{"debug", "info", "warn", "error"}, Selected: val("LOG_LEVEL", cfg.LogLevel), Error: errFor("LOG_LEVEL")},
		{Name: "LOG_BUFFER_SIZE", Label: "Log Buffer Size", Value: val("LOG_BUFFER_SIZE", strconv.Itoa(cfg.LogBufferSize)), Type: "number", RestartRequired: true, Help: "In-memory ring buffer entries. Requires restart to apply.", Error: errFor("LOG_BUFFER_SIZE")},
		{Name: "EINK_MODE", Label: "Force E-Ink Mode", Type: "checkbox", Checked: checked("EINK_MODE", cfg.EinkMode), Help: "Enables high-contrast E-Ink theme for all users."},
	}}

	backup := configGroup{Title: "Backups", Fields: []configField{
		{Name: "BACKUP_DIR", Label: "Backup Directory", Value: val("BACKUP_DIR", cfg.BackupDir), Type: "text", Error: errFor("BACKUP_DIR")},
		{Name: "BACKUP_RETENTION_DAYS", Label: "Backup Retention (days)", Value: val("BACKUP_RETENTION_DAYS", strconv.Itoa(cfg.BackupRetentionDays)), Type: "number", Error: errFor("BACKUP_RETENTION_DAYS")},
	}}

	email := configGroup{Title: "Email (SMTP)", Fields: []configField{
		{Name: "SMTP_HOST", Label: "SMTP Host", Value: val("SMTP_HOST", cfg.SMTPHost), Type: "text", Error: errFor("SMTP_HOST")},
		{Name: "SMTP_PORT", Label: "SMTP Port", Value: val("SMTP_PORT", strconv.Itoa(cfg.SMTPPort)), Type: "number", Error: errFor("SMTP_PORT")},
		{Name: "SMTP_USER", Label: "SMTP User", Value: val("SMTP_USER", cfg.SMTPUser), Type: "text", Error: errFor("SMTP_USER")},
		{Name: "SMTP_PASS", Label: "SMTP Password", Value: val("SMTP_PASS", cfg.SMTPPass), Type: "text", Secret: true, Error: errFor("SMTP_PASS")},
		{Name: "SMTP_TLS", Label: "Use TLS", Type: "checkbox", Checked: checked("SMTP_TLS", cfg.SMTPTLS)},
		{Name: "SMTP_TIMEOUT", Label: "SMTP Timeout (seconds)", Value: val("SMTP_TIMEOUT", strconv.Itoa(cfg.SMTPTimeout)), Type: "number", Error: errFor("SMTP_TIMEOUT")},
		{Name: "NOTIFICATION_EMAIL", Label: "Notification Email (recipient)", Value: val("NOTIFICATION_EMAIL", cfg.NotifyEmail), Type: "text", Error: errFor("NOTIFICATION_EMAIL")},
	}}

	gotify := configGroup{Title: "Gotify", Fields: []configField{
		{Name: "GOTIFY_URL", Label: "Gotify URL", Value: val("GOTIFY_URL", cfg.GotifyURL), Type: "text", Error: errFor("GOTIFY_URL")},
		{Name: "GOTIFY_TOKEN", Label: "Gotify Token", Value: val("GOTIFY_TOKEN", cfg.GotifyToken), Type: "text", Secret: true, Error: errFor("GOTIFY_TOKEN")},
	}}

	telegram := configGroup{Title: "Telegram", Fields: []configField{
		{Name: "TELEGRAM_BOT_TOKEN", Label: "Bot Token", Value: val("TELEGRAM_BOT_TOKEN", cfg.TelegramBotToken), Type: "text", Secret: true, Error: errFor("TELEGRAM_BOT_TOKEN")},
		{Name: "TELEGRAM_CHAT_ID", Label: "Chat ID", Value: val("TELEGRAM_CHAT_ID", cfg.TelegramChatID), Type: "text", Error: errFor("TELEGRAM_CHAT_ID")},
	}}

	analytics := configGroup{Title: "Analytics", Fields: []configField{
		{Name: "UMAMI_URL", Label: "Umami URL", Value: val("UMAMI_URL", cfg.UmamiURL), Type: "text", Error: errFor("UMAMI_URL")},
		{Name: "UMAMI_WEBSITE_ID", Label: "Umami Website ID", Value: val("UMAMI_WEBSITE_ID", cfg.UmamiWebsiteID), Type: "text", Error: errFor("UMAMI_WEBSITE_ID")},
	}}

	obs := configGroup{Title: "Observability", Fields: []configField{
		{Name: "OTEL_ENDPOINT", Label: "OTLP Endpoint", Value: val("OTEL_ENDPOINT", cfg.OTLPEndpoint), Type: "text", RestartRequired: true, Help: "Requires restart to apply.", Error: errFor("OTEL_ENDPOINT")},
	}}

	return []configGroup{general, backup, email, gotify, telegram, analytics, obs}
}