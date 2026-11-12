package web

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"maps"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/giftidea"
	"github.com/datey/datey/internal/age"
	"github.com/datey/datey/internal/i18n"
)

// dqBirthdayWindowDays is how far ahead the "birthday soon" section looks.
const dqBirthdayWindowDays = 45

// dataQualityDefaults is a render-time fallback for the page's i18n keys so it
// is readable before the keys land in internal/i18n/locales/en.json. i18n.T
// still wins as soon as a catalog entry exists. Keys prefixed with "summary_"
// are fmt templates: the handler formats them so the numbers are localizable.
var dataQualityDefaults = map[string]string{
	"data_quality.title":                      "Data quality",
	"data_quality.subtitle":                   "Address-book hygiene problems worth a look. This page is read-only: open a person to fix an issue.",
	"data_quality.back":                       "Back to people",
	"data_quality.missing_birth_year":         "Missing birth year",
	"data_quality.missing_photo":              "Missing photo",
	"data_quality.birthday_soon":              "Birthday soon without a gift idea",
	"data_quality.duplicates":                 "Possible duplicates",
	"data_quality.col_person":                 "Person",
	"data_quality.col_detail":                 "Detail",
	"data_quality.col_names":                  "Matching names",
	"data_quality.no_year":                    "no year",
	"data_quality.no_photo":                   "No local photo and no Immich person linked",
	"data_quality.today":                      "today",
	"data_quality.tomorrow":                   "tomorrow",
	"data_quality.in_days":                    "in %d days",
	"data_quality.empty_missing_birth_year":   "Everyone with a birthday has a birth year.",
	"data_quality.empty_missing_photo":        "Everyone has a photo.",
	"data_quality.empty_birthday_soon":        "No upcoming birthday is missing a gift idea.",
	"data_quality.empty_duplicates":           "No names look duplicated.",
	"data_quality.summary_missing_birth_year": "%d of %d people have no birth year",
	"data_quality.summary_missing_photo":      "%d of %d people have no photo",
	"data_quality.summary_birthday_soon":      "%d of %d people have a birthday within %d days and no gift idea",
	"data_quality.summary_duplicates":         "%d possible duplicate group(s)",
}

// dataQualityPersonView is one affected person with a short explanation.
type dataQualityPersonView struct {
	ID     int
	Name   string
	Detail string
}

// dataQualityDuplicateView is one cluster of people whose normalized names
// collide. Each cluster renders as a single row listing every member.
type dataQualityDuplicateView struct {
	Members []dataQualityPersonView
}

// dataQualityView is the whole page payload.
type dataQualityView struct {
	Total                   int
	WindowDays              int
	MissingBirthYear        []dataQualityPersonView
	MissingPhoto            []dataQualityPersonView
	BirthdaySoonNoGift      []dataQualityPersonView
	Duplicates              []dataQualityDuplicateView
	SummaryMissingBirthYear string
	SummaryMissingPhoto     string
	SummaryBirthdaySoon     string
	SummaryDuplicates       string
}

// fields flattens the view for the page render, which merges it into the base
// template data (a map).
func (v dataQualityView) fields() map[string]any {
	return map[string]any{
		"Total":                   v.Total,
		"WindowDays":              v.WindowDays,
		"MissingBirthYear":        v.MissingBirthYear,
		"MissingPhoto":            v.MissingPhoto,
		"BirthdaySoonNoGift":      v.BirthdaySoonNoGift,
		"Duplicates":              v.Duplicates,
		"SummaryMissingBirthYear": v.SummaryMissingBirthYear,
		"SummaryMissingPhoto":     v.SummaryMissingPhoto,
		"SummaryBirthdaySoon":     v.SummaryBirthdaySoon,
		"SummaryDuplicates":       v.SummaryDuplicates,
	}
}

var (
	dqTemplateOnce sync.Once
	dqTemplate     *template.Template
	dqTemplateErr  error
)

// dqTemplateLoad parses the data-quality page together with the base layout.
// The page stays out of loadTemplates so the feature touches no shared
// template registry.
func dqTemplateLoad() (*template.Template, error) {
	dqTemplateOnce.Do(func() {
		dqTemplate, dqTemplateErr = template.New("base.html").
			Funcs(funcMap).
			ParseFS(templateFS, "templates/base.html", "templates/data_quality.html")
	})
	return dqTemplate, dqTemplateErr
}

