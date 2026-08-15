package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoad_DefaultDataDir(t *testing.T) {
	os.Unsetenv("DATA_DIR")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.DataDir != "/db" {
		t.Errorf("DataDir = %q, want %q (must match Docker volume mount)", cfg.DataDir, "/db")
	}
}

func TestLoad_BackupDirDerivedFromDataDir(t *testing.T) {
	os.Unsetenv("DATA_DIR")
	os.Unsetenv("BACKUP_DIR")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	want := cfg.DataDir + "/backups"
	if cfg.BackupDir != want {
		t.Errorf("BackupDir = %q, want %q", cfg.BackupDir, want)
	}
}

func TestLoad_PortDefault(t *testing.T) {
	os.Unsetenv("PORT")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.Port != 6270 {
		t.Errorf("Port = %d, want %d", cfg.Port, 6270)
	}
}

func TestLoad_LogLevelDefault(t *testing.T) {
	os.Unsetenv("LOG_LEVEL")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
}

func TestLoad_BackupRetentionDefault(t *testing.T) {
	os.Unsetenv("BACKUP_RETENTION_DAYS")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.BackupRetentionDays != 30 {
		t.Errorf("BackupRetentionDays = %d, want %d", cfg.BackupRetentionDays, 30)
	}
}

func TestLoad_BackupDirFromEnvVar(t *testing.T) {
	t.Setenv("BACKUP_DIR", "/custom/backups")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.BackupDir != "/custom/backups" {
		t.Errorf("BackupDir = %q, want %q", cfg.BackupDir, "/custom/backups")
	}
}

func TestLoad_ExplicitDataDir(t *testing.T) {
	t.Setenv("DATA_DIR", "/custom/data")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.DataDir != "/custom/data" {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, "/custom/data")
	}
}

func TestLoad_EmptyDataDirExplicitlySet_Fails(t *testing.T) {
	t.Setenv("DATA_DIR", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for explicitly empty DATA_DIR, got nil")
	}
}

func TestLoad_BackupRetentionFromEnv(t *testing.T) {
	t.Setenv("BACKUP_RETENTION_DAYS", "90")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.BackupRetentionDays != 90 {
		t.Errorf("BackupRetentionDays = %d, want %d", cfg.BackupRetentionDays, 90)
	}
}

func TestLoad_WeeklyBackupDayDefault(t *testing.T) {
	os.Unsetenv("WEEKLY_BACKUP_DAY")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.WeeklyBackupDay != 0 {
		t.Errorf("WeeklyBackupDay = %d, want %d (Sunday)", cfg.WeeklyBackupDay, 0)
	}
}

func TestLoad_WeeklyBackupRetentionWeeksDefault(t *testing.T) {
	os.Unsetenv("WEEKLY_BACKUP_RETENTION_WEEKS")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.WeeklyBackupRetentionWeeks != 52 {
		t.Errorf("WeeklyBackupRetentionWeeks = %d, want %d", cfg.WeeklyBackupRetentionWeeks, 52)
	}
}

func TestValidate_WeeklyBackupDayOutOfRange(t *testing.T) {
	cfg := &Config{WeeklyBackupDay: 7, SchedulerHour: 8, ReminderDays: 7, SMTPPort: 587, LogLevel: "info", DateVariant: "european"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for WEEKLY_BACKUP_DAY=7, got nil")
	}
	if !strings.Contains(err.Error(), "WEEKLY_BACKUP_DAY") {
		t.Errorf("expected error mentioning WEEKLY_BACKUP_DAY, got %q", err)
	}
}


// --- Validation tests (task 2.7) ---

func TestValidate_SchedulerHourTooLow(t *testing.T) {
	cfg := &Config{SchedulerHour: -1, ReminderDays: 7, SMTPPort: 587, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for SCHEDULER_HOUR=-1, got nil")
	}
}

func TestValidate_SchedulerHourTooHigh(t *testing.T) {
	cfg := &Config{SchedulerHour: 24, ReminderDays: 7, SMTPPort: 587, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for SCHEDULER_HOUR=24, got nil")
	}
}

func TestValidate_SchedulerHourBoundary(t *testing.T) {
	for _, h := range []int{0, 12, 23} {
		cfg := &Config{SchedulerHour: h, ReminderDays: 7, SMTPPort: 587, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60}
		if err := cfg.Validate(); err != nil {
			t.Errorf("SCHEDULER_HOUR=%d should be valid, got error: %v", h, err)
		}
	}
}

func TestValidate_ReminderDaysTooLow(t *testing.T) {
	cfg := &Config{SchedulerHour: 8, ReminderDays: 0, SMTPPort: 587, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for REMINDER_DAYS=0, got nil")
	}
}

func TestValidate_ReminderDaysTooHigh(t *testing.T) {
	cfg := &Config{SchedulerHour: 8, ReminderDays: 366, SMTPPort: 587, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for REMINDER_DAYS=366, got nil")
	}
}

func TestValidate_ReminderDaysBoundary(t *testing.T) {
	for _, d := range []int{1, 180, 365} {
		cfg := &Config{SchedulerHour: 8, ReminderDays: d, SMTPPort: 587, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60}
		if err := cfg.Validate(); err != nil {
			t.Errorf("REMINDER_DAYS=%d should be valid, got error: %v", d, err)
		}
	}
}

func TestValidate_SMTPPortTooLow(t *testing.T) {
	cfg := &Config{SchedulerHour: 8, ReminderDays: 7, SMTPPort: 0, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for SMTP_PORT=0, got nil")
	}
}

