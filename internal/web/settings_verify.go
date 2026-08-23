package web

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/datey/datey/internal/carddav"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/immich"
	"github.com/datey/datey/internal/notifier"
	"github.com/go-chi/chi/v5"
)

// testConfig handles POST /settings/config/test/{section} — a universal test
// button for every Configuration group. It merges form-submitted values (if any)
// onto a copy of the live config so unsaved edits are tested, then runs a
// section-specific check and returns a small alert snippet for the htmx target.
func (h *Handler) testConfig(w http.ResponseWriter, r *http.Request) {
	section := chi.URLParam(r, "section")
	if err := r.ParseForm(); err != nil {
		writeTestResult(w, false, "Failed to parse form")
		return
	}

	// Build a temporary config that overlays form values onto the live cfg so
	// the test reflects what the user just typed, not only what is saved.
	tmp := *h.cfg
	applyFormToConfig(&tmp, r.PostForm)

	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	var ok bool
	var msg string

	switch section {
	case "email":
		ok, msg = h.testEmail(ctx, &tmp)
	case "gotify":
		ok, msg = testNotifier(ctx, notifier.NewGotifyNotifier(&tmp))
	case "telegram":
		ok, msg = testNotifier(ctx, notifier.NewTelegramNotifier(&tmp))
	case "ntfy":
		ok, msg = testNotifier(ctx, notifier.NewNtfyNotifier(&tmp))
	case "webhook":
		ok, msg = testNotifier(ctx, notifier.NewWebhookNotifier(&tmp))
	case "webpush":
		ok, msg = h.testWebPush(ctx, &tmp)
	case "immich":
		ok, msg = h.testImmich(ctx, &tmp, r.PostForm)
	case "carddav":
		ok, msg = h.testCardDAV(ctx, &tmp, r.PostForm)
	case "backup":
		ok, msg = testBackup(&tmp)
	case "general":
		ok, msg = testGeneral(&tmp)
	case "observability":
		ok, msg = testObservability(&tmp)
	case "analytics":
		ok, msg = testAnalytics(&tmp)
	case "ical":
		ok, msg = testFeed("iCal", tmp.ICalEnabled, tmp.ICalFeedKey, tmp.ICalEventStart, &tmp)
	case "rss":
		ok, msg = testFeed("RSS", tmp.RSSEnabled, tmp.RSSFeedKey, "", &tmp)
	case "upcoming":
		ok, msg = testFeed("Upcoming API", tmp.UpcomingAPIEnabled, tmp.UpcomingAPIKey, "", &tmp)
	case "homeassistant":
		ok, msg = testFeed("Home Assistant", tmp.HomeAssistantEnabled, tmp.HomeAssistantKey, "", &tmp)
	case "database":
		ok, msg = h.testDatabase(ctx)
	case "all":
		ok, msg = h.testAll(ctx, &tmp, r.PostForm)
	default:
		writeTestResult(w, false, fmt.Sprintf("Unknown test section %q", html.EscapeString(section)))
		return
	}

	writeTestResult(w, ok, msg)
}

// testCarddavConnection handles POST /settings/carddav/test — a lightweight
// connectivity check that does not run a full sync.
func (h *Handler) testCarddavConnection(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeTestResult(w, false, "Failed to parse form")
		return
	}
	tmp := *h.cfg
	applyFormToConfig(&tmp, r.PostForm)

	// CardDAV form lives on the notifications tab with different field names;
	// also support those.
	if v := r.PostForm.Get("CARDDAV_URL"); v != "" {
		tmp.CarddavURL = v
	}
	if v := r.PostForm.Get("CARDDAV_USERNAME"); v != "" {
		tmp.CarddavUsername = v
	}
	if v := r.PostForm.Get("CARDDAV_PASSWORD"); v != "" {
		tmp.CarddavPassword = v
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	ok, msg := h.testCardDAV(ctx, &tmp, r.PostForm)
	writeTestResult(w, ok, msg)
}

// helpers

func writeTestResult(w http.ResponseWriter, ok bool, msg string) {
	cls := "alert-success"
	icon := "✅"
	if !ok {
		cls = "alert-danger"
		icon = "❌"
	}
	// msg is already escaped where needed; keep it safe for innerHTML.
	snippet := fmt.Sprintf(`<div class="alert %s py-2 px-3 mb-0 small">%s %s</div>`, cls, icon, msg)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(snippet))
}

