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

	if !strings.Contains(body, "btn-light") {
		t.Errorf("expected active toggle button style (btn-light), got: %s", body)
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

	if !strings.Contains(body, "btn-outline-light") {
		t.Errorf("expected inactive toggle style (btn-outline-light, readable on dark navbar), got: %s", body)
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

// TestEinkToggle_ClassesMatchBaseTemplate ensures the HTMX-swapped button uses
// the same classes as base.html's initial render. If they diverge, contrast
// breaks after a toggle without a full page reload (the original bug).
func TestEinkToggle_ClassesMatchBaseTemplate(t *testing.T) {
	baseBytes, err := templateFS.ReadFile("templates/base.html")
	if err != nil {
		t.Fatalf("read embedded base.html: %v", err)
	}
	base := string(baseBytes)

	// base.html must render both states with readable classes on the dark navbar.
	for _, c := range []struct{ name, class string }{
		{"on (e-ink active)", "btn-light"},
		{"off (e-ink inactive)", "btn-outline-light"},
	} {
		if !strings.Contains(base, c.class) {
			t.Errorf("base.html missing initial class %q for %s state", c.class, c.name)
		}
	}

	// The toggle handler must return the same two classes so a swap doesn't
	// regress contrast. Drive both states and compare against base.html.
	h := newTestWebHandler(t)
	router := setupEinkRouter(h)
	u := seedEinkTestUser(t, h, "consistencyadmin", user.RoleAdmin)
	ctx := withEinkUserContext(context.Background(), u)

	seen := map[string]bool{}
	for i := 0; i < 2; i++ { // toggle on, then off
		req := httptest.NewRequest("POST", "/settings/eink-toggle", nil).WithContext(ctx)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("toggle %d: expected 200, got %d", i, w.Code)
		}
		body := w.Body.String()
		for _, c := range []string{"btn-light", "btn-outline-light"} {
			if strings.Contains(body, c) {
				seen[c] = true
				if !strings.Contains(base, c) {
					t.Errorf("handler returns %q but base.html does not — divergence causes post-swap contrast regression", c)
				}
			}
		}
	}
	if !seen["btn-light"] || !seen["btn-outline-light"] {
		t.Errorf("expected handler to emit both btn-light and btn-outline-light across toggles, got: %v", seen)
	}
}

// TestEinkToggle_NeverReturnsLowContrastClasses is a regression guard against
// the original bug: btn-outline-secondary (#6c757d) on the dark navy navbar
// (#2d3a5c) is ~2.4:1 contrast (fails WCAG AA 4.5:1), and btn-dark is
// near-invisible on the dark navbar. Neither must ever be returned.
func TestEinkToggle_NeverReturnsLowContrastClasses(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupEinkRouter(h)
	u := seedEinkTestUser(t, h, "contrastadmin", user.RoleAdmin)
	ctx := withEinkUserContext(context.Background(), u)

	forbidden := []string{"btn-outline-secondary", "btn-dark"}

	for i := 0; i < 4; i++ { // on/off/on/off
		req := httptest.NewRequest("POST", "/settings/eink-toggle", nil).WithContext(ctx)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("toggle %d: expected 200, got %d", i, w.Code)
		}
		body := w.Body.String()
		for _, bad := range forbidden {
			if strings.Contains(body, bad) {
				t.Errorf("toggle %d: returned low-contrast class %q: %s", i, bad, body)
			}
		}
	}
}

// TestEinkCSS_HasNavbarTogglerOverride ensures the mobile navbar toggler stays
// visible in e-ink mode. The navbar keeps Bootstrap's navbar-dark (white bars)
// while eink.css makes the navbar white, so without an override the hamburger
// icon is invisible.
func TestEinkCSS_HasNavbarTogglerOverride(t *testing.T) {
	cssBytes, err := staticFS.ReadFile("static/eink.css")
	if err != nil {
		t.Fatalf("read embedded eink.css: %v", err)
	}
	css := string(cssBytes)

	if !strings.Contains(css, ".navbar-dark .navbar-toggler") {
		t.Error("eink.css missing .navbar-dark .navbar-toggler override — mobile menu button border invisible in e-ink mode")
	}
	if !strings.Contains(css, "navbar-toggler-icon") {
		t.Error("eink.css missing navbar-toggler-icon override — white bars invisible on white e-ink navbar")
	}
	if !strings.Contains(css, "#000") {
		t.Error("eink.css toggler override must use black (#000) for visible bars/border on the white e-ink navbar")
	}
}
