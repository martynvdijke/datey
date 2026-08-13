package settings

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/appconfig"
	"github.com/datey/datey/internal/config"
)

// Store reads and writes the singleton app_config row that backs the
// admin-configurable settings. The Config struct remains the in-memory source
// of truth for running services; the DB row overrides env-derived values.
type Store struct {
	client *ent.Client
}

func New(client *ent.Client) *Store {
	return &Store{client: client}
}

// EnsureSeeded creates the singleton app_config row if none exists. The row is
// created with every column NULL so that, on first boot, env values remain
// authoritative until an administrator saves a value via the UI.
func (s *Store) EnsureSeeded(ctx context.Context) error {
	count, err := s.client.AppConfig.Query().Count(ctx)
	if err != nil {
		return fmt.Errorf("count app_config: %w", err)
	}
	if count > 0 {
		return nil
	}
	if _, err := s.client.AppConfig.Create().
		Save(ctx); err != nil {
		return fmt.Errorf("seed app_config: %w", err)
	}
	slog.Info("app_config seeded", "source", "settings")
	return nil
}

// Current returns the singleton row, seeding it first if necessary. Only the
// id-lowest row is ever treated as the singleton; extras (if any) are ignored.
func (s *Store) Current(ctx context.Context) (*ent.AppConfig, error) {
	if err := s.EnsureSeeded(ctx); err != nil {
		return nil, err
	}
	row, err := s.client.AppConfig.Query().Order(ent.Asc(appconfig.FieldID)).First(ctx)
	if err != nil {
		return nil, fmt.Errorf("load app_config: %w", err)
	}
	return row, nil
}

