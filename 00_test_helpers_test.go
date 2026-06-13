package main

import (
	"bytes"
	"context"
	"database/sql"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/session"
	"github.com/datey/datey/internal/web"
)

var (
	testRouter  *chi.Mux
	adminToken  string
	testDataDir string
	testClient  *ent.Client
)

func TestMain(m *testing.M) {
	// Create temp directory for test data (used by backup tests)
	var err error
	testDataDir, err = os.MkdirTemp("", "datey-test-*")
	if err != nil {
		log.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(testDataDir)

	// Create a real SQLite database file so backup tests have a valid .db to work with
	testDBPath := filepath.Join(testDataDir, "datey.db")
	dbFile, err := sql.Open("sqlite3", testDBPath)
	if err != nil {
		log.Fatalf("failed to create test db file: %v", err)
	}
	dbFile.Close()

	// Create in-memory database for the application
	db, err := sql.Open("sqlite3", "file:datey_test?mode=memory&cache=shared&_fk=1")
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	drv := entsql.OpenDB(dialect.SQLite, db)
	testClient = ent.NewClient(ent.Driver(drv))
	if err := testClient.Schema.Create(context.Background()); err != nil {
		log.Fatalf("failed to create schema: %v", err)
	}

	// Seed admin user
	hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("failed to hash password: %v", err)
	}

	adminUser, err := testClient.User.Create().
		SetUsername("admin").
		SetPasswordHash(string(hash)).
		SetRole(user.RoleAdmin).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(context.Background())
	if err != nil {
		log.Fatalf("failed to create admin user: %v", err)
	}

	// Create session for admin user
	raw, hashHex, err := session.GenerateToken()
	if err != nil {
		log.Fatalf("failed to generate session token: %v", err)
	}

	_, err = testClient.Session.Create().
		SetTokenHash(hashHex).
		SetUserID(adminUser.ID).
		SetExpiresAt(time.Now().Add(30 * 24 * time.Hour)).
		SetCreatedAt(time.Now()).
		Save(context.Background())
	if err != nil {
		log.Fatalf("failed to create admin session: %v", err)
	}

	adminToken = raw

	// Create config
	backupDir := filepath.Join(testDataDir, "backups")
	cfg := &config.Config{
		Port:                6270,
		DataDir:             testDataDir,
		LogLevel:            "debug",
		LogBufferSize:       100,
		BackupDir:           backupDir,
		BackupRetentionDays: 30,
	}

	// Create log store
	logStore := logstore.NewStore(cfg.LogBufferSize)
	level := slog.LevelInfo
	if l, ok := logstore.ParseLogLevel(cfg.LogLevel); ok {
		level = l
	}
	logStore.InitLevel(level)

	// Create empty notifier registry
	notifReg := notifier.NewRegistry()

	// Create handler and register routes
	handler := web.NewHandler(cfg, testClient, notifReg, logStore)
	testRouter = chi.NewRouter()
	handler.RegisterRoutes(testRouter)

	os.Exit(m.Run())
}

// adminRequest creates an authenticated HTTP request with the admin session cookie.
func adminRequest(method, path string, body []byte) *http.Request {
	req, _ := http.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(&http.Cookie{Name: "session", Value: adminToken})
	return req
}

// seedTestContact creates a contact with an event and returns the contact ID.
func seedTestContact() int {
	contact, err := testClient.Contact.Create().
		SetName("Test Contact").
		SetNotes("Created by test").
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(context.Background())
	if err != nil {
		log.Printf("seedTestContact: failed to create contact: %v", err)
		return 0
	}

	_, err = testClient.Event.Create().
		SetType("birthday").
		SetDate(time.Date(2026, time.July, 4, 0, 0, 0, 0, time.UTC)).
		SetDescription("Test event").
		SetCreatedAt(time.Now()).
		SetContactID(contact.ID).
		Save(context.Background())
	if err != nil {
		log.Printf("seedTestContact: failed to create event: %v", err)
		return contact.ID
	}

	return contact.ID
}

// unauthenticatedRequest creates an HTTP request without any session cookie.
func unauthenticatedRequest(method, path string) *http.Request {
	req, _ := http.NewRequest(method, path, nil)
	return req
}

// seedTestEvent creates an event for the given contact and returns the event ID.
func seedTestEvent(contactID int, eventType string, date time.Time) int {
	event, err := testClient.Event.Create().
		SetType(eventType).
		SetDate(date).
		SetDescription("Test event").
		SetCreatedAt(time.Now()).
		SetContactID(contactID).
		Save(context.Background())
	if err != nil {
		log.Printf("seedTestEvent: failed to create event: %v", err)
		return 0
	}
	return event.ID
}

// seedTestNotification creates a one-time notification and returns its ID.
func seedTestNotification() int {
	n, err := testClient.OneTimeNotification.Create().
		SetMessage("Test notification").
		SetScheduledAt(time.Now().Add(24 * time.Hour)).
		SetChannelTargets(`["email"]`).
		SetStatus("pending").
		SetCreatedAt(time.Now()).
		Save(context.Background())
	if err != nil {
		log.Printf("seedTestNotification: failed to create notification: %v", err)
		return 0
	}
	return n.ID
}
