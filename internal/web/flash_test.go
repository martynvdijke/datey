package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFlashMessageRenderedAsToast verifies that redirect-based flash messages
// (?success= / ?error=) are rendered into the page as show-toast dispatches so
// that toasts appear after create/delete redirect flows (polish-ui-ux task 6.3).
func TestFlashMessageRenderedAsToast(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupAuthRouter(h)

	// Success flash: message text and success toast type must be present.
	req := httptest.NewRequest("GET", "/login?success=Person+created", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "show-toast") {
		t.Errorf("expected show-toast dispatch in body")
	}
	if !strings.Contains(body, "Person created") {
		t.Errorf("expected flash message text in body")
	}
	if !strings.Contains(body, "type: 'success'") {
		t.Errorf("expected success toast type, got: %s", body[:500])
	}

	// Error flash: message text and error toast type must be present.
	req = httptest.NewRequest("GET", "/login?error=Something+went+wrong", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body = w.Body.String()
	if !strings.Contains(body, "Something went wrong") {
		t.Errorf("expected error flash message text in body")
	}
	if !strings.Contains(body, "type: 'error'") {
		t.Errorf("expected error toast type, got: %s", body[:500])
	}

	// No flash: variables render as null and no toast dispatch should be emitted.
	req = httptest.NewRequest("GET", "/login", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body = w.Body.String()
	if !strings.Contains(body, "var flashSuccess = null;") {
		t.Errorf("expected flashSuccess null without ?success param")
	}
	if !strings.Contains(body, "var flashError = null;") {
		t.Errorf("expected flashError null without ?error param")
	}
	if strings.Contains(body, "Person created") {
		t.Errorf("expected no success flash payload without ?success param")
	}
}
