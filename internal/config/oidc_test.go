package config

import (
	"os"
	"testing"
)

func TestValidate_OIDCDisabledRequiresNothing(t *testing.T) {
	cfg := &Config{SchedulerHour: 8, ReminderDays: 7, SMTPPort: 587, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("disabled OIDC should validate: %v", err)
	}
}

func TestValidate_OIDCEnabledRequiresIssuerClientSecret(t *testing.T) {
	base := Config{SchedulerHour: 8, ReminderDays: 7, SMTPPort: 587, LogLevel: "info", DateVariant: "european", DataDir: "/db", ICalDurationMinutes: 60}
	for _, tc := range []struct {
		name   string
		issuer string
		id     string
		secret string
	}{
		{"missing all", "", "", ""},
		{"missing client", "https://authelia.example.com", "", "s3cret"},
		{"missing secret", "https://authelia.example.com", "datey", ""},
		{"bad issuer", "not-a-url", "datey", "s3cret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			cfg.OIDCEnabled = true
			cfg.OIDCIssuerURL = tc.issuer
			cfg.OIDCClientID = tc.id
			cfg.OIDCClientSecret = tc.secret
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	cfg := base
	cfg.OIDCEnabled = true
	cfg.OIDCIssuerURL = "https://authelia.example.com"
	cfg.OIDCClientID = "datey"
	cfg.OIDCClientSecret = "s3cret"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid OIDC config rejected: %v", err)
	}
}

func TestLoad_OIDCScopesDefault(t *testing.T) {
	os.Unsetenv("OIDC_SCOPES")
	os.Unsetenv("DATA_DIR")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.OIDCScopes != "openid email profile groups" {
		t.Errorf("OIDCScopes = %q, want default", cfg.OIDCScopes)
	}
}
