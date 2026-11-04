package web

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/datey/datey/internal/account"
	"github.com/datey/datey/internal/session"
)

// LegalData is the identity of the instance operator plus the deployment facts
// the published policy is allowed to assert. Every field is either
// configuration or read from the notifier registry, so the text cannot claim a
// processor the running server does not use.
type LegalData struct {
	Controller     string
	ContactEmail   string
	ContactAddress string
	HostingRegion  string
	BaseURL        string
	Year           int
	SessionCookie  string
	Analytics      bool
	AnalyticsURL   string
	Traces         bool
	Mail           bool
	Push           bool
	OIDC           bool
	Google         bool
	Processors     []string
}

// legal collects the policy facts once per request.
func (h *Handler) legal() LegalData {
	d := LegalData{
		Controller:    "the operator of this Datey instance",
		HostingRegion: "the European Union",
		Year:          time.Now().Year(),
		SessionCookie: session.CookieName,
		Processors:    []string{},
	}
	if h.cfg != nil {
		d.BaseURL = h.cfg.AppURL
		if v := strings.TrimSpace(h.cfg.LegalControllerName); v != "" {
			d.Controller = v
		}
		d.ContactEmail = strings.TrimSpace(h.cfg.LegalContactEmail)
		d.ContactAddress = strings.TrimSpace(h.cfg.LegalContactAddress)
		if v := strings.TrimSpace(h.cfg.LegalHostingRegion); v != "" {
			d.HostingRegion = v
		}
		d.Analytics = h.cfg.UmamiURL != ""
		d.AnalyticsURL = h.cfg.UmamiURL
		d.Traces = h.cfg.OTLPEndpoint != ""
		d.Mail = h.cfg.SMTPHost != ""
		d.OIDC = h.cfg.OIDCEnabled
		d.Google = h.cfg.GoogleContactsEnabled
	}
	if h.notifReg != nil {
		names := h.notifReg.ConfiguredNames()
		sort.Strings(names)
		labels := map[string]string{
			"email":    "Email (SMTP)",
			"gotify":   "Gotify",
			"telegram": "Telegram",
			"ntfy":     "ntfy",
			"webhook":  "Webhook",
			"discord":  "Discord",
			"slack":    "Slack",
			"matrix":   "Matrix",
			"webpush":  "Web Push",
		}
		for _, n := range names {
			label := labels[n]
			if label == "" {
				label = n
			}
			d.Processors = append(d.Processors, label)
		}
		d.Push = h.notifReg.IsConfigured("webpush")
	}
	return d
}

// privacyPage serves the public privacy policy. It stays reachable without a
// session because both app stores and data-protection law require the policy
// to be readable before anyone signs up.
func (h *Handler) privacyPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "privacy.html", map[string]any{
		"Title":           "Privacy",
		"Legal":           h.legal(),
		"ActiveNav":       "",
		"MetaDescription": "What Datey stores, why, who processes it, and how to get it back or delete it.",
	})
}

// termsPage serves the acceptable-use terms for the same reason.
func (h *Handler) termsPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "terms.html", map[string]any{
		"Title":           "Terms",
		"Legal":           h.legal(),
		"ActiveNav":       "",
		"MetaDescription": "The rules for using this Datey instance, and what you keep ownership of.",
	})
}

// accountDeletePage is the deletion route Google Play asks every app with
// accounts to publish. It is readable while signed out, because a store
// review and a user who cannot get back into their account both need to reach
// it without a session. Signed-in members also get the self-service form.
func (h *Handler) accountDeletePage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "account_delete.html", map[string]any{
		"Title":           "Delete your account",
		"Legal":           h.legal(),
		"ActiveNav":       "",
		"MetaDescription": "How to delete a Datey account and what happens to the data afterwards.",
	})
}

// accountDeleteSelf is the authenticated web form behind /account/delete.
// Confirmation must be the member's exact username or the literal DELETE.
func (h *Handler) accountDeleteSelf(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	confirm := strings.TrimSpace(r.FormValue("confirm"))
	if confirm != u.Username && confirm != "DELETE" {
		http.Redirect(w, r, "/account/delete?error=Type+your+username+or+DELETE+to+confirm", http.StatusSeeOther)
		return
	}
	if h.accounts == nil {
		http.Error(w, "account deletion unavailable", http.StatusInternalServerError)
		return
	}
	if err := h.accounts.Delete(r.Context(), u.ID); err != nil {
		switch err {
		case account.ErrLastAdmin:
			http.Redirect(w, r, "/account/delete?error=The+last+admin+cannot+delete+their+account", http.StatusSeeOther)
		case account.ErrNotFound:
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		default:
			http.Error(w, "could not delete account", http.StatusInternalServerError)
		}
		return
	}
	session.ClearCookie(w)
	h.auditRecord(r, "account.delete_self", u.Username)
	http.Redirect(w, r, "/login?success=Your+account+has+been+deleted", http.StatusSeeOther)
}

// apiDeleteMe closes the authenticated account through the JSON API. Bearer
// token and session-cookie clients use the same service and confirmation rule
// as the web form.
func (h *Handler) apiDeleteMe(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeAPIError(w, http.StatusUnauthorized, "unauthorized", "Not authenticated")
		return
	}
	var body struct {
		Confirm string `json:"confirm"`
	}
	// An absent body is allowed only so the handler can return a clear
	// validation error instead of a JSON decoder noise message.
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	confirm := strings.TrimSpace(body.Confirm)
	if confirm != u.Username && confirm != "DELETE" {
		writeAPIError(w, http.StatusBadRequest, "confirmation_required", "Send {\"confirm\":\"<username>\"} or {\"confirm\":\"DELETE\"}")
		return
	}
	if h.accounts == nil {
		writeAPIError(w, http.StatusInternalServerError, "internal", "Account deletion is not configured")
		return
	}
	if err := h.accounts.Delete(r.Context(), u.ID); err != nil {
		switch err {
		case account.ErrLastAdmin:
			writeAPIError(w, http.StatusConflict, "last_admin", "The last admin cannot be deleted")
		case account.ErrNotFound:
			writeAPIError(w, http.StatusNotFound, "not_found", "Account not found")
		default:
			writeAPIError(w, http.StatusInternalServerError, "internal", "Could not delete account")
		}
		return
	}
	session.ClearCookie(w)
	h.auditRecord(r, "account.delete_self", u.Username)
	w.WriteHeader(http.StatusNoContent)
}

// apiExportMe returns the requester's attributable data as JSON.
func (h *Handler) apiExportMe(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		writeAPIError(w, http.StatusUnauthorized, "unauthorized", "Not authenticated")
		return
	}
	if h.accounts == nil {
		writeAPIError(w, http.StatusInternalServerError, "internal", "Account export is not configured")
		return
	}
	export, err := h.accounts.Export(r.Context(), u.ID)
	if err != nil {
		if err == account.ErrNotFound {
			writeAPIError(w, http.StatusNotFound, "not_found", "Account not found")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "internal", "Could not build export")
		return
	}
	writeAPIJSON(w, http.StatusOK, export)
}