// Overlay applies non-null columns from the singleton row onto cfg.
// DataDir is never overlaid: the database is already open at the env-derived
// path, so a DB-stored DataDir cannot take effect without a manual relocation
// and is surfaced read-only in the UI instead.
func (s *Store) Overlay(ctx context.Context, cfg *config.Config) error {
	row, err := s.Current(ctx)
	if err != nil {
		return err
	}
	if v := row.Port; v != nil {
		cfg.Port = *v
	}
	// DataDir intentionally not overlaid (env-only).
	if v := row.SchedulerHour; v != nil {
		cfg.SchedulerHour = *v
	}
	if v := row.ReminderDays; v != nil {
		cfg.ReminderDays = *v
	}
	if v := row.SchedulerCatchup; v != nil {
		cfg.SchedulerCatchup = *v
	}
	if v := row.DateVariant; v != nil {
		cfg.DateVariant = *v
	}
	if v := row.LogLevel; v != nil {
		cfg.LogLevel = *v
	}
	if v := row.LogBufferSize; v != nil {
		cfg.LogBufferSize = *v
	}
	if v := row.OtelEndpoint; v != nil {
		cfg.OTLPEndpoint = *v
	}
	if v := row.BackupDir; v != nil {
		cfg.BackupDir = *v
	}
	if v := row.BackupRetentionDays; v != nil {
		cfg.BackupRetentionDays = *v
	}
	if v := row.SMTPHost; v != nil {
		cfg.SMTPHost = *v
	}
	if v := row.SMTPPort; v != nil {
		cfg.SMTPPort = *v
	}
	if v := row.SMTPUser; v != nil {
		cfg.SMTPUser = *v
	}
	if v := row.SMTPPass; v != nil {
		cfg.SMTPPass = *v
	}
	if v := row.SMTPTLS; v != nil {
		cfg.SMTPTLS = *v
	}
	if v := row.SMTPTimeout; v != nil {
		cfg.SMTPTimeout = *v
	}
	if v := row.NotifyEmail; v != nil {
		cfg.NotifyEmail = *v
	}
	if v := row.GotifyURL; v != nil {
		cfg.GotifyURL = *v
	}
	if v := row.GotifyToken; v != nil {
		cfg.GotifyToken = *v
	}
	if v := row.TelegramBotToken; v != nil {
		cfg.TelegramBotToken = *v
	}
	if v := row.TelegramChatID; v != nil {
		cfg.TelegramChatID = *v
	}
	if v := row.NtfyURL; v != nil {
		cfg.NtfyURL = *v
	}
	if v := row.NtfyTopic; v != nil {
		cfg.NtfyTopic = *v
	}
	if v := row.NtfyToken; v != nil {
		cfg.NtfyToken = *v
	}
	if v := row.NtfyPriority; v != nil {
		cfg.NtfyPriority = *v
	}
	if v := row.WebhookURL; v != nil {
		cfg.WebhookURL = *v
	}
	if v := row.WebhookSecret; v != nil {
		cfg.WebhookSecret = *v
	}
	if v := row.UmamiURL; v != nil {
		cfg.UmamiURL = *v
	}
	if v := row.UmamiWebsiteID; v != nil {
		cfg.UmamiWebsiteID = *v
	}
	if v := row.EinkMode; v != nil {
		cfg.EinkMode = *v
	}
	if v := row.IcalEnabled; v != nil {
		cfg.ICalEnabled = *v
	}
	if v := row.IcalEventStart; v != nil {
		cfg.ICalEventStart = *v
	}
	if v := row.IcalDurationMinutes; v != nil {
		cfg.ICalDurationMinutes = *v
	}
	if v := row.IcalFeedKey; v != nil {
		cfg.ICalFeedKey = *v
	}
	if v := row.RssEnabled; v != nil {
		cfg.RSSEnabled = *v
	}
	if v := row.RssFeedKey; v != nil {
		cfg.RSSFeedKey = *v
	}
	if v := row.UpcomingAPIEnabled; v != nil {
		cfg.UpcomingAPIEnabled = *v
	}
	if v := row.UpcomingAPIKey; v != nil {
		cfg.UpcomingAPIKey = *v
	}
	if v := row.HomeassistantEnabled; v != nil {
		cfg.HomeAssistantEnabled = *v
	}
	if v := row.HomeassistantKey; v != nil {
		cfg.HomeAssistantKey = *v
	}
	if v := row.PushEnabled; v != nil {
		cfg.PushEnabled = *v
	}
	if v := row.PushVapidPublicKey; v != nil {
		cfg.PushVAPIDPublicKey = *v
	}
	if v := row.PushVapidPrivateKey; v != nil {
		cfg.PushVAPIDPrivateKey = *v
	}
	if v := row.ImmichURL; v != nil {
		cfg.ImmichURL = *v
	}
	if v := row.ImmichAPIKey; v != nil {
		cfg.ImmichAPIKey = *v
	}
	if v := row.CarddavEnabled; v != nil {
		cfg.CarddavEnabled = *v
	}
	if v := row.CarddavURL; v != nil {
		cfg.CarddavURL = *v
	}
	if v := row.CarddavUsername; v != nil {
		cfg.CarddavUsername = *v
	}
	if v := row.CarddavPassword; v != nil {
		cfg.CarddavPassword = *v
	}
	if v := row.CarddavDeletePolicy; v != nil {
		cfg.CarddavDeletePolicy = *v
	}
	return nil
}

// ErrInvalid signals validation failures distinguishable from persistence errors.
var ErrInvalid = errors.New("invalid settings form")

// LastSchedulerRun returns the timestamp of the last completed reminder pass,
// or nil if no pass has been recorded yet.
func (s *Store) LastSchedulerRun(ctx context.Context) (*time.Time, error) {
	row, err := s.Current(ctx)
	if err != nil {
		return nil, err
	}
	return row.LastSchedulerRun, nil
}

// SetLastSchedulerRun records the timestamp of a completed reminder pass.
func (s *Store) SetLastSchedulerRun(ctx context.Context, t time.Time) error {
	row, err := s.Current(ctx)
	if err != nil {
		return err
	}
	if _, err := s.client.AppConfig.UpdateOneID(row.ID).
		SetLastSchedulerRun(t).
		Save(ctx); err != nil {
		return fmt.Errorf("persist last_scheduler_run: %w", err)
	}
	return nil
}

// CarddavSyncToken returns the stored CardDAV sync-collection sync-token, or
// an empty string if a sync has never succeeded (first pull must be full).
func (s *Store) CarddavSyncToken(ctx context.Context) (string, error) {
	row, err := s.Current(ctx)
	if err != nil {
		return "", err
	}
	if row.CarddavSyncToken == nil {
		return "", nil
	}
	return *row.CarddavSyncToken, nil
}

