package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func authConfigRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/api/auth/config", h.authConfig)
	return r
}

func TestAuthConfig_DemoEnabled(t *testing.T) {
	h := newTestWebHandler(t)
	h.cfg.DemoUserEnabled = true
	h.cfg.DemoUserHandle = "Demo"
	h.cfg.DemoUserPassword = "demo-pass"

	req := httptest.NewRequest("GET", "/api/auth/config", nil)
	w := httptest.NewRecorder()
	authConfigRouter(h).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body struct {
		Demo struct {
			Enabled  bool   `json:"enabled"`
			Handle   string `json:"handle"`
			Password string `json:"password"`
		} `json:"demo"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	if !body.Demo.Enabled {
		t.Error("expected demo.enabled = true")
	}
	if body.Demo.Handle != "demo" {
		t.Errorf("handle = %q, want demo", body.Demo.Handle)
	}
	if body.Demo.Password != "demo-pass" {
		t.Errorf("password = %q, want demo-pass", body.Demo.Password)
	}
}

func TestAuthConfig_DemoDisabled(t *testing.T) {
	h := newTestWebHandler(t)

	req := httptest.NewRequest("GET", "/api/auth/config", nil)
	w := httptest.NewRecorder()
	authConfigRouter(h).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body struct {
		Demo struct {
			Enabled  bool   `json:"enabled"`
			Handle   string `json:"handle"`
			Password string `json:"password"`
		} `json:"demo"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	if body.Demo.Enabled {
		t.Error("expected demo.enabled = false")
	}
	if body.Demo.Handle != "" || body.Demo.Password != "" {
		t.Errorf("expected credentials omitted, got handle=%q password=%q", body.Demo.Handle, body.Demo.Password)
	}
}

func TestLoginPage_DemoFormOnlyWhenEnabled(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		h := newTestWebHandler(t)
		h.cfg.DemoUserEnabled = true
		h.cfg.DemoUserHandle = "demo"
		h.cfg.DemoUserPassword = "demo-pass"

		req := httptest.NewRequest("GET", "/login", nil)
		w := httptest.NewRecorder()
		setupAuthRouter(h).ServeHTTP(w, req)

		body := w.Body.String()
		for _, want := range []string{
			"Try the demo account",
			"Shared demo account @demo",
			`name="username" value="demo"`,
			`name="password" value="demo-pass"`,
			"Log in as @demo",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("expected login page to contain %q", want)
			}
		}
	})

	t.Run("disabled", func(t *testing.T) {
		h := newTestWebHandler(t)

		req := httptest.NewRequest("GET", "/login", nil)
		w := httptest.NewRecorder()
		setupAuthRouter(h).ServeHTTP(w, req)

		body := w.Body.String()
		if strings.Contains(body, "Try the demo account") {
			t.Error("did not expect demo form when disabled")
		}
		if strings.Contains(body, "demo-pass") {
			t.Error("did not expect demo password when disabled")
		}
	})
}

func TestLoginPost_DemoFormShownOnError(t *testing.T) {
	h := newTestWebHandler(t)
	h.cfg.DemoUserEnabled = true
	h.cfg.DemoUserHandle = "demo"
	h.cfg.DemoUserPassword = "demo-pass"

	body := "username=nobody&password=wrong"
	req := httptest.NewRequest("POST", "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	setupAuthRouter(h).ServeHTTP(w, req)

	got := w.Body.String()
	if !strings.Contains(got, "Invalid username or password") {
		t.Fatalf("expected login error, got: %s", got)
	}
	if !strings.Contains(got, "Try the demo account") {
		t.Error("expected demo form on login error")
	}
}
