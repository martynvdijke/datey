package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
)

type Config struct {
	Port              int
	DataDir           string
	SchedulerHour     int
	ReminderDays      int
	SchedulerCatchup  bool
	DateVariant       string
	LogLevel          string
	LogBufferSize     int
	OTLPEndpoint      string

	// AppURL is the externally reachable base URL (e.g. "https://datey.example.com").
	// Used to build links inside emailed content such as password reset links.
	// When empty, links are derived from the incoming request instead.
	AppURL string

	BackupDir                string
	BackupRetentionDays      int
	WeeklyBackupDay          int
	WeeklyBackupRetentionWeeks int

	SMTPHost    string
	SMTPPort    int
	SMTPUser    string
	SMTPPass    string
	SMTPTLS     bool
	SMTPTimeout int
	NotifyEmail string

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

	PushEnabled         bool
	PushVAPIDPublicKey  string
	PushVAPIDPrivateKey string

	ImmichURL    string
	ImmichAPIKey string

	CarddavEnabled    bool
	CarddavURL        string
	CarddavUsername   string
	CarddavPassword   string
	CarddavDeletePolicy string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:             getEnvInt("PORT", 6270),
		DataDir:          getEnvExplicit("DATA_DIR", "/db"),
		SchedulerHour:    getEnvInt("SCHEDULER_HOUR", 8),
		ReminderDays:  getEnvInt("REMINDER_DAYS", 7),
		SchedulerCatchup: getEnvBool("SCHEDULER_CATCHUP", true),
		DateVariant:   getEnvExplicit("DATE_VARIANT", "european"),
		LogLevel:      getEnvExplicit("LOG_LEVEL", "info"),
		LogBufferSize:    getEnvInt("LOG_BUFFER_SIZE", 10000),
		OTLPEndpoint:     getEnv("OTEL_ENDPOINT", ""),
		AppURL:           getEnv("APP_URL", ""),
		SMTPHost:         getEnv("SMTP_HOST", ""),
		SMTPPort:         getEnvInt("SMTP_PORT", 587),
		SMTPUser:         getEnv("SMTP_USER", ""),
		SMTPPass:         getEnv("SMTP_PASS", ""),
		SMTPTLS:          getEnv("SMTP_TLS", "true") == "true",
		SMTPTimeout:      getEnvInt("SMTP_TIMEOUT", 10),
		NotifyEmail:      getEnv("NOTIFICATION_EMAIL", ""),
		GotifyURL:        getEnv("GOTIFY_URL", ""),
		GotifyToken:      getEnv("GOTIFY_TOKEN", ""),
		TelegramBotToken: getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:   getEnv("TELEGRAM_CHAT_ID", ""),
		NtfyURL:          getEnv("NTFY_URL", "https://ntfy.sh"),
		NtfyTopic:        getEnv("NTFY_TOPIC", ""),
		NtfyToken:        getEnv("NTFY_TOKEN", ""),
		NtfyPriority:     getEnvInt("NTFY_PRIORITY", 3),
		WebhookURL:       getEnv("WEBHOOK_URL", ""),
		WebhookSecret:    getEnv("WEBHOOK_SECRET", ""),

		BackupDir:                   getEnv("BACKUP_DIR", ""),
		BackupRetentionDays:         getEnvInt("BACKUP_RETENTION_DAYS", 0),
		WeeklyBackupDay:             getEnvInt("WEEKLY_BACKUP_DAY", 0),
		WeeklyBackupRetentionWeeks:  getEnvInt("WEEKLY_BACKUP_RETENTION_WEEKS", 52),

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

		PushEnabled:         getEnv("PUSH_ENABLED", "") == "true",
		PushVAPIDPublicKey:  getEnv("PUSH_VAPID_PUBLIC_KEY", ""),
		PushVAPIDPrivateKey: getEnv("PUSH_VAPID_PRIVATE_KEY", ""),
		ImmichURL:           getEnv("IMMICH_URL", ""),
		ImmichAPIKey:        getEnv("IMMICH_API_KEY", ""),

		CarddavEnabled:      getEnv("CARDDAV_ENABLED", "") == "true",
		CarddavURL:          getEnv("CARDDAV_URL", ""),
		CarddavUsername:     getEnv("CARDDAV_USERNAME", ""),
		CarddavPassword:     getEnv("CARDDAV_PASSWORD", ""),
		CarddavDeletePolicy: getEnv("CARDDAV_DELETE_POLICY", "keep"),
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

// validDateVariants lists the allowed date display variants. european renders
// day-first ("25 Dec"), us renders month-first ("Dec 25").
var validDateVariants = map[string]bool{
	"european": true,
	"us":       true,
}

// Validate checks that configuration values are within allowed ranges.
func (c *Config) Validate() error {
	if c.SchedulerHour < 0 || c.SchedulerHour > 23 {
		return fmt.Errorf("SCHEDULER_HOUR must be between 0 and 23, got %d", c.SchedulerHour)
	}
	if c.WeeklyBackupDay < 0 || c.WeeklyBackupDay > 6 {
		return fmt.Errorf("WEEKLY_BACKUP_DAY must be between 0 (Sunday) and 6 (Saturday), got %d", c.WeeklyBackupDay)
	}
	if c.ReminderDays < 1 || c.ReminderDays > 365 {
		return fmt.Errorf("REMINDER_DAYS must be between 1 and 365, got %d", c.ReminderDays)
	}
	if !validDateVariants[c.DateVariant] {
		return fmt.Errorf("DATE_VARIANT must be one of european, us; got %q", c.DateVariant)
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
	if c.PushEnabled && (c.PushVAPIDPublicKey == "" || c.PushVAPIDPrivateKey == "") {
		return fmt.Errorf("PUSH_VAPID_PUBLIC_KEY and PUSH_VAPID_PRIVATE_KEY must be set when PUSH_ENABLED is true")
	}
	if c.ImmichURL != "" {
		u, err := url.ParseRequestURI(c.ImmichURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("IMMICH_URL must be an absolute URL, got %q", c.ImmichURL)
		}
		if c.ImmichAPIKey == "" {
			return fmt.Errorf("IMMICH_API_KEY must be set when IMMICH_URL is configured")
		}
	}
	if c.CarddavURL != "" {
		u, err := url.ParseRequestURI(c.CarddavURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("CARDDAV_URL must be an absolute URL, got %q", c.CarddavURL)
		}
	}
	if c.CarddavDeletePolicy != "" && c.CarddavDeletePolicy != "keep" && c.CarddavDeletePolicy != "delete" {
		return fmt.Errorf("CARDDAV_DELETE_POLICY must be one of keep, delete; got %q", c.CarddavDeletePolicy)
	}
	if c.AppURL != "" {
		u, err := url.ParseRequestURI(c.AppURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("APP_URL must be an absolute URL, got %q", c.AppURL)
		}
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

// getEnvBool parses the env var as a bool (true/false/1/0/on/off), falling
// back to the default on unset or unparseable values.
func getEnvBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
