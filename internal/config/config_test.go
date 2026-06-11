package config

import (
	"os"
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

func TestLoad_BackupDirFromEnvVar(t *testing.T) {
	os.Setenv("BACKUP_DIR", "/custom/backups")
	defer os.Unsetenv("BACKUP_DIR")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.BackupDir != "/custom/backups" {
		t.Errorf("BackupDir = %q, want %q", cfg.BackupDir, "/custom/backups")
	}
}

func TestLoad_ExplicitDataDir(t *testing.T) {
	os.Setenv("DATA_DIR", "/custom/data")
	defer os.Unsetenv("DATA_DIR")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.DataDir != "/custom/data" {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, "/custom/data")
	}
}

func TestLoad_EmptyDataDirFallsBackToDefault(t *testing.T) {
	// Setting DATA_DIR="" should fall back to "/db" because getEnv returns fallback for empty
	os.Setenv("DATA_DIR", "")
	defer os.Unsetenv("DATA_DIR")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.DataDir != "/db" {
		t.Errorf("DataDir = %q, want default %q", cfg.DataDir, "/db")
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
	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "warn")
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

func TestLoad_BackupRetentionFromEnv(t *testing.T) {
	os.Setenv("BACKUP_RETENTION_DAYS", "90")
	defer os.Unsetenv("BACKUP_RETENTION_DAYS")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.BackupRetentionDays != 90 {
		t.Errorf("BackupRetentionDays = %d, want %d", cfg.BackupRetentionDays, 90)
	}
}
