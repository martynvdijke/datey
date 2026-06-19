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

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected JSON response, got %s", w.Header().Get("Content-Type"))
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

	if w2.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected JSON response, got %s", w2.Header().Get("Content-Type"))
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
		t.Errorf("expected 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected JSON response, got %s", w.Header().Get("Content-Type"))
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

func TestEinkToggle_WithEnabledParam(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupEinkRouter(h)
	user := seedEinkTestUser(t, h, "enabledparam", user.RoleAdmin)
	ctx := withEinkUserContext(context.Background(), user)

	// Set enabled=true
	req := httptest.NewRequest("POST", "/settings/eink-toggle?enabled=true", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	u, err := h.users.GetByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !u.EinkMode {
		t.Errorf("expected eink_mode to be true after enabled=true")
	}

	// Set enabled=false
	req2 := httptest.NewRequest("POST", "/settings/eink-toggle?enabled=false", nil)
	req2 = req2.WithContext(ctx)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w2.Code)
	}

	u, err = h.users.GetByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if u.EinkMode {
		t.Errorf("expected eink_mode to be false after enabled=false")
	}
}

func TestEinkToggle_EnabledParamIsIdempotent(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupEinkRouter(h)
	user := seedEinkTestUser(t, h, "idempotent", user.RoleAdmin)
	ctx := withEinkUserContext(context.Background(), user)

	// Toggle on first
	h.users.SetEinkMode(context.Background(), user.ID, true)

	// Then set enabled=true again — should stay true
	req := httptest.NewRequest("POST", "/settings/eink-toggle?enabled=true", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	u, err := h.users.GetByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !u.EinkMode {
		t.Errorf("expected eink_mode to remain true after setting enabled=true again")
	}
}

// TestEinkCSS_HasNavbarTogglerOverride ensures the mobile navbar toggler stays
// visible in e-ink mode. The custom toggler uses the page text color by default;
// eink.css must force black bars/border on the white e-ink navbar.
func TestEinkCSS_HasNavbarTogglerOverride(t *testing.T) {
	cssBytes, err := staticFS.ReadFile("static/eink.css")
	if err != nil {
		t.Fatalf("read embedded eink.css: %v", err)
	}
	css := string(cssBytes)

	if !strings.Contains(css, ".navbar-toggler") {
		t.Error("eink.css missing .navbar-toggler override — mobile menu button invisible in e-ink mode")
	}
	if !strings.Contains(css, "#000") {
		t.Error("eink.css toggler override must use black (#000) for visible bars/border on the white e-ink navbar")
	}
}
