package web

import (
	"context"
	"net/http"
	"strconv"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/user"
	"github.com/datey/datey/internal/session"
)

type contextKey string

const userContextKey contextKey = "user"

// UserFromContext returns the User from the request context.
func UserFromContext(ctx context.Context) *ent.User {
	u, ok := ctx.Value(userContextKey).(*ent.User)
	if !ok {
		return nil
	}
	return u
}

// IsAdmin checks if the user in context is an admin.
func IsAdmin(ctx context.Context) bool {
	u := UserFromContext(ctx)
	if u == nil {
		return false
	}
	return u.Role == user.RoleAdmin
}

// Auth middleware checks for a valid session cookie and injects the user into the request context.
func (h *Handler) Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := session.ReadCookie(r)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		sess, err := h.sessions.GetByToken(r.Context(), token)
		if err != nil {
			session.ClearCookie(w)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		u, err := h.users.GetByID(r.Context(), sess.Edges.User.ID)
		if err != nil {
			session.ClearCookie(w)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Admin middleware checks that the authenticated user has the admin role.
func (h *Handler) Admin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsAdmin(r.Context()) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SetupRedirect middleware checks if any users exist in the database.
// If no users exist and the request is not for /setup or /login, redirects to /setup.
func (h *Handler) SetupRedirect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// Skip for setup, login, logout, and health endpoints
	if r.URL.Path == "/setup" || r.URL.Path == "/login" || r.URL.Path == "/logout" || r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		exists, err := h.users.Exists(r.Context())
		if err != nil || !exists {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// userFromRequest is a helper to get user ID from request (for handler methods).
func getUserID(r *http.Request) int {
	u := UserFromContext(r.Context())
	if u == nil {
		return 0
	}
	return u.ID
}

// parseIntParam parses an ID from a URL param.
func parseIntParam(r *http.Request, param string) (int, error) {
	return strconv.Atoi(r.PathValue(param))
}
