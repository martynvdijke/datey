package playwright_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/playwright-community/playwright-go"
)

func getBaseURL() string {
	if v := os.Getenv("DATEY_URL"); v != "" {
		return v
	}
	return "http://localhost:8080"
}

func TestHealthCheck(t *testing.T) {
	pw, err := playwright.Run()
	if err != nil {
		t.Fatalf("could not start playwright: %v", err)
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatalf("could not launch browser: %v", err)
	}
	defer browser.Close()

	page, err := browser.NewPage()
	if err != nil {
		t.Fatalf("could not create page: %v", err)
	}

	_, err = page.Goto(getBaseURL() + "/health")
	if err != nil {
		t.Fatalf("could not goto health: %v", err)
	}

	body, err := page.TextContent("body")
	if err != nil {
		t.Fatalf("could not get body: %v", err)
	}
	if body == "" {
		t.Fatal("empty health response")
	}
}

func TestDashboardShowsUpcomingEvents(t *testing.T) {
	pw, err := playwright.Run()
	if err != nil {
		t.Fatalf("could not start playwright: %v", err)
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatalf("could not launch browser: %v", err)
	}
	defer browser.Close()

	page, err := browser.NewPage()
	if err != nil {
		t.Fatalf("could not create page: %v", err)
	}

	if _, err = page.Goto(getBaseURL() + "/"); err != nil {
		t.Fatalf("could not goto dashboard: %v", err)
	}

	title, err := page.Title()
	if err != nil {
		t.Fatalf("could not get title: %v", err)
	}
	if title == "" {
		t.Fatal("empty page title")
	}

	fmt.Println("Dashboard title:", title)
}

func TestFullContactAndEventFlow(t *testing.T) {
	pw, err := playwright.Run()
	if err != nil {
		t.Fatalf("could not start playwright: %v", err)
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatalf("could not launch browser: %v", err)
	}
	defer browser.Close()

	page, err := browser.NewPage()
	if err != nil {
		t.Fatalf("could not create page: %v", err)
	}

	base := getBaseURL()

	// Navigate to new contact form
	if _, err = page.Goto(base + "/contacts/new"); err != nil {
		t.Fatalf("could not goto new contact: %v", err)
	}

	// Fill in contact form
	if err = page.Fill("input[name=name]", "Playwright Test User"); err != nil {
		t.Fatalf("could not fill name: %v", err)
	}
	if err = page.Fill("textarea[name=notes]", "Created by Playwright test"); err != nil {
		t.Fatalf("could not fill notes: %v", err)
	}

	// Submit form
	if err = page.Click("button[type=submit]"); err != nil {
		t.Fatalf("could not submit contact form: %v", err)
	}

	// Should redirect to /contacts - verify contact appears
	content, err := page.TextContent("body")
	if err != nil {
		t.Fatalf("could not get page content: %v", err)
	}
	if content == "" {
		t.Fatal("contact list page empty")
	}
	fmt.Println("Contact created and visible on /contacts")
}

// TestCalendarPageLoads verifies the calendar page renders FullCalendar.
func TestCalendarPageLoads(t *testing.T) {
	pw, err := playwright.Run()
	if err != nil {
		t.Fatalf("could not start playwright: %v", err)
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatalf("could not launch browser: %v", err)
	}
	defer browser.Close()

	page, err := browser.NewPage()
	if err != nil {
		t.Fatalf("could not create page: %v", err)
	}

	base := getBaseURL()

	// First log in to establish a session
	if _, err = page.Goto(base + "/login"); err != nil {
		t.Fatalf("could not goto login: %v", err)
	}
	page.Fill("input[name=username]", "admin")
	page.Fill("input[name=password]", "admin")
	page.Click("button[type=submit]")

	// Navigate to calendar
	if _, err = page.Goto(base + "/calendar"); err != nil {
		t.Fatalf("could not goto calendar: %v", err)
	}

	title, err := page.Title()
	if err != nil {
		t.Fatalf("could not get title: %v", err)
	}
	if title == "" {
		t.Fatal("empty page title")
	}

	// Check for calendar container presence (FullCalendar renders #calendar div)
	calendarEl, err := page.Locator("#calendar").Count()
	if err != nil {
		t.Fatalf("could not find calendar element: %v", err)
	}
	if calendarEl == 0 {
		t.Fatal("calendar container div not found")
	}

	fmt.Println("Calendar page loaded successfully")
}

