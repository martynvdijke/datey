package web

import (
	"net/http"

	"github.com/datey/datey/internal/i18n"
)

func (h *Handler) settingsLocaleSave(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	locale := r.FormValue("locale")
	// Allow empty = clear to fallback to Accept-Language/en
	if locale != "" {
		norm := i18n.NormalizeLocale(locale)
		if norm == "" || !i18n.Supported(norm) {
			http.Error(w, "unsupported locale", http.StatusBadRequest)
			return
		}
		locale = norm
	}
	if err := h.users.SetLocale(r.Context(), u.ID, locale); err != nil {
		http.Error(w, "failed to save locale", http.StatusInternalServerError)
		return
	}
	// Update context for subsequent rendering? Client will reload; set toast and refresh
	toastHeader(w, "Language saved", "success")
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}