// SetCarddavSyncToken records the sync-token returned by the last successful
// sync-collection REPORT so the next pull can be incremental.
func (s *Store) SetCarddavSyncToken(ctx context.Context, token string) error {
	row, err := s.Current(ctx)
	if err != nil {
		return err
	}
	if _, err := s.client.AppConfig.UpdateOneID(row.ID).
		SetCarddavSyncToken(token).
		Save(ctx); err != nil {
		return fmt.Errorf("persist carddav_sync_token: %w", err)
	}
	return nil
}

// CarddavLastSync returns the timestamp of the last completed CardDAV sync,
// or nil if no sync has been run yet.
func (s *Store) CarddavLastSync(ctx context.Context) (*time.Time, error) {
	row, err := s.Current(ctx)
	if err != nil {
		return nil, err
	}
	return row.CarddavLastSync, nil
}

// SetCarddavLastSync records the timestamp of a completed CardDAV sync. It is
// used to rate-limit the daily scheduler hook.
func (s *Store) SetCarddavLastSync(ctx context.Context, t time.Time) error {
	row, err := s.Current(ctx)
	if err != nil {
		return err
	}
	if _, err := s.client.AppConfig.UpdateOneID(row.ID).
		SetCarddavLastSync(t).
		Save(ctx); err != nil {
		return fmt.Errorf("persist carddav_last_sync: %w", err)
	}
	return nil
}

var validDeletePolicies = map[string]bool{"keep": true, "delete": true}

// ApplyCarddavForm persists the CardDAV section of the Notifications settings
// tab. An empty password submission keeps the existing stored password (the
// password field is masked in the UI, mirroring the IMMICH_API_KEY pattern).
// The sync_token and last_sync fields are intentionally not form-editable;
// they are maintained by the sync engine.
func (s *Store) ApplyCarddavForm(ctx context.Context, cfg *config.Config, form url.Values) (map[string]string, error) {
	row, err := s.Current(ctx)
	if err != nil {
		return nil, err
	}
	errs := map[string]string{}

	enabled := form.Get("CARDDAV_ENABLED") == "on"
	carddavURL := form.Get("CARDDAV_URL")
	username := form.Get("CARDDAV_USERNAME")
	password := form.Get("CARDDAV_PASSWORD")
	deletePolicy := form.Get("CARDDAV_DELETE_POLICY")
	if deletePolicy == "" {
		deletePolicy = "keep"
	}

	if carddavURL != "" {
		u, err := url.ParseRequestURI(carddavURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			errs["CARDDAV_URL"] = "CardDAV URL must be an absolute URL (e.g. https://cloud.example.com/remote.php/dav/addressbooks/user/book)"
		}
	}
	if !validDeletePolicies[deletePolicy] {
		errs["CARDDAV_DELETE_POLICY"] = "Delete policy must be one of: keep, delete"
	}

	if len(errs) > 0 {
		return errs, errInvalid
	}

	// An empty password submission means "keep the current one".
	effectivePassword := password
	if effectivePassword == "" {
		effectivePassword = cfg.CarddavPassword
	}

	upd := s.client.AppConfig.UpdateOneID(row.ID).
		SetNillableCarddavEnabled(&enabled).
		SetNillableCarddavURL(nillableStr(carddavURL)).
		SetNillableCarddavUsername(nillableStr(username)).
		SetNillableCarddavPassword(nillableStr(effectivePassword)).
		SetNillableCarddavDeletePolicy(nillableStr(deletePolicy)).
		SetUpdatedAt(time.Now())

	if _, err := upd.Save(ctx); err != nil {
		return nil, fmt.Errorf("persist carddav settings: %w", err)
	}

	cfg.CarddavEnabled = enabled
	cfg.CarddavURL = carddavURL
	cfg.CarddavUsername = username
	cfg.CarddavPassword = effectivePassword
	cfg.CarddavDeletePolicy = deletePolicy

	return nil, nil
}

// errInvalid signals validation failures distinguishable from persistence errors.
var errInvalid = ErrInvalid

var validLogLevels = map[string]bool{"debug": true, "info": true, "warn": true, "error": true}

