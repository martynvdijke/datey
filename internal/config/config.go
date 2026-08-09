package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port          int
	DataDir       string
	SchedulerHour int
	ReminderDays  int
	LogLevel      string
	LogBufferSize int
	OTLPEndpoint  string

	BackupDir          string
	BackupRetentionDays int

	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPass     string
	SMTPTLS      bool
	SMTPTimeout  int
	NotifyEmail  string

	GotifyURL   string
	GotifyToken string

	TelegramBotToken string
	TelegramChatID   string

	NtfyURL      string
	NtfyTopic    string
	NtfyToken    string
	NtfyPriority int

	WebhookURL    string
	WebhookSecret string

	UmamiURL       string
	UmamiWebsiteID string

	EinkMode bool

	ICalEnabled         bool
	ICalEventStart      string
	ICalDurationMinutes int
	ICalFeedKey         string

	RSSEnabled bool
	RSSFeedKey string

	UpcomingAPIEnabled bool
	UpcomingAPIKey     string

	HomeAssistantEnabled bool
	HomeAssistantKey     string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:          getEnvInt("PORT", 6270),
		DataDir:       getEnvExplicit("DATA_DIR", "/db"),
		SchedulerHour: getEnvInt("SCHEDULER_HOUR", 8),
		ReminderDays:  getEnvInt("REMINDER_DAYS", 7),
		LogLevel:      getEnvExplicit("LOG_LEVEL", "info"),
		LogBufferSize: getEnvInt("LOG_BUFFER_SIZE", 10000),
		OTLPEndpoint:  getEnv("OTEL_ENDPOINT", ""),
		SMTPHost:      getEnv("SMTP_HOST", ""),
		SMTPPort:      getEnvInt("SMTP_PORT", 587),
		SMTPUser:      getEnv("SMTP_USER", ""),
		SMTPPass:      getEnv("SMTP_PASS", ""),
		SMTPTLS:       getEnv("SMTP_TLS", "true") == "true",
		SMTPTimeout:   getEnvInt("SMTP_TIMEOUT", 10),
		NotifyEmail:   getEnv("NOTIFICATION_EMAIL", ""),
		GotifyURL:     getEnv("GOTIFY_URL", ""),
		GotifyToken:   getEnv("GOTIFY_TOKEN", ""),
		TelegramBotToken: getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:   getEnv("TELEGRAM_CHAT_ID", ""),
		NtfyURL:          getEnv("NTFY_URL", "https://ntfy.sh"),
		NtfyTopic:        getEnv("NTFY_TOPIC", ""),
		NtfyToken:        getEnv("NTFY_TOKEN", ""),
		NtfyPriority:     getEnvInt("NTFY_PRIORITY", 3),
		WebhookURL:       getEnv("WEBHOOK_URL", ""),
		WebhookSecret:    getEnv("WEBHOOK_SECRET", ""),

		BackupDir:           getEnv("BACKUP_DIR", ""),
		BackupRetentionDays: getEnvInt("BACKUP_RETENTION_DAYS", 0),

		UmamiURL:       getEnv("UMAMI_URL", ""),
		UmamiWebsiteID: getEnv("UMAMI_WEBSITE_ID", ""),

		EinkMode: getEnv("EINK_MODE", "") == "true",

		ICalEnabled:         getEnv("ICAL_FEED_ENABLED", "") == "true",
		ICalEventStart:      getEnv("ICAL_EVENT_START", ""),
		ICalDurationMinutes: getEnvInt("ICAL_EVENT_DURATION", 60),
		ICalFeedKey:         getEnv("ICAL_FEED_KEY", ""),

		RSSEnabled: getEnv("RSS_FEED_ENABLED", "") == "true",
		RSSFeedKey: getEnv("RSS_FEED_KEY", ""),

		UpcomingAPIEnabled: getEnv("UPCOMING_API_ENABLED", "") == "true",
		UpcomingAPIKey:     getEnv("UPCOMING_API_KEY", ""),

		HomeAssistantEnabled: getEnv("HOMEASSISTANT_ENABLED", "") == "true",
		HomeAssistantKey:     getEnv("HOMEASSISTANT_KEY", ""),
	}

	if cfg.DataDir == "" {
		return nil, fmt.Errorf("DATA_DIR must not be empty")
	}

	if cfg.BackupDir == "" {
		cfg.BackupDir = cfg.DataDir + "/backups"
	}
	if cfg.BackupRetentionDays <= 0 {
		cfg.BackupRetentionDays = 30
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

// Validate checks that configuration values are within allowed ranges.
func (c *Config) Validate() error {
	if c.SchedulerHour < 0 || c.SchedulerHour > 23 {
		return fmt.Errorf("SCHEDULER_HOUR must be between 0 and 23, got %d", c.SchedulerHour)
	}
	if c.ReminderDays < 1 || c.ReminderDays > 365 {
		return fmt.Errorf("REMINDER_DAYS must be between 1 and 365, got %d", c.ReminderDays)
	}
	if c.SMTPPort < 1 || c.SMTPPort > 65535 {
		return fmt.Errorf("SMTP_PORT must be between 1 and 65535, got %d", c.SMTPPort)
	}
	if !validLogLevels[c.LogLevel] {
		return fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, error; got %q", c.LogLevel)
	}
	// NtfyPriority 0 means "not set"; Load() defaults it to 3.
	if c.NtfyPriority != 0 && (c.NtfyPriority < 1 || c.NtfyPriority > 5) {
		return fmt.Errorf("NTFY_PRIORITY must be between 1 and 5, got %d", c.NtfyPriority)
	}
	if c.ICalEventStart != "" {
		hour, minute, err := ParseClockTime(c.ICalEventStart)
		if err != nil {
			return fmt.Errorf("ICAL_EVENT_START must be in HH:MM format (e.g. 09:00), got %q", c.ICalEventStart)
		}
		if hour < 0 || hour > 23 {
			return fmt.Errorf("ICAL_EVENT_START hour must be between 0 and 23, got %d", hour)
		}
		if minute < 0 || minute > 59 {
			return fmt.Errorf("ICAL_EVENT_START minute must be between 0 and 59, got %d", minute)
		}
	}
	if c.ICalDurationMinutes < 1 || c.ICalDurationMinutes > 1440 {
		return fmt.Errorf("ICAL_EVENT_DURATION must be between 1 and 1440, got %d", c.ICalDurationMinutes)
	}
	if c.ICalEnabled && c.ICalFeedKey == "" {
		return fmt.Errorf("ICAL_FEED_KEY must be set when ICAL_FEED_ENABLED is true")
	}
	if c.RSSEnabled && c.RSSFeedKey == "" {
		return fmt.Errorf("RSS_FEED_KEY must be set when RSS_FEED_ENABLED is true")
	}
	if c.UpcomingAPIEnabled && c.UpcomingAPIKey == "" {
		return fmt.Errorf("UPCOMING_API_KEY must be set when UPCOMING_API_ENABLED is true")
	}
	if c.HomeAssistantEnabled && c.HomeAssistantKey == "" {
		return fmt.Errorf("HOMEASSISTANT_KEY must be set when HOMEASSISTANT_ENABLED is true")
	}
	return nil
}

// ParseClockTime parses a 24-hour "HH:MM" clock string (e.g. "09:30") into its
// hour and minute components.
func ParseClockTime(s string) (hour, minute int, err error) {
	parts := []byte(s)
	if len(parts) != 5 || parts[2] != ':' {
		return 0, 0, fmt.Errorf("invalid clock time %q", s)
	}
	hour = int(parts[0]-'0')*10 + int(parts[1]-'0')
	minute = int(parts[3]-'0')*10 + int(parts[4]-'0')
	if parts[0] < '0' || parts[0] > '9' || parts[1] < '0' || parts[1] > '9' ||
		parts[3] < '0' || parts[3] > '9' || parts[4] < '0' || parts[4] > '9' {
		return 0, 0, fmt.Errorf("invalid clock time %q", s)
	}
	return hour, minute, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvExplicit returns the env var value if set (including empty string),
// or the fallback if the var is not set at all. Uses os.LookupEnv to
// distinguish explicitly-set empty from unset.
func getEnvExplicit(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
