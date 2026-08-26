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
	Title       string
	Fields      []configField
	TestSection string // section slug for the "Test" button (POST /settings/config/test/{section}); empty = no button
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
		"Title":         "Datey - Settings",
		"SettingsTab":   "config",
		"ConfigGroups":  buildConfigGroups(cfg, submitted, errs),
		"ImmichEnabled": h.immich.Enabled(),
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

	h.auditRecord(r, "config.save", "")
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
		Name:     "DATA_DIR",
		Label:    "Data Directory",
		Value:    cfg.DataDir,
		Type:     "readonly",
		Help:     "Set via the DATA_DIR environment variable. Determines where the SQLite database lives and cannot be changed at runtime.",
		ReadOnly: true,
	}

	general := configGroup{Title: "General", TestSection: "general", Fields: []configField{
		{Name: "PORT", Label: "Server Port", Value: val("PORT", strconv.Itoa(cfg.Port)), Type: "number", RestartRequired: true, Help: "Requires restart to apply.", Error: errFor("PORT")},
		dataDir,
		{Name: "SCHEDULER_HOUR", Label: "Scheduler Hour (0-23)", Value: val("SCHEDULER_HOUR", strconv.Itoa(cfg.SchedulerHour)), Type: "number", RestartRequired: true, Help: "Daily reminder run hour. Requires restart to apply.", Error: errFor("SCHEDULER_HOUR")},
		{Name: "REMINDER_DAYS", Label: "Reminder Window (1-365 days)", Value: val("REMINDER_DAYS", strconv.Itoa(cfg.ReminderDays)), Type: "number", Error: errFor("REMINDER_DAYS")},
		{Name: "SCHEDULER_CATCHUP", Label: "Catch Up Missed Reminders", Type: "checkbox", Checked: checked("SCHEDULER_CATCHUP", cfg.SchedulerCatchup), Help: "On startup, send reminders for dates that fell inside the reminder window while the server was offline. Past dates use \"was N days ago\" phrasing; already-sent notifications are never repeated.", Error: errFor("SCHEDULER_CATCHUP")},
		{Name: "DATE_VARIANT", Label: "Date Variant", Value: val("DATE_VARIANT", cfg.DateVariant), Type: "select", Options: []string{"european", "us"}, Selected: val("DATE_VARIANT", cfg.DateVariant), Help: "How dates are displayed: european is day-first (\"25 Dec\"), us is month-first (\"Dec 25\").", Error: errFor("DATE_VARIANT")},
		{Name: "LOG_LEVEL", Label: "Log Level", Value: val("LOG_LEVEL", cfg.LogLevel), Type: "select", Options: []string{"debug", "info", "warn", "error"}, Selected: val("LOG_LEVEL", cfg.LogLevel), Error: errFor("LOG_LEVEL")},
		{Name: "LOG_BUFFER_SIZE", Label: "Log Buffer Size", Value: val("LOG_BUFFER_SIZE", strconv.Itoa(cfg.LogBufferSize)), Type: "number", RestartRequired: true, Help: "In-memory ring buffer entries. Requires restart to apply.", Error: errFor("LOG_BUFFER_SIZE")},
		{Name: "EINK_MODE", Label: "Force E-Ink Mode", Type: "checkbox", Checked: checked("EINK_MODE", cfg.EinkMode), Help: "Enables high-contrast E-Ink theme for all users."},
	}}

	backup := configGroup{Title: "Backups", TestSection: "backup", Fields: []configField{
		{Name: "BACKUP_DIR", Label: "Backup Directory", Value: val("BACKUP_DIR", cfg.BackupDir), Type: "text", Error: errFor("BACKUP_DIR")},
		{Name: "BACKUP_RETENTION_DAYS", Label: "Backup Retention (days)", Value: val("BACKUP_RETENTION_DAYS", strconv.Itoa(cfg.BackupRetentionDays)), Type: "number", Error: errFor("BACKUP_RETENTION_DAYS")},
	}}

	email := configGroup{Title: "Email (SMTP)", TestSection: "email", Fields: []configField{
		{Name: "SMTP_HOST", Label: "SMTP Host", Value: val("SMTP_HOST", cfg.SMTPHost), Type: "text", Error: errFor("SMTP_HOST")},
		{Name: "SMTP_PORT", Label: "SMTP Port", Value: val("SMTP_PORT", strconv.Itoa(cfg.SMTPPort)), Type: "number", Error: errFor("SMTP_PORT")},
		{Name: "SMTP_USER", Label: "SMTP User", Value: val("SMTP_USER", cfg.SMTPUser), Type: "text", Error: errFor("SMTP_USER")},
		{Name: "SMTP_PASS", Label: "SMTP Password", Value: val("SMTP_PASS", cfg.SMTPPass), Type: "text", Secret: true, Error: errFor("SMTP_PASS")},
		{Name: "SMTP_TLS", Label: "Use TLS", Type: "checkbox", Checked: checked("SMTP_TLS", cfg.SMTPTLS)},
		{Name: "SMTP_TIMEOUT", Label: "SMTP Timeout (seconds)", Value: val("SMTP_TIMEOUT", strconv.Itoa(cfg.SMTPTimeout)), Type: "number", Error: errFor("SMTP_TIMEOUT")},
		{Name: "NOTIFICATION_EMAIL", Label: "Notification Email (recipient)", Value: val("NOTIFICATION_EMAIL", cfg.NotifyEmail), Type: "text", Error: errFor("NOTIFICATION_EMAIL")},
	}}

	gotify := configGroup{Title: "Gotify", TestSection: "gotify", Fields: []configField{
		{Name: "GOTIFY_URL", Label: "Gotify URL", Value: val("GOTIFY_URL", cfg.GotifyURL), Type: "text", Error: errFor("GOTIFY_URL")},
		{Name: "GOTIFY_TOKEN", Label: "Gotify Token", Value: val("GOTIFY_TOKEN", cfg.GotifyToken), Type: "text", Secret: true, Error: errFor("GOTIFY_TOKEN")},
	}}

	telegram := configGroup{Title: "Telegram", TestSection: "telegram", Fields: []configField{
		{Name: "TELEGRAM_BOT_TOKEN", Label: "Bot Token", Value: val("TELEGRAM_BOT_TOKEN", cfg.TelegramBotToken), Type: "text", Secret: true, Error: errFor("TELEGRAM_BOT_TOKEN")},
		{Name: "TELEGRAM_CHAT_ID", Label: "Chat ID", Value: val("TELEGRAM_CHAT_ID", cfg.TelegramChatID), Type: "text", Error: errFor("TELEGRAM_CHAT_ID")},
	}}

	ntfy := configGroup{Title: "ntfy", TestSection: "ntfy", Fields: []configField{
		{Name: "NTFY_URL", Label: "ntfy Server URL", Value: val("NTFY_URL", cfg.NtfyURL), Type: "text", Help: "Base URL of your ntfy server. Defaults to https://ntfy.sh.", Error: errFor("NTFY_URL")},
		{Name: "NTFY_TOPIC", Label: "Topic", Value: val("NTFY_TOPIC", cfg.NtfyTopic), Type: "text", Help: "Topic to publish reminders to. Required to enable ntfy.", Error: errFor("NTFY_TOPIC")},
		{Name: "NTFY_TOKEN", Label: "Access Token", Value: val("NTFY_TOKEN", cfg.NtfyToken), Type: "text", Secret: true, Help: "Optional bearer token for authenticated ntfy servers.", Error: errFor("NTFY_TOKEN")},
		{Name: "NTFY_PRIORITY", Label: "Priority (1-5)", Value: val("NTFY_PRIORITY", strconv.Itoa(cfg.NtfyPriority)), Type: "number", Help: "1 = min, 3 = default, 5 = max.", Error: errFor("NTFY_PRIORITY")},
	}}

	webhook := configGroup{Title: "Webhook", TestSection: "webhook", Fields: []configField{
		{Name: "WEBHOOK_URL", Label: "Webhook URLs", Value: val("WEBHOOK_URL", cfg.WebhookURL), Type: "text", Help: "Comma-separated list of URLs that receive a JSON POST per reminder. Required to enable webhook.", Error: errFor("WEBHOOK_URL")},
		{Name: "WEBHOOK_SECRET", Label: "Webhook Secret", Value: val("WEBHOOK_SECRET", cfg.WebhookSecret), Type: "text", Secret: true, Help: "Optional secret used to sign requests (X-Datey-Signature: sha256=...).", Error: errFor("WEBHOOK_SECRET")},
	}}

	discord := configGroup{Title: "Discord", TestSection: "discord", Fields: []configField{
		{Name: "DISCORD_WEBHOOK_URL", Label: "Discord Webhook URL", Value: val("DISCORD_WEBHOOK_URL", cfg.DiscordWebhookURL), Type: "text", Secret: true, Help: "Discord channel webhook URL. Create in Server Settings → Integrations → Webhooks.", Error: errFor("DISCORD_WEBHOOK_URL")},
	}}

	slack := configGroup{Title: "Slack", TestSection: "slack", Fields: []configField{
		{Name: "SLACK_WEBHOOK_URL", Label: "Slack Webhook URL", Value: val("SLACK_WEBHOOK_URL", cfg.SlackWebhookURL), Type: "text", Secret: true, Help: "Slack incoming webhook URL. Create in Slack App → Incoming Webhooks.", Error: errFor("SLACK_WEBHOOK_URL")},
	}}

	matrix := configGroup{Title: "Matrix", TestSection: "matrix", Fields: []configField{
		{Name: "MATRIX_HOMESERVER_URL", Label: "Homeserver URL", Value: val("MATRIX_HOMESERVER_URL", cfg.MatrixHomeserverURL), Type: "text", Help: "Matrix homeserver URL, e.g. https://matrix.example.com.", Error: errFor("MATRIX_HOMESERVER_URL")},
		{Name: "MATRIX_ACCESS_TOKEN", Label: "Access Token", Value: val("MATRIX_ACCESS_TOKEN", cfg.MatrixAccessToken), Type: "text", Secret: true, Help: "Matrix access token. Generate in Element → Settings → Help & About → Access Token.", Error: errFor("MATRIX_ACCESS_TOKEN")},
		{Name: "MATRIX_ROOM_ID", Label: "Room ID", Value: val("MATRIX_ROOM_ID", cfg.MatrixRoomID), Type: "text", Help: "Matrix room ID, e.g. !abc123:example.com.", Error: errFor("MATRIX_ROOM_ID")},
	}}

	analytics := configGroup{Title: "Analytics", TestSection: "analytics", Fields: []configField{
		{Name: "UMAMI_URL", Label: "Umami URL", Value: val("UMAMI_URL", cfg.UmamiURL), Type: "text", Error: errFor("UMAMI_URL")},
		{Name: "UMAMI_WEBSITE_ID", Label: "Umami Website ID", Value: val("UMAMI_WEBSITE_ID", cfg.UmamiWebsiteID), Type: "text", Error: errFor("UMAMI_WEBSITE_ID")},
	}}

	obs := configGroup{Title: "Observability", TestSection: "observability", Fields: []configField{
		{Name: "OTEL_ENDPOINT", Label: "OTLP Endpoint", Value: val("OTEL_ENDPOINT", cfg.OTLPEndpoint), Type: "text", RestartRequired: true, Help: "Requires restart to apply.", Error: errFor("OTEL_ENDPOINT")},
	}}

	icalFeedKey := cfg.ICalFeedKey
	if submitted != nil {
		if v, ok := submitted["ICAL_FEED_KEY"]; ok && len(v) > 0 {
			icalFeedKey = v[0]
		}
	}

	ical := configGroup{Title: "iCal Feed", TestSection: "ical", Fields: []configField{
		{Name: "ICAL_FEED_ENABLED", Label: "Enable Public iCal Feed", Type: "checkbox", Checked: checked("ICAL_FEED_ENABLED", cfg.ICalEnabled), Help: "Exposes all tracked dates as a public iCal feed (e.g. for Google Calendar). Disabled by default; dates are personal data."},
		{Name: "ICAL_EVENT_START", Label: "Event Start Time", Value: val("ICAL_EVENT_START", cfg.ICalEventStart), Type: "text", Help: "Start time in 24h HH:MM format (e.g. 09:00), or leave empty for all-day events.", Error: errFor("ICAL_EVENT_START")},
		{Name: "ICAL_EVENT_DURATION", Label: "Event Duration (minutes)", Value: val("ICAL_EVENT_DURATION", strconv.Itoa(cfg.ICalDurationMinutes)), Type: "number", Help: "How long each event lasts (1-1440). Used when a start time is set.", Error: errFor("ICAL_EVENT_DURATION")},
		{Name: "ICAL_FEED_KEY", Label: "Feed Secret Key", Value: val("ICAL_FEED_KEY", icalFeedKey), Type: "text", Help: "Required as ?key=... in feed URLs. Auto-generated on first enable; change it here to rotate."},
		{Name: "ICAL_FEED_URL_GLOBAL", Label: "iCal Feed URL (all dates)", Value: "/ical.ics?key=" + icalFeedKey, Type: "readonly", ReadOnly: true, Help: "Subscribe in Google Calendar. Prepend your server origin (e.g. https://datey.example.com)."},
		{Name: "ICAL_FEED_URL_PERSON", Label: "iCal Feed URL (single person)", Value: "/ical/{personID}.ics?key=" + icalFeedKey, Type: "readonly", ReadOnly: true, Help: "Replace {personID} with the person's ID (the number in their page URL)."},
	}}

	rssFeedKey := cfg.RSSFeedKey
	if submitted != nil {
		if v, ok := submitted["RSS_FEED_KEY"]; ok && len(v) > 0 {
			rssFeedKey = v[0]
		}
	}

	rssGroup := configGroup{Title: "RSS Feed", TestSection: "rss", Fields: []configField{
		{Name: "RSS_FEED_ENABLED", Label: "Enable Public RSS Feed", Type: "checkbox", Checked: checked("RSS_FEED_ENABLED", cfg.RSSEnabled), Help: "Exposes upcoming events as an RSS 2.0 feed for feed readers and aggregators. Disabled by default; dates are personal data."},
		{Name: "RSS_FEED_KEY", Label: "Feed Secret Key", Value: val("RSS_FEED_KEY", rssFeedKey), Type: "text", Help: "Required as ?key=... in the feed URL. Auto-generated on first enable; change it here to rotate."},
		{Name: "RSS_FEED_URL", Label: "RSS Feed URL", Value: "/rss.xml?key=" + rssFeedKey, Type: "readonly", ReadOnly: true, Help: "Subscribe in your feed reader. Prepend your server origin (e.g. https://datey.example.com)."},
	}}

	upcomingAPIKey := cfg.UpcomingAPIKey
	if submitted != nil {
		if v, ok := submitted["UPCOMING_API_KEY"]; ok && len(v) > 0 {
			upcomingAPIKey = v[0]
		}
	}

	upcomingAPIGroup := configGroup{Title: "Upcoming Events API", TestSection: "upcoming", Fields: []configField{
		{Name: "UPCOMING_API_ENABLED", Label: "Enable Upcoming Events API", Type: "checkbox", Checked: checked("UPCOMING_API_ENABLED", cfg.UpcomingAPIEnabled), Help: "Exposes upcoming events as JSON for scripts, dashboards and automations. Disabled by default; dates are personal data."},
		{Name: "UPCOMING_API_KEY", Label: "API Secret Key", Value: val("UPCOMING_API_KEY", upcomingAPIKey), Type: "text", Help: "Required as ?key=... in the endpoint URL. Auto-generated on first enable; change it here to rotate."},
		{Name: "UPCOMING_API_URL", Label: "API Endpoint URL", Value: "/api/upcoming?key=" + upcomingAPIKey, Type: "readonly", ReadOnly: true, Help: "Optionally add &days=N to override the horizon (default: reminder window, max 365)."},
	}}

	homeAssistantKey := cfg.HomeAssistantKey
	if submitted != nil {
		if v, ok := submitted["HOMEASSISTANT_KEY"]; ok && len(v) > 0 {
			homeAssistantKey = v[0]
		}
	}

	homeAssistantGroup := configGroup{Title: "Home Assistant", TestSection: "homeassistant", Fields: []configField{
		{Name: "HOMEASSISTANT_ENABLED", Label: "Enable Home Assistant Feed", Type: "checkbox", Checked: checked("HOMEASSISTANT_ENABLED", cfg.HomeAssistantEnabled), Help: "Exposes upcoming events as a JSON stats feed for a Home Assistant RESTful sensor. Disabled by default; dates are personal data."},
		{Name: "HOMEASSISTANT_KEY", Label: "Feed Secret Key", Value: val("HOMEASSISTANT_KEY", homeAssistantKey), Type: "text", Help: "Required as ?key=... in the feed URL. Auto-generated on first enable; change it here to rotate."},
		{Name: "HOMEASSISTANT_URL", Label: "Stats Feed URL", Value: "/api/homeassistant/stats?key=" + homeAssistantKey, Type: "readonly", ReadOnly: true, Help: "Use in the RESTful sensor's resource. Prepend your server origin (e.g. http://datey.local:6270)."},
		{Name: "HOMEASSISTANT_CALENDAR_URL", Label: "Calendar Feed URL", Value: "/api/homeassistant/calendar?key=" + homeAssistantKey + "&start=YYYY-MM-DD&end=YYYY-MM-DD", Type: "readonly", ReadOnly: true, Help: "Use for the calendar entity (all-day events). Replace dates with your window (max 365 days) or omit start/end to use the reminder window. Prepend your server origin."},
	}}

	pushGroup := configGroup{Title: "Web Push", TestSection: "webpush", Fields: []configField{
		{Name: "PUSH_ENABLED", Label: "Enable Web Push Notifications", Type: "checkbox", Checked: checked("PUSH_ENABLED", cfg.PushEnabled), Help: "Send reminders as browser notifications via Web Push. Requires HTTPS (or localhost). VAPID keys are generated automatically on first enable."},
		{Name: "PUSH_VAPID_PUBLIC_KEY", Label: "VAPID Public Key", Value: val("PUSH_VAPID_PUBLIC_KEY", cfg.PushVAPIDPublicKey), Type: "readonly", ReadOnly: true, Help: "Public part of the VAPID key pair; served to browsers to establish subscriptions."},
		{Name: "PUSH_VAPID_PRIVATE_KEY", Label: "VAPID Private Key", Value: val("PUSH_VAPID_PRIVATE_KEY", cfg.PushVAPIDPrivateKey), Type: "text", Secret: true, Help: "Secret signing key. Never share it; rotate by entering a new key or clearing this field and saving while enabled."},
	}}
	immichKeyValue := ""
	if submitted != nil {
		immichKeyValue = val("IMMICH_API_KEY", "")
	}
	immichGroup := configGroup{Title: "Immich", TestSection: "immich", Fields: []configField{
		{Name: "IMMICH_URL", Label: "Immich URL", Value: val("IMMICH_URL", cfg.ImmichURL), Type: "text", Help: "Optional Immich server URL, for example https://photos.example.com.", Error: errFor("IMMICH_URL")},
		{Name: "IMMICH_API_KEY", Label: "Immich API Key", Value: immichKeyValue, Type: "text", Secret: true, Help: "Leave blank to keep the saved key. Used only by the server.", Error: errFor("IMMICH_API_KEY")},
	}}

	return []configGroup{general, backup, email, gotify, telegram, ntfy, webhook, discord, slack, matrix, analytics, obs, ical, rssGroup, upcomingAPIGroup, homeAssistantGroup, pushGroup, immichGroup}
}
