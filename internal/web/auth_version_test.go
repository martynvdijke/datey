package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/datey/datey/handlers"
)

// TestAuthPagesShowVersion covers the login-page-polish requirement that the
// unauthenticated auth surfaces render the running version, sourced from
// handlers.Version rather than a hardcoded template string.
func TestAuthPagesShowVersion(t *testing.T) {
	prev := handlers.Version
	handlers.Version = "9.9.9-test"
	t.Cleanup(func() { handlers.Version = prev })

	h := newTestWebHandler(t)
	router := setupAuthRouter(h)

	for _, path := range []string{"/login", "/setup"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("GET %s: expected 200, got %d", path, w.Code)
		}
		if body := w.Body.String(); !strings.Contains(body, "Datey v9.9.9-test") {
			t.Errorf("GET %s: version footer missing, body: %s", path, body)
		}
	}
}

// TestLoginErrorIsAnnounced guards the inline error region: a failed sign-in
// must render a live region so assistive technology reads the failure.
func TestLoginErrorIsAnnounced(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupAuthRouter(h)

	body := "username=nobody&password=wrong"
	req := httptest.NewRequest("POST", "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	got := w.Body.String()
	if !strings.Contains(got, "Invalid username or password") {
		t.Fatalf("expected the credential error in the response, got: %s", got)
	}
	if !strings.Contains(got, `role="alert"`) {
		t.Error("expected the login error to be exposed as role=\"alert\"")
	}
	if !strings.Contains(got, `id="login-error"`) {
		t.Error("expected the login error to carry id=\"login-error\" for aria-describedby")
	}
}
