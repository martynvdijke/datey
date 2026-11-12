package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/age"
	"github.com/datey/datey/internal/i18n"
	"github.com/datey/datey/internal/repository"
	"github.com/datey/datey/internal/sharedlink"
	"github.com/go-chi/chi/v5"
)

// Share-link policy. Tokens are stateless and signed with an existing feed
// key, so rotating that key revokes every outstanding link.
const (
	// shareDaysMin / shareDaysMax bound the optional ?days= (or form "days")
	// override on creation.
	shareDaysMin = 1
	shareDaysMax = 90

	// shareHorizonDays is how far ahead the shared page looks: one year plus a
	// day so every annually recurring event has exactly one occurrence.
	shareHorizonDays = 366
)

// shareSecret returns the secret share tokens are signed with. There is no
// dedicated accessor in the feed handlers — each reads its own cfg field — so
// the first configured feed key is reused, in a fixed order. Rotating the iCal
// feed key therefore invalidates share links too.
func (h *Handler) shareSecret() []byte {
	for _, key := range []string{h.cfg.ICalFeedKey, h.cfg.UpcomingAPIKey, h.cfg.RSSFeedKey} {
		if key != "" {
			return []byte(key)
		}
	}
	return nil
}

// shareEventView is one upcoming event as shown on a shared page. It carries
// only what the privacy rules allow: type, date, day count, derived age and
// years-since. Descriptions, notes, gifts and relationships never enter it.
type shareEventView struct {
	Type          string
	Date          string
	DaysRemaining int
	RelativeLabel string
	AgeInfo       age.Info
	YearsSince    int
	HasYears      bool
}

// shareMemberView is one member of a shared group with their upcoming events.
type shareMemberView struct {
	ID     int
	Name   string
	Events []shareEventView
}

// handleShareView serves GET /s/{token}: a public, read-only page for the
// person or group the token names. Invalid, expired and tampered tokens all
// produce the same 404 page so no check is revealed.
func (h *Handler) handleShareView(w http.ResponseWriter, r *http.Request) {
	// A share URL is personal data: never cache it and never let a crawler
	// index or follow it.
	w.Header().Set("Cache-Control", "private, max-age=0, no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	now := time.Now()
	payload, err := sharedlink.NewSigner(h.shareSecret()).Parse(chi.URLParam(r, "token"), now)
	if err != nil {
		h.renderError(w, r, http.StatusNotFound)
		return
	}

	switch payload.Kind {
	case sharedlink.KindPerson:
		h.renderSharedPerson(w, r, payload, now)
	case sharedlink.KindGroup:
		h.renderSharedGroup(w, r, payload, now)
	default:
		h.renderError(w, r, http.StatusNotFound)
	}
}

// renderSharedPerson renders the person page: name plus upcoming events.
func (h *Handler) renderSharedPerson(w http.ResponseWriter, r *http.Request, payload sharedlink.Payload, now time.Time) {
	person, err := h.people.Get(r.Context(), payload.ID)
	if err != nil {
		// Missing person: same 404 as a bad token.
		h.renderError(w, r, http.StatusNotFound)
		return
	}

	occs, err := h.shareOccurrences(r)
	if err != nil {
		slog.Error("share: list upcoming occurrences", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}
	events := h.shareEventViews(now, occs, func(e *ent.Event) bool {
		return e.Edges.Person != nil && e.Edges.Person.ID == person.ID
	})

	h.render(w, r, "share_view.html", map[string]any{
		"Title":   fmt.Sprintf("Datey - %s", person.Name),
		"Kind":    sharedlink.KindPerson,
		"Name":    person.Name,
		"Expires": longDate(h.cfg.DateVariant, payload.Expires),
		"Events":  events,
	})
}

// renderSharedGroup renders the group page: the group name, its members and
// each member's upcoming events. Nobody outside the membership is listed.
func (h *Handler) renderSharedGroup(w http.ResponseWriter, r *http.Request, payload sharedlink.Payload, now time.Time) {
	group, err := h.groups.GetWithRelations(r.Context(), payload.ID)
	if err != nil {
		h.renderError(w, r, http.StatusNotFound)
		return
	}

	occs, err := h.shareOccurrences(r)
	if err != nil {
		slog.Error("share: list upcoming occurrences", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	members := make([]shareMemberView, 0, len(group.Edges.People))
	for _, p := range group.Edges.People {
		id := p.ID
		members = append(members, shareMemberView{
			ID:   id,
			Name: p.Name,
			Events: h.shareEventViews(now, occs, func(e *ent.Event) bool {
				return e.Edges.Person != nil && e.Edges.Person.ID == id
			}),
		})
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Name < members[j].Name })

	h.render(w, r, "share_view.html", map[string]any{
		"Title":   fmt.Sprintf("Datey - %s", group.Name),
		"Kind":    sharedlink.KindGroup,
		"Name":    group.Name,
		"Expires": longDate(h.cfg.DateVariant, payload.Expires),
		"Members": members,
	})
}

// shareOccurrences lists every event occurrence in the share horizon. One
// query feeds both the person and group views.
func (h *Handler) shareOccurrences(r *http.Request) ([]repository.EventOccurrence, error) {
	from := todayStartUTC()
	to := from.AddDate(0, 0, shareHorizonDays)
	return h.events.ListUpcomingOccurrences(r.Context(), from, to)
}

// shareEventViews converts occurrences into display rows, mirroring the
// dashboard's day-count, relative-label and age wording exactly.
func (h *Handler) shareEventViews(now time.Time, occs []repository.EventOccurrence, keep func(*ent.Event) bool) []shareEventView {
	out := make([]shareEventView, 0)
	for _, occ := range occs {
		e := occ.Event
		if e == nil || !keep(e) {
			continue
		}

		days := int(occ.Date.Sub(now).Hours() / 24)
		view := shareEventView{
			Type:          e.Type,
			Date:          shortDate(h.cfg.DateVariant, occ.Date),
			DaysRemaining: days,
		}
		switch {
		case days <= 0:
			view.RelativeLabel = "Today"
		case days == 1:
			view.RelativeLabel = "Tomorrow"
		case days <= 7:
			view.RelativeLabel = fmt.Sprintf("In %d days", days)
		}
		if e.Type == "birthday" {
			view.AgeInfo = shareAgeInfo(e, now)
		}
		if e.Type == "anniversary" && e.Date.Year() > 1 {
			if years := occ.Date.Year() - e.Date.Year(); years >= 0 {
				view.YearsSince = years
				view.HasYears = true
			}
		}
		out = append(out, view)
	}
	return out
}

// shareAgeInfo mirrors the dashboard's birthday age derivation, including the
// lunar-calendar branch.
func shareAgeInfo(e *ent.Event, now time.Time) age.Info {
	if e.CalendarSystem == "lunar" && e.LunarMonth != nil && e.LunarDay != nil {
		if e.Date.Year() <= 1 {
			return age.Info{}
		}
		occ, _ := displayDateForEvent(e, now)
		years := now.Year() - e.Date.Year()
		if dateOnly(occ, time.UTC).After(dateOnly(now, now.Location())) {
			years--
		}
		if years < 0 {
			return age.Info{}
		}
		return age.Info{Current: years, Next: years + 1, HasAge: true}
	}
	return age.InfoFor(e.Date, now)
}

// handleShareCreatePerson serves POST /people/{id}/share (authenticated).
func (h *Handler) handleShareCreatePerson(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		h.renderError(w, r, http.StatusBadRequest)
		return
	}
	if _, err := h.people.Get(r.Context(), id); err != nil {
		h.renderError(w, r, http.StatusNotFound)
		return
	}
	h.createShareLink(w, r, sharedlink.KindPerson, id, "/people/"+strconv.Itoa(id))
}

// handleShareCreateGroup serves POST /groups/{id}/share (authenticated).
func (h *Handler) handleShareCreateGroup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		h.renderError(w, r, http.StatusBadRequest)
		return
	}
	if _, err := h.groups.GetByID(r.Context(), id); err != nil {
		h.renderError(w, r, http.StatusNotFound)
		return
	}
	h.createShareLink(w, r, sharedlink.KindGroup, id, "/groups/"+strconv.Itoa(id))
}

