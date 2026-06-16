package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/user"
	"github.com/go-chi/chi/v5"
)

func setupEinkRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Post("/settings/eink-toggle", h.settingsEinkToggle)
	return r
}

func seedEinkTestUser(t *testing.T, h *Handler, username string, role user.Role) *ent.User {
	t.Helper()
	u, err := h.users.Create(context.Background(), username, "hash", role)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u
}

func withEinkUserContext(ctx context.Context, u *ent.User) context.Context {
	return context.WithValue(ctx, userContextKey, u)
}

func TestEinkToggle_ToggleOn(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupEinkRouter(h)
	user := seedEinkTestUser(t, h, "einkadmin", user.RoleAdmin)

	req := httptest.NewRequest("POST", "/settings/eink-toggle", nil)
	req = req.WithContext(withEinkUserContext(req.Context(), user))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "E-Ink: On") {
		t.Errorf("expected toggle button showing On, got: %s", body)
	}

	if !strings.Contains(body, "btn-dark") {
		t.Errorf("expected active toggle button style (btn-dark), got: %s", body)
	}

	// Verify persisted in DB
	u, err := h.users.GetByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !u.EinkMode {
		t.Errorf("expected eink_mode to be true in DB after toggle")
	}
}

func TestEinkToggle_ToggleOff(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupEinkRouter(h)
	user := seedEinkTestUser(t, h, "einkadmin2", user.RoleAdmin)

	ctx := withEinkUserContext(context.Background(), user)

	// First toggle on
	req := httptest.NewRequest("POST", "/settings/eink-toggle", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("first toggle expected 200, got %d", w.Code)
	}

	// Then toggle off
	req2 := httptest.NewRequest("POST", "/settings/eink-toggle", nil)
	req2 = req2.WithContext(ctx)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("second toggle expected 200, got %d", w2.Code)
	}

	body := w2.Body.String()
	if !strings.Contains(body, "E-Ink: Off") {
		t.Errorf("expected toggle button showing Off, got: %s", body)
	}

	if !strings.Contains(body, "btn-outline-secondary") {
		t.Errorf("expected inactive toggle style (btn-outline-secondary), got: %s", body)
	}

	// Verify persisted in DB
	u, err := h.users.GetByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if u.EinkMode {
		t.Errorf("expected eink_mode to be false in DB after toggling off")
	}
}

func TestEinkToggle_Unauthenticated(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupEinkRouter(h)

	req := httptest.NewRequest("POST", "/settings/eink-toggle", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated request, got %d", w.Code)
	}
}

func TestEinkToggle_NonAdminUser(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupEinkRouter(h)
	user := seedEinkTestUser(t, h, "einkregular", user.RoleUser)

	req := httptest.NewRequest("POST", "/settings/eink-toggle", nil)
	req = req.WithContext(withEinkUserContext(req.Context(), user))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (auth handled at route level), got %d", w.Code)
	}

	// Verify it toggled the non-admin user's preference
	u, err := h.users.GetByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !u.EinkMode {
		t.Errorf("expected eink_mode to be true for non-admin user after toggle")
	}
}

func TestEinkConfigForce(t *testing.T) {
	h := newTestWebHandler(t)
	// Force e-ink mode via config
	h.cfg.EinkMode = true

	enabled := h.einkModeEnabled(httptest.NewRequest("GET", "/", nil))
	if !enabled {
		t.Errorf("expected einkModeEnabled to be true when config forces it")
	}

	// Even without user context, should still be true
	enabled = h.einkModeEnabled(httptest.NewRequest("GET", "/", nil))
	if !enabled {
		t.Errorf("expected einkModeEnabled to be true with no user context when config forces it")
	}
}

func TestEinkConfigNotForced_Defaults(t *testing.T) {
	h := newTestWebHandler(t)
	h.cfg.EinkMode = false

	req := httptest.NewRequest("GET", "/", nil)
	enabled := h.einkModeEnabled(req)
	if enabled {
		t.Errorf("expected einkModeEnabled to be false with no user and no force")
	}
}
