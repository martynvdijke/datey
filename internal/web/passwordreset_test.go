package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/datey/datey/ent/user"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

func setupPasswordResetRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/forgot-password", h.forgotPasswordPage)
	r.Post("/forgot-password", h.forgotPasswordPost)
	r.Get("/reset-password", h.resetPasswordPage)
	r.Post("/reset-password", h.resetPasswordPost)
	return r
}

// seedUserForReset creates a user with the given password directly in the DB.
func seedUserForReset(t *testing.T, h *Handler, username, password string) int {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	u, err := h.client.User.Create().
		SetUsername(username).
		SetPasswordHash(string(hash)).
		SetRole(user.RoleUser).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(context.Background())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u.ID
}

// newEmailConfiguredTestWebHandler returns a handler with a mock notifier
// registered as the "email" channel.
func newEmailConfiguredTestWebHandler(t *testing.T, mock *mockNotifier) *Handler {
	t.Helper()
	return newTestNotificationsHandlerWithMock(t, mock)
}

func TestForgotPasswordPage(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPasswordResetRouter(h)

	req := httptest.NewRequest("GET", "/forgot-password", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Reset your password") {
		t.Errorf("expected reset form, got: %s", w.Body.String()[:300])
	}
}

func TestForgotPasswordPage_NoEmailConfigured(t *testing.T) {
	h := newTestWebHandler(t) // empty registry — email not configured
	router := setupPasswordResetRouter(h)

	req := httptest.NewRequest("GET", "/forgot-password", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "no email channel is configured") {
		t.Errorf("expected unavailable message, got: %s", w.Body.String()[:300])
	}
}

func TestForgotPasswordPost_EmptyUsername(t *testing.T) {
	mock := &mockNotifier{name: "email", configured: true}
	h := newEmailConfiguredTestWebHandler(t, mock)
	router := setupPasswordResetRouter(h)

	body := "username="
	req := httptest.NewRequest("POST", "/forgot-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with error, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Username is required") {
		t.Errorf("expected validation error, got: %s", w.Body.String()[:300])
	}
	if len(mock.sent) != 0 {
		t.Errorf("expected no email sent, got %d", len(mock.sent))
	}
}

func TestForgotPasswordPost_UnknownUsername_GenericResponse(t *testing.T) {
	mock := &mockNotifier{name: "email", configured: true}
	h := newEmailConfiguredTestWebHandler(t, mock)
	router := setupPasswordResetRouter(h)

	body := "username=nobody"
	req := httptest.NewRequest("POST", "/forgot-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "If that username exists") {
		t.Errorf("expected generic anti-enumeration message, got: %s", w.Body.String()[:300])
	}
	// Must not reveal that the account does not exist, and no email is sent.
	if len(mock.sent) != 0 {
		t.Errorf("expected no email sent for unknown username, got %d", len(mock.sent))
	}
}

func TestForgotPasswordPost_ExistingUser_SendsEmail(t *testing.T) {
	mock := &mockNotifier{name: "email", configured: true}
	h := newEmailConfiguredTestWebHandler(t, mock)
	router := setupPasswordResetRouter(h)

	seedUserForReset(t, h, "alice", "oldpassword123")

	body := "username=alice"
	req := httptest.NewRequest("POST", "/forgot-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if len(mock.sent) != 1 {
		t.Fatalf("expected 1 email sent, got %d", len(mock.sent))
	}
	if !strings.Contains(mock.sent[0].message, "/reset-password?token=") {
		t.Errorf("expected reset link in email, got: %s", mock.sent[0].message)
	}
	if !strings.Contains(w.Body.String(), "If that username exists") {
		t.Errorf("expected success message, got: %s", w.Body.String()[:300])
	}
}

func TestForgotPasswordPost_NoEmailConfigured(t *testing.T) {
	h := newTestWebHandler(t) // empty registry
	router := setupPasswordResetRouter(h)

	seedUserForReset(t, h, "alice", "oldpassword123")

	body := "username=alice"
	req := httptest.NewRequest("POST", "/forgot-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with error, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "no email channel is configured") {
		t.Errorf("expected unavailable message, got: %s", w.Body.String()[:300])
	}
}