// createShareLink mints a token and either returns the HTMX snippet or
// redirects back to the detail page with a flash.
func (h *Handler) createShareLink(w http.ResponseWriter, r *http.Request, kind string, id int, backPath string) {
	now := time.Now()
	ttl := shareTTLFromRequest(r)

	token, err := sharedlink.NewSigner(h.shareSecret()).Sign(kind, id, ttl, now)
	if err != nil {
		slog.Error("share: sign token", "error", err, "kind", kind, "id", id)
		message := i18n.T(localeFromRequest(r), "share.unavailable")
		if isHTMX(r) {
			h.renderShareSnippet(w, map[string]any{"Error": message})
			return
		}
		http.Redirect(w, r, backPath+"?error="+url.QueryEscape(message), http.StatusSeeOther)
		return
	}

	link := h.absoluteShareURL(r, token)
	expires := longDate(h.cfg.DateVariant, now.Add(ttl))

	if isHTMX(r) {
		h.renderShareSnippet(w, map[string]any{
			"URL":     link,
			"Expires": expires,
		})
		return
	}

	message := i18n.T(localeFromRequest(r), "share.created_flash") + " " + link
	http.Redirect(w, r, backPath+"?success="+url.QueryEscape(message), http.StatusSeeOther)
}

// renderShareSnippet writes the HTMX partial. It is a standalone fragment, not
// wrapped in the base layout.
func (h *Handler) renderShareSnippet(w http.ResponseWriter, data map[string]any) {
	tmpl, ok := h.templates["share_link.html"]
	if !ok {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := tmpl.ExecuteTemplate(w, "share_link.html", data); err != nil {
		slog.Error("share: render snippet", "error", err)
	}
}

// absoluteShareURL builds the link with the configured APP_URL when present,
// otherwise from the request's own origin.
func (h *Handler) absoluteShareURL(r *http.Request, token string) string {
	base := strings.TrimRight(h.cfg.AppURL, "/")
	if base == "" {
		base = feedOrigin(r)
	}
	return base + "/s/" + token
}

// shareTTLFromRequest reads the optional "days" field, clamped to
// [shareDaysMin, shareDaysMax]; absent or unparsable means DefaultTTL.
func shareTTLFromRequest(r *http.Request) time.Duration {
	raw := strings.TrimSpace(r.FormValue("days"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("days"))
	}
	days, err := strconv.Atoi(raw)
	if err != nil {
		return sharedlink.DefaultTTL
	}
	if days < shareDaysMin {
		days = shareDaysMin
	}
	if days > shareDaysMax {
		days = shareDaysMax
	}
	return time.Duration(days) * 24 * time.Hour
}

// isHTMX reports whether the request was issued by HTMX.
func isHTMX(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("HX-Request"), "true")
}
