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

	UmamiURL       string
	UmamiWebsiteID string

	EinkMode bool
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

		BackupDir:           getEnv("BACKUP_DIR", ""),
		BackupRetentionDays: getEnvInt("BACKUP_RETENTION_DAYS", 0),

		UmamiURL:       getEnv("UMAMI_URL", ""),
		UmamiWebsiteID: getEnv("UMAMI_WEBSITE_ID", ""),

		EinkMode: getEnv("EINK_MODE", "") == "true",
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
	return nil
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
