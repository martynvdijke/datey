package settings

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/internal/config"
	_ "github.com/mattn/go-sqlite3"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_settings_store?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return New(client)
}

func baseCfg() *config.Config {
	return &config.Config{
		Port:                8080,
		DataDir:             "/tmp/datey-test",
		SchedulerHour:       7,
		ReminderDays:        7,
		LogLevel:            "info",
		LogBufferSize:       100,
		OTLPEndpoint:        "",
		BackupDir:           "/tmp/datey-test/backups",
		BackupRetentionDays: 30,
		SMTPHost:            "smtp.example.com",
		SMTPPort:            587,
		SMTPUser:            "user",
		SMTPPass:            "pass",
		SMTPTLS:             true,
		SMTPTimeout:         10,
		NotifyEmail:         "notify@example.com",
		GotifyURL:           "",
		GotifyToken:         "",
		TelegramBotToken:     "",
		TelegramChatID:       "",
		UmamiURL:             "",
		UmamiWebsiteID:       "",
		EinkMode:            false,
	}
}

func TestEnsureSeeded_CreatesSingletonRow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.EnsureSeeded(ctx); err != nil {
		t.Fatalf("ensure seeded: %v", err)
	}
	row, err := s.Current(ctx)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	// EnsureSeeded is idempotent.
	if err := s.EnsureSeeded(ctx); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	row2, err := s.Current(ctx)
	if err != nil {
		t.Fatalf("current after re-seed: %v", err)
	}
	if row.ID != row2.ID {
		t.Errorf("EnsureSeeded created a second row: %d != %d", row.ID, row2.ID)
	}
}

func TestOverlay_NullRow_KeepsEnvValues(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.EnsureSeeded(ctx); err != nil {
		t.Fatalf("ensure seeded: %v", err)
	}
	want := baseCfg()
	got := baseCfg()
	if err := s.Overlay(ctx, got); err != nil {
		t.Fatalf("overlay: %v", err)
	}
	if *got != *want {
		t.Errorf("overlay on all-null row changed cfg:\n got  %+v\n want %+v", *got, *want)
	}
}

func TestOverlay_AppliesNonNullColumns(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.EnsureSeeded(ctx); err != nil {
		t.Fatalf("ensure seeded: %v", err)
	}
	row, err := s.Current(ctx)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	port, sched, reminder, loglevel, logbuf := 9100, 9, 21, "warn", 250
	smtpHost, smtpPort, smtpTLS, smtpTimeout := "mail.test", 465, false, 30
	gotifyURL, gotifyToken := "https://gotify.test", "gt-token"
	umamiURL := "https://umami.test"
	eink := true
	if _, err := s.client.AppConfig.UpdateOneID(row.ID).
		SetNillablePort(&port).
		SetNillableSchedulerHour(&sched).
		SetNillableReminderDays(&reminder).
		SetNillableLogLevel(&loglevel).
		SetNillableLogBufferSize(&logbuf).
		SetNillableSMTPHost(&smtpHost).
		SetNillableSMTPPort(&smtpPort).
		SetNillableSMTPTLS(&smtpTLS).
		SetNillableSMTPTimeout(&smtpTimeout).
		SetNillableGotifyURL(&gotifyURL).
		SetNillableGotifyToken(&gotifyToken).
		SetNillableUmamiURL(&umamiURL).
		SetNillableEinkMode(&eink).
		Save(ctx); err != nil {
		t.Fatalf("seed override: %v", err)
	}

	cfg := baseCfg()
	if err := s.Overlay(ctx, cfg); err != nil {
		t.Fatalf("overlay: %v", err)
	}
	if cfg.Port != port {
		t.Errorf("Port: got %d want %d", cfg.Port, port)
	}
	if cfg.SchedulerHour != sched {
		t.Errorf("SchedulerHour: got %d want %d", cfg.SchedulerHour, sched)
	}
	if cfg.ReminderDays != reminder {
		t.Errorf("ReminderDays: got %d want %d", cfg.ReminderDays, reminder)
	}
	if cfg.LogLevel != loglevel {
		t.Errorf("LogLevel: got %q want %q", cfg.LogLevel, loglevel)
	}
	if cfg.LogBufferSize != logbuf {
		t.Errorf("LogBufferSize: got %d want %d", cfg.LogBufferSize, logbuf)
	}
	if cfg.SMTPHost != smtpHost {
		t.Errorf("SMTPHost: got %q want %q", cfg.SMTPHost, smtpHost)
	}
	if cfg.SMTPPort != smtpPort {
		t.Errorf("SMTPPort: got %d want %d", cfg.SMTPPort, smtpPort)
	}
	if cfg.SMTPTLS != smtpTLS {
		t.Errorf("SMTPTLS: got %v want %v", cfg.SMTPTLS, smtpTLS)
	}
	if cfg.SMTPTimeout != smtpTimeout {
		t.Errorf("SMTPTimeout: got %d want %d", cfg.SMTPTimeout, smtpTimeout)
	}
	if cfg.GotifyURL != gotifyURL {
		t.Errorf("GotifyURL: got %q want %q", cfg.GotifyURL, gotifyURL)
	}
	if cfg.GotifyToken != gotifyToken {
		t.Errorf("GotifyToken: got %q want %q", cfg.GotifyToken, gotifyToken)
	}
	if cfg.UmamiURL != umamiURL {
		t.Errorf("UmamiURL: got %q want %q", cfg.UmamiURL, umamiURL)
	}
	if cfg.EinkMode != eink {
		t.Errorf("EinkMode: got %v want %v", cfg.EinkMode, eink)
	}
}

