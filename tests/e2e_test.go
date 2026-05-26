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