// dqTranslator returns a per-request T that falls back to the built-in English
// copy above when a catalog entry is missing.
func dqTranslator(locale string) func(string) string {
	return func(key string) string {
		if v := i18n.T(locale, key); v != key {
			return v
		}
		if v, ok := dataQualityDefaults[key]; ok {
			return v
		}
		return key
	}
}

// renderDataQuality renders the full page. It is a plain GET so HTMX clients
// that follow the link get the same document as a normal navigation.
func (h *Handler) renderDataQuality(w http.ResponseWriter, r *http.Request, view dataQualityView) {
	tmpl, err := dqTemplateLoad()
	if err != nil {
		slog.Error("data quality: load template", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	cloned, err := tmpl.Clone()
	if err != nil {
		slog.Error("data quality: clone template", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	cloned.Funcs(template.FuncMap{"T": dqTranslator(i18n.LocaleFromContext(r.Context()))})

	const title = "Datey - Data quality"
	data := h.baseData(r, title)
	maps.Copy(data, view.fields())
	data["Title"] = title

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := cloned.ExecuteTemplate(w, "base.html", data); err != nil {
		slog.Error("data quality: render page", "error", err)
	}
}

// handleDataQuality builds the four read-only hygiene sections and renders
// them. Nothing is written here.
func (h *Handler) handleDataQuality(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now()
	today := todayStartUTC()
	locale := i18n.LocaleFromContext(ctx)
	t := dqTranslator(locale)

	people, err := h.people.List(ctx)
	if err != nil {
		slog.Error("data quality: list people", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	view := dataQualityView{Total: len(people), WindowDays: dqBirthdayWindowDays}
	duplicateKeys := make(map[string][]dataQualityPersonView)

	for _, p := range people {
		viewEntry := dataQualityPersonView{ID: p.ID, Name: p.Name}

		birthdays := dqBirthdayEvents(p)
		if len(birthdays) > 0 && !dqHasBirthYear(birthdays, now) {
			entry := viewEntry
			entry.Detail = fmt.Sprintf("%s (%s)", shortDate(h.cfg.DateVariant, birthdays[0].Date), t("data_quality.no_year"))
			view.MissingBirthYear = append(view.MissingBirthYear, entry)
		}

		if dqMissingPhoto(p) {
			entry := viewEntry
			entry.Detail = t("data_quality.no_photo")
			view.MissingPhoto = append(view.MissingPhoto, entry)
		}

		if occ, days, ok := dqUpcomingBirthdayWithin(birthdays, today, dqBirthdayWindowDays); ok && !dqHasGiftIdea(ctx, h, p.ID) {
			entry := viewEntry
			entry.Detail = fmt.Sprintf("%s · %s", shortDate(h.cfg.DateVariant, occ), dqRelativeDays(t, days))
			view.BirthdaySoonNoGift = append(view.BirthdaySoonNoGift, entry)
		}

		if key := dqNormalizedName(p.Name); key != "" {
			duplicateKeys[key] = append(duplicateKeys[key], viewEntry)
		}
	}

	keys := make([]string, 0, len(duplicateKeys))
	for key, members := range duplicateKeys {
		if len(members) < 2 {
			continue
		}
		sort.Slice(members, func(i, j int) bool {
			return strings.ToLower(members[i].Name) < strings.ToLower(members[j].Name)
		})
		duplicateKeys[key] = members
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return strings.ToLower(duplicateKeys[keys[i]][0].Name) < strings.ToLower(duplicateKeys[keys[j]][0].Name)
	})
	for _, key := range keys {
		view.Duplicates = append(view.Duplicates, dataQualityDuplicateView{Members: duplicateKeys[key]})
	}

	view.SummaryMissingBirthYear = fmt.Sprintf(t("data_quality.summary_missing_birth_year"), len(view.MissingBirthYear), view.Total)
	view.SummaryMissingPhoto = fmt.Sprintf(t("data_quality.summary_missing_photo"), len(view.MissingPhoto), view.Total)
	view.SummaryBirthdaySoon = fmt.Sprintf(t("data_quality.summary_birthday_soon"), len(view.BirthdaySoonNoGift), view.Total, view.WindowDays)
	view.SummaryDuplicates = fmt.Sprintf(t("data_quality.summary_duplicates"), len(view.Duplicates))

	h.renderDataQuality(w, r, view)
}

// dqBirthdayEvents returns the person's birthday events. PersonRepository.List
// loads the events edge, so this needs no extra query.
func dqBirthdayEvents(p *ent.Person) []*ent.Event {
	var out []*ent.Event
	for _, e := range p.Edges.Events {
		if e != nil && e.Type == "birthday" {
			out = append(out, e)
		}
	}
	return out
}

// dqHasBirthYear reports whether any birthday event carries a usable year.
// Yearless birthdays are stored with year 0; internal/age also rejects future
// dated entries, which a manual birthday typed with the occurrence year
// produces. Lunar birthdays store the birth date, so year > 1 is the test used
// elsewhere in the UI.
func dqHasBirthYear(birthdays []*ent.Event, now time.Time) bool {
	for _, e := range birthdays {
		if e.CalendarSystem == "lunar" && e.LunarMonth != nil && e.LunarDay != nil {
			if e.Date.Year() > 1 {
				return true
			}
			continue
		}
		if age.InfoFor(e.Date, now).HasAge {
			return true
		}
	}
	return false
}

// dqMissingPhoto reports a person with neither a local photo nor an Immich
// person link.
func dqMissingPhoto(p *ent.Person) bool {
	hasLocalPhoto := p.PhotoPath != nil && strings.TrimSpace(*p.PhotoPath) != ""
	hasImmich := p.ImmichPersonID != nil && strings.TrimSpace(*p.ImmichPersonID) != ""
	return !hasLocalPhoto && !hasImmich
}

// dqHasGiftIdea reports whether the person has an open (idea or purchased)
// gift idea, which counts as "a gift idea exists".
func dqHasGiftIdea(ctx context.Context, h *Handler, personID int) bool {
	ideas, err := h.giftIdeas.ListByPerson(ctx, personID)
	if err != nil {
		// A read failure here must not invent a problem; logging keeps the
		// page honest instead of flagging the person.
		slog.Error("data quality: list gift ideas", "person_id", personID, "error", err)
		return true
	}
	for _, idea := range ideas {
		if idea.Status == giftidea.StatusIdea || idea.Status == giftidea.StatusPurchased {
			return true
		}
	}
	return false
}

// dqUpcomingBirthdayWithin returns the nearest birthday occurrence within the
// window (today included) and how many days away it is.
func dqUpcomingBirthdayWithin(birthdays []*ent.Event, today time.Time, windowDays int) (time.Time, int, bool) {
	var (
		best time.Time
		days int
		ok   bool
	)
	for _, e := range birthdays {
		occ, occOK := dqNextBirthday(e.Date, today)
		if !occOK {
			continue
		}
		d := int(occ.Sub(today).Hours() / 24)
		if d < 0 || d > windowDays {
			continue
		}
		if !ok || d < days {
			best, days, ok = occ, d, true
		}
	}
	return best, days, ok
}

// dqNextBirthday returns the next occurrence of the date's month/day on or
// after today. A Feb 29 birthday falls on Feb 28 in non-leap years, matching
// internal/age.
func dqNextBirthday(date, today time.Time) (time.Time, bool) {
	month, day := date.Month(), date.Day()
	if month < time.January || month > time.December || day < 1 || day > 31 {
		return time.Time{}, false
	}
	occ := dqBirthdayInYear(today.Year(), month, day)
	// time.Date normalizes impossible dates (e.g. Feb 30 → Mar 2); reject
	// those instead of reporting a wrong day.
	if occ.Month() != month {
		return time.Time{}, false
	}
	if occ.Before(today) {
		occ = dqBirthdayInYear(today.Year()+1, month, day)
		if occ.Month() != month {
			return time.Time{}, false
		}
	}
	return occ, true
}

func dqBirthdayInYear(year int, month time.Month, day int) time.Time {
	if month == time.February && day == 29 && !dqIsLeapYear(year) {
		return time.Date(year, time.February, 28, 0, 0, 0, 0, time.UTC)
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func dqIsLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// dqRelativeDays renders "today", "tomorrow" or "in N days" through the
// render-time translator.
func dqRelativeDays(t func(string) string, days int) string {
	switch days {
	case 0:
		return t("data_quality.today")
	case 1:
		return t("data_quality.tomorrow")
	default:
		return fmt.Sprintf(t("data_quality.in_days"), days)
	}
}

// dqNormalizedName folds a name to a collision key: lowercase, punctuation
// stripped, whitespace collapsed, and tokens sorted so "Doe, Jane" and
// "Jane Doe" match. It also matches names that differ only by case or
// whitespace.
func dqNormalizedName(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range lower {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		default:
			// Punctuation (including the comma of "Last, First") separates
			// tokens instead of gluing them together.
			b.WriteRune(' ')
		}
	}
	tokens := strings.Fields(b.String())
	if len(tokens) == 0 {
		return ""
	}
	sort.Strings(tokens)
	return strings.Join(tokens, " ")
}