func TestOverlay_DataDirNeverOverlaid(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.EnsureSeeded(ctx); err != nil {
		t.Fatalf("ensure seeded: %v", err)
	}
	row, err := s.Current(ctx)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	dbDir := "/should/not/be/applied"
	if _, err := s.client.AppConfig.UpdateOneID(row.ID).
		SetNillableDataDir(&dbDir).
		Save(ctx); err != nil {
		t.Fatalf("set data_dir: %v", err)
	}
	cfg := baseCfg()
	want := cfg.DataDir
	if err := s.Overlay(ctx, cfg); err != nil {
		t.Fatalf("overlay: %v", err)
	}
	if cfg.DataDir != want {
		t.Errorf("Overlay applied DataDir: got %q want %q", cfg.DataDir, want)
	}
}

func TestApplyForm_Success_PersistsAndHotReloads(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.EnsureSeeded(ctx); err != nil {
		t.Fatalf("ensure seeded: %v", err)
	}
	cfg := baseCfg()

	form := url.Values{}
	form.Set("PORT", "9200")
	form.Set("SCHEDULER_HOUR", "5")
	form.Set("REMINDER_DAYS", "14")
	form.Set("LOG_LEVEL", "debug")
	form.Set("LOG_BUFFER_SIZE", "500")
	form.Set("SMTP_HOST", "mail.test")
	form.Set("SMTP_PORT", "465")
	form.Set("SMTP_USER", "u")
	form.Set("SMTP_PASS", "secret-pw")
	form.Set("SMTP_TLS", "on")
	form.Set("SMTP_TIMEOUT", "30")
	form.Set("NOTIFICATION_EMAIL", "ok@example.com")
	form.Set("GOTIFY_URL", "https://gotify.test")
	form.Set("GOTIFY_TOKEN", "tok")
	form.Set("UMAMI_URL", "https://umami.test")
	form.Set("UMAMI_WEBSITE_ID", "abc")
	form.Set("EINK_MODE", "on")

	errs, err := s.ApplyForm(ctx, cfg, form)
	if err != nil {
		t.Fatalf("apply form: %v (errs=%v)", err, errs)
	}
	if len(errs) > 0 {
		t.Fatalf("expected no validation errors, got %v", errs)
	}

	// Hot-reloadable fields mutate cfg.
	if cfg.ReminderDays != 14 {
		t.Errorf("ReminderDays hot-reload: got %d want 14", cfg.ReminderDays)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel hot-reload: got %q want debug", cfg.LogLevel)
	}
	if cfg.SMTPHost != "mail.test" {
		t.Errorf("SMTPHost hot-reload: got %q want mail.test", cfg.SMTPHost)
	}
	if cfg.SMTPPort != 465 {
		t.Errorf("SMTPPort hot-reload: got %d want 465", cfg.SMTPPort)
	}
	if !cfg.SMTPTLS {
		t.Error("SMTPTLS hot-reload: expected true")
	}
	if cfg.EinkMode != true {
		t.Error("EinkMode hot-reload: expected true")
	}

	// Restart-required fields are NOT mutated in memory.
	if cfg.Port == 9200 {
		t.Errorf("Port should NOT hot-reload (restart-required): got 9200, want original %d", baseCfg().Port)
	}
	if cfg.SchedulerHour == 5 {
		t.Errorf("SchedulerHour should NOT hot-reload (restart-required): got 5")
	}
	if cfg.LogBufferSize == 500 {
		t.Errorf("LogBufferSize should NOT hot-reload (restart-required): got 500")
	}

	// But persisted to DB.
	row, err := s.Current(ctx)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if row.Port == nil || *row.Port != 9200 {
		t.Errorf("DB Port not persisted, got %+v", row.Port)
	}
	if row.SchedulerHour == nil || *row.SchedulerHour != 5 {
		t.Errorf("DB SchedulerHour not persisted, got %+v", row.SchedulerHour)
	}
	if row.LogBufferSize == nil || *row.LogBufferSize != 500 {
		t.Errorf("DB LogBufferSize not persisted, got %+v", row.LogBufferSize)
	}
}

