package web

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// resetTokenTTL is how long a password reset link stays valid.
const resetTokenTTL = 1 * time.Hour

// emailNotifierName is the registered name of the SMTP notifier. Password
// reset links are emailed to the configured notification address, matching
// how the app delivers its other email notifications.
const emailNotifierName = "email"

// forgotPasswordPage renders the "request a password reset" form.
func (h *Handler) forgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	// Already logged in? Nothing to reset.
	if u := UserFromContext(r.Context()); u != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	data := map[string]any{"Title": "Datey - Reset Password"}
	if !h.notifReg.IsConfigured(emailNotifierName) {
		data["Error"] = "Password reset by email is unavailable because no email channel is configured."
	}
	h.render(w, r, "forgot_password.html", data)
}

// forgotPasswordPost validates the submitted username and, when a matching
// account exists, emails a one-time reset link to the configured notification
// address. The response is identical whether or not the username exists so the
// endpoint cannot be used to enumerate accounts.
func (h *Handler) forgotPasswordPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	if username == "" {
		h.render(w, r, "forgot_password.html", map[string]any{
			"Title": "Datey - Reset Password",
			"Error": "Username is required",
			"FormData": map[string]string{
				"Username": username,
			},
		})
		return
	}

	// Throttle reset requests per IP to prevent email bombing.
	ip := requestIP(r)
	allowed, retryAfter := h.forgotLimiter.allow(ip)
	if !allowed {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())+1))
		w.WriteHeader(http.StatusTooManyRequests)
		h.render(w, r, "forgot_password.html", map[string]any{
			"Title": "Datey - Reset Password",
			"Error": "Too many reset requests. Please try again later.",
		})
		return
	}

	if !h.notifReg.IsConfigured(emailNotifierName) {
		h.render(w, r, "forgot_password.html", map[string]any{
			"Title": "Datey - Reset Password",
			"Error": "Password reset by email is unavailable because no email channel is configured.",
		})
		return
	}

	// Look up the account without revealing whether it exists.
	u, err := h.users.GetByUsername(r.Context(), username)
	if err == nil {
		raw, _, err := h.passwordResetTokens.Create(r.Context(), u.ID, resetTokenTTL)
		if err != nil {
			slog.Error("forgot password: create reset token", "error", err, "user_id", u.ID)
			h.renderError(w, r, http.StatusInternalServerError)
			return
		}

		link := h.resetLink(r, raw)
		msg := fmt.Sprintf(
			"A password reset was requested for the Datey account '%s'.\n\nTo choose a new password, open this link within the next hour:\n\n%s\n\nIf you did not request this, you can safely ignore this email and your password will stay unchanged.",
			u.Username, link,
		)
		if err := h.notifReg.Send(r.Context(), emailNotifierName, "Datey - Reset your password", msg); err != nil {
			slog.Error("forgot password: send reset email", "error", err, "user_id", u.ID)
			h.renderError(w, r, http.StatusInternalServerError)
			return
		}
		slog.Info("password reset link sent", "user_id", u.ID)
	}

	// Same message for existing and unknown usernames (anti-enumeration).
	h.render(w, r, "forgot_password.html", map[string]any{
		"Title":   "Datey - Reset Password",
		"Success": "If that username exists, a password reset link has been emailed to the configured notification address.",
	})
}

// resetPasswordPage renders the new-password form, carrying the token through
// in a hidden field.
func (h *Handler) resetPasswordPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		h.render(w, r, "reset_password.html", map[string]any{
			"Title": "Datey - Reset Password",
			"Error": "Missing reset token. Request a new password reset link from the login page.",
		})
		return
	}
	h.render(w, r, "reset_password.html", map[string]any{
		"Title": "Datey - Reset Password",
		"Token": token,
	})
}

// resetPasswordPost validates the reset token and sets a new password. A
// successful reset consumes the token and invalidates every existing session
// for the account.
func (h *Handler) resetPasswordPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	token := r.FormValue("token")
	password := r.FormValue("password")
	confirm := r.FormValue("confirm")

	// Resolve the token first; invalid/expired/used tokens all get the same
	// generic error so a leaked link yields nothing.
	tok, err := h.passwordResetTokens.GetByRawToken(r.Context(), token)
	if err != nil {
		slog.Info("reset password: rejected token", "error", err)
		h.render(w, r, "reset_password.html", map[string]any{
			"Title": "Datey - Reset Password",
			"Token": token,
			"Error": "This reset link is invalid, has already been used, or has expired. Please request a new one.",
		})
		return
	}

	u := tok.Edges.User
	if u == nil {
		slog.Error("reset password: token without user", "token_id", tok.ID)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	errors := make(map[string]string)
	if len(password) < 8 {
		errors["password"] = "Password must be at least 8 characters"
	}
	if confirm != password {
		errors["confirm"] = "Passwords do not match"
	}
	if len(errors) > 0 {
		h.render(w, r, "reset_password.html", map[string]any{
			"Title":  "Datey - Reset Password",
			"Token":  token,
			"Errors": errors,
		})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		slog.Error("reset password: hash password", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	if err := h.users.UpdatePassword(r.Context(), u.ID, string(hash)); err != nil {
		slog.Error("reset password: update password", "error", err, "user_id", u.ID)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	// Consume the token so it can never be reused.
	if err := h.passwordResetTokens.MarkUsed(r.Context(), tok.ID); err != nil {
		slog.Warn("reset password: mark token used", "error", err)
	}

	// Invalidate every existing session so old logins are forced out.
	if err := h.sessions.DeleteByUserID(r.Context(), u.ID); err != nil {
		slog.Warn("reset password: delete sessions", "error", err, "user_id", u.ID)
	}

	slog.Info("password reset completed", "user_id", u.ID)
	h.auditRecord(r, "auth.password_reset", u.Username)
	http.Redirect(w, r, "/login?success=Password+reset.+Please+log+in.", http.StatusSeeOther)
}

// resetLink builds the absolute URL for a password reset link. It prefers the
// configured APP_URL and otherwise derives the base from the incoming request.
func (h *Handler) resetLink(r *http.Request, token string) string {
	base := strings.TrimRight(h.cfg.AppURL, "/")
	if base == "" {
		scheme := "http"
		if r.TLS != nil || forwardedProto(r) == "https" {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}

	u := url.URL{Path: "/reset-password"}
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return base + u.String()
}

// forwardedProto returns the X-Forwarded-Proto header value, trimmed of
// surrounding whitespace.
func forwardedProto(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
}

// requestIP extracts the client IP from the request, stripping any port.
func requestIP(r *http.Request) string {
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	return ip
}
