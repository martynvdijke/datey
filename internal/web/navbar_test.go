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

func setupNavbarRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.dashboard)
	r.Get("/login", h.loginPage)
	return r
}

func seedNavbarTestUser(t *testing.T, h *Handler, username string, role user.Role) *ent.User {
	t.Helper()
	u, err := h.users.Create(context.Background(), username, "hash", role)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u
}

func TestNavbar_RendersForAuthenticatedUser(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupNavbarRouter(h)
	u := seedNavbarTestUser(t, h, "navuser", user.RoleUser)

	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, u))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()

	mustContain := []string{
		"Datey",
		"Dashboard",
		"People",
		"Calendar",
		"Notifications",
		"Search people",
		"theme-toggle",
		"theme-icon-eink",
		"E-Ink",
		"Light",
	}
	for _, s := range mustContain {
		if !strings.Contains(body, s) {
			t.Errorf("navbar missing %q", s)
		}
	}

	// Non-admin users must not see admin-only top nav links (Groups/Users).
	// Settings is reachable from the user dropdown for everyone.
	adminOnly := []string{"href=\"/groups\"", "href=\"/users\""}
	for _, s := range adminOnly {
		if strings.Contains(body, s) {
			t.Errorf("non-admin navbar should not contain %q", s)
		}
	}
}

func TestNavbar_RendersAdminLinksForAdmin(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupNavbarRouter(h)
	u := seedNavbarTestUser(t, h, "navadmin", user.RoleAdmin)

	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, u))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()

	adminLinks := []string{"Groups", "Settings", "Users"}
	for _, s := range adminLinks {
		if !strings.Contains(body, s) {
			t.Errorf("admin navbar missing %q", s)
		}
	}
}

func TestNavbar_SearchFormTargetsPeople(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupNavbarRouter(h)
	u := seedNavbarTestUser(t, h, "searchuser", user.RoleUser)

	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, u))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	body := w.Body.String()

	if !strings.Contains(body, `action="/people"`) {
		t.Error("navbar search form must POST/GET to /people")
	}
	if !strings.Contains(body, `name="q"`) {
		t.Error("navbar search input must be named q")
	}
}

func TestThemeScript_PresentInBaseTemplate(t *testing.T) {
	baseBytes, err := templateFS.ReadFile("templates/base.html")
	if err != nil {
		t.Fatalf("read embedded base.html: %v", err)
	}
	base := string(baseBytes)

	mustContain := []string{
		`data-bs-theme="light"`,
		"localStorage.getItem('datey-theme')",
		"localStorage.setItem('datey-theme', theme)",
		"prefers-color-scheme",
		"theme-toggle",
		"theme-icon-eink",
		"eink-stylesheet",
	}
	for _, s := range mustContain {
		if !strings.Contains(base, s) {
			t.Errorf("base.html missing theme support: %q", s)
		}
	}
}

func TestStyleCSS_HasEssentials(t *testing.T) {
	cssBytes, err := staticFS.ReadFile("static/style.css")
	if err != nil {
		t.Fatalf("read embedded style.css: %v", err)
	}
	css := string(cssBytes)

	mustContain := []string{
		".event-card::before",
		"#toast-container",
		".empty-state",
		"htmx-indicator",
		".theme-icon-eink",
		"data-bs-theme",
	}
	for _, s := range mustContain {
		if !strings.Contains(css, s) {
			t.Errorf("style.css missing essential style: %q", s)
		}
	}
}

func TestDashboard_RendersEmptyState(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupNavbarRouter(h)
	u := seedNavbarTestUser(t, h, "dashuser", user.RoleUser)

	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, u))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()

	if !strings.Contains(body, "Upcoming Events") {
		t.Error("dashboard missing heading")
	}
	if !strings.Contains(body, "Add a person") {
		t.Error("dashboard empty state missing add-person action")
	}
}