// validDateVariants lists the allowed date display variants; empty means
// "fall back to the environment/default" (mirrors validLogLevels semantics).
var validDateVariants = map[string]bool{"european": true, "us": true}

// ApplyForm persists posted form values to the singleton row and mutates the
// hot-reloadable fields of cfg in place. Restart-required fields (Port,
// SchedulerHour, LogBufferSize, OTLPEndpoint) are persisted but cfg is left
// untouched — they take effect on the next boot via Overlay. DataDir is not
// writable from the form.
//
// Returns a map of form-field name → human-readable error for invalid input.
// On success the map is empty.
func (s *Store) ApplyForm(ctx context.Context, cfg *config.Config, form url.Values) (map[string]string, error) {
	row, err := s.Current(ctx)
	if err != nil {
		return nil, err
	}
	errs := map[string]string{}

	port := parseIntPtr(form, "PORT", errs)
	schedulerHour := parseIntPtr(form, "SCHEDULER_HOUR", errs)
	reminderDays := parseIntPtr(form, "REMINDER_DAYS", errs)
	schedulerCatchupRaw := form.Get("SCHEDULER_CATCHUP")
	schedulerCatchup := schedulerCatchupRaw == "on"
	dateVariant := form.Get("DATE_VARIANT")
	logLevel := form.Get("LOG_LEVEL")
	logBufferSize := parseIntPtr(form, "LOG_BUFFER_SIZE", errs)
	otelEndpoint := form.Get("OTEL_ENDPOINT")
	backupDir := form.Get("BACKUP_DIR")
	backupRetention := parseIntPtr(form, "BACKUP_RETENTION_DAYS", errs)
	smtpHost := form.Get("SMTP_HOST")
	smtpPort := parseIntPtr(form, "SMTP_PORT", errs)
	smtpUser := form.Get("SMTP_USER")
	smtpPass := form.Get("SMTP_PASS")
	smtpTLS := form.Get("SMTP_TLS") == "on"
	smtpTimeout := parseIntPtr(form, "SMTP_TIMEOUT", errs)
	notifyEmail := form.Get("NOTIFICATION_EMAIL")
	gotifyURL := form.Get("GOTIFY_URL")
	gotifyToken := form.Get("GOTIFY_TOKEN")
	telegramBotToken := form.Get("TELEGRAM_BOT_TOKEN")
	telegramChatID := form.Get("TELEGRAM_CHAT_ID")
	ntfyURL := form.Get("NTFY_URL")
	ntfyTopic := form.Get("NTFY_TOPIC")
	ntfyToken := form.Get("NTFY_TOKEN")
	ntfyPriority := parseIntPtr(form, "NTFY_PRIORITY", errs)
	webhookURL := form.Get("WEBHOOK_URL")
	webhookSecret := form.Get("WEBHOOK_SECRET")
	umamiURL := form.Get("UMAMI_URL")
	umamiWebsiteID := form.Get("UMAMI_WEBSITE_ID")
	einkMode := form.Get("EINK_MODE") == "on"
	icalEnabled := form.Get("ICAL_FEED_ENABLED") == "on"
	icalEventStart := form.Get("ICAL_EVENT_START")
	icalDuration := parseIntPtr(form, "ICAL_EVENT_DURATION", errs)
	icalFeedKey := form.Get("ICAL_FEED_KEY")
	rssEnabled := form.Get("RSS_FEED_ENABLED") == "on"
	rssFeedKey := form.Get("RSS_FEED_KEY")
	upcomingAPIEnabled := form.Get("UPCOMING_API_ENABLED") == "on"
	upcomingAPIKey := form.Get("UPCOMING_API_KEY")
	homeAssistantEnabled := form.Get("HOMEASSISTANT_ENABLED") == "on"
	homeAssistantKey := form.Get("HOMEASSISTANT_KEY")
	pushEnabled := form.Get("PUSH_ENABLED") == "on"
	pushVAPIDPublicKey := form.Get("PUSH_VAPID_PUBLIC_KEY")
	pushVAPIDPrivateKey := form.Get("PUSH_VAPID_PRIVATE_KEY")
	immichURL := form.Get("IMMICH_URL")
	immichAPIKey := form.Get("IMMICH_API_KEY")
	if immichAPIKey == "" {
		immichAPIKey = cfg.ImmichAPIKey
	}

	if port != nil && (*port < 1 || *port > 65535) {
		errs["PORT"] = "Port must be between 1 and 65535"
	}
	if schedulerHour != nil && (*schedulerHour < 0 || *schedulerHour > 23) {
		errs["SCHEDULER_HOUR"] = "Scheduler hour must be between 0 and 23"
	}
	if reminderDays != nil && (*reminderDays < 1 || *reminderDays > 365) {
		errs["REMINDER_DAYS"] = "Reminder days must be between 1 and 365"
	}
	if schedulerCatchupRaw != "" && schedulerCatchupRaw != "on" {
		errs["SCHEDULER_CATCHUP"] = "Catch up missed reminders must be a boolean"
	}
	if dateVariant != "" && !validDateVariants[dateVariant] {
		errs["DATE_VARIANT"] = "Date variant must be one of: european, us"
	}
	if logLevel != "" && !validLogLevels[logLevel] {
		errs["LOG_LEVEL"] = "Log level must be one of: debug, info, warn, error"
	}
	if logBufferSize != nil && *logBufferSize < 1 {
		errs["LOG_BUFFER_SIZE"] = "Log buffer size must be at least 1"
	}
	if backupRetention != nil && *backupRetention < 1 {
		errs["BACKUP_RETENTION_DAYS"] = "Backup retention days must be at least 1"
	}
	if smtpPort != nil && (*smtpPort < 1 || *smtpPort > 65535) {
		errs["SMTP_PORT"] = "SMTP port must be between 1 and 65535"
	}
	if smtpTimeout != nil && *smtpTimeout < 0 {
		errs["SMTP_TIMEOUT"] = "SMTP timeout cannot be negative"
	}
	if icalEventStart != "" {
		hour, minute, err := config.ParseClockTime(icalEventStart)
		switch {
		case err != nil:
			errs["ICAL_EVENT_START"] = "Start time must be in HH:MM format (e.g. 09:00)"
		case hour < 0 || hour > 23:
			errs["ICAL_EVENT_START"] = "Start hour must be between 0 and 23"
		case minute < 0 || minute > 59:
			errs["ICAL_EVENT_START"] = "Start minute must be between 0 and 59"
		}
	}
	if icalDuration != nil && (*icalDuration < 1 || *icalDuration > 1440) {
		errs["ICAL_EVENT_DURATION"] = "Event duration must be between 1 and 1440 minutes"
	}
	if ntfyPriority != nil && (*ntfyPriority < 1 || *ntfyPriority > 5) {
		errs["NTFY_PRIORITY"] = "Priority must be between 1 and 5"
	}
	if immichURL != "" {
		u, err := url.ParseRequestURI(immichURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			errs["IMMICH_URL"] = "Immich URL must be an absolute URL"
		}
		if immichAPIKey == "" && cfg.ImmichAPIKey == "" {
			errs["IMMICH_API_KEY"] = "API key is required when Immich is configured"
		}
	}

	if len(errs) > 0 {
		return errs, errInvalid
	}

	effectiveBackupDir := backupDir
	if effectiveBackupDir == "" {
		effectiveBackupDir = cfg.DataDir + "/backups"
	}
	effectiveRetention := 30
	if backupRetention != nil {
		effectiveRetention = *backupRetention
	}
	if effectiveRetention < 1 {
		effectiveRetention = 30
	}

	// Keep the existing key unless the admin supplies a new one; generate a
	// fresh key when the feed is first enabled so the URLs shown in the UI
	// are immediately usable.
	effectiveICalKey := icalFeedKey
	if effectiveICalKey == "" {
		effectiveICalKey = cfg.ICalFeedKey
	}
	if icalEnabled && effectiveICalKey == "" {
		effectiveICalKey = generateFeedKey()
	}

	// Same key semantics for the RSS feed: keep the existing key unless the
	// admin supplies a new one; generate a fresh key on first enable.
	effectiveRSSKey := rssFeedKey
	if effectiveRSSKey == "" {
		effectiveRSSKey = cfg.RSSFeedKey
	}
	if rssEnabled && effectiveRSSKey == "" {
		effectiveRSSKey = generateFeedKey()
	}

	// Same key semantics for the Upcoming Events API.
	effectiveUpcomingAPIKey := upcomingAPIKey
	if effectiveUpcomingAPIKey == "" {
		effectiveUpcomingAPIKey = cfg.UpcomingAPIKey
	}
	if upcomingAPIEnabled && effectiveUpcomingAPIKey == "" {
		effectiveUpcomingAPIKey = generateFeedKey()
	}

	// Same key semantics for the Home Assistant feed.
	effectiveHomeAssistantKey := homeAssistantKey
	if effectiveHomeAssistantKey == "" {
		effectiveHomeAssistantKey = cfg.HomeAssistantKey
	}
	if homeAssistantEnabled && effectiveHomeAssistantKey == "" {
		effectiveHomeAssistantKey = generateFeedKey()
	}

	// VAPID key semantics: keep the existing keys unless the admin supplies new
	// ones (the private key is never rendered in the form, so an empty
	// submission means "keep the current one"); generate a fresh keypair on
	// first enable so the public key endpoint is immediately usable.
	effectivePushPublicKey := pushVAPIDPublicKey
	if effectivePushPublicKey == "" {
		effectivePushPublicKey = cfg.PushVAPIDPublicKey
	}
	effectivePushPrivateKey := pushVAPIDPrivateKey
	if effectivePushPrivateKey == "" {
		effectivePushPrivateKey = cfg.PushVAPIDPrivateKey
	}
	if pushEnabled && (effectivePushPublicKey == "" || effectivePushPrivateKey == "") {
		priv, pub, err := webpush.GenerateVAPIDKeys()
		if err != nil {
			return nil, fmt.Errorf("generate VAPID keys: %w", err)
		}
		effectivePushPublicKey = pub
		effectivePushPrivateKey = priv
	}

	upd := s.client.AppConfig.UpdateOneID(row.ID).
		SetNillablePort(port).
		SetNillableSchedulerHour(schedulerHour).
		SetNillableReminderDays(reminderDays).
		SetNillableSchedulerCatchup(&schedulerCatchup).
		SetNillableDateVariant(nillableStr(dateVariant)).
		SetNillableLogLevel(nillableStr(logLevel)).
		SetNillableLogBufferSize(logBufferSize).
		SetNillableOtelEndpoint(nillableStr(otelEndpoint)).
		SetNillableBackupDir(&effectiveBackupDir).
		SetNillableBackupRetentionDays(&effectiveRetention).
		SetNillableSMTPHost(nillableStr(smtpHost)).
		SetNillableSMTPPort(smtpPort).
		SetNillableSMTPUser(nillableStr(smtpUser)).
		SetNillableSMTPPass(nillableStr(smtpPass)).
		SetNillableSMTPTLS(&smtpTLS).
		SetNillableSMTPTimeout(smtpTimeout).
		SetNillableNotifyEmail(nillableStr(notifyEmail)).
		SetNillableGotifyURL(nillableStr(gotifyURL)).
		SetNillableGotifyToken(nillableStr(gotifyToken)).
		SetNillableTelegramBotToken(nillableStr(telegramBotToken)).
		SetNillableTelegramChatID(nillableStr(telegramChatID)).
		SetNillableNtfyURL(nillableStr(ntfyURL)).
		SetNillableNtfyTopic(nillableStr(ntfyTopic)).
		SetNillableNtfyToken(nillableStr(ntfyToken)).
		SetNillableNtfyPriority(ntfyPriority).
		SetNillableWebhookURL(nillableStr(webhookURL)).
		SetNillableWebhookSecret(nillableStr(webhookSecret)).
		SetNillableUmamiURL(nillableStr(umamiURL)).
		SetNillableUmamiWebsiteID(nillableStr(umamiWebsiteID)).
		SetNillableEinkMode(&einkMode).
		SetNillableIcalEnabled(&icalEnabled).
		SetNillableIcalEventStart(nillableStr(icalEventStart)).
		SetNillableIcalDurationMinutes(icalDuration).
		SetNillableIcalFeedKey(nillableStr(effectiveICalKey)).
		SetNillableRssEnabled(&rssEnabled).
		SetNillableRssFeedKey(nillableStr(effectiveRSSKey)).
		SetNillableUpcomingAPIEnabled(&upcomingAPIEnabled).
		SetNillableUpcomingAPIKey(nillableStr(effectiveUpcomingAPIKey)).
		SetNillableHomeassistantEnabled(&homeAssistantEnabled).
		SetNillableHomeassistantKey(nillableStr(effectiveHomeAssistantKey)).
		SetNillablePushEnabled(&pushEnabled).
		SetNillablePushVapidPublicKey(nillableStr(effectivePushPublicKey)).
		SetNillablePushVapidPrivateKey(nillableStr(effectivePushPrivateKey)).
		SetNillableImmichURL(nillableStr(immichURL)).
		SetNillableImmichAPIKey(nillableStr(immichAPIKey)).
		SetUpdatedAt(time.Now())

	if _, err := upd.Save(ctx); err != nil {
		return nil, fmt.Errorf("persist app_config: %w", err)
	}

	// Hot-reload: mutate cfg in place so notifiers/scheduler/dashboard pick up
	// changes immediately. Restart-required fields are left untouched.
	cfg.ReminderDays = deref(reminderDays, cfg.ReminderDays)
	cfg.SchedulerCatchup = schedulerCatchup
	if dateVariant != "" {
		cfg.DateVariant = dateVariant
	}
	if logLevel != "" {
		cfg.LogLevel = logLevel
	}
	cfg.OTLPEndpoint = otelEndpoint
	cfg.BackupDir = effectiveBackupDir
	cfg.BackupRetentionDays = effectiveRetention
	cfg.SMTPHost = smtpHost
	cfg.SMTPPort = deref(smtpPort, cfg.SMTPPort)
	cfg.SMTPUser = smtpUser
	cfg.SMTPPass = smtpPass
	cfg.SMTPTLS = smtpTLS
	cfg.SMTPTimeout = deref(smtpTimeout, cfg.SMTPTimeout)
	cfg.NotifyEmail = notifyEmail
	cfg.GotifyURL = gotifyURL
	cfg.GotifyToken = gotifyToken
	cfg.TelegramBotToken = telegramBotToken
	cfg.TelegramChatID = telegramChatID
	cfg.NtfyURL = ntfyURL
	cfg.NtfyTopic = ntfyTopic
	cfg.NtfyToken = ntfyToken
	cfg.NtfyPriority = deref(ntfyPriority, cfg.NtfyPriority)
	cfg.WebhookURL = webhookURL
	cfg.WebhookSecret = webhookSecret
	cfg.UmamiURL = umamiURL
	cfg.UmamiWebsiteID = umamiWebsiteID
	cfg.EinkMode = einkMode
	cfg.ICalEnabled = icalEnabled
	cfg.ICalEventStart = icalEventStart
	cfg.ICalDurationMinutes = deref(icalDuration, cfg.ICalDurationMinutes)
	cfg.ICalFeedKey = effectiveICalKey
	cfg.RSSEnabled = rssEnabled
	cfg.RSSFeedKey = effectiveRSSKey
	cfg.UpcomingAPIEnabled = upcomingAPIEnabled
	cfg.UpcomingAPIKey = effectiveUpcomingAPIKey
	cfg.HomeAssistantEnabled = homeAssistantEnabled
	cfg.HomeAssistantKey = effectiveHomeAssistantKey
	cfg.PushEnabled = pushEnabled
	cfg.PushVAPIDPublicKey = effectivePushPublicKey
	cfg.PushVAPIDPrivateKey = effectivePushPrivateKey
	cfg.ImmichURL = immichURL
	if immichAPIKey != "" {
		cfg.ImmichAPIKey = immichAPIKey
	}

	return nil, nil
}

func parseIntPtr(form url.Values, key string, errs map[string]string) *int {
	raw := form.Get(key)
	if raw == "" {
		return nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		errs[key] = key + " must be a whole number"
		return nil
	}
	return &v
}

func nillableStr(s string) *string {
	return &s
}

// generateFeedKey returns a 32-hex-char random secret used to protect feed
// URLs from enumeration.
func generateFeedKey() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand does not fail on supported platforms; this fallback
		// keeps settings saveable as a last resort.
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func deref[T any](v *T, fallback T) T {
	if v == nil {
		return fallback
	}
	return *v
}