func TestValidate_SMTPPortTooHigh(t *testing.T) {
	cfg := &Config{SchedulerHour: 8, ReminderDays: 7, SMTPPort: 65536, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for SMTP_PORT=65536, got nil")
	}
}

func TestValidate_SMTPPortBoundary(t *testing.T) {
	for _, p := range []int{1, 587, 65535} {
		cfg := &Config{SchedulerHour: 8, ReminderDays: 7, SMTPPort: p, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60}
		if err := cfg.Validate(); err != nil {
			t.Errorf("SMTP_PORT=%d should be valid, got error: %v", p, err)
		}
	}
}

func TestValidate_InvalidLogLevel(t *testing.T) {
	cfg := &Config{SchedulerHour: 8, ReminderDays: 7, SMTPPort: 587, LogLevel: "verbose", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for LOG_LEVEL=verbose, got nil")
	}
}

func TestValidate_EmptyLogLevel(t *testing.T) {
	cfg := &Config{SchedulerHour: 8, ReminderDays: 7, SMTPPort: 587, LogLevel: "", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty LOG_LEVEL, got nil")
	}
}

func TestValidate_ValidLogLevels(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error"} {
		cfg := &Config{SchedulerHour: 8, ReminderDays: 7, SMTPPort: 587, LogLevel: level, DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60}
		if err := cfg.Validate(); err != nil {
			t.Errorf("LOG_LEVEL=%q should be valid, got error: %v", level, err)
		}
	}
}

func TestValidate_AllValid(t *testing.T) {
	cfg := &Config{SchedulerHour: 8, ReminderDays: 7, SMTPPort: 587, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for valid config, got: %v", err)
	}
}

func TestValidate_ICalEventStartInvalid(t *testing.T) {
	for _, start := range []string{"9:00", "09:0", "0900", "aa:bb", "24:00", "09:60", "09 :00"} {
		cfg := &Config{SchedulerHour: 8, ReminderDays: 7, SMTPPort: 587, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60, ICalEventStart: start}
		if err := cfg.Validate(); err == nil {
			t.Errorf("ICAL_EVENT_START=%q should be invalid, got nil", start)
		}
	}
}

func TestValidate_ICalEventStartValid(t *testing.T) {
	for _, start := range []string{"", "00:00", "09:00", "23:59"} {
		cfg := &Config{SchedulerHour: 8, ReminderDays: 7, SMTPPort: 587, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60, ICalEventStart: start}
		if err := cfg.Validate(); err != nil {
			t.Errorf("ICAL_EVENT_START=%q should be valid, got error: %v", start, err)
		}
	}
}

func TestValidate_ICalDurationBoundary(t *testing.T) {
	for _, d := range []int{1, 60, 1440} {
		cfg := &Config{SchedulerHour: 8, ReminderDays: 7, SMTPPort: 587, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: d}
		if err := cfg.Validate(); err != nil {
			t.Errorf("ICAL_EVENT_DURATION=%d should be valid, got error: %v", d, err)
		}
	}
	for _, d := range []int{0, -1, 1441} {
		cfg := &Config{SchedulerHour: 8, ReminderDays: 7, SMTPPort: 587, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: d}
		if err := cfg.Validate(); err == nil {
			t.Errorf("ICAL_EVENT_DURATION=%d should be invalid, got nil", d)
		}
	}
}

func TestValidate_NtfyPriorityBoundary(t *testing.T) {
	for _, p := range []int{1, 3, 5} {
		cfg := &Config{SchedulerHour: 8, ReminderDays: 7, SMTPPort: 587, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60, NtfyPriority: p}
		if err := cfg.Validate(); err != nil {
			t.Errorf("NTFY_PRIORITY=%d should be valid, got error: %v", p, err)
		}
	}
	// 0 = unset (default 3) must be valid too.
	cfg := &Config{SchedulerHour: 8, ReminderDays: 7, SMTPPort: 587, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60}
	if err := cfg.Validate(); err != nil {
		t.Errorf("NTFY_PRIORITY unset (0) should be valid, got error: %v", err)
	}
	for _, p := range []int{-1, 6, 99} {
		cfg := &Config{SchedulerHour: 8, ReminderDays: 7, SMTPPort: 587, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60, NtfyPriority: p}
		if err := cfg.Validate(); err == nil {
			t.Errorf("NTFY_PRIORITY=%d should be invalid, got nil", p)
		}
	}
}

func TestValidate_PushEnabledRequiresKeys(t *testing.T) {
	cfg := &Config{SchedulerHour: 8, ReminderDays: 7, SMTPPort: 587, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60, PushEnabled: true}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for PUSH_ENABLED=true without VAPID keys, got nil")
	}

	cfg = &Config{SchedulerHour: 8, ReminderDays: 7, SMTPPort: 587, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60, PushEnabled: true, PushVAPIDPublicKey: "pub", PushVAPIDPrivateKey: "priv"}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config when both keys are set, got error: %v", err)
	}
}

func TestValidate_ICalEnabledRequiresKey(t *testing.T) {
	cfg := &Config{SchedulerHour: 8, ReminderDays: 7, SMTPPort: 587, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60, ICalEnabled: true}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for ICAL_FEED_ENABLED=true without ICAL_FEED_KEY, got nil")
	}

	cfg = &Config{SchedulerHour: 8, ReminderDays: 7, SMTPPort: 587, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60, ICalEnabled: true, ICalFeedKey: "secret"}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config when key is set, got error: %v", err)
	}
}
