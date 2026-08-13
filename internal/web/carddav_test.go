package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func setupCarddavRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/settings", h.settings)
	r.Post("/settings/carddav", h.settingsCarddavSave)
	r.Post("/settings/carddav/sync", h.settingsCarddavSync)
	return r
}

func TestSettings_NotificationsShowsCarddavSection(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCarddavRouter(h)

	req := httptest.NewRequest("GET", "/settings", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, s := range []string{
		"CardDAV Sync",
		`name="CARDDAV_ENABLED"`,
		`name="CARDDAV_URL"`,
		`name="CARDDAV_USERNAME"`,
		`name="CARDDAV_PASSWORD"`,
		`name="CARDDAV_DELETE_POLICY"`,
	} {
		if !strings.Contains(body, s) {
			t.Errorf("expected %q in settings page, missing", s)
		}
	}
}

func TestSettingsCarddavSave_SuccessHotReloads(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCarddavRouter(h)

	form := url.Values{}
	form.Set("CARDDAV_ENABLED", "on")
	form.Set("CARDDAV_URL", "https://cloud.example.com/dav/")
	form.Set("CARDDAV_USERNAME", "alice")
	form.Set("CARDDAV_PASSWORD", "s3cret")
	form.Set("CARDDAV_DELETE_POLICY", "delete")

	req := httptest.NewRequest("POST", "/settings/carddav", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String()[:300])
	}
	if w.Header().Get("HX-Refresh") != "true" {
		t.Errorf("expected HX-Refresh header on success")
	}
	if !h.cfg.CarddavEnabled {
		t.Error("expected CarddavEnabled hot-reload to true")
	}
	if h.cfg.CarddavURL != "https://cloud.example.com/dav/" {
		t.Errorf("CarddavURL = %q", h.cfg.CarddavURL)
	}
	if h.cfg.CarddavUsername != "alice" {
		t.Errorf("CarddavUsername = %q", h.cfg.CarddavUsername)
	}
	if h.cfg.CarddavPassword != "s3cret" {
		t.Errorf("CarddavPassword not hot-reloaded")
	}
	if h.cfg.CarddavDeletePolicy != "delete" {
		t.Errorf("CarddavDeletePolicy = %q", h.cfg.CarddavDeletePolicy)
	}
}

func TestSettingsCarddavSave_EmptyPasswordKeepsExisting(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCarddavRouter(h)

	// Seed an existing password via a first save.
	form := url.Values{}
	form.Set("CARDDAV_ENABLED", "on")
	form.Set("CARDDAV_URL", "https://cloud.example.com/dav/")
	form.Set("CARDDAV_PASSWORD", "keep-me")
	req := httptest.NewRequest("POST", "/settings/carddav", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if h.cfg.CarddavPassword != "keep-me" {
		t.Fatalf("seed password = %q", h.cfg.CarddavPassword)
	}

	// Second save without a password keeps the stored one.
	form = url.Values{}
	form.Set("CARDDAV_URL", "https://cloud.example.com/other/")
	req = httptest.NewRequest("POST", "/settings/carddav", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if h.cfg.CarddavPassword != "keep-me" {
		t.Errorf("expected stored password kept, got %q", h.cfg.CarddavPassword)
	}
}

func TestSettingsCarddavSave_InvalidURLReRenders(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCarddavRouter(h)

	form := url.Values{}
	form.Set("CARDDAV_URL", "not-a-url")

	req := httptest.NewRequest("POST", "/settings/carddav", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (re-render), got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "CardDAV URL must be an absolute URL") {
		t.Errorf("expected inline error for CARDDAV_URL, got: %s", body[:400])
	}
}

func TestSettingsCarddavSave_InvalidDeletePolicyReRenders(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCarddavRouter(h)

	form := url.Values{}
	form.Set("CARDDAV_URL", "https://cloud.example.com/dav/")
	form.Set("CARDDAV_DELETE_POLICY", "obliterate")

	req := httptest.NewRequest("POST", "/settings/carddav", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (re-render), got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Delete policy must be one of: keep, delete") {
		t.Errorf("expected inline error for CARDDAV_DELETE_POLICY, got: %s", body[:400])
	}
}

func TestSettingsCarddavSync_DisabledShowsWarning(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCarddavRouter(h)

	req := httptest.NewRequest("POST", "/settings/carddav/sync", nil)
	req = req.WithContext(withUserContext(req.Context()))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "not enabled") {
		t.Errorf("expected disabled warning, got: %s", w.Body.String()[:300])
	}
}
