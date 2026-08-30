package web

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/datey/datey/ent"
)

// apiTokenView is the metadata shown in the token management UI. It never
// contains the token secret or its hash — secrets are displayed exactly once,
// immediately after creation or rotation.
type apiTokenView struct {
	ID         int
	Name       string
	CreatedAt  time.Time
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
	LastUsedAt *time.Time
}

func toTokenView(t *ent.ApiToken) apiTokenView {
	return apiTokenView{
		ID:         t.ID,
		Name:       t.Name,
		CreatedAt:  t.CreatedAt,
		ExpiresAt:  t.ExpiresAt,
		RevokedAt:  t.RevokedAt,
		LastUsedAt: t.LastUsedAt,
	}
}

// apiTokensPage renders the API token management page (owner-scoped list).
func (h *Handler) apiTokensPage(w http.ResponseWriter, r *http.Request) {
	h.renderApiTokensPage(w, r, nil, "")
}

// renderApiTokensPage lists the current user's tokens, optionally surfacing a
// one-time secret and/or an error message.
func (h *Handler) renderApiTokensPage(w http.ResponseWriter, r *http.Request, newSecret *string, errMsg string) { //nolint:staticcheck // ST1003: consistent with ApiToken naming
	tokens, err := h.apiTokens.ListByUser(r.Context(), getUserID(r))
	if err != nil {
		slog.Error("api tokens: list", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	views := make([]apiTokenView, 0, len(tokens))
	for _, t := range tokens {
		views = append(views, toTokenView(t))
	}

	data := map[string]any{
		"Title":  "Datey - API Tokens",
		"Tokens": views,
	}
	if errMsg != "" {
		data["Error"] = errMsg
	}
	if newSecret != nil {
		data["NewToken"] = *newSecret
	}
	h.render(w, r, "api_tokens.html", data)
}

// apiTokenCreate provisions a new API token for the current user. The raw
// secret is rendered once on the resulting page; only its hash is stored.
func (h *Handler) apiTokenCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = "api token"
	}
	if len(name) > 100 {
		h.renderApiTokensPage(w, r, nil, "Token name must be at most 100 characters.")
		return
	}

	var expiresAt *time.Time
	if daysStr := strings.TrimSpace(r.FormValue("expiry_days")); daysStr != "" {
		days, err := strconv.Atoi(daysStr)
		if err != nil || days < 1 || days > 3650 {
			h.renderApiTokensPage(w, r, nil, "Expiry must be between 1 and 3650 days.")
			return
		}
		t := time.Now().AddDate(0, 0, days)
		expiresAt = &t
	}

	raw, _, err := h.apiTokens.Create(r.Context(), getUserID(r), name, expiresAt)
	if err != nil {
		slog.Error("api tokens: create", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	h.auditRecord(r, "apitoken.create", name)
	h.renderApiTokensPage(w, r, &raw, "")
}

// apiTokenRevoke marks one of the current user's tokens revoked. Attempts to
// revoke another user's token answer 404 without revealing state.
func (h *Handler) apiTokenRevoke(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		h.renderError(w, r, http.StatusNotFound)
		return
	}

	if err := h.apiTokens.Revoke(r.Context(), id, getUserID(r)); err != nil {
		slog.Warn("api tokens: revoke denied or failed", "error", err, "token_id", id)
		h.renderError(w, r, http.StatusNotFound)
		return
	}

	h.auditRecord(r, "apitoken.revoke", strconv.Itoa(id))
	http.Redirect(w, r, "/settings/api-tokens", http.StatusSeeOther)
}

// apiTokenRotate replaces one of the current user's token secrets with a fresh
// one; the new secret is displayed exactly once. Foreign token IDs answer 404.
func (h *Handler) apiTokenRotate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		h.renderError(w, r, http.StatusNotFound)
		return
	}

	raw, err := h.apiTokens.Rotate(r.Context(), id, getUserID(r), nil)
	if err != nil {
		slog.Warn("api tokens: rotate denied or failed", "error", err, "token_id", id)
		h.renderError(w, r, http.StatusNotFound)
		return
	}

	h.auditRecord(r, "apitoken.rotate", strconv.Itoa(id))
	h.renderApiTokensPage(w, r, &raw, "")
}
