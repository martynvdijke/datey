package web

import (
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/rss"
)

// rssFeedAccessOK reports whether the public RSS feed is enabled and the
// request carries the correct secret key. Like the iCal feed, failures are
// indistinguishable (404) so the endpoint's existence stays hidden.
func (h *Handler) rssFeedAccessOK(r *http.Request) bool {
	if !h.cfg.RSSEnabled || h.cfg.RSSFeedKey == "" {
		return false
	}
	given := r.URL.Query().Get("key")
	return subtle.ConstantTimeCompare([]byte(given), []byte(h.cfg.RSSFeedKey)) == 1
}

// rssFeed serves the upcoming events within the reminder window as an RSS
// 2.0 document for feed readers and aggregators.
func (h *Handler) rssFeed(w http.ResponseWriter, r *http.Request) {
	if !h.rssFeedAccessOK(r) {
		h.renderError(w, r, http.StatusNotFound)
		return
	}

	now := time.Now()
	occurrences, err := h.events.ListUpcomingOccurrences(r.Context(), todayStartUTC(), now.AddDate(0, 0, h.cfg.ReminderDays))
	if err != nil {
		slog.Error("rss: list upcoming events", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	items := make([]rss.Item, 0, len(occurrences))
	for _, occ := range occurrences {
		items = append(items, h.rssItem(occ.Event, occ.Date, now, r))
	}

	origin := feedOrigin(r)
	body, err := rss.Render(rss.Channel{
		Title:       "Datey — Upcoming events",
		Link:        origin,
		Description: "Upcoming events tracked in Datey.",
		Language:    "en",
		TTL:         360,
	}, items, now)
	if err != nil {
		slog.Error("rss: render feed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if _, err := w.Write(body); err != nil {
		slog.Error("rss: write response", "error", err)
	}
}

// rssItem converts a stored event into an RSS item, mirroring the dashboard's
// day-count logic against the occurrence date. Title format: "Birthday —
// Dana in 3 days".
func (h *Handler) rssItem(e *ent.Event, date, now time.Time, r *http.Request) rss.Item {
	name := eventOwnerName(e)

	days := int(date.Sub(now).Hours() / 24)
	var relative string
	switch {
	case days <= 0:
		relative = "today"
	case days == 1:
		relative = "tomorrow"
	default:
		relative = fmt.Sprintf("in %d days", days)
	}

	link := feedOrigin(r)
	if url := eventOwnerURL(e); url != "" {
		link += url
	}

	return rss.Item{
		Title: titleCase(e.Type) + " — " + name + " " + relative,
		Link:  link,
	}
}

// titleCase uppercases the first rune of s (e.g. "birthday" → "Birthday").
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// feedOrigin builds the absolute origin (scheme + host) for feed links from
// the request. Links point at the same origin the feed was fetched from.
func feedOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