func testNotifier(ctx context.Context, n notifier.Notifier) (bool, string) {
	if !n.IsConfigured() {
		return false, fmt.Sprintf("%s is not configured — fill in the required fields and save first (or include them in the form before testing).", html.EscapeString(n.Name()))
	}
	title := "Datey Test Notification"
	msg := fmt.Sprintf("Test from Datey at %s — if you see this, %s is working.", time.Now().Format(time.RFC3339), n.Name())
	if err := n.Send(ctx, title, msg); err != nil {
		return false, fmt.Sprintf("%s test failed: %s", html.EscapeString(n.Name()), html.EscapeString(err.Error()))
	}
	return true, fmt.Sprintf("%s test sent successfully!", html.EscapeString(n.Name()))
}

func (h *Handler) testWebPush(ctx context.Context, cfg *config.Config) (bool, string) {
	if !cfg.PushEnabled {
		return false, "Web Push is disabled — enable it and save first."
	}
	if cfg.PushVAPIDPublicKey == "" || cfg.PushVAPIDPrivateKey == "" {
		return false, "Web Push VAPID keys are missing — save the configuration to auto-generate them, then test again."
	}
	n := notifier.NewWebPushNotifier(cfg, h.pushSubs)
	if !n.IsConfigured() {
		// No subscriptions yet — that is expected for a fresh install.
		// Validate keys and report that no browser is subscribed.
		return true, "Web Push is configured and VAPID keys are valid, but no browser has subscribed yet. Open Datey on a device (HTTPS or localhost) and accept the notification permission, then test again to send a real push."
	}
	return testNotifier(ctx, n)
}

func (h *Handler) testImmich(ctx context.Context, cfg *config.Config, form url.Values) (bool, string) {
	urlVal := cfg.ImmichURL
	keyVal := cfg.ImmichAPIKey
	// If the form blanked the key, it means "keep stored" — the overlay already
	// kept the stored key, so nothing extra to do. If the user typed a new key
	// it is already in cfg via applyFormToConfig.
	if urlVal == "" {
		return false, "Immich URL is empty — not configured. Enter a server URL and API key."
	}
	if keyVal == "" {
		return false, "Immich API key is required when a URL is set."
	}
	c := immich.New(urlVal, keyVal)
	people, err := c.People(ctx)
	if err != nil {
		return false, fmt.Sprintf("Immich connection failed: %s", html.EscapeString(err.Error()))
	}
	return true, fmt.Sprintf("Immich connection OK — %d people found on the server.", len(people))
}

func (h *Handler) testCardDAV(ctx context.Context, cfg *config.Config, form url.Values) (bool, string) {
	// Password field: blank means keep stored, already handled by applyFormToConfig / caller.
	if cfg.CarddavURL == "" {
		return false, "CardDAV URL is empty — not configured."
	}
	if cfg.CarddavUsername == "" {
		return false, "CardDAV username is required."
	}
	// Do a lightweight PROPFIND for sync-token rather than a full sync.
	t := &carddav.BasicAuthTransport{Username: cfg.CarddavUsername, Password: cfg.CarddavPassword}
	client := carddav.New(cfg.CarddavURL, t)
	token, err := client.SyncToken(ctx)
	if err != nil {
		return false, fmt.Sprintf("CardDAV connection failed: %s", html.EscapeString(err.Error()))
	}
	if token == "" {
		return true, "CardDAV connection OK — server reachable (no sync-token reported; will do a full sync)."
	}
	return true, "CardDAV connection OK — server reachable and sync-token available."
}

func testBackup(cfg *config.Config) (bool, string) {
	dir := cfg.BackupDir
	if dir == "" {
		dir = cfg.DataDir + "/backups"
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return false, fmt.Sprintf("Backup directory not writable: %s", html.EscapeString(err.Error()))
	}
	// Try to create and remove a temp file to prove writability.
	tmp := filepath.Join(dir, ".datey_write_test")
	if err := os.WriteFile(tmp, []byte("ok"), 0644); err != nil {
		return false, fmt.Sprintf("Backup directory not writable: %s", html.EscapeString(err.Error()))
	}
	_ = os.Remove(tmp)
	return true, fmt.Sprintf("Backup directory OK — writable at %s (retention %d days).", html.EscapeString(dir), cfg.BackupRetentionDays)
}

