package web

import (
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/datey/datey/internal/db"
	"github.com/datey/datey/internal/i18n"
	"github.com/datey/datey/internal/search"
)

// searchMinQueryRunes is the shortest query that is worth running. A single
// character matches almost everything and costs a full index scan.
const searchMinQueryRunes = 2

// searchDefaults is a render-time fallback for the page's i18n keys so the
// page is readable even before the catalog entries land in
// internal/i18n/locales/*.json. i18n.T still wins as soon as an entry exists.
var searchDefaults = map[string]string{
	"search.title":              "Search",
	"search.label":              "Search everything",
	"search.placeholder":        "Search people, notes, gifts, events…",
	"search.nav_placeholder":    "Search people, notes, gifts…",
	"search.submit":             "Search",
	"search.hint":               "Search across people, notes, gift ideas, events and groups.",
	"search.unavailable":        "Full-text search is unavailable right now. Please try again later.",
	"search.no_results":         "No matches found.",
	"search.summary":            "%d result(s) for “%s”",
	"search.group_people":       "People",
	"search.group_person_notes": "Notes",
	"search.group_gift_ideas":   "Gift ideas",
	"search.group_events":       "Events",
	"search.group_groups":       "Groups",
	"search.group_group_notes":  "Group notes",
	"search.results_heading":    "Results",
}

// searchGroupTitleKeys maps an index entity to the i18n key of its section
// heading.
var searchGroupTitleKeys = map[search.Entity]string{
	search.EntityPerson:     "search.group_people",
	search.EntityPersonNote: "search.group_person_notes",
	search.EntityGiftIdea:   "search.group_gift_ideas",
	search.EntityEvent:      "search.group_events",
	search.EntityGroup:      "search.group_groups",
	search.EntityGroupNote:  "search.group_group_notes",
}

// searchResultView is one rendered row.
type searchResultView struct {
	URL     string
	Label   string
	Snippet template.HTML
}

// searchGroupView is one rendered section.
type searchGroupView struct {
	Entity   string
	TitleKey string
	Count    int
	Results  []searchResultView
}

// searchView is the whole page payload.
type searchView struct {
	Query       string
	Searched    bool
	Unavailable bool
	Total       int
	Summary     string
	Groups      []searchGroupView
}

// fields flattens the view for the page render, which merges it into the base
// template data (a map).
func (v searchView) fields() map[string]any {
	return map[string]any{
		"Query":       v.Query,
		"Searched":    v.Searched,
		"Unavailable": v.Unavailable,
		"Total":       v.Total,
		"Summary":     v.Summary,
		"Groups":      v.Groups,
	}
}

var (
	searchTemplateOnce sync.Once
	searchTemplate     *template.Template
	searchTemplateErr  error
)

// searchTemplateLoad parses the search page together with the base layout. The
// page stays out of loadTemplates so the feature touches no shared template
// registry.
func searchTemplateLoad() (*template.Template, error) {
	searchTemplateOnce.Do(func() {
		searchTemplate, searchTemplateErr = template.New("base.html").
			Funcs(funcMap).
			ParseFS(templateFS, "templates/base.html", "templates/search.html")
	})
	return searchTemplate, searchTemplateErr
}

// searchTranslator returns a per-request T that falls back to the built-in
// English copy above when a catalog entry is missing.
func searchTranslator(locale string) func(string) string {
	return func(key string) string {
		if v := i18n.T(locale, key); v != key {
			return v
		}
		if v, ok := searchDefaults[key]; ok {
			return v
		}
		return key
	}
}

// renderSearch renders the page. When the request came from HTMX only the
// content block is returned, which is what the navbar search box swaps into
// #main-content; a plain navigation gets the full document.
func (h *Handler) renderSearch(w http.ResponseWriter, r *http.Request, view searchView) {
	tmpl, err := searchTemplateLoad()
	if err != nil {
		slog.Error("search: load template", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	cloned, err := tmpl.Clone()
	if err != nil {
		slog.Error("search: clone template", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	cloned.Funcs(template.FuncMap{"T": searchTranslator(i18n.LocaleFromContext(r.Context()))})

	const title = "Datey - Search"
	data := h.baseData(r, title)
	maps.Copy(data, view.fields())
	data["Title"] = title
	// autofocus only makes sense on a real page load; an HTMX swap must not
	// steal focus from the navbar search box.
	data["HXRequest"] = r.Header.Get("HX-Request") == "true"

	// The response body differs by request header, so tell caches.
	w.Header().Add("Vary", "HX-Request")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Progressive enhancement: HTMX asks for just the content fragment.
	if r.Header.Get("HX-Request") == "true" {
		if err := cloned.ExecuteTemplate(w, "content", data); err != nil {
			slog.Error("search: render partial", "error", err)
		}
		return
	}
	if err := cloned.ExecuteTemplate(w, "base.html", data); err != nil {
		slog.Error("search: render page", "error", err)
	}
}

// handleSearch serves GET /search?q=… across every indexed entity type.
func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	locale := i18n.LocaleFromContext(ctx)
	t := searchTranslator(locale)

	view := searchView{Query: query}
	if utf8.RuneCountInString(query) >= searchMinQueryRunes {
		view.Searched = true
		conn, ok := db.SQLDB(h.client)
		if !ok {
			// No SQL handle (e.g. a client built outside db.Init): degrade
			// rather than fail.
			slog.Warn("search: no SQL handle registered for the ent client")
			view.Unavailable = true
		} else {
			groups, err := search.New(conn).Search(ctx, query)
			switch {
			case errors.Is(err, search.ErrIndexUnavailable):
				slog.Warn("search: index unavailable", "error", err)
				view.Unavailable = true
			case err != nil:
				slog.Error("search: query failed", "error", err)
				h.renderError(w, r, http.StatusInternalServerError)
				return
			default:
				view.Groups = buildSearchGroups(groups)
				for _, g := range view.Groups {
					view.Total += g.Count
				}
				view.Summary = fmt.Sprintf(t("search.summary"), view.Total, query)
			}
		}
	}

	h.renderSearch(w, r, view)
}

// buildSearchGroups turns the query-layer groups into render data, composing a
// human label out of the document title and its owner ("Alice Smith ·
// birthday", or just the person for a note).
func buildSearchGroups(groups []search.Group) []searchGroupView {
	out := make([]searchGroupView, 0, len(groups))
	for _, g := range groups {
		titleKey, ok := searchGroupTitleKeys[g.Entity]
		if !ok {
			titleKey = "search.results_heading"
		}
		rows := make([]searchResultView, 0, len(g.Results))
		for _, res := range g.Results {
			rows = append(rows, searchResultView{
				URL:     res.URL,
				Label:   searchResultLabel(res),
				Snippet: res.Snippet,
			})
		}
		out = append(out, searchGroupView{
			Entity:   string(g.Entity),
			TitleKey: titleKey,
			Count:    len(rows),
			Results:  rows,
		})
	}
	return out
}

// searchResultLabel combines the document title with the owner's name.
func searchResultLabel(res search.Result) string {
	title := strings.TrimSpace(res.Title)
	owner := strings.TrimSpace(res.ParentTitle)
	switch {
	case owner == "":
		return title
	case title == "":
		return owner
	case strings.EqualFold(title, owner):
		return owner
	default:
		return owner + " · " + title
	}
}
