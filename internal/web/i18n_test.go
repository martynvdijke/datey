package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/i18n"
)

func TestLocaleMiddleware_Resolve(t *testing.T) {
	h := newTestWebHandler(t)

	// header de should set locale de
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "de")
	rr := httptest.NewRecorder()
	var gotLocale string
	handler := h.Locale(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLocale = i18n.LocaleFromContext(r.Context())
	}))
	handler.ServeHTTP(rr, req)
	if gotLocale != "de" {
		t.Fatalf("expected de got %q", gotLocale)
	}

	// user pref wins over header
	loc := "de"
	u := &ent.User{ID: 1, Locale: &loc}
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Accept-Language", "en")
	ctx := context.WithValue(req2.Context(), userContextKey, u)
	req2 = req2.WithContext(ctx)
	rr2 := httptest.NewRecorder()
	gotLocale = ""
	handler.ServeHTTP(rr2, req2)
	if gotLocale != "de" {
		t.Fatalf("expected de from user pref got %q", gotLocale)
	}

	// anonymous fallback en
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	rr3 := httptest.NewRecorder()
	gotLocale = ""
	handler.ServeHTTP(rr3, req3)
	if gotLocale != "en" {
		t.Fatalf("expected en fallback got %q", gotLocale)
	}
}

func TestTemplateSmoke_De(t *testing.T) {
	h := newTestWebHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// set de via context
	ctx := i18n.WithLocale(withUserContext(req.Context()), "de")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	h.dashboard(rr, req)
	body := rr.Body.String()
	if !strings.Contains(body, "Personen") && !strings.Contains(body, "Anstehend") && !strings.Contains(body, "Meilensteine") {
		t.Fatalf("expected German string in dashboard under de, got body snippet %q", body[:2000])
	}
}

func TestSettingsLocaleSave(t *testing.T) {
	h := newTestWebHandler(t)
	// create user
	_, _ = h.users.Create(httptestContext(), "alice", "hash", "admin")
	// Use user from context with ID 1 (test helper uses ID 1)
	loc := "en"
	u := &ent.User{ID: 1, Locale: &loc}
	// Ensure user exists in DB for update: create via repository if not exists
	// Try to fetch; if not found, create
	if _, err := h.users.GetByID(context.Background(), 1); err != nil {
		// create will allocate new ID; not needed for test - we test via handler's users.SetLocale path
		// Use the handler's client directly
		h.client.User.Create().SetUsername("alice2").SetPasswordHash("hash").SetRole("admin").SaveX(context.Background())
		// fetch first user
		users, _ := h.users.List(context.Background())
		if len(users) > 0 {
			u.ID = users[0].ID
		}
	}
	form := "locale=de&csrf_token=dummy"
	req := httptest.NewRequest(http.MethodPost, "/settings/locale", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// set CSRF cookie to match
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "dummy"})
	req.Header.Set("X-CSRF-Token", "dummy")
	ctx := context.WithValue(req.Context(), userContextKey, u)
	// also need csrf token in context for middleware? The handler's CSRF middleware validates, but we bypass it by calling handler directly
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	h.settingsLocaleSave(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body %q", rr.Code, rr.Body.String())
	}
}

func httptestContext() context.Context { return context.Background() }
