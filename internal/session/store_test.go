package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/datey/datey/ent/enttest"
	"github.com/datey/datey/ent/user"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

func newTestSessionStore(t *testing.T) *Store {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:test_session_store?mode=memory&cache=shared&_fk=1")
	t.Cleanup(func() { client.Close() })
	return NewStore(client)
}

func seedUser(t *testing.T, store *Store) int {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	u, err := store.client.User.Create().
		SetUsername("testuser").
		SetPasswordHash(string(hash)).
		SetRole(user.RoleAdmin).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u.ID
}

func TestGenerateToken(t *testing.T) {
	raw, hash, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if raw == "" {
		t.Error("expected non-empty raw token")
	}
	if hash == "" {
		t.Error("expected non-empty hash")
	}
	if raw == hash {
		t.Error("raw and hash should differ")
	}
}

func TestCreateAndGetByToken(t *testing.T) {
	store := newTestSessionStore(t)
	userID := seedUser(t, store)

	token, err := store.Create(context.Background(), userID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	sess, err := store.GetByToken(context.Background(), token)
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if sess == nil {
		t.Fatal("expected session")
	}
}

func TestGetByToken_Invalid(t *testing.T) {
	store := newTestSessionStore(t)

	_, err := store.GetByToken(context.Background(), "invalid-token")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestDelete(t *testing.T) {
	store := newTestSessionStore(t)
	userID := seedUser(t, store)

	token, err := store.Create(context.Background(), userID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.Delete(context.Background(), token); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = store.GetByToken(context.Background(), token)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestDeleteByUserID(t *testing.T) {
	store := newTestSessionStore(t)
	userID := seedUser(t, store)

	_, err := store.Create(context.Background(), userID)
	if err != nil {
		t.Fatalf("Create session 1: %v", err)
	}
	_, err = store.Create(context.Background(), userID)
	if err != nil {
		t.Fatalf("Create session 2: %v", err)
	}

	if err := store.DeleteByUserID(context.Background(), userID); err != nil {
		t.Fatalf("DeleteByUserID: %v", err)
	}

	// Verify cleanup - list sessions should return 0
	count, err := store.client.Session.Query().Count(context.Background())
	if err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 sessions, got %d", count)
	}
}

func TestCleanupExpired(t *testing.T) {
	store := newTestSessionStore(t)
	userID := seedUser(t, store)

	// Create expired session (write directly to db to set past expiry)
	_, hash, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	_, err = store.client.Session.Create().
		SetTokenHash(hash).
		SetUserID(userID).
		SetExpiresAt(time.Now().Add(-1 * time.Hour)).
		SetCreatedAt(time.Now()).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create expired session: %v", err)
	}

	// Create valid session
	_, err = store.Create(context.Background(), userID)
	if err != nil {
		t.Fatalf("create valid session: %v", err)
	}

	deleted, err := store.CleanupExpired(context.Background())
	if err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	// Verify the valid session still exists
	count, err := store.client.Session.Query().Count(context.Background())
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 remaining session, got %d", count)
	}
}

func TestCookie_SetCookie(t *testing.T) {
	w := httptest.NewRecorder()
	SetCookie(w, "test-token", true)

	resp := w.Result()
	cookies := resp.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	c := cookies[0]
	if c.Name != "session" {
		t.Errorf("expected name 'session', got %q", c.Name)
	}
	if c.Value != "test-token" {
		t.Errorf("expected value 'test-token', got %q", c.Value)
	}
	if !c.HttpOnly {
		t.Error("expected HttpOnly")
	}
	if !c.Secure {
		t.Error("expected Secure")
	}
	if c.MaxAge != 60*60*24*30 {
		t.Errorf("expected MaxAge %d, got %d", 60*60*24*30, c.MaxAge)
	}
}

func TestCookie_ClearCookie(t *testing.T) {
	w := httptest.NewRecorder()
	ClearCookie(w)

	resp := w.Result()
	cookies := resp.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	c := cookies[0]
	if c.Name != "session" {
		t.Errorf("expected name 'session', got %q", c.Name)
	}
	if c.MaxAge != -1 {
		t.Errorf("expected MaxAge -1, got %d", c.MaxAge)
	}
}

func TestCookie_ReadCookie(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "my-token"})

	token, err := ReadCookie(req)
	if err != nil {
		t.Fatalf("ReadCookie: %v", err)
	}
	if token != "my-token" {
		t.Errorf("expected 'my-token', got %q", token)
	}
}

func TestCookie_ReadCookie_Missing(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)

	_, err := ReadCookie(req)
	if err == nil {
		t.Fatal("expected error for missing cookie")
	}
}
