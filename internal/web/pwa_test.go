package web

import (
	"bytes"
	"encoding/json"
	"image"
	_ "image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func setupPWARouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

func TestManifestRoute(t *testing.T) {
	h := newTestWebHandler(t)
	r := setupPWARouter(h)

	for _, path := range []string{"/manifest.json", "/static/manifest.json"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s expected 200 got %d", path, w.Code)
		}
		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "application/manifest+json") {
			t.Errorf("GET %s expected manifest content-type got %q", path, ct)
		}
		var m map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
			t.Fatalf("invalid JSON for %s: %v", path, err)
		}
		for _, field := range []string{"name", "short_name", "display", "start_url", "theme_color", "icons"} {
			if _, ok := m[field]; !ok {
				t.Errorf("manifest missing field %q", field)
			}
		}
		if m["name"] != "Datey" {
			t.Errorf("expected name Datey got %v", m["name"])
		}
		if m["theme_color"] != "#4f46e5" {
			t.Errorf("expected theme_color #4f46e5 got %v", m["theme_color"])
		}
		icons, ok := m["icons"].([]any)
		if !ok || len(icons) < 2 {
			t.Errorf("expected at least 2 icons")
		}
	}
}

func TestBaseHTMLContainsPWAMarkup(t *testing.T) {
	data, err := os.ReadFile("templates/base.html")
	if err != nil {
		t.Fatalf("read base.html: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `rel="manifest"`) {
		t.Errorf("base.html missing manifest link")
	}
	if !strings.Contains(s, `name="theme-color"`) {
		t.Errorf("base.html missing theme-color meta")
	}
	if !strings.Contains(s, `offline-banner`) {
		t.Errorf("base.html missing offline banner element")
	}
	if !strings.Contains(s, `navigator.onLine`) {
		t.Errorf("base.html missing offline JS")
	}
}

func TestServiceWorkerContent(t *testing.T) {
	h := newTestWebHandler(t)
	r := setupPWARouter(h)
	req := httptest.NewRequest("GET", "/sw.js", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /sw.js expected 200 got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "CACHE_VERSION") {
		t.Errorf("sw.js missing CACHE_VERSION")
	}
	if !strings.Contains(body, "CACHE_NAME") {
		t.Errorf("sw.js missing CACHE_NAME")
	}
	if !strings.Contains(body, "offline.html") {
		t.Errorf("sw.js missing offline fallback")
	}
	// ensure old caches are purged logic present
	if !strings.Contains(body, "caches.delete") {
		t.Errorf("sw.js missing cache busting")
	}
	// push handlers untouched
	if !strings.Contains(body, "addEventListener('push'") {
		t.Errorf("sw.js missing push handler")
	}
	// fetch handler routing
	if !strings.Contains(body, "addEventListener('fetch'") {
		t.Errorf("sw.js missing fetch handler")
	}
	// fetch handler routing
	if !strings.Contains(body, "addEventListener('fetch'") {
		t.Errorf("sw.js missing fetch handler")
	}
	// version placeholder must be substituted with the real app version
	if strings.Contains(body, "__DATEY_VERSION__") {
		t.Errorf("sw.js served with unsubstituted version placeholder")
	}

	// icons must be valid PNGs at the advertised sizes
	for _, size := range []string{"192", "512"} {
		data, err := staticFS.ReadFile("static/icon-" + size + ".png")
		if err != nil {
			t.Fatalf("icon-%s.png missing: %v", size, err)
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("icon-%s.png is not a valid PNG: %v", size, err)
		}
		if format != "png" || cfg.Width != cfg.Height {
			t.Errorf("icon-%s.png: got format=%s %dx%d, want square PNG", size, format, cfg.Width, cfg.Height)
		}
		if want, _ := strconv.Atoi(size); cfg.Width != want {
			t.Errorf("icon-%s.png has width %d", size, cfg.Width)
		}
	}
}
