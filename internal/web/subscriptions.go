package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/datey/datey/internal/i18n"
	"github.com/datey/datey/internal/icsub"
	"github.com/datey/datey/internal/repository"
)

// subscriptionsPage renders the admin calendar-subscription list.
func (h *Handler) subscriptionsPage(w http.ResponseWriter, r *http.Request) {
	h.renderSubscriptions(w, r, "", "", "", true)
}

// renderSubscriptions lists subscriptions and optionally surfaces a validation
// error together with the values submitted in the add form.
func (h *Handler) renderSubscriptions(w http.ResponseWriter, r *http.Request, errMsg, formName, formURL string, formEnabled bool) {
	subs, err := repository.NewSubscriptionRepository(h.client).List(r.Context())
	if err != nil {
		slog.Error("subscriptions: list", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	data := map[string]any{
		"Title":         i18n.T(localeFromRequest(r), "subscriptions.title"),
		"Subscriptions": subs,
		"FormName":      formName,
		"FormURL":       formURL,
		"FormEnabled":   formEnabled,
	}
	if errMsg != "" {
		data["Error"] = errMsg
	}
	h.render(w, r, "subscriptions.html", data)
}

// subscriptionCreate adds a subscription from the add form.
func (h *Handler) subscriptionCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	feedURL := strings.TrimSpace(r.FormValue("url"))
	enabled := r.FormValue("enabled") == "on"

	if errMsg := h.validateSubscriptionForm(localeFromRequest(r), name, feedURL); errMsg != "" {
		h.renderSubscriptions(w, r, errMsg, name, feedURL, enabled)
		return
	}

	if _, err := repository.NewSubscriptionRepository(h.client).
		Create(r.Context(), name, feedURL, enabled); err != nil {
		slog.Error("subscriptions: create", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}
	h.auditRecord(r, "subscription.create", name)
	h.redirectSubscriptions(w, r, "success", "subscriptions.created")
}

// subscriptionUpdate saves the editable fields of one subscription.
func (h *Handler) subscriptionUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := subscriptionID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	feedURL := strings.TrimSpace(r.FormValue("url"))
	enabled := r.FormValue("enabled") == "on"

	if errMsg := h.validateSubscriptionForm(localeFromRequest(r), name, feedURL); errMsg != "" {
		h.renderSubscriptions(w, r, errMsg, name, feedURL, enabled)
		return
	}
	if _, err := repository.NewSubscriptionRepository(h.client).
		Update(r.Context(), id, name, feedURL, enabled); err != nil {
		slog.Error("subscriptions: update", "error", err, "subscription", id)
		h.renderError(w, r, http.StatusNotFound)
		return
	}
	h.auditRecord(r, "subscription.update", strconv.Itoa(id))
	h.redirectSubscriptions(w, r, "success", "subscriptions.saved")
}

// subscriptionToggle flips the enabled flag of one subscription.
func (h *Handler) subscriptionToggle(w http.ResponseWriter, r *http.Request) {
	id, ok := subscriptionID(w, r)
	if !ok {
		return
	}
	repo := repository.NewSubscriptionRepository(h.client)
	sub, err := repo.Get(r.Context(), id)
	if err != nil {
		h.renderError(w, r, http.StatusNotFound)
		return
	}
	if _, err := repo.Update(r.Context(), id, sub.Name, sub.URL, !sub.Enabled); err != nil {
		slog.Error("subscriptions: toggle", "error", err, "subscription", id)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}
	h.auditRecord(r, "subscription.toggle", strconv.Itoa(id))
	key := "subscriptions.disabled_done"
	if !sub.Enabled {
		key = "subscriptions.enabled_done"
	}
	h.redirectSubscriptions(w, r, "success", key)
}

// subscriptionRefresh runs the sync engine for one subscription and flashes
// the created/updated/deleted counts.
func (h *Handler) subscriptionRefresh(w http.ResponseWriter, r *http.Request) {
	id, ok := subscriptionID(w, r)
	if !ok {
		return
	}
	summary, err := icsub.NewSyncer(h.client, nil).SyncID(r.Context(), id)
	if err != nil {
		slog.Warn("subscriptions: refresh failed", "error", err, "subscription", id)
		h.redirectSubscriptions(w, r, "error", "subscriptions.refresh_failed", err.Error())
		return
	}
	h.auditRecord(r, "subscription.refresh", strconv.Itoa(id))
	h.redirectSubscriptions(w, r, "success", "subscriptions.refresh_result",
		summary.Created, summary.Updated, summary.Deleted)
}

// subscriptionDelete removes a subscription; its events are detached, not
// deleted.
func (h *Handler) subscriptionDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := subscriptionID(w, r)
	if !ok {
		return
	}
	if err := repository.NewSubscriptionRepository(h.client).Delete(r.Context(), id); err != nil {
		slog.Error("subscriptions: delete", "error", err, "subscription", id)
		h.renderError(w, r, http.StatusNotFound)
		return
	}
	h.auditRecord(r, "subscription.delete", strconv.Itoa(id))
	h.redirectSubscriptions(w, r, "success", "subscriptions.deleted")
}

// validateSubscriptionForm returns a translated error message, or "" when the
// form is acceptable. The feed must be an absolute http(s) URL.
func (h *Handler) validateSubscriptionForm(locale, name, feedURL string) string {
	if name == "" || feedURL == "" {
		return i18n.T(locale, "subscriptions.error_name_url")
	}
	u, err := url.Parse(feedURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return i18n.T(locale, "subscriptions.error_url")
	}
	return ""
}

// redirectSubscriptions flashes a translated message through the query string
// (baseData renders success/error alerts) and returns to the list.
func (h *Handler) redirectSubscriptions(w http.ResponseWriter, r *http.Request, kind, key string, args ...any) {
	msg := i18n.T(localeFromRequest(r), key)
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	http.Redirect(w, r, "/settings/subscriptions?"+kind+"="+url.QueryEscape(msg), http.StatusSeeOther)
}

// subscriptionID parses the {id} path parameter, writing 404 on failure.
func subscriptionID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return 0, false
	}
	return id, true
}