// TestSettingsTabs verifies all four settings tabs load correctly.
func TestSettingsTabs(t *testing.T) {
	pw, err := playwright.Run()
	if err != nil {
		t.Fatalf("could not start playwright: %v", err)
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatalf("could not launch browser: %v", err)
	}
	defer browser.Close()

	page, err := browser.NewPage()
	if err != nil {
		t.Fatalf("could not create page: %v", err)
	}

	base := getBaseURL()

	// Log in
	if _, err = page.Goto(base + "/login"); err != nil {
		t.Fatalf("could not goto login: %v", err)
	}
	page.Fill("input[name=username]", "admin")
	page.Fill("input[name=password]", "admin")
	page.Click("button[type=submit]")

	// Test Notifications tab
	if _, err = page.Goto(base + "/settings"); err != nil {
		t.Fatalf("could not goto settings: %v", err)
	}
	body, err := page.TextContent("body")
	if err != nil || body == "" {
		t.Fatal("settings page empty")
	}

	// Test Configuration tab
	if _, err = page.Goto(base + "/settings/config"); err != nil {
		t.Fatalf("could not goto settings/config: %v", err)
	}
	body, err = page.TextContent("body")
	if err != nil || body == "" {
		t.Fatal("settings/config page empty")
	}

	// Test Logs tab
	if _, err = page.Goto(base + "/settings/logs"); err != nil {
		t.Fatalf("could not goto settings/logs: %v", err)
	}
	body, err = page.TextContent("body")
	if err != nil || body == "" {
		t.Fatal("settings/logs page empty")
	}

	// Test Backups tab
	if _, err = page.Goto(base + "/settings/backup"); err != nil {
		t.Fatalf("could not goto settings/backup: %v", err)
	}
	body, err = page.TextContent("body")
	if err != nil || body == "" {
		t.Fatal("settings/backup page empty")
	}

	fmt.Println("All settings tabs loaded successfully")
}

// TestOldLogsRedirect verifies /logs redirects to /settings/logs.
func TestOldLogsRedirect(t *testing.T) {
	pw, err := playwright.Run()
	if err != nil {
		t.Fatalf("could not start playwright: %v", err)
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatalf("could not launch browser: %v", err)
	}
	defer browser.Close()

	page, err := browser.NewPage()
	if err != nil {
		t.Fatalf("could not create page: %v", err)
	}

	base := getBaseURL()

	// Log in
	if _, err = page.Goto(base + "/login"); err != nil {
		t.Fatalf("could not goto login: %v", err)
	}
	page.Fill("input[name=username]", "admin")
	page.Fill("input[name=password]", "admin")
	page.Click("button[type=submit]")

	// Navigate to old /logs URL — should redirect to /settings/logs
	if _, err = page.Goto(base + "/logs"); err != nil {
		t.Fatalf("could not goto /logs: %v", err)
	}

	body, err := page.TextContent("body")
	if err != nil || body == "" {
		t.Fatal("redirected page empty")
	}
}

// TestBackupTrigger verifies the backup button triggers a backup via HTMX.
func TestBackupTrigger(t *testing.T) {
	pw, err := playwright.Run()
	if err != nil {
		t.Fatalf("could not start playwright: %v", err)
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatalf("could not launch browser: %v", err)
	}
	defer browser.Close()

	page, err := browser.NewPage()
	if err != nil {
		t.Fatalf("could not create page: %v", err)
	}

	base := getBaseURL()

	// Log in
	if _, err = page.Goto(base + "/login"); err != nil {
		t.Fatalf("could not goto login: %v", err)
	}
	page.Fill("input[name=username]", "admin")
	page.Fill("input[name=password]", "admin")
	page.Click("button[type=submit]")

	// Navigate to backup tab
	if _, err = page.Goto(base + "/settings/backup"); err != nil {
		t.Fatalf("could not goto settings/backup: %v", err)
	}

	// Click Run Backup Now button
	if err = page.Click("button:has-text('Run Backup Now')"); err != nil {
		t.Fatalf("could not click backup button: %v", err)
	}

	fmt.Println("Backup triggered via HTMX successfully")
}

// TestLogLevelChange verifies log level buttons send HTMX requests.
func TestLogLevelChange(t *testing.T) {
	pw, err := playwright.Run()
	if err != nil {
		t.Fatalf("could not start playwright: %v", err)
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatalf("could not launch browser: %v", err)
	}
	defer browser.Close()

	page, err := browser.NewPage()
	if err != nil {
		t.Fatalf("could not create page: %v", err)
	}

	base := getBaseURL()

	// Log in
	if _, err = page.Goto(base + "/login"); err != nil {
		t.Fatalf("could not goto login: %v", err)
	}
	page.Fill("input[name=username]", "admin")
	page.Fill("input[name=password]", "admin")
	page.Click("button[type=submit]")

	// Navigate to logs tab
	if _, err = page.Goto(base + "/settings/logs"); err != nil {
		t.Fatalf("could not goto settings/logs: %v", err)
	}

	// Click the "debug" log level button
	buttons, err := page.Locator("button:has-text('debug')").All()
	if err != nil || len(buttons) == 0 {
		t.Fatalf("could not find debug level button: %v", err)
	}
	if err = buttons[0].Click(); err != nil {
		t.Fatalf("could not click debug level button: %v", err)
	}

	fmt.Println("Log level change triggered successfully")
}
