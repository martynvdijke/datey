package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/datey/datey/internal/reminderplan"
	"github.com/datey/datey/internal/tzutil"
)

type Config struct {
	Port          int
	DataDir       string
	SchedulerHour int
	ReminderDays  int
	// ReminderStages is the ascending list of lead times (in days) at which a
	// reminder is sent. It always contains at least one entry and supersedes
	// ReminderDays, which remains the single-stage fallback.
	ReminderStages []int
	// ReminderDigest sends one combined message per channel per day instead of
	// an individual message per event.
	ReminderDigest   bool
	SchedulerCatchup bool
	// SubscriptionPollHours controls how often enabled ICS subscriptions are
	// fetched. Zero disables automatic polling.
	SubscriptionPollHours int
	DateVariant           string
	// Timezone is the instance-wide IANA zone used for people who do not have
	// their own. Empty means the server's local zone.
	Timezone      string
	LogLevel      string
	LogBufferSize int
	OTLPEndpoint  string

	// AppURL is the externally reachable base URL (e.g. "https://datey.example.com").
	// Used to build links inside emailed content such as password reset links.
	// When empty, links are derived from the incoming request instead.
	AppURL string

	// Demo account. Only meant for the hosted demo instance: the credentials
	// are advertised publicly on the login page, and the account is a normal
	// read-write user. Demo seeding is skipped on instances that already have
	// people in the shared address book.
	DemoUserEnabled  bool
	DemoUserHandle   string
	DemoUserPassword string
	DemoUserEmail    string

	BackupDir                  string
	BackupRetentionDays        int
	WeeklyBackupDay            int
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

	TelegramBotToken      string
	TelegramChatID        string
	TelegramWebhookSecret string

	NtfyURL      string
	NtfyTopic    string
	NtfyToken    string
	NtfyPriority int

	PushoverToken string
	PushoverUser  string

	WebhookURL    string
	WebhookSecret string

	DiscordWebhookURL string
	SlackWebhookURL   string

	MatrixHomeserverURL string
	MatrixAccessToken   string
	MatrixRoomID        string

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

	TRMNLEnabled bool
	TRMNLKey     string

	PushEnabled         bool
	PushVAPIDPublicKey  string
	PushVAPIDPrivateKey string

	// Firebase Cloud Messaging for the Android app. The service-account JSON is
	// a secret and must never be logged or rendered in clear.
	FCMEnabled            bool
	FCMProjectID          string
	FCMServiceAccountJSON string
	// NotifyNewItems is the instance-wide default for "notify me when someone
	// adds a new item". True when unset (opt-out).
	NotifyNewItems bool

	ImmichURL    string
	ImmichAPIKey string

	CarddavEnabled      bool
	CarddavURL          string
	CarddavUsername     string
	CarddavPassword     string
	CarddavDeletePolicy string

	GoogleContactsEnabled bool
	GoogleClientID        string
	GoogleClientSecret    string
	GoogleRefreshToken    string
	GoogleSyncToken       string
	GoogleDeletePolicy    string

	OIDCEnabled          bool
	OIDCIssuerURL        string
	OIDCClientID         string
	OIDCClientSecret     string
	OIDCClientSecretFile string
	OIDCRedirectURL      string
	OIDCScopes           string

	AuditRetentionMax int

	// Legal identity of the instance operator. These appear verbatim in the
	// published privacy policy, terms and deletion page, so they are
	// deployment configuration rather than runtime-editable content.
	LegalControllerName string
	LegalContactEmail   string
	LegalContactAddress string
	LegalHostingRegion  string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:                  getEnvInt("PORT", 6270),
		DataDir:               getEnvExplicit("DATA_DIR", "/db"),
		SchedulerHour:         getEnvInt("SCHEDULER_HOUR", 8),
		ReminderDays:          getEnvInt("REMINDER_DAYS", 7),
		SchedulerCatchup:      getEnvBool("SCHEDULER_CATCHUP", true),
		SubscriptionPollHours: getEnvInt("SUBSCRIPTION_POLL_HOURS", 24),
		DateVariant:           getEnvExplicit("DATE_VARIANT", "european"),
		Timezone:              getEnv("TIMEZONE", ""),
		LogLevel:              getEnvExplicit("LOG_LEVEL", "info"),
		LogBufferSize:         getEnvInt("LOG_BUFFER_SIZE", 10000),
		OTLPEndpoint:          getEnv("OTEL_ENDPOINT", ""),
		AppURL:                getEnv("APP_URL", ""),
		DemoUserEnabled:       getEnvBool("DEMO_USER_ENABLED", isDemoHost(getEnv("APP_URL", ""))),
		DemoUserHandle:        strings.ToLower(getEnvExplicit("DEMO_USER_HANDLE", "demo")),
		DemoUserPassword:      getEnvExplicit("DEMO_USER_PASSWORD", "demo"),
		DemoUserEmail:         getEnv("DEMO_USER_EMAIL", "demo@datey.local"),
		SMTPHost:              getEnv("SMTP_HOST", ""),
		SMTPPort:              getEnvInt("SMTP_PORT", 587),
		SMTPUser:              getEnv("SMTP_USER", ""),
		SMTPPass:              getEnv("SMTP_PASS", ""),
		SMTPTLS:               getEnv("SMTP_TLS", "true") == "true",
		SMTPTimeout:           getEnvInt("SMTP_TIMEOUT", 10),
		NotifyEmail:           getEnv("NOTIFICATION_EMAIL", ""),
		GotifyURL:             getEnv("GOTIFY_URL", ""),
		GotifyToken:           getEnv("GOTIFY_TOKEN", ""),
		TelegramBotToken:      getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:        getEnv("TELEGRAM_CHAT_ID", ""),
		TelegramWebhookSecret: getEnv("TELEGRAM_WEBHOOK_SECRET", ""),
		NtfyURL:               getEnv("NTFY_URL", "https://ntfy.sh"),
		NtfyTopic:             getEnv("NTFY_TOPIC", ""),
		NtfyToken:             getEnv("NTFY_TOKEN", ""),
		NtfyPriority:          getEnvInt("NTFY_PRIORITY", 3),
		PushoverToken:         getEnv("PUSHOVER_TOKEN", ""),
		PushoverUser:          getEnv("PUSHOVER_USER", ""),
		WebhookURL:            getEnv("WEBHOOK_URL", ""),
		WebhookSecret:         getEnv("WEBHOOK_SECRET", ""),

		DiscordWebhookURL:   getEnv("DISCORD_WEBHOOK_URL", ""),
		SlackWebhookURL:     getEnv("SLACK_WEBHOOK_URL", ""),
		MatrixHomeserverURL: getEnv("MATRIX_HOMESERVER_URL", ""),
		MatrixAccessToken:   getEnv("MATRIX_ACCESS_TOKEN", ""),
		MatrixRoomID:        getEnv("MATRIX_ROOM_ID", ""),

		BackupDir:                  getEnv("BACKUP_DIR", ""),
		BackupRetentionDays:        getEnvInt("BACKUP_RETENTION_DAYS", 0),
		WeeklyBackupDay:            getEnvInt("WEEKLY_BACKUP_DAY", 0),
		WeeklyBackupRetentionWeeks: getEnvInt("WEEKLY_BACKUP_RETENTION_WEEKS", 52),

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
		TRMNLEnabled:         getEnv("TRMNL_ENABLED", "") == "true",
		TRMNLKey:             getEnv("TRMNL_KEY", ""),

		PushEnabled:           getEnv("PUSH_ENABLED", "") == "true",
		PushVAPIDPublicKey:    getEnv("PUSH_VAPID_PUBLIC_KEY", ""),
		PushVAPIDPrivateKey:   getEnv("PUSH_VAPID_PRIVATE_KEY", ""),
		FCMEnabled:            getEnvBool("FCM_ENABLED", false),
		FCMProjectID:          getEnv("FCM_PROJECT_ID", ""),
		FCMServiceAccountJSON: getEnv("FCM_SERVICE_ACCOUNT_JSON", ""),
		NotifyNewItems:        getEnvBool("NOTIFY_NEW_ITEMS", true),
		ImmichURL:             getEnv("IMMICH_URL", ""),
		ImmichAPIKey:          getEnv("IMMICH_API_KEY", ""),

		CarddavEnabled:      getEnv("CARDDAV_ENABLED", "") == "true",
		CarddavURL:          getEnv("CARDDAV_URL", ""),
		CarddavUsername:     getEnv("CARDDAV_USERNAME", ""),
		CarddavPassword:     getEnv("CARDDAV_PASSWORD", ""),
		CarddavDeletePolicy: getEnv("CARDDAV_DELETE_POLICY", "keep"),

		GoogleContactsEnabled: getEnv("GOOGLE_CONTACTS_ENABLED", "") == "true",
		GoogleClientID:        getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret:    getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRefreshToken:    getEnv("GOOGLE_REFRESH_TOKEN", ""),
		GoogleSyncToken:       getEnv("GOOGLE_SYNC_TOKEN", ""),
		GoogleDeletePolicy:    getEnv("GOOGLE_DELETE_POLICY", "keep"),

		OIDCEnabled:          getEnv("OIDC_ENABLED", "") == "true",
		OIDCIssuerURL:        getEnv("OIDC_ISSUER_URL", ""),
		OIDCClientID:         getEnv("OIDC_CLIENT_ID", ""),
		OIDCClientSecret:     getEnv("OIDC_CLIENT_SECRET", ""),
		OIDCClientSecretFile: getEnv("OIDC_CLIENT_SECRET_FILE", ""),
		OIDCRedirectURL:      getEnv("OIDC_REDIRECT_URL", ""),
		OIDCScopes:           getEnv("OIDC_SCOPES", "openid email profile groups"),

		AuditRetentionMax: getEnvInt("AUDIT_RETENTION_MAX", 10000),

		LegalControllerName: strings.TrimSpace(getEnv("LEGAL_CONTROLLER_NAME", "")),
		LegalContactEmail:   strings.TrimSpace(getEnv("LEGAL_CONTACT_EMAIL", "")),
		LegalContactAddress: strings.TrimSpace(getEnv("LEGAL_CONTACT_ADDRESS", "")),
		LegalHostingRegion:  strings.TrimSpace(getEnv("LEGAL_HOSTING_REGION", "")),
	}
	// Secret file wins over env value so Docker secrets work without leaking
	// the value into the process environment (file content is trimmed).
	if cfg.OIDCClientSecretFile != "" {
		if b, err := os.ReadFile(cfg.OIDCClientSecretFile); err == nil {
			if s := string(b); s != "" {
				cfg.OIDCClientSecret = trimSpaceNewline(s)
			}
		} else if cfg.OIDCEnabled {
			return nil, fmt.Errorf("OIDC_CLIENT_SECRET_FILE unreadable: %w", err)
		}
	}
	if cfg.AuditRetentionMax < 100 {
		cfg.AuditRetentionMax = 100
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

	if raw := strings.TrimSpace(os.Getenv("REMINDER_STAGES")); raw != "" {
		st, err := reminderplan.ParseStages(raw)
		if err != nil {
			return nil, fmt.Errorf("REMINDER_STAGES: %w", err)
		}
		cfg.ReminderStages = st
	}
	if len(cfg.ReminderStages) == 0 {
		cfg.ReminderStages = []int{cfg.ReminderDays}
	}
	cfg.ReminderDigest = getEnvBool("REMINDER_DIGEST", false)

	if cfg.Timezone != "" {
		if err := tzutil.Validate(cfg.Timezone); err != nil {
			return nil, fmt.Errorf("TIMEZONE: %w", err)
		}
	}
	tzutil.SetDefault(cfg.Timezone)

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
	if c.SubscriptionPollHours < 0 || c.SubscriptionPollHours > 168 {
		return fmt.Errorf("SUBSCRIPTION_POLL_HOURS must be between 0 and 168, got %d", c.SubscriptionPollHours)
	}
	if c.WeeklyBackupDay < 0 || c.WeeklyBackupDay > 6 {
		return fmt.Errorf("WEEKLY_BACKUP_DAY must be between 0 (Sunday) and 6 (Saturday), got %d", c.WeeklyBackupDay)
	}
	if c.ReminderDays < 1 || c.ReminderDays > 365 {
		return fmt.Errorf("REMINDER_DAYS must be between 1 and 365, got %d", c.ReminderDays)
	}
	if len(c.ReminderStages) > 0 {
		if len(c.ReminderStages) > reminderplan.MaxStages {
			return fmt.Errorf("REMINDER_STAGES must have at most %d entries", reminderplan.MaxStages)
		}
		for i, st := range c.ReminderStages {
			if st < reminderplan.MinStage || st > reminderplan.MaxStage {
				return fmt.Errorf("REMINDER_STAGES entries must be between %d and %d, got %d", reminderplan.MinStage, reminderplan.MaxStage, st)
			}
			if i > 0 && st <= c.ReminderStages[i-1] {
				return fmt.Errorf("REMINDER_STAGES must be strictly ascending")
			}
		}
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
	if c.TRMNLEnabled && c.TRMNLKey == "" {
		return fmt.Errorf("TRMNL_KEY must be set when TRMNL_ENABLED is true")
	}
	if c.PushEnabled && (c.PushVAPIDPublicKey == "" || c.PushVAPIDPrivateKey == "") {
		return fmt.Errorf("PUSH_VAPID_PUBLIC_KEY and PUSH_VAPID_PRIVATE_KEY must be set when PUSH_ENABLED is true")
	}
	if c.FCMEnabled && (c.FCMProjectID == "" || c.FCMServiceAccountJSON == "") {
		return fmt.Errorf("FCM_PROJECT_ID and FCM_SERVICE_ACCOUNT_JSON must be set when FCM_ENABLED is true")
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
	if c.GoogleDeletePolicy != "" && c.GoogleDeletePolicy != "keep" && c.GoogleDeletePolicy != "delete" {
		return fmt.Errorf("GOOGLE_DELETE_POLICY must be one of keep, delete; got %q", c.GoogleDeletePolicy)
	}
	if c.AuditRetentionMax != 0 && c.AuditRetentionMax < 100 {
		return fmt.Errorf("AUDIT_RETENTION_MAX must be at least 100, got %d", c.AuditRetentionMax)
	}
	if c.AppURL != "" {
		u, err := url.ParseRequestURI(c.AppURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("APP_URL must be an absolute URL, got %q", c.AppURL)
		}
	}
	if c.DiscordWebhookURL != "" {
		u, err := url.ParseRequestURI(c.DiscordWebhookURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("DISCORD_WEBHOOK_URL must be an absolute URL, got %q", c.DiscordWebhookURL)
		}
	}
	if c.SlackWebhookURL != "" {
		u, err := url.ParseRequestURI(c.SlackWebhookURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("SLACK_WEBHOOK_URL must be an absolute URL, got %q", c.SlackWebhookURL)
		}
	}
	if c.MatrixHomeserverURL != "" {
		u, err := url.ParseRequestURI(c.MatrixHomeserverURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("MATRIX_HOMESERVER_URL must be an absolute URL, got %q", c.MatrixHomeserverURL)
		}
	}
	if c.OIDCEnabled {
		if c.OIDCIssuerURL == "" {
			return fmt.Errorf("OIDC_ISSUER_URL must be set when OIDC_ENABLED is true")
		}
		if u, err := url.ParseRequestURI(c.OIDCIssuerURL); err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("OIDC_ISSUER_URL must be an absolute URL, got %q", c.OIDCIssuerURL)
		}
		if c.OIDCClientID == "" {
			return fmt.Errorf("OIDC_CLIENT_ID must be set when OIDC_ENABLED is true")
		}
		if c.OIDCClientSecret == "" {
			return fmt.Errorf("OIDC_CLIENT_SECRET or OIDC_CLIENT_SECRET_FILE must be set when OIDC_ENABLED is true")
		}
		if c.OIDCRedirectURL != "" {
			if u, err := url.ParseRequestURI(c.OIDCRedirectURL); err != nil || u.Scheme == "" || u.Host == "" {
				return fmt.Errorf("OIDC_REDIRECT_URL must be an absolute URL, got %q", c.OIDCRedirectURL)
			}
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

// isDemoHost reports whether appURL points at the hosted demo instance. It
// tolerates a missing scheme so a bare hostname still resolves, and returns
// false for empty or unparseable input.
func isDemoHost(appURL string) bool {
	appURL = strings.TrimSpace(appURL)
	if appURL == "" {
		return false
	}
	u, err := url.Parse(appURL)
	if err != nil || u.Host == "" {
		u, err = url.Parse("https://" + appURL)
		if err != nil {
			return false
		}
	}
	return strings.EqualFold(u.Hostname(), "datey.vandijke.xyz")
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

// trimSpaceNewline trims surrounding whitespace including trailing newlines
// (Docker secrets often end with a newline).
func trimSpaceNewline(s string) string {
	return strings.TrimSpace(s)
}
