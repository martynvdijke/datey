package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func setupCSRFRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Use(h.CSRF)
	r.Get("/form", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Post("/submit", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return r
}

// TestCSRF_GETSetsCookie verifies that a GET request sets the CSRF cookie.
func TestCSRF_GETSetsCookie(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCSRFRouter(h)

	req := httptest.NewRequest("GET", "/form", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var hasCSRFCookie bool
	for _, c := range w.Result().Cookies() {
		if c.Name == "csrf_token" && c.Value != "" {
			hasCSRFCookie = true
		}
	}
	if !hasCSRFCookie {
		t.Error("expected csrf_token cookie to be set on GET")
	}
}

// TestCSRF_PostWithoutTokenRejected verifies that a POST without a CSRF token is rejected.
// Spec: security-hardening — Request without token is rejected.
func TestCSRF_PostWithoutTokenRejected(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCSRFRouter(h)

	// First GET to set the cookie.
	getReq := httptest.NewRequest("GET", "/form", nil)
	getW := httptest.NewRecorder()
	router.ServeHTTP(getW, getReq)
	cookie := getW.Result().Cookies()[0]

	// POST without CSRF token (no header, no form field).
	postReq := httptest.NewRequest("POST", "/submit", strings.NewReader(""))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(cookie)
	postW := httptest.NewRecorder()
	router.ServeHTTP(postW, postReq)

	if postW.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", postW.Code)
	}
}

// TestCSRF_PostWithValidTokenAccepted verifies that a POST with a valid CSRF token passes.
// Spec: security-hardening — Request with valid token is accepted.
func TestCSRF_PostWithValidTokenAccepted(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCSRFRouter(h)

	// First GET to set the cookie.
	getReq := httptest.NewRequest("GET", "/form", nil)
	getW := httptest.NewRecorder()
	router.ServeHTTP(getW, getReq)
	var csrfCookie *http.Cookie
	for _, c := range getW.Result().Cookies() {
		if c.Name == "csrf_token" {
			csrfCookie = c
		}
	}
	if csrfCookie == nil {
		t.Fatal("expected csrf_token cookie from GET")
	}

	// POST with matching token in header.
	postReq := httptest.NewRequest("POST", "/submit", strings.NewReader(""))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(csrfCookie)
	postReq.Header.Set("X-CSRF-Token", csrfCookie.Value)
	postW := httptest.NewRecorder()
	router.ServeHTTP(postW, postReq)

	if postW.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", postW.Code)
	}
}

// TestCSRF_PostWithValidFormFieldAccepted verifies that a POST with a valid CSRF token in the form field passes.
func TestCSRF_PostWithValidFormFieldAccepted(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCSRFRouter(h)

	// First GET to set the cookie.
	getReq := httptest.NewRequest("GET", "/form", nil)
	getW := httptest.NewRecorder()
	router.ServeHTTP(getW, getReq)
	var csrfCookie *http.Cookie
	for _, c := range getW.Result().Cookies() {
		if c.Name == "csrf_token" {
			csrfCookie = c
		}
	}
	if csrfCookie == nil {
		t.Fatal("expected csrf_token cookie from GET")
	}

	// POST with matching token in form field.
	body := "csrf_token=" + csrfCookie.Value
	postReq := httptest.NewRequest("POST", "/submit", strings.NewReader(body))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(csrfCookie)
	postW := httptest.NewRecorder()
	router.ServeHTTP(postW, postReq)

	if postW.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", postW.Code)
	}
}

// TestCSRF_PostWithMismatchedTokenRejected verifies that a POST with a wrong token is rejected.
// Spec: security-hardening — Request with mismatched token is rejected.
func TestCSRF_PostWithMismatchedTokenRejected(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCSRFRouter(h)

	// First GET to set the cookie.
	getReq := httptest.NewRequest("GET", "/form", nil)
	getW := httptest.NewRecorder()
	router.ServeHTTP(getW, getReq)
	var csrfCookie *http.Cookie
	for _, c := range getW.Result().Cookies() {
		if c.Name == "csrf_token" {
			csrfCookie = c
		}
	}

	// POST with mismatched token in header.
	postReq := httptest.NewRequest("POST", "/submit", strings.NewReader(""))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(csrfCookie)
	postReq.Header.Set("X-CSRF-Token", "wrong-token-value")
	postW := httptest.NewRecorder()
	router.ServeHTTP(postW, postReq)

	if postW.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", postW.Code)
	}
}

// TestCSRF_GETAlwaysAllowed verifies that GET requests are always allowed without a token.
func TestCSRF_GETAlwaysAllowed(t *testing.T) {
	h := newTestWebHandler(t)
	router := setupCSRFRouter(h)

	// GET without any CSRF cookie or token.
	req := httptest.NewRequest("GET", "/form", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