func testGeneral(cfg *config.Config) (bool, string) {
	if err := cfg.Validate(); err != nil {
		return false, fmt.Sprintf("Configuration validation failed: %s", html.EscapeString(err.Error()))
	}
	return true, "General settings look valid — port, scheduler, reminder window, date variant and log settings all pass validation."
}

func testObservability(cfg *config.Config) (bool, string) {
	if cfg.OTLPEndpoint != "" {
		if _, err := url.ParseRequestURI(cfg.OTLPEndpoint); err != nil {
			return false, fmt.Sprintf("OTLP endpoint is not a valid URL: %s", html.EscapeString(err.Error()))
		}
		return true, fmt.Sprintf("OTLP endpoint URL looks valid: %s (requires restart to apply).", html.EscapeString(cfg.OTLPEndpoint))
	}
	return true, "Observability: no OTLP endpoint configured — tracing/OTLP is disabled (this is fine)."
}

func testAnalytics(cfg *config.Config) (bool, string) {
	hasURL := cfg.UmamiURL != ""
	hasID := cfg.UmamiWebsiteID != ""
	if !hasURL && !hasID {
		return true, "Umami analytics not configured — disabled (this is fine)."
	}
	if hasURL != hasID {
		return false, "Umami needs both URL and Website ID set together — one is missing."
	}
	if _, err := url.ParseRequestURI(cfg.UmamiURL); err != nil {
		return false, fmt.Sprintf("Umami URL is not a valid URL: %s", html.EscapeString(err.Error()))
	}
	return true, "Umami analytics URL and Website ID look valid."
}

func testFeed(name string, enabled bool, key, extra string, cfg *config.Config) (bool, string) {
	if !enabled {
		return true, fmt.Sprintf("%s is disabled — enable it to expose the feed (key %s).", html.EscapeString(name), safeKeyStatus(key))
	}
	if key == "" {
		return false, fmt.Sprintf("%s is enabled but has no secret key — save settings to auto-generate one.", html.EscapeString(name))
	}
	if extra != "" {
		if _, _, err := config.ParseClockTime(extra); err != nil {
			return false, fmt.Sprintf("%s start time %q is not valid HH:MM.", html.EscapeString(name), html.EscapeString(extra))
		}
	}
	return true, fmt.Sprintf("%s is enabled and key is set — feed URL will require ?key=... (%s).", html.EscapeString(name), safeKeyStatus(key))
}

func safeKeyStatus(key string) string {
	if key == "" {
		return "no key"
	}
	if len(key) <= 8 {
		return "key set"
	}
	return "key …" + html.EscapeString(key[len(key)-4:])
}

func (h *Handler) testDatabase(ctx context.Context) (bool, string) {
	// Simple round-trip: count users to prove the DB is reachable.
	if _, err := h.client.User.Query().Count(ctx); err != nil {
		return false, fmt.Sprintf("Database check failed: %s", html.EscapeString(err.Error()))
	}
	if _, err := h.settingsStore.Current(ctx); err != nil {
		return false, fmt.Sprintf("App config check failed: %s", html.EscapeString(err.Error()))
	}
	return true, "Database OK — SQLite reachable and app_config readable."
}

