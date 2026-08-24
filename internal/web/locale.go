package web

import (
	"net/http"

	"github.com/datey/datey/internal/i18n"
)

// Locale middleware resolves locale per request: user pref > Accept-Language > en.
func (h *Handler) Locale(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userLocale := ""
		if u := UserFromContext(r.Context()); u != nil && u.Locale != nil {
			userLocale = *u.Locale
		}
		locale := i18n.ResolveLocale(userLocale, r.Header.Get("Accept-Language"))
		ctx := i18n.WithLocale(r.Context(), locale)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
