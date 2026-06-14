package datey_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/session"
	"github.com/datey/datey/internal/web"
	"github.com/go-chi/chi/v5"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

func TestUpcomingEvents(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:datey_test?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	contact, err := client.Contact.Create().
		SetName("Alice").
		SetNotes("").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(context.Background())
	if err != nil {
		t.Fatalf("failed to create contact: %v", err)
	}

	_, err = client.Event.Create().
		SetType("birthday").
		SetDate(time.Date(2026, time.July, 4, 0, 0, 0, 0, time.UTC)).
		SetDescription("test").
		SetCreatedAt(time.Now()).
		SetContact(contact).
		Save(context.Background())
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	events, err := client.Event.Query().WithContact().All(context.Background())
	if err != nil {
		t.Fatalf("failed to query events: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	e := events[0]
	contact2, _ := e.Edges.ContactOrErr()
	if contact2.Name != "Alice" {
		t.Errorf("expected contact name Alice, got %s", contact2.Name)
	}
}

func createTestUser(t *testing.T, client *ent.Client) *ent.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("testpass"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	u, err := client.User.Create().
		SetUsername("testuser").
		SetPasswordHash(string(hash)).
		SetRole(user.RoleAdmin).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(context.Background())
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	return u
}

func createTestSessionToken(t *testing.T, client *ent.Client, userID int) string {
	t.Helper()
	store := session.NewStore(client)
	token, err := store.Create(context.Background(), userID)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	return token
}

func TestDashboardHandlerWithEvents(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:datey_dashboard_test?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ctx := context.Background()

	// Create a user and session
	u := createTestUser(t, client)
	token := createTestSessionToken(t, client, u.ID)

	// Create a contact
	contact, err := client.Contact.Create().
		SetName("Test Contact").
		SetNotes("").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create contact: %v", err)
	}

	// Create an upcoming event (3 days from now)
	upcomingDate := time.Now().AddDate(0, 0, 3)
	_, err = client.Event.Create().
		SetType("birthday").
		SetDate(upcomingDate).
		SetDescription("Upcoming birthday").
		SetCreatedAt(time.Now()).
		SetContact(contact).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create upcoming event: %v", err)
	}

	// Create a past event (should not show up)
	pastDate := time.Now().AddDate(0, 0, -10)
	_, err = client.Event.Create().
		SetType("anniversary").
		SetDate(pastDate).
		SetDescription("Past anniversary").
		SetCreatedAt(time.Now()).
		SetContact(contact).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create past event: %v", err)
	}

	// Create handler
	cfg := &config.Config{
		Port:          6270,
		DataDir:       "/tmp",
		ReminderDays:  7,
		LogLevel:      "info",
		LogBufferSize: 1000,
	}
	reg := notifier.NewRegistry()
	store := logstore.NewStore(1000)
	store.InitLevel(0)

	handler := web.NewHandler(cfg, client, reg, store)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Test dashboard with session cookie
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	// Verify the upcoming event is shown
	if !contains(body, "Test Contact") {
		t.Error("dashboard should show contact name 'Test Contact'")
	}
	if !contains(body, "birthday") {
		t.Error("dashboard should show event type 'birthday'")
	}
	// Check for days remaining (could be 2 or 3 days depending on exact time)
	if !contains(body, "days") {
		t.Error("dashboard should show days remaining")
	}

	// Verify past event is NOT shown
	if contains(body, "Past anniversary") {
		t.Error("dashboard should NOT show past event 'Past anniversary'")
	}
}

func TestDashboardHandlerNoEvents(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:datey_no_events_test?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	// Create a user and session
	u := createTestUser(t, client)
	token := createTestSessionToken(t, client, u.ID)

	cfg := &config.Config{
		Port:          6270,
		DataDir:       "/tmp",
		ReminderDays:  7,
		LogLevel:      "info",
		LogBufferSize: 1000,
	}
	reg := notifier.NewRegistry()
	store := logstore.NewStore(1000)
	store.InitLevel(0)

	handler := web.NewHandler(cfg, client, reg, store)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	// Verify the "no events" message is shown
	if !contains(body, "No upcoming events") {
		t.Error("dashboard should show 'No upcoming events' message when no events exist")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