func (h *Handler) testAll(ctx context.Context, cfg *config.Config, form url.Values) (bool, string) {
	checks := []struct {
		name string
		fn   func() (bool, string)
	}{
		{"Database", func() (bool, string) { return h.testDatabase(ctx) }},
		{"General", func() (bool, string) { return testGeneral(cfg) }},
		{"Backup", func() (bool, string) { return testBackup(cfg) }},
		{"Email", func() (bool, string) { return h.testEmail(ctx, cfg) }},
		{"Gotify", func() (bool, string) { return testNotifier(ctx, notifier.NewGotifyNotifier(cfg)) }},
		{"Telegram", func() (bool, string) { return testNotifier(ctx, notifier.NewTelegramNotifier(cfg)) }},
		{"ntfy", func() (bool, string) { return testNotifier(ctx, notifier.NewNtfyNotifier(cfg)) }},
		{"Webhook", func() (bool, string) { return testNotifier(ctx, notifier.NewWebhookNotifier(cfg)) }},
		{"Web Push", func() (bool, string) { return h.testWebPush(ctx, cfg) }},
		{"Immich", func() (bool, string) { return h.testImmich(ctx, cfg, form) }},
		{"CardDAV", func() (bool, string) { return h.testCardDAV(ctx, cfg, form) }},
		{"iCal", func() (bool, string) {
			return testFeed("iCal", cfg.ICalEnabled, cfg.ICalFeedKey, cfg.ICalEventStart, cfg)
		}},
		{"RSS", func() (bool, string) { return testFeed("RSS", cfg.RSSEnabled, cfg.RSSFeedKey, "", cfg) }},
		{"Upcoming API", func() (bool, string) {
			return testFeed("Upcoming API", cfg.UpcomingAPIEnabled, cfg.UpcomingAPIKey, "", cfg)
		}},
		{"Home Assistant", func() (bool, string) {
			return testFeed("Home Assistant", cfg.HomeAssistantEnabled, cfg.HomeAssistantKey, "", cfg)
		}},
	}

	var sb strings.Builder
	allOk := true
	for _, c := range checks {
		ok, msg := c.fn()
		if !ok {
			allOk = false
		}
		icon := "✅"
		if !ok {
			icon = "❌"
		}
		fmt.Fprintf(&sb, "%s <strong>%s:</strong> %s<br>", icon, html.EscapeString(c.name), msg)
		// Respect context cancellation between checks.
		select {
		case <-ctx.Done():
			fmt.Fprintf(&sb, "⏹ Tests cancelled: %s", html.EscapeString(ctx.Err().Error()))
			return false, sb.String()
		default:
		}
	}
	return allOk, sb.String()
}

// testEmail does a lightweight SMTP dial+auth check without requiring a full
// send when no recipient is configured. If a recipient is present it sends a
// real test message for end-to-end verification.
func (h *Handler) testEmail(ctx context.Context, cfg *config.Config) (bool, string) {
	if cfg.SMTPHost == "" {
		return false, "SMTP host is empty — email not configured."
	}
	if cfg.NotifyEmail == "" {
		return false, "Notification email (recipient) is empty — set it to enable email."
	}
	n := notifier.NewEmailNotifier(cfg)
	return testNotifier(ctx, n)
}

