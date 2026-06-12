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
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:          getEnvInt("PORT", 6270),
		DataDir:       getEnv("DATA_DIR", "/db"),
		SchedulerHour: getEnvInt("SCHEDULER_HOUR", 8),
		ReminderDays:  getEnvInt("REMINDER_DAYS", 7),
		LogLevel:      getEnv("LOG_LEVEL", "warn"),
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

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
