package web

import (
	"fmt"
	"log/slog"
	"net/http"
)

// appError is a typed application error with a safe user-facing message.
// The internal cause is logged server-side but never shown to the user.
type appError struct {
	status  int    // HTTP status code
	message string // safe user-facing message
	cause   error  // internal error detail (logged, not shown to user)
}

func (e *appError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.message, e.cause)
	}
	return e.message
}

func (e *appError) Unwrap() error {
	return e.cause
}

// renderAppError logs the internal cause and renders a safe error response
// through the shared error template. This is the single formatter for all
// user-facing errors (spec: tech-debt-cleanup — consolidated error handling).
func (h *Handler) renderAppError(w http.ResponseWriter, r *http.Request, appErr *appError) {
	if appErr.cause != nil {
		slog.Error(appErr.message, "error", appErr.cause, "path", r.URL.Path, "method", r.Method)
	}
	w.WriteHeader(appErr.status)
	statusText := http.StatusText(appErr.status)
	h.render(w, r, "error.html", map[string]any{
		"Title":      "Datey - " + statusText,
		"StatusCode": appErr.status,
		"StatusText": statusText,
	})
	if r.Header.Get("HX-Request") == "true" {
		toastHeader(w, statusText, "error")
	}
}
