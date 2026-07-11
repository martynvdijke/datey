package settings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"time"

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
	if v := row.UmamiURL; v != nil {
		cfg.UmamiURL = *v
	}
	if v := row.UmamiWebsiteID; v != nil {
		cfg.UmamiWebsiteID = *v
	}
	if v := row.EinkMode; v != nil {
		cfg.EinkMode = *v
	}
	return nil
}

// ErrInvalid signals validation failures distinguishable from persistence errors.
var ErrInvalid = errors.New("invalid settings form")

// errInvalid signals validation failures distinguishable from persistence errors.
var errInvalid = ErrInvalid

var validLogLevels = map[string]bool{"debug": true, "info": true, "warn": true, "error": true}

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
	umamiURL := form.Get("UMAMI_URL")
	umamiWebsiteID := form.Get("UMAMI_WEBSITE_ID")
	einkMode := form.Get("EINK_MODE") == "on"

	if port != nil && (*port < 1 || *port > 65535) {
		errs["PORT"] = "Port must be between 1 and 65535"
	}
	if schedulerHour != nil && (*schedulerHour < 0 || *schedulerHour > 23) {
		errs["SCHEDULER_HOUR"] = "Scheduler hour must be between 0 and 23"
	}
	if reminderDays != nil && (*reminderDays < 1 || *reminderDays > 365) {
		errs["REMINDER_DAYS"] = "Reminder days must be between 1 and 365"
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

	upd := s.client.AppConfig.UpdateOneID(row.ID).
		SetNillablePort(port).
		SetNillableSchedulerHour(schedulerHour).
		SetNillableReminderDays(reminderDays).
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
		SetNillableUmamiURL(nillableStr(umamiURL)).
		SetNillableUmamiWebsiteID(nillableStr(umamiWebsiteID)).
		SetNillableEinkMode(&einkMode).
		SetUpdatedAt(time.Now())

	if _, err := upd.Save(ctx); err != nil {
		return nil, fmt.Errorf("persist app_config: %w", err)
	}

	// Hot-reload: mutate cfg in place so notifiers/scheduler/dashboard pick up
	// changes immediately. Restart-required fields are left untouched.
	cfg.ReminderDays = deref(reminderDays, cfg.ReminderDays)
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
	cfg.UmamiURL = umamiURL
	cfg.UmamiWebsiteID = umamiWebsiteID
	cfg.EinkMode = einkMode

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

func deref[T any](v *T, fallback T) T {
	if v == nil {
		return fallback
	}
	return *v
}