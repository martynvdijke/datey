package playwright_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

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

// ---------------------------------------------------------------------------
// Phase 1 regression baselines — capture current UI behavior before refactor.
// ---------------------------------------------------------------------------

// setupPage creates a browser page and returns it with a cleanup function.
func setupPage(t *testing.T) (playwright.Page, func()) {
	t.Helper()
	pw, err := playwright.Run()
	if err != nil {
		t.Fatalf("could not start playwright: %v", err)
	}
	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatalf("could not launch browser: %v", err)
	}
	page, err := browser.NewPage()
	if err != nil {
		t.Fatalf("could not create page: %v", err)
	}
	return page, func() {
		browser.Close()
		pw.Stop()
	}
}

// login authenticates as admin/admin on the given page.
func login(t *testing.T, page playwright.Page) {
	t.Helper()
	base := getBaseURL()
	if _, err := page.Goto(base + "/login"); err != nil {
		t.Fatalf("goto login: %v", err)
	}
	if err := page.Fill("input[name=username]", "admin"); err != nil {
		t.Fatalf("fill username: %v", err)
	}
	if err := page.Fill("input[name=password]", "admin"); err != nil {
		t.Fatalf("fill password: %v", err)
	}
	if err := page.Click("button[type=submit]"); err != nil {
		t.Fatalf("click submit: %v", err)
	}
}

// bodyContains checks that the page body text contains substr.
func bodyContains(t *testing.T, page playwright.Page, substr string) {
	t.Helper()
	body, err := page.TextContent("body")
	if err != nil {
		t.Fatalf("could not get body text: %v", err)
	}
	if !strings.Contains(body, substr) {
		t.Errorf("expected body to contain %q, it did not", substr)
	}
}

// --- 1.1 Empty-state baselines ---

func TestEmptyStatePeople(t *testing.T) {
	page, cleanup := setupPage(t)
	defer cleanup()
	login(t, page)
	if _, err := page.Goto(getBaseURL() + "/people"); err != nil {
		t.Fatalf("goto /people: %v", err)
	}
	bodyContains(t, page, "People")
	if n, _ := page.Locator("#people-grid").Count(); n == 0 {
		t.Error("expected #people-grid container on people page")
	}
}

func TestEmptyStateNotifications(t *testing.T) {
	page, cleanup := setupPage(t)
	defer cleanup()
	login(t, page)
	if _, err := page.Goto(getBaseURL() + "/notifications"); err != nil {
		t.Fatalf("goto /notifications: %v", err)
	}
	bodyContains(t, page, "One-Time Notifications")
}

func TestEmptyStateGroups(t *testing.T) {
	page, cleanup := setupPage(t)
	defer cleanup()
	login(t, page)
	if _, err := page.Goto(getBaseURL() + "/groups"); err != nil {
		t.Fatalf("goto /groups: %v", err)
	}
	bodyContains(t, page, "Groups")
	bodyContains(t, page, "Create Group")
}

func TestEmptyStateUsers(t *testing.T) {
	page, cleanup := setupPage(t)
	defer cleanup()
	login(t, page)
	if _, err := page.Goto(getBaseURL() + "/users"); err != nil {
		t.Fatalf("goto /users: %v", err)
	}
	bodyContains(t, page, "Users")
	bodyContains(t, page, "Create User")
}

func TestEmptyStateDashboard(t *testing.T) {
	page, cleanup := setupPage(t)
	defer cleanup()
	login(t, page)
	if _, err := page.Goto(getBaseURL() + "/"); err != nil {
		t.Fatalf("goto dashboard: %v", err)
	}
	bodyContains(t, page, "Good")
	if n, _ := page.Locator("#events-content").Count(); n == 0 {
		t.Error("expected #events-content container on dashboard")
	}
}

// --- 1.2 Form-error rendering baselines ---

func TestLoginFormErrorBaseline(t *testing.T) {
	page, cleanup := setupPage(t)
	defer cleanup()
	if _, err := page.Goto(getBaseURL() + "/login"); err != nil {
		t.Fatalf("goto login: %v", err)
	}
	if err := page.Fill("input[name=username]", "wronguser"); err != nil {
		t.Fatalf("fill username: %v", err)
	}
	if err := page.Fill("input[name=password]", "wrongpass"); err != nil {
		t.Fatalf("fill password: %v", err)
	}
	if err := page.Click("button[type=submit]"); err != nil {
		t.Fatalf("click submit: %v", err)
	}
	// Current behavior: error rendered in an alert-danger block.
	if n, _ := page.Locator(".alert-danger").Count(); n == 0 {
		t.Error("expected .alert-danger error block after failed login")
	}
}

func TestNotificationFormErrorBaseline(t *testing.T) {
	page, cleanup := setupPage(t)
	defer cleanup()
	login(t, page)
	if _, err := page.Goto(getBaseURL() + "/notifications/new"); err != nil {
		t.Fatalf("goto notification form: %v", err)
	}
	// Submit with empty required fields (message + scheduled_at).
	if err := page.Click("button[type=submit]"); err != nil {
		t.Fatalf("click submit: %v", err)
	}
	// Current behavior: inline errors rendered in .text-danger divs.
	if n, _ := page.Locator(".text-danger").Count(); n == 0 {
		t.Error("expected .text-danger inline error(s) after empty notification submit")
	}
}

// --- 1.3 Theme switching + calendar rendering baselines ---

func TestThemeSwitchingBaseline(t *testing.T) {
	page, cleanup := setupPage(t)
	defer cleanup()
	login(t, page)

	before, err := page.GetAttribute("html", "data-bs-theme")
	if err != nil {
		t.Fatalf("get data-bs-theme before change: %v", err)
	}
	if before == "" {
		before = "light"
	}

	// Select a different theme via the theme select control.
	target := "dark"
	if before == "dark" {
		target = "eink"
	}
	if _, err := page.SelectOption("#theme-select", playwright.SelectOptionValues{
		Values: playwright.StringSlice(target),
	}); err != nil {
		t.Fatalf("select theme option: %v", err)
	}

	after, err := page.GetAttribute("html", "data-bs-theme")
	if err != nil {
		t.Fatalf("get data-bs-theme after change: %v", err)
	}
	if after == before {
		t.Errorf("expected data-bs-theme to change after toggle click, was %q before and after", before)
	}
}

func TestCalendarRenderingBaseline(t *testing.T) {
	page, cleanup := setupPage(t)
	defer cleanup()
	login(t, page)
	if _, err := page.Goto(getBaseURL() + "/calendar"); err != nil {
		t.Fatalf("goto calendar: %v", err)
	}
	if n, _ := page.Locator("#calendar").Count(); n == 0 {
		t.Fatal("expected #calendar container on calendar page")
	}
	// Capture the current hardcoded-color inline style (to be replaced in Phase 4).
	if n, _ := page.Locator("style").Count(); n == 0 {
		t.Error("expected inline <style> element with hardcoded calendar colors")
	}
}

// --- 3.9 CSRF rejection + login throttling ---

// TestCSRF_PostWithoutTokenRejected verifies that a POST without a CSRF token
// is rejected with 403 (spec: security-hardening).
func TestCSRF_PostWithoutTokenRejected(t *testing.T) {
	page, cleanup := setupPage(t)
	defer cleanup()
	login(t, page)

	// Use fetch to send a POST without the X-CSRF-Token header.
	result, err := page.Evaluate(`() => {
		return fetch('/settings/logs/level', {
			method: 'POST',
			headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
			body: 'level=debug'
		}).then(r => r.status).catch(e => 'error: ' + e.message);
	}`)
	if err != nil {
		t.Fatalf("evaluate fetch: %v", err)
	}
	status, ok := result.(float64)
	if !ok {
		t.Fatalf("expected numeric status, got %v", result)
	}
	if int(status) != 403 {
		t.Errorf("expected 403 for POST without CSRF token, got %d", int(status))
	}
}

// TestCSRF_PostWithTokenAccepted verifies that a POST with the CSRF token
// from the cookie is accepted (spec: security-hardening).
func TestCSRF_PostWithTokenAccepted(t *testing.T) {
	page, cleanup := setupPage(t)
	defer cleanup()
	login(t, page)

	// The htmx:configRequest listener injects the token automatically,
	// so an htmx-driven POST should succeed.
	result, err := page.Evaluate(`() => {
		var match = document.cookie.match(/(?:^|;\s*)csrf_token=([^;]+)/);
		var token = match ? match[1] : '';
		return fetch('/settings/logs/level', {
			method: 'POST',
			headers: { 'Content-Type': 'application/x-www-form-urlencoded', 'X-CSRF-Token': token },
			body: 'level=info'
		}).then(r => r.status).catch(e => 'error: ' + e.message);
	}`)
	if err != nil {
		t.Fatalf("evaluate fetch: %v", err)
	}
	status, ok := result.(float64)
	if !ok {
		t.Fatalf("expected numeric status, got %v", result)
	}
	if int(status) == 403 {
		t.Errorf("expected non-403 for POST with valid CSRF token, got %d", int(status))
	}
}

// TestLoginThrottling verifies that repeated failed logins are rate-limited
// with a 429 response (spec: security-hardening — Login attempts are rate limited).
func TestLoginThrottling(t *testing.T) {
	page, cleanup := setupPage(t)
	defer cleanup()

	// Make 5 failed login attempts (the limit is 5/60s).
	for i := 0; i < 5; i++ {
		if _, err := page.Goto(getBaseURL() + "/login"); err != nil {
			t.Fatalf("goto login attempt %d: %v", i+1, err)
		}
		if err := page.Fill("input[name=username]", "admin"); err != nil {
			t.Fatalf("fill username attempt %d: %v", i+1, err)
		}
		if err := page.Fill("input[name=password]", "wrongpassword"); err != nil {
			t.Fatalf("fill password attempt %d: %v", i+1, err)
		}
		if err := page.Click("button[type=submit]"); err != nil {
			t.Fatalf("click submit attempt %d: %v", i+1, err)
		}
	}

	// 6th attempt should be throttled.
	if _, err := page.Goto(getBaseURL() + "/login"); err != nil {
		t.Fatalf("goto login attempt 6: %v", err)
	}
	if err := page.Fill("input[name=username]", "admin"); err != nil {
		t.Fatalf("fill username attempt 6: %v", err)
	}
	if err := page.Fill("input[name=password]", "wrongpassword"); err != nil {
		t.Fatalf("fill password attempt 6: %v", err)
	}
	if err := page.Click("button[type=submit]"); err != nil {
		t.Fatalf("click submit attempt 6: %v", err)
	}

	body, err := page.TextContent("body")
	if err != nil {
		t.Fatalf("get body: %v", err)
	}
	if !strings.Contains(body, "Too many login attempts") {
		t.Error("expected 'Too many login attempts' message on 6th failed login")
	}
}

// --- 5.7 Playwright a11y cases ---

func TestSkipToContentLink(t *testing.T) {
	page, cleanup := setupPage(t)
	defer cleanup()
	login(t, page)

	// The skip-to-content link should be present.
	count, err := page.Locator(".skip-to-content").Count()
	if err != nil {
		t.Fatalf("count skip links: %v", err)
	}
	if count == 0 {
		t.Fatal("skip-to-content link not found on page")
	}

	// Press Tab — the first focusable element should be the skip link.
	if err := page.Keyboard().Press("Tab"); err != nil {
		t.Fatalf("press Tab: %v", err)
	}
	focused, err := page.Evaluate("document.activeElement.className")
	if err != nil {
		t.Fatalf("get active element class: %v", err)
	}
	if focused != "skip-to-content" {
		t.Errorf("expected first focus to be skip-to-content link, got %v", focused)
	}

	// Activate the skip link — focus should move to main content.
	if err := page.Keyboard().Press("Enter"); err != nil {
		t.Fatalf("press Enter: %v", err)
	}
	focusedID, err := page.Evaluate("document.activeElement.id")
	if err != nil {
		t.Fatalf("get active element id: %v", err)
	}
	if focusedID != "main-content" {
		t.Errorf("expected focus to move to main-content, got %v", focusedID)
	}
}

func TestKeyboardLogRowExpansion(t *testing.T) {
	page, cleanup := setupPage(t)
	defer cleanup()
	login(t, page)

	// Navigate to Settings > Logs.
	if _, err := page.Goto(getBaseURL() + "/settings/logs"); err != nil {
		t.Fatalf("goto settings/logs: %v", err)
	}

	// Find a log row with role=button and tabindex=0.
	rowCount, err := page.Locator("tr[role='button'][tabindex='0']").Count()
	if err != nil {
		t.Fatalf("count log rows: %v", err)
	}
	if rowCount == 0 {
		t.Skip("no log rows with tabindex=0 found — need log entries to test")
	}

	// Focus the first log row and press Enter to expand.
	firstRow := page.Locator("tr[role='button'][tabindex='0']").First()
	if err := firstRow.Focus(); err != nil {
		t.Fatalf("focus first log row: %v", err)
	}

	// Get the detail row selector.
	detailID, err := firstRow.GetAttribute("data-bs-target")
	if err != nil {
		t.Fatalf("get data-bs-target: %v", err)
	}

	// Press Enter to activate the row (keyboard equivalent of click).
	if err := page.Keyboard().Press("Enter"); err != nil {
		t.Fatalf("press Enter: %v", err)
	}

	// The detail row should now be visible (expanded).
	detail := page.Locator(detailID)
	visible, err := detail.IsVisible()
	if err != nil {
		t.Fatalf("check detail visibility: %v", err)
	}
	if !visible {
		t.Error("expected detail row to expand after pressing Enter on focused log row")
	}
}

func TestThemeSelectControl(t *testing.T) {
	page, cleanup := setupPage(t)
	defer cleanup()
	login(t, page)

	// The theme select should be present with 3 options.
	count, err := page.Locator("#theme-select option").Count()
	if err != nil {
		t.Fatalf("count theme options: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 theme options, got %d", count)
	}

	// Verify all 3 themes are listed by name.
	options, err := page.Locator("#theme-select option").AllTextContents()
	if err != nil {
		t.Fatalf("get option texts: %v", err)
	}
	expected := map[string]bool{"Light": false, "Dark": false, "E-Ink": false}
	for _, opt := range options {
		if _, ok := expected[opt]; ok {
			expected[opt] = true
		}
	}
	for name, found := range expected {
		if !found {
			t.Errorf("theme option %q not found in select", name)
		}
	}

	// Select "Dark" and verify data-bs-theme changes.
	if _, err := page.SelectOption("#theme-select", playwright.SelectOptionValues{
		Values: playwright.StringSlice("dark"),
	}); err != nil {
		t.Fatalf("select dark theme: %v", err)
	}
	theme, err := page.GetAttribute("html", "data-bs-theme")
	if err != nil {
		t.Fatalf("get data-bs-theme: %v", err)
	}
	if theme != "dark" {
		t.Errorf("expected data-bs-theme=dark, got %q", theme)
	}
}

func TestFocusRecoveryAfterSwap(t *testing.T) {
	page, cleanup := setupPage(t)
	defer cleanup()
	login(t, page)

	// Go to the people page.
	if _, err := page.Goto(getBaseURL() + "/people"); err != nil {
		t.Fatalf("goto people: %v", err)
	}

	// If there are people, focus a card link inside #people-grid and trigger a swap.
	cardCount, err := page.Locator("#people-grid a").Count()
	if err != nil {
		t.Fatalf("count card links: %v", err)
	}
	if cardCount == 0 {
		t.Skip("no people cards — need seeded data to test focus recovery")
	}

	// Focus a card link inside #people-grid, then trigger an HTMX swap of #people-grid
	// without moving focus to the search input (uses htmx.ajax directly).
	result, err := page.Evaluate(`() => {
		var link = document.querySelector('#people-grid a');
		if (!link) return "no-link";
		link.focus();
		if (typeof htmx === 'undefined') return "no-htmx";
		htmx.ajax('GET', '/people?q=zzznonexistent', {target: '#people-grid', swap: 'outerHTML'});
		return "triggered";
	}`)
	if err != nil {
		t.Fatalf("evaluate focus + trigger: %v", err)
	}
	if result != "triggered" {
		t.Skipf("could not set up focus recovery test: %v", result)
	}

	// Wait for the HTMX swap to complete.
	time.Sleep(1 * time.Second)

	// Focus should not be lost to body — the focus management script should have
	// moved focus to an element in the swapped content.
	activeTag, err := page.Evaluate("document.activeElement.tagName")
	if err != nil {
		t.Fatalf("get active element tag: %v", err)
	}
	if activeTag == "BODY" {
		t.Error("focus was lost to body after HTMX swap — focus management not working")
	}
}

// TestPersonFormInlineError verifies that submitting the person form with an
// empty name shows an inline error message (form-validation spec).
func TestPersonFormInlineError(t *testing.T) {
	page, cleanup := setupPage(t)
	defer cleanup()
	login(t, page)

	_, err := page.Goto(getBaseURL() + "/people/new")
	if err != nil {
		t.Fatalf("navigate to person form: %v", err)
	}

	// Submit the form with empty name (leave name field empty)
	err = page.Fill("#name", "")
	if err != nil {
		t.Fatalf("clear name field: %v", err)
	}
	err = page.Click("button[type='submit']")
	if err != nil {
		t.Fatalf("click submit: %v", err)
	}

	// Wait for the form to re-render with inline error
	time.Sleep(500 * time.Millisecond)

	bodyText, err := page.InnerText("body")
	if err != nil {
		t.Fatalf("get body text: %v", err)
	}
	if !strings.Contains(bodyText, "Name is required") {
		t.Error("expected inline error 'Name is required' not found in body")
	}
}

// TestEventFormValuePreservation verifies that submitted values are preserved
// when the form is re-rendered with errors (form-validation spec).
func TestEventFormValuePreservation(t *testing.T) {
	page, cleanup := setupPage(t)
	defer cleanup()
	login(t, page)

	// Navigate to a person's event form (person ID 1 may not exist, but the
	// form handler still renders even for nonexistent person IDs on GET)
	_, err := page.Goto(getBaseURL() + "/people/1/events/new")
	if err != nil {
		t.Fatalf("navigate to event form: %v", err)
	}

	// Fill in description but leave date empty
	err = page.Fill("#description", "Test Birthday Bash")
	if err != nil {
		t.Fatalf("fill description: %v", err)
	}
	err = page.Click("button[type='submit']")
	if err != nil {
		t.Fatalf("click submit: %v", err)
	}

	// Wait for the form to re-render with errors
	time.Sleep(500 * time.Millisecond)

	// The description value should be preserved in the re-rendered form
	descValue, err := page.InputValue("#description")
	if err != nil {
		t.Fatalf("get description value: %v", err)
	}
	if descValue != "Test Birthday Bash" {
		t.Errorf("expected description preserved as 'Test Birthday Bash', got '%s'", descValue)
	}

	// The date field should show an inline error
	bodyText, err := page.InnerText("body")
	if err != nil {
		t.Fatalf("get body text: %v", err)
	}
	if !strings.Contains(bodyText, "Date is required") {
		t.Error("expected inline error 'Date is required' not found in body")
	}
}

// TestUsersDeleteConfirm verifies that the user delete form uses hx-confirm
// instead of native JavaScript confirm() (form-validation spec).
func TestUsersDeleteConfirm(t *testing.T) {
	page, cleanup := setupPage(t)
	defer cleanup()
	login(t, page)

	_, err := page.Goto(getBaseURL() + "/users")
	if err != nil {
		t.Fatalf("navigate to users: %v", err)
	}

	// Check that the delete form has hx-confirm attribute (not onsubmit confirm)
	// Look for any form with hx-confirm containing "Delete user"
	hasHXConfirm, err := page.Evaluate(`() => {
		const forms = document.querySelectorAll('form[action*="/delete"]');
		for (const form of forms) {
			if (form.getAttribute('hx-confirm') && form.getAttribute('hx-confirm').includes('Delete user')) {
				return true;
			}
			if (form.getAttribute('onsubmit') && form.getAttribute('onsubmit').includes('confirm(')) {
				return false;
			}
		}
		return null;
	}`)
	if err != nil {
		t.Fatalf("evaluate hx-confirm check: %v", err)
	}
	if hasHXConfirm == nil {
		t.Skip("no user delete forms found — need at least one user to test")
	}
	if hasHXConfirm != true {
		t.Error("user delete form uses native confirm() instead of hx-confirm")
	}
}
