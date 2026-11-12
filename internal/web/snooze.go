package web

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/datey/datey/internal/i18n"
	"github.com/datey/datey/internal/reminderaction"
	"github.com/datey/datey/internal/repository"
	"github.com/go-chi/chi/v5"
)

// Reminder action policy. The token itself is the authentication: it names one
// occurrence, one recipient and one action, so these pages sit outside the
// authenticated group.
const (
	// actionSnoozeDaysMin / actionSnoozeDaysMax bound the "days" value a
	// recipient may pick; actionSnoozeDaysDefault is used when none is sent.
	actionSnoozeDaysMin     = 1
	actionSnoozeDaysMax     = 30
	actionSnoozeDaysDefault = 3

	// actionAckHoldDays is how long an acknowledgement keeps an occurrence
	// quiet. The row is keyed by (event, occasion, user), so it can never
	// suppress a *different* occurrence of the same event; the value only has
	// to outlast the reminder window for this one occurrence.
	actionAckHoldDays = 400
)

// actionSecret returns the secret action tokens are signed with. Like share
// links there is no dedicated field, so the first configured feed key is
// reused in a fixed order. Rotating that key invalidates every outstanding
// action token (a reminder still in an inbox stops working) — acceptable, and
// the same revocation behaviour share links already have.
func (h *Handler) actionSecret() []byte {
	for _, key := range []string{h.cfg.ICalFeedKey, h.cfg.UpcomingAPIKey, h.cfg.RSSFeedKey} {
		if key != "" {
			return []byte(key)
		}
	}
	return nil
}

// snoozes returns the snooze repository. It is a thin wrapper around the ent
// client, so no Handler field or constructor wiring is needed.
func (h *Handler) snoozes() *repository.SnoozeRepository {
	return repository.NewSnoozeRepository(h.client)
}

// handleReminderAction serves GET /a/{token}: a small confirmation page for the
// action the token names. A bad, expired or tampered token is a plain 404, so
// nothing about the event or the person can be probed.
func (h *Handler) handleReminderAction(w http.ResponseWriter, r *http.Request) {
	h.prepareActionResponse(w)

	action, ok := h.parseReminderAction(w, r)
	if !ok {
		return
	}

	h.render(w, r, "action_confirm.html", map[string]any{
		"Title":    i18n.T(localeFromRequest(r), "reminder_action.title"),
		"Kind":     action.Kind(),
		"IsAck":    action.IsAck(),
		"IsSnooze": action.IsSnooze(),
		"Occasion": longDate(h.cfg.DateVariant, action.Occasion()),
		"Days":     clampActionDays(r.URL.Query().Get("days")),
		"DaysMax":  actionSnoozeDaysMax,
		"Done":     false,
	})
}

// handleReminderActionSubmit serves POST /a/{token}: it records the action and
// renders the same page in its "done" state. Re-posting is idempotent — the
// snooze row is upserted, so a double click or a mail-client prefetch cannot
// fail or duplicate anything.
func (h *Handler) handleReminderActionSubmit(w http.ResponseWriter, r *http.Request) {
	h.prepareActionResponse(w)

	action, ok := h.parseReminderAction(w, r)
	if !ok {
		return
	}

	now := time.Now()
	var until time.Time
	days := actionSnoozeDaysDefault
	if action.IsAck() {
		until = action.Occasion().AddDate(0, 0, actionAckHoldDays)
	} else {
		days = clampActionDays(actionDaysFromRequest(r))
		until = now.AddDate(0, 0, days)
	}

	if err := h.snoozes().Upsert(r.Context(), action.EventID(), action.Occasion(), action.UserID(), until); err != nil {
		// The token itself is never logged.
		slog.Error("reminder action: record", "error", err, "action", action.Kind(), "event", action.EventID())
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	h.render(w, r, "action_confirm.html", map[string]any{
		"Title":    i18n.T(localeFromRequest(r), "reminder_action.done_title"),
		"Kind":     action.Kind(),
		"IsAck":    action.IsAck(),
		"IsSnooze": action.IsSnooze(),
		"Occasion": longDate(h.cfg.DateVariant, action.Occasion()),
		"Days":     days,
		"DaysMax":  actionSnoozeDaysMax,
		"Done":     true,
	})
}

// prepareActionResponse sets the headers every action response carries: the
// URL contains a bearer-like token, so it must not be cached, indexed or leak
// through the Referer header.
func (h *Handler) prepareActionResponse(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, max-age=0, no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// parseReminderAction verifies the token in the path. On failure it writes the
// 404 response and reports false; the caller must then stop.
func (h *Handler) parseReminderAction(w http.ResponseWriter, r *http.Request) (reminderaction.Action, bool) {
	action, err := reminderaction.Parse(h.actionSecret(), chi.URLParam(r, "token"))
	if err != nil {
		h.renderError(w, r, http.StatusNotFound)
		return reminderaction.Action{}, false
	}
	return action, true
}

// actionDaysFromRequest reads the optional "days" form field, falling back to
// the query string for a link that already carries a choice.
func actionDaysFromRequest(r *http.Request) string {
	if raw := strings.TrimSpace(r.FormValue("days")); raw != "" {
		return raw
	}
	return strings.TrimSpace(r.URL.Query().Get("days"))
}

// clampActionDays parses a day count and bounds it to
// [actionSnoozeDaysMin, actionSnoozeDaysMax]; anything unparsable becomes the
// default. Never fails, so a nonsense value cannot turn into an error page.
func clampActionDays(raw string) int {
	days, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return actionSnoozeDaysDefault
	}
	if days < actionSnoozeDaysMin {
		days = actionSnoozeDaysMin
	}
	if days > actionSnoozeDaysMax {
		days = actionSnoozeDaysMax
	}
	return days
}