func TestForgotPasswordPost_RateLimited(t *testing.T) {
	mock := &mockNotifier{name: "email", configured: true}
	h := newEmailConfiguredTestWebHandler(t, mock)
	router := setupPasswordResetRouter(h)

	// Limit is 5 per window per IP; the 6th request must be throttled.
	for i := 0; i < 5; i++ {
		body := "username=alice"
		req := httptest.NewRequest("POST", "/forgot-password", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	req := httptest.NewRequest("POST", "/forgot-password", strings.NewReader("username=alice"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Too many reset requests") {
		t.Errorf("expected rate limit message, got: %s", w.Body.String()[:300])
	}
}

func TestResetPasswordPage_MissingToken(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPasswordResetRouter(h)

	req := httptest.NewRequest("GET", "/reset-password", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with error, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Missing reset token") {
		t.Errorf("expected missing token message, got: %s", w.Body.String()[:300])
	}
}

func TestResetPasswordPage_WithToken(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPasswordResetRouter(h)

	req := httptest.NewRequest("GET", "/reset-password?token=abc123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Choose a new password") {
		t.Errorf("expected reset form, got: %s", w.Body.String()[:300])
	}
}

func TestResetPasswordPost_InvalidToken(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPasswordResetRouter(h)

	body := "token=notarealtoken&password=newpassword123&confirm=newpassword123"
	req := httptest.NewRequest("POST", "/reset-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with error, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid, has already been used, or has expired") {
		t.Errorf("expected generic token error, got: %s", w.Body.String()[:300])
	}
}

func TestResetPasswordPost_ShortPassword(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPasswordResetRouter(h)

	userID := seedUserForReset(t, h, "alice", "oldpassword123")
	raw, _, err := h.passwordResetTokens.Create(context.Background(), userID, time.Hour)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	body := "token=" + raw + "&password=short&confirm=short"
	req := httptest.NewRequest("POST", "/reset-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with error, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "at least 8 characters") {
		t.Errorf("expected password length error, got: %s", w.Body.String()[:300])
	}

	// Password must be unchanged.
	u, err := h.users.GetByID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("oldpassword123")); err != nil {
		t.Errorf("password should not have changed")
	}
}

func TestResetPasswordPost_PasswordMismatch(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPasswordResetRouter(h)

	userID := seedUserForReset(t, h, "alice", "oldpassword123")
	raw, _, err := h.passwordResetTokens.Create(context.Background(), userID, time.Hour)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	body := "token=" + raw + "&password=newpassword123&confirm=different123"
	req := httptest.NewRequest("POST", "/reset-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with error, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Passwords do not match") {
		t.Errorf("expected mismatch error, got: %s", w.Body.String()[:300])
	}
}

func TestResetPasswordPost_Success(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPasswordResetRouter(h)

	userID := seedUserForReset(t, h, "alice", "oldpassword123")

	// Create an active session that must be invalidated by the reset.
	sessionToken, err := h.sessions.Create(context.Background(), userID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	raw, _, err := h.passwordResetTokens.Create(context.Background(), userID, time.Hour)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	body := "token=" + raw + "&password=newpassword123&confirm=newpassword123"
	req := httptest.NewRequest("POST", "/reset-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "/login") {
		t.Errorf("expected redirect to /login, got %s", loc)
	}

	// Password must now match the new value.
	u, err := h.users.GetByID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("newpassword123")); err != nil {
		t.Errorf("password was not updated")
	}

	// All sessions must have been invalidated.
	if _, err := h.sessions.GetByToken(context.Background(), sessionToken); err == nil {
		t.Errorf("expected session to be invalidated")
	}
}

func TestResetPasswordPost_UsedTokenRejected(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPasswordResetRouter(h)

	userID := seedUserForReset(t, h, "alice", "oldpassword123")
	raw, _, err := h.passwordResetTokens.Create(context.Background(), userID, time.Hour)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	// First use succeeds.
	body := "token=" + raw + "&password=newpassword123&confirm=newpassword123"
	req := httptest.NewRequest("POST", "/reset-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 on first use, got %d", w.Code)
	}

	// Second use must be rejected.
	req2 := httptest.NewRequest("POST", "/reset-password", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200 with error on second use, got %d", w2.Code)
	}
	if !strings.Contains(w2.Body.String(), "invalid, has already been used, or has expired") {
		t.Errorf("expected token rejection, got: %s", w2.Body.String()[:300])
	}
}

func TestResetPasswordPost_ExpiredTokenRejected(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPasswordResetRouter(h)

	userID := seedUserForReset(t, h, "alice", "oldpassword123")
	// TTL in the past → token is already expired.
	raw, _, err := h.passwordResetTokens.Create(context.Background(), userID, -time.Minute)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	body := "token=" + raw + "&password=newpassword123&confirm=newpassword123"
	req := httptest.NewRequest("POST", "/reset-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with error, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid, has already been used, or has expired") {
		t.Errorf("expected token rejection, got: %s", w.Body.String()[:300])
	}
}

func TestResetPasswordPost_NewerTokenInvalidatesOlder(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupPasswordResetRouter(h)

	userID := seedUserForReset(t, h, "alice", "oldpassword123")

	firstRaw, _, err := h.passwordResetTokens.Create(context.Background(), userID, time.Hour)
	if err != nil {
		t.Fatalf("create first token: %v", err)
	}
	// A second request for the same user deletes the first token.
	secondRaw, _, err := h.passwordResetTokens.Create(context.Background(), userID, time.Hour)
	if err != nil {
		t.Fatalf("create second token: %v", err)
	}
	if firstRaw == secondRaw {
		t.Fatalf("expected distinct tokens")
	}

	// The older token must no longer work.
	body := "token=" + firstRaw + "&password=newpassword123&confirm=newpassword123"
	req := httptest.NewRequest("POST", "/reset-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with error for older token, got %d", w.Code)
	}

	// The newer token still works.
	body2 := "token=" + secondRaw + "&password=newpassword123&confirm=newpassword123"
	req2 := httptest.NewRequest("POST", "/reset-password", strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusSeeOther {
		t.Errorf("expected 303 for newer token, got %d", w2.Code)
	}
}
