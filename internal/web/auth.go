package web

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"golang.org/x/crypto/bcrypt"

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

// getUserID is a helper to get user ID from request.
func getUserID(r *http.Request) int {
	u := UserFromContext(r.Context())
	if u == nil {
		return 0
	}
	return u.ID
}

func (h *Handler) loginPage(w http.ResponseWriter, r *http.Request) {
	// If already authenticated, redirect to dashboard
	if u := UserFromContext(r.Context()); u != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	h.render(w, r, "login.html", map[string]any{
		"Title": "Datey - Login",
	})
}

func (h *Handler) loginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		h.render(w, r, "login.html", map[string]any{
			"Title": "Datey - Login",
			"Error": "Username and password are required",
			"FormData": map[string]string{
				"Username": username,
			},
		})
		return
	}

	// Rate-limit login attempts per IP+username (spec: security-hardening).
	rlKey := rateLimitKey(r, username)
	allowed, retryAfter := h.loginLimiter.allow(rlKey)
	if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
		h.render(w, r, "login.html", map[string]any{
			"Title": "Datey - Login",
			"Error": "Too many login attempts. Please try again later.",
			"FormData": map[string]string{
				"Username": username,
			},
		})
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}

	u, err := h.users.GetByUsername(r.Context(), username)
	if err != nil {
		h.render(w, r, "login.html", map[string]any{
			"Title": "Datey - Login",
			"Error": "Invalid username or password",
			"FormData": map[string]string{
				"Username": username,
			},
		})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		h.render(w, r, "login.html", map[string]any{
			"Title": "Datey - Login",
			"Error": "Invalid username or password",
			"FormData": map[string]string{
				"Username": username,
			},
		})
		return
	}

	// Successful login — reset the rate-limit counter.
	h.loginLimiter.reset(rlKey)

	token, err := h.sessions.Create(r.Context(), u.ID)
	if err != nil {
		slog.Error("login: create session", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	session.SetCookie(w, token, r.TLS != nil)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	token, err := session.ReadCookie(r)
	if err == nil && token != "" {
		if err := h.sessions.Delete(r.Context(), token); err != nil {
			slog.Warn("logout: delete session", "error", err)
		}
	}
	session.ClearCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handler) setupPage(w http.ResponseWriter, r *http.Request) {
	exists, err := h.users.Exists(r.Context())
	if err == nil && exists {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	h.render(w, r, "setup.html", map[string]any{
		"Title": "Datey - Setup",
	})
}

func (h *Handler) setupCreate(w http.ResponseWriter, r *http.Request) {
	exists, _ := h.users.Exists(r.Context())
	if exists {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" {
		h.render(w, r, "setup.html", map[string]any{
			"Title": "Datey - Setup",
			"Error": "Username is required",
			"FormData": map[string]string{
				"Username": username,
			},
		})
		return
	}

	if len(password) < 8 {
		h.render(w, r, "setup.html", map[string]any{
			"Title": "Datey - Setup",
			"Error": "Password must be at least 8 characters",
			"FormData": map[string]string{
				"Username": username,
			},
		})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		slog.Error("setup: hash password", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	_, err = h.users.Create(r.Context(), username, string(hash), user.RoleAdmin)
	if err != nil {
		slog.Error("setup: create admin user", "error", err, "username", username)
		h.render(w, r, "setup.html", map[string]any{
			"Title": "Datey - Setup",
			"Error": "Failed to create admin user. Please try again.",
			"FormData": map[string]string{
				"Username": username,
			},
		})
		return
	}

	slog.Info("admin user created", "username", username)
	http.Redirect(w, r, "/login?success=Admin+account+created.+Please+log+in.", http.StatusSeeOther)
}
