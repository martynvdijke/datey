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

func setupAuthRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/login", h.loginPage)
	r.Post("/login", h.loginPost)
	r.Get("/setup", h.setupPage)
	r.Post("/setup", h.setupCreate)
	r.Get("/logout", h.logout)
	return r
}

func TestLoginPage(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupAuthRouter(h)

	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Login") {
		t.Errorf("expected Login in body, got: %s", w.Body.String()[:200])
	}
}

func TestLoginPost_EmptyFields(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupAuthRouter(h)

	body := "username=&password="
	req := httptest.NewRequest("POST", "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "required") {
		t.Errorf("expected validation error, got: %s", w.Body.String()[:300])
	}
}

func TestLoginPost_InvalidCredentials(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupAuthRouter(h)

	body := "username=nonexistent&password=wrong"
	req := httptest.NewRequest("POST", "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with error, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Invalid username or password") {
		t.Errorf("expected error message, got: %s", w.Body.String()[:300])
	}
}

func TestSetupPage(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupAuthRouter(h)

	req := httptest.NewRequest("GET", "/setup", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Setup") {
		t.Errorf("expected Setup in body, got: %s", w.Body.String()[:200])
	}
}

func TestSetupCreate_EmptyUsername(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupAuthRouter(h)

	body := "username=&password=password123"
	req := httptest.NewRequest("POST", "/setup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with error, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Username is required") {
		t.Errorf("expected username error, got: %s", w.Body.String()[:300])
	}
}

func TestSetupCreate_ShortPassword(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupAuthRouter(h)

	body := "username=admin&password=1234567"
	req := httptest.NewRequest("POST", "/setup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with error, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "at least 8 characters") {
		t.Errorf("expected password length error, got: %s", w.Body.String()[:300])
	}
}

func TestSetupCreate_Success(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupAuthRouter(h)

	body := "username=admin&password=password123"
	req := httptest.NewRequest("POST", "/setup", strings.NewReader(body))
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
}

func TestSetupCreate_AlreadyExists(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupAuthRouter(h)

	// Seed a user first
	ctx := context.Background()
	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
	_, err := h.client.User.Create().
		SetUsername("existing").
		SetPasswordHash(string(hash)).
		SetRole(user.RoleAdmin).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	body := "username=admin&password=password123"
	req := httptest.NewRequest("POST", "/setup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should redirect to login since a user already exists
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", w.Code)
	}
}

func TestUserContextFunctions(t *testing.T) {
	// Test UserFromContext returns nil for empty context
	if u := UserFromContext(context.Background()); u != nil {
		t.Errorf("expected nil, got %v", u)
	}

	// Test IsAdmin returns false for empty context
	if IsAdmin(context.Background()) {
		t.Errorf("expected false for empty context")
	}

	// Test getUserID returns 0 for empty context
	req := httptest.NewRequest("GET", "/", nil)
	if id := getUserID(req); id != 0 {
		t.Errorf("expected 0, got %d", id)
	}
}

func TestAuthMiddleware_NoCookie(t *testing.T) {
	h := newTestWebHandler(t)
	middleware := h.Auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/login" {
		t.Errorf("expected Location: /login, got %s", loc)
	}
}
