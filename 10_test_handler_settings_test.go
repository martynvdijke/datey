package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 2.1 Test GET /settings returns notifications tab with channel status table
func TestSettingsNotificationsTab(t *testing.T) {
	req := adminRequest("GET", "/settings", nil)
	rr := httptest.NewRecorder()
	testRouter.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /settings: expected 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "Settings") {
		t.Errorf("expected Settings title, got body missing 'Settings'")
	}
	if !strings.Contains(body, `href="/settings"`) {
		t.Errorf("expected Notifications tab link")
	}
	if !strings.Contains(body, "Notification Channels") {
		t.Errorf("expected Notification Channels section")
	}
	if !strings.Contains(body, "email") {
		t.Errorf("expected channel name in status table")
	}
	if !strings.Contains(body, "gotify") {
		t.Errorf("expected channel name in status table")
	}
}

// 2.2 Test GET /settings/config returns config table with secrets masked
func TestSettingsConfigTab(t *testing.T) {
	req := adminRequest("GET", "/settings/config", nil)
	rr := httptest.NewRecorder()
	testRouter.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /settings/config: expected 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, `href="/settings/config"`) {
		t.Errorf("expected Configuration tab link")
	}
	if !strings.Contains(body, "System Configuration") {
		t.Errorf("expected System Configuration heading")
	}
	if !strings.Contains(body, "Port") {
		t.Errorf("expected config key Port")
	}
	if !strings.Contains(body, "6270") {
		t.Errorf("expected config value 6270")
	}
	if strings.Contains(body, "****") {
		t.Errorf("expected masked secrets to not contain literal '****' in body (secrets should be empty/unconfigured)")
	}
}

// 2.3 Test GET /settings/logs returns log viewer in settings tab layout
func TestSettingsLogsTab(t *testing.T) {
	req := adminRequest("GET", "/settings/logs", nil)
	rr := httptest.NewRecorder()
	testRouter.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /settings/logs: expected 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, `href="/settings/logs"`) {
		t.Errorf("expected Logs tab link")
	}
	if !strings.Contains(body, "Application Logs") {
		t.Errorf("expected Application Logs heading")
	}
	if !strings.Contains(body, "Current:") {
		t.Errorf("expected current level display")
	}
}

// 2.4 Test GET /settings/logs with level query param
func TestSettingsLogsWithLevelFilter(t *testing.T) {
	req := adminRequest("GET", "/settings/logs?level=error", nil)
	rr := httptest.NewRecorder()
	testRouter.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /settings/logs?level=error: expected 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "Logs") {
		t.Errorf("expected Logs tab content")
	}
}

// 2.5 Test GET /settings/backup shows backup config and trigger button
func TestSettingsBackupTab(t *testing.T) {
	req := adminRequest("GET", "/settings/backup", nil)
	rr := httptest.NewRecorder()
	testRouter.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /settings/backup: expected 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, `href="/settings/backup"`) {
		t.Errorf("expected Backups tab link")
	}
	if !strings.Contains(body, "Database Backups") {
		t.Errorf("expected Database Backups heading")
	}
	if !strings.Contains(body, "Run Backup Now") {
		t.Errorf("expected Run Backup Now button")
	}
	if !strings.Contains(body, "Backup Directory") {
		t.Errorf("expected Backup Directory info")
	}
}

// 2.6 Test POST /settings/backup triggers backup
func TestSettingsBackupRun(t *testing.T) {
	req := adminRequest("POST", "/settings/backup", nil)
	rr := httptest.NewRecorder()
	testRouter.ServeHTTP(rr, req)

	// When a real datey.db file exists in the test data dir, backup should succeed
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /settings/backup: expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	if !strings.Contains(body, "Backup completed") {
		t.Errorf("expected success message, got: %s", body[:min(len(body), 200)])
	}
}
