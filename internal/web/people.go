package web

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/age"
	"github.com/go-chi/chi/v5"
)

// personCard holds the data for a single person card in the grid.
type personCard struct {
	ID            int
	Name          string
	Notes         string
	EventCount    int
	NextEventType string
	NextEventDate string
	Initial       string
	AvatarColor   int
	Age           int // current age, shown when HasAge
	HasAge        bool
	PhotoURL      string
	Groups        []string
}

// listPeopleInGroups returns the de-duplicated union of the members of all
// given group IDs.
func (h *Handler) listPeopleInGroups(r *http.Request, ids []int) ([]*ent.Person, error) {
	seen := make(map[int]bool)
	var out []*ent.Person
	for _, id := range ids {
		members, err := h.groups.ListPeopleInGroup(r.Context(), id)
		if err != nil {
			return nil, err
		}
		for _, m := range members {
			if seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			out = append(out, m)
		}
	}
	return out, nil
}

func (h *Handler) listPeople(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	groupIDStr := r.URL.Query().Get("group")

	// "group:Family rest of query" resolves the prefix to a group filter;
	// any remaining text searches within that group's members.
	searchText := ""
	if strings.HasPrefix(q, "group:") {
		groupName := strings.TrimSpace(strings.TrimPrefix(q, "group:"))
		if idx := strings.Index(groupName, " "); idx >= 0 {
			searchText = strings.TrimSpace(groupName[idx+1:])
			groupName = groupName[:idx]
		}
		if groups, err := h.groups.List(r.Context()); err == nil {
			for _, g := range groups {
				if strings.EqualFold(g.Name, groupName) {
					groupIDStr = strconv.Itoa(g.ID)
					break
				}
			}
		}
	} else {
		searchText = q
	}

	var people []*ent.Person
	var err error

	switch {
	case groupIDStr != "":
		var ids []int
		for _, part := range strings.Split(groupIDStr, ",") {
			if id, parseErr := strconv.Atoi(strings.TrimSpace(part)); parseErr == nil && id > 0 {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			people, err = h.listPeopleInGroups(r, ids)
		} else {
			people, err = h.people.List(r.Context())
		}
		if err == nil && searchText != "" {
			people = filterPeopleByName(people, searchText)
		}
	case searchText != "":
		people, err = h.people.Search(r.Context(), searchText)
	default:
		people, err = h.people.List(r.Context())
	}

	if err != nil {
		slog.Error("list people", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	// Load events and build enriched card data
	now := time.Now()
	cards := make([]personCard, 0, len(people))
	for _, p := range people {
		events, err := h.events.ListByPerson(r.Context(), p.ID)
		eventCount := 0
		var nextEventType, nextEventDate string
		cardAge, hasAge := birthdayAgeForEvents(events, now)
		if err == nil {
			eventCount = len(events)
			// Find the next upcoming event
			var nearest *ent.Event
			for _, e := range events {
				if !calendarDayBefore(e.Date, now, now.Location()) {
					if nearest == nil || e.Date.Before(nearest.Date) {
						nearest = e
					}
				}
			}
			if nearest != nil {
				nextEventType = nearest.Type
				nextEventDate = shortDate(h.cfg.DateVariant, nearest.Date)
			}
		}
		var groupNames []string
		if memberGroups, e := h.groups.ListByPerson(r.Context(), p.ID); e == nil {
			for _, g := range memberGroups {
				groupNames = append(groupNames, g.Name)
			}
		}
		cards = append(cards, personCard{
			ID:            p.ID,
			Name:          p.Name,
			Notes:         p.Notes,
			EventCount:    eventCount,
			NextEventType: nextEventType,
			NextEventDate: nextEventDate,
			Initial:       personInitial(p.Name),
			AvatarColor:   avatarColorIndex(p.Name),
			Age:           cardAge,
			HasAge:        hasAge,
			PhotoURL:      h.photoURL(r, p),
			Groups:        groupNames,
		})
	}

	// Load all groups for the group filter dropdown
	groups, _ := h.groups.List(r.Context())

	h.render(w, r, "people.html", map[string]any{
		"Title":   "Datey - People",
		"Cards":   cards,
		"Groups":  groups,
		"GroupID": groupIDStr,
		"Query":   q,
	})
}

// filterPeopleByName keeps people whose name contains the search text
// (case-insensitive), mirroring repository.Search for in-memory lists.
func filterPeopleByName(people []*ent.Person, q string) []*ent.Person {
	out := make([]*ent.Person, 0, len(people))
	for _, p := range people {
		if strings.Contains(strings.ToLower(p.Name), strings.ToLower(q)) {
			out = append(out, p)
		}
	}
	return out
}

func (h *Handler) newPersonForm(w http.ResponseWriter, r *http.Request) {
	groups, _ := h.groups.List(r.Context())
	h.render(w, r, "person_form.html", map[string]any{
		"Title":  "Datey - Add Person",
		"Groups": groups,
	})
}

func (h *Handler) createPerson(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	notes := r.FormValue("notes")
	groupIDs := r.Form["groups"]

	errors := make(map[string]string)
	if name == "" {
		errors["name"] = "Name is required"
	}

	if len(errors) > 0 {
		groups, _ := h.groups.List(r.Context())
		h.render(w, r, "person_form.html", map[string]any{
			"Title":  "Datey - Add Person",
			"Groups": groups,
			"Errors": errors,
			"FormData": map[string]any{
				"Name":     name,
				"Notes":    notes,
				"GroupIDs": groupIDs,
			},
		})
		return
	}

	p, err := h.people.Create(r.Context(), name, notes, "")
	if err != nil {
		slog.Error("create person", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	// Add to selected groups
	for _, gidStr := range groupIDs {
		gid, parseErr := strconv.Atoi(gidStr)
		if parseErr == nil {
			_ = h.groups.AddPerson(r.Context(), gid, p.ID)
		}
	}

	http.Redirect(w, r, "/people?success=Person+created", http.StatusSeeOther)
}

func (h *Handler) viewPerson(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	person, err := h.people.Get(r.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		slog.Error("get person", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	events, err := h.events.ListByPerson(r.Context(), id)
	if err != nil {
		slog.Error("list events by person", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	groups, err := h.groups.ListByPerson(r.Context(), id)
	if err != nil {
		groups = nil
	}

	// Split events into upcoming and past using calendar days, not elapsed
	// 24-hour periods. This avoids midnight/timezone boundary errors.
	now := time.Now()
	today := dateOnly(now, now.Location())
	type eventRow struct {
		ID            int
		Type          string
		Date          string
		EventDate     time.Time
		RelativeLabel string
		Description   string
		IsUpcoming    bool
	}
	eventRows := make([]eventRow, 0, len(events))
	for _, e := range events {
		days := calendarDayDelta(today, dateOnly(e.Date, now.Location()))
		var rel string
		switch {
		case days == 0:
			rel = "Today"
		case days == 1:
			rel = "Tomorrow"
		case days <= 7:
			rel = "In " + strconv.Itoa(days) + " days"
		}
		eventRows = append(eventRows, eventRow{
			ID:            e.ID,
			Type:          e.Type,
			Date:          formatEventDate(h.cfg.DateVariant, e.Date),
			EventDate:     e.Date,
			RelativeLabel: rel,
			Description:   e.Description,
			IsUpcoming:    days >= 0,
		})
	}

	h.render(w, r, "person_detail.html", map[string]any{
		"Title":       "Datey - " + person.Name,
		"Person":      person,
		"Initial":     personInitial(person.Name),
		"AvatarColor": avatarColorIndex(person.Name),
		"PhotoURL":    h.photoURL(r, person),
		"EventRows":   eventRows,
		"Groups":      groups,
		"Now":         now,
	})
}

func dateOnly(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	// Use UTC for date arithmetic so daylight-saving transitions do not turn a
	// calendar day into a 23- or 25-hour duration.
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

func calendarDayDelta(from, to time.Time) int {
	return int(to.Sub(from).Hours() / 24)
}

func calendarDayBefore(event, reference time.Time, loc *time.Location) bool {
	return dateOnly(event, loc).Before(dateOnly(reference, loc))
}

// toggleNotifyBirthdays updates the per-person birthday notification
// opt-out. The checkbox posts the form value "on" when enabled.
func (h *Handler) toggleNotifyBirthdays(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	enabled := r.FormValue("notify_birthdays") == "on"

	if _, err := h.people.SetNotifyBirthdays(r.Context(), id, enabled); err != nil {
		if ent.IsNotFound(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		slog.Error("set notify birthdays", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/people/"+strconv.Itoa(id), http.StatusSeeOther)
}

func (h *Handler) deletePerson(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := h.people.Delete(r.Context(), id); err != nil {
		slog.Error("delete person", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	// Best-effort cleanup of any stored profile photo files.
	if h.photoStore != nil {
		if err := h.photoStore.DeletePerson(id); err != nil {
			slog.Warn("delete person photo files", "person_id", id, "error", err)
		}
	}

	http.Redirect(w, r, "/people?success=Person+deleted", http.StatusSeeOther)
}

// --- Redirect handlers for legacy /contacts routes ---
func (h *Handler) redirectContactsList(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/people", http.StatusMovedPermanently)
}

func (h *Handler) redirectContactsNew(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/people/new", http.StatusMovedPermanently)
}

func (h *Handler) redirectContactsView(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	http.Redirect(w, r, "/people/"+id, http.StatusMovedPermanently)
}

// birthdayAgeForEvents derives the current age from a person's birthday
// events. When multiple birthday events exist, the most recent birth date
// (largest year) is used. ok is false when no birthday event carries a usable
// birth year.
func birthdayAgeForEvents(events []*ent.Event, now time.Time) (currentAge int, ok bool) {
	var latest *ent.Event
	for _, e := range events {
		if e.Type != "birthday" {
			continue
		}
		if latest == nil || e.Date.After(latest.Date) {
			latest = e
		}
	}
	if latest == nil {
		return 0, false
	}
	return age.AgeAt(latest.Date, now)
}

// formatEventDate renders an event date for display in the configured date
// variant. Dates without a usable year (year <= 1, e.g. year-less vCard
// birthdays parsed to year 0) show as month/day only ("Jun 8") instead of the
// misleading "Jun 8, 1".
func formatEventDate(variant string, t time.Time) string {
	if t.Year() <= 1 {
		return shortDate(variant, t)
	}
	return longDate(variant, t)
}
