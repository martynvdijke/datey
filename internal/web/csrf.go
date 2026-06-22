package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

const (
	csrfCookieName = "csrf_token"
	csrfHeaderName = "X-CSRF-Token"
	csrfFormField  = "csrf_token"
)

var csrfContextKey contextKey = "csrf_token"

// generateCSRFToken returns a cryptographically random 32-byte hex-encoded token.
func generateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CSRF is a double-submit cookie CSRF defense-in-depth middleware.
// On every request it ensures a CSRF cookie exists (setting one if needed)
// and stores the token in the request context for template rendering.
// On state-changing requests (POST/PUT/DELETE) it validates that the
// submitted token (from the X-CSRF-Token header or csrf_token form field)
// matches the cookie value.  GET/HEAD/OPTIONS requests are always allowed.
//
// Spec: security-hardening — State-changing requests require CSRF tokens.
func (h *Handler) CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get or set the CSRF cookie.
		cookie, err := r.Cookie(csrfCookieName)
		var token string
		if err == nil && cookie.Value != "" {
			token = cookie.Value
		} else {
			token, err = generateCSRFToken()
			if err != nil {
				h.renderError(w, r, http.StatusInternalServerError)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     csrfCookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: false, // JS must read it for HTMX header injection
				SameSite: http.SameSiteLaxMode,
				Secure:   r.TLS != nil,
				MaxAge:   60 * 60 * 24 * 30, // 30 days
			})
		}

		// Validate token on state-changing requests.
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
			submitted := r.Header.Get(csrfHeaderName)
			if submitted == "" {
				// Only parse the form if the header was not sent (traditional form POST).
				submitted = r.FormValue(csrfFormField)
			}
			if submitted == "" || submitted != token {
				h.renderAppError(w, r, &appError{
					status:  http.StatusForbidden,
					message: "CSRF token validation failed",
				})
				return
			}
		}

		ctx := context.WithValue(r.Context(), csrfContextKey, token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// csrfTokenFromContext returns the CSRF token stored by the middleware, or "".
func csrfTokenFromContext(ctx context.Context) string {
	v, ok := ctx.Value(csrfContextKey).(string)
	if !ok {
		return ""
	}
	return v
}