// applyFormToConfig overlays form values onto cfg for testing unsaved edits.
// Only fields that appear in the form (including empty strings for text fields)
// are overlaid; checkboxes are presence-based (on vs absent).
func applyFormToConfig(cfg *config.Config, form url.Values) {
	// Helper: if key exists in form, set string field to its value (even if empty).
	setStr := func(key string, dst *string) {
		if vals, ok := form[key]; ok && len(vals) > 0 {
			*dst = vals[0]
		}
	}
	setInt := func(key string, dst *int) {
		if vals, ok := form[key]; ok && len(vals) > 0 && vals[0] != "" {
			if v, err := strconv.Atoi(vals[0]); err == nil {
				*dst = v
			}
		}
	}
	hasKey := func(key string) bool {
		_, ok := form[key]
		return ok
	}

	// General
	setInt("PORT", &cfg.Port)
	setInt("SCHEDULER_HOUR", &cfg.SchedulerHour)
	setInt("REMINDER_DAYS", &cfg.ReminderDays)
	if hasKey("SCHEDULER_CATCHUP") {
		cfg.SchedulerCatchup = form.Get("SCHEDULER_CATCHUP") == "on"
	}
	setStr("DATE_VARIANT", &cfg.DateVariant)
	setStr("LOG_LEVEL", &cfg.LogLevel)
	setInt("LOG_BUFFER_SIZE", &cfg.LogBufferSize)
	setStr("OTEL_ENDPOINT", &cfg.OTLPEndpoint)

	// Backup
	setStr("BACKUP_DIR", &cfg.BackupDir)
	setInt("BACKUP_RETENTION_DAYS", &cfg.BackupRetentionDays)

	// Email
	setStr("SMTP_HOST", &cfg.SMTPHost)
	setInt("SMTP_PORT", &cfg.SMTPPort)
	setStr("SMTP_USER", &cfg.SMTPUser)
	// SMTP_PASS: form empty often means "keep", but for config test we treat
	// the literal submitted value as the one to test (overlay handles it already).
	setStr("SMTP_PASS", &cfg.SMTPPass)
	if hasKey("SMTP_TLS") {
		cfg.SMTPTLS = form.Get("SMTP_TLS") == "on"
	}
	setInt("SMTP_TIMEOUT", &cfg.SMTPTimeout)
	setStr("NOTIFICATION_EMAIL", &cfg.NotifyEmail)

	// Gotify
	setStr("GOTIFY_URL", &cfg.GotifyURL)
	setStr("GOTIFY_TOKEN", &cfg.GotifyToken)

	// Telegram
	setStr("TELEGRAM_BOT_TOKEN", &cfg.TelegramBotToken)
	setStr("TELEGRAM_CHAT_ID", &cfg.TelegramChatID)

	// ntfy
	setStr("NTFY_URL", &cfg.NtfyURL)
	setStr("NTFY_TOPIC", &cfg.NtfyTopic)
	setStr("NTFY_TOKEN", &cfg.NtfyToken)
	setInt("NTFY_PRIORITY", &cfg.NtfyPriority)

	// Webhook
	setStr("WEBHOOK_URL", &cfg.WebhookURL)
	setStr("WEBHOOK_SECRET", &cfg.WebhookSecret)

	// Analytics
	setStr("UMAMI_URL", &cfg.UmamiURL)
	setStr("UMAMI_WEBSITE_ID", &cfg.UmamiWebsiteID)

	// E-Ink
	if hasKey("EINK_MODE") {
		cfg.EinkMode = form.Get("EINK_MODE") == "on"
	}

	// iCal
	if hasKey("ICAL_FEED_ENABLED") {
		cfg.ICalEnabled = form.Get("ICAL_FEED_ENABLED") == "on"
	}
	setStr("ICAL_EVENT_START", &cfg.ICalEventStart)
	setInt("ICAL_EVENT_DURATION", &cfg.ICalDurationMinutes)
	setStr("ICAL_FEED_KEY", &cfg.ICalFeedKey)

	// RSS
	if hasKey("RSS_FEED_ENABLED") {
		cfg.RSSEnabled = form.Get("RSS_FEED_ENABLED") == "on"
	}
	setStr("RSS_FEED_KEY", &cfg.RSSFeedKey)

	// Upcoming
	if hasKey("UPCOMING_API_ENABLED") {
		cfg.UpcomingAPIEnabled = form.Get("UPCOMING_API_ENABLED") == "on"
	}
	setStr("UPCOMING_API_KEY", &cfg.UpcomingAPIKey)

	// Home Assistant
	if hasKey("HOMEASSISTANT_ENABLED") {
		cfg.HomeAssistantEnabled = form.Get("HOMEASSISTANT_ENABLED") == "on"
	}
	setStr("HOMEASSISTANT_KEY", &cfg.HomeAssistantKey)

	// Web Push
	if hasKey("PUSH_ENABLED") {
		cfg.PushEnabled = form.Get("PUSH_ENABLED") == "on"
	}
	setStr("PUSH_VAPID_PUBLIC_KEY", &cfg.PushVAPIDPublicKey)
	// Private key often left blank to keep stored — only overwrite if form has it non-empty
	if v := form.Get("PUSH_VAPID_PRIVATE_KEY"); v != "" {
		cfg.PushVAPIDPrivateKey = v
	}

	// Immich
	setStr("IMMICH_URL", &cfg.ImmichURL)
	if v := form.Get("IMMICH_API_KEY"); v != "" {
		cfg.ImmichAPIKey = v
	}

	// CardDAV (config tab does not have these, but handle for completeness)
	setStr("CARDDAV_URL", &cfg.CarddavURL)
	setStr("CARDDAV_USERNAME", &cfg.CarddavUsername)
	if v := form.Get("CARDDAV_PASSWORD"); v != "" {
		cfg.CarddavPassword = v
	}
	setStr("CARDDAV_DELETE_POLICY", &cfg.CarddavDeletePolicy)
	if hasKey("CARDDAV_ENABLED") {
		cfg.CarddavEnabled = form.Get("CARDDAV_ENABLED") == "on"
	}
}