func TestApplyForm_Checkbox_UnsetMeansFalse(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.EnsureSeeded(ctx); err != nil {
		t.Fatalf("ensure seeded: %v", err)
	}
	cfg := baseCfg()
	cfg.SMTPTLS = true
	cfg.EinkMode = true

	form := url.Values{}
	// No SMTP_TLS, no EINK_MODE in form.
	_, err := s.ApplyForm(ctx, cfg, form)
	if err != nil {
		t.Fatalf("apply form: %v", err)
	}
	if cfg.SMTPTLS {
		t.Error("Expected SMTP_TLS false when checkbox omitted")
	}
	if cfg.EinkMode {
		t.Error("Expected EINK_MODE false when checkbox omitted")
	}
}

func TestApplyForm_ValidationErrors(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.EnsureSeeded(ctx); err != nil {
		t.Fatalf("ensure seeded: %v", err)
	}
	base := baseCfg()

	cases := []struct {
		name   string
		form   url.Values
		fields []string
	}{
		{"bad PORT", url.Values{"PORT": {"70000"}}, []string{"PORT"}},
		{"bad SCHEDULER_HOUR", url.Values{"SCHEDULER_HOUR": {"25"}}, []string{"SCHEDULER_HOUR"}},
		{"bad REMINDER_DAYS", url.Values{"REMINDER_DAYS": {"0"}}, []string{"REMINDER_DAYS"}},
		{"bad LOG_LEVEL", url.Values{"LOG_LEVEL": {"trace"}}, []string{"LOG_LEVEL"}},
		{"bad LOG_BUFFER_SIZE", url.Values{"LOG_BUFFER_SIZE": {"0"}}, []string{"LOG_BUFFER_SIZE"}},
		{"bad SMTP_PORT", url.Values{"SMTP_PORT": {"99999"}}, []string{"SMTP_PORT"}},
		{"bad timeout", url.Values{"SMTP_TIMEOUT": {"-1"}}, []string{"SMTP_TIMEOUT"}},
		{"non-numeric port", url.Values{"PORT": {"abc"}}, []string{"PORT"}},
		{"non-numeric retention", url.Values{"BACKUP_RETENTION_DAYS": {"twelve"}}, []string{"BACKUP_RETENTION_DAYS"}},
		{"multiple",
			url.Values{"PORT": {"0"}, "SCHEDULER_HOUR": {"99"}, "LOG_LEVEL": {"nope"}},
			[]string{"PORT", "SCHEDULER_HOUR", "LOG_LEVEL"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := baseCfg()
			if cfg.Port != base.Port {
				t.Fatalf("sanity: base port mismatch")
			}
			errs, err := s.ApplyForm(ctx, cfg, tc.form)
			if !errors.Is(err, errInvalid) {
				t.Fatalf("expected errInvalid sentinel, got %v", err)
			}
			for _, f := range tc.fields {
				if _, ok := errs[f]; !ok {
					t.Errorf("expected error for field %q, errs=%v", f, errs)
				}
			}
			// cfg must not be mutated when validation fails.
			if *cfg != *base {
				t.Errorf("validation failure mutated cfg:\n got  %+v\n want %+v", *cfg, *base)
			}
		})
	}
}

func TestApplyForm_EmptyNumericFallsBackToEnv(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.EnsureSeeded(ctx); err != nil {
		t.Fatalf("ensure seeded: %v", err)
	}
	cfg := baseCfg()

	form := url.Values{} // all empty -> cfg unchanged
	first, err := s.Current(ctx)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	_ = first
	errs, err := s.ApplyForm(ctx, cfg, form)
	if err != nil {
		t.Fatalf("apply empty form: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("expected no errors for empty form, got %v", errs)
	}

	row, err := s.Current(ctx)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if row.Port != nil {
		t.Errorf("empty form should leave Port NULL in DB, got %+v", row.Port)
	}
}