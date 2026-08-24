package web

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/age"
	"github.com/datey/datey/internal/milestone"
	personname "github.com/datey/datey/internal/person"
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
	Tags          []string
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
	tagParam := strings.TrimSpace(r.URL.Query().Get("tag"))

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

	// Apply tag filter (comma-separated AND semantics), composing with q/group.
	if tagParam != "" {
		var tagNames []string
		for _, part := range strings.Split(tagParam, ",") {
			if t := strings.TrimSpace(part); t != "" {
				tagNames = append(tagNames, t)
			}
		}
		if len(tagNames) > 0 {
			tagPeople, tagErr := h.tags.ListPeopleByTags(r.Context(), tagNames)
			if tagErr != nil {
				slog.Error("list people by tags", "error", tagErr)
				h.renderError(w, r, http.StatusInternalServerError)
				return
			}
			// Intersect with already-filtered people
			tagSet := make(map[int]bool, len(tagPeople))
			for _, tp := range tagPeople {
				tagSet[tp.ID] = true
			}
			filtered := make([]*ent.Person, 0, len(people))
			for _, p := range people {
				if tagSet[p.ID] {
					filtered = append(filtered, p)
				}
			}
			people = filtered
		}
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
		var tagNames []string
		if personTags, e := h.tags.ListByPerson(r.Context(), p.ID); e == nil {
			for _, t := range personTags {
				tagNames = append(tagNames, t.Name)
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
			Tags:          tagNames,
		})
	}

	// Load all groups for the group filter dropdown
	groups, _ := h.groups.List(r.Context())

	h.render(w, r, "people.html", map[string]any{
		"Title":     "Datey - People",
		"Cards":     cards,
		"Groups":    groups,
		"GroupID":   groupIDStr,
		"Query":     q,
		"TagFilter": tagParam,
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

// parseNameForm extracts the structured name parts from a form submission.
// The legacy single `name` field is the fallback display when no structured
// part is provided (backward compat for API clients).
func parseNameForm(r *http.Request) (first, middle, last, legacy string) {
	return strings.TrimSpace(r.FormValue("first_name")),
		strings.TrimSpace(r.FormValue("middle_name")),
		strings.TrimSpace(r.FormValue("last_name")),
		strings.TrimSpace(r.FormValue("name"))
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

	first, middle, last, legacy := parseNameForm(r)
	notes := r.FormValue("notes")
	groupIDs := r.Form["groups"]
	displayName := personname.DisplayName(first, middle, last, legacy)

	errors := make(map[string]string)
	if displayName == "" {
		errors["first_name"] = "Name is required"
	}

	if len(errors) > 0 {
		groups, _ := h.groups.List(r.Context())
		h.render(w, r, "person_form.html", map[string]any{
			"Title":  "Datey - Add Person",
			"Groups": groups,
			"Errors": errors,
			"FormData": map[string]any{
				"FirstName":  first,
				"MiddleName": middle,
				"LastName":   last,
				"Name":       displayName,
				"Notes":      notes,
				"GroupIDs":   groupIDs,
			},
		})
		return
	}

	p, err := h.people.CreateStructured(r.Context(), displayName, first, middle, last, notes, "")
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

func (h *Handler) editPersonForm(w http.ResponseWriter, r *http.Request) {
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

	h.renderEditPersonForm(w, r, person, nil, nil)
}

// renderEditPersonForm renders person_form.html in edit mode with the
// person's current values pre-filled. formData and errors are non-nil when
// re-displaying after a validation failure.
func (h *Handler) renderEditPersonForm(w http.ResponseWriter, r *http.Request, person *ent.Person, formData map[string]any, errors map[string]string) {
	groups, _ := h.groups.List(r.Context())
	memberGroups, _ := h.groups.ListByPerson(r.Context(), person.ID)
	memberIDs := make([]string, 0, len(memberGroups))
	for _, g := range memberGroups {
		memberIDs = append(memberIDs, strconv.Itoa(g.ID))
	}
	birthday := ""
	if bd, err := h.events.FindByPersonAndType(r.Context(), person.ID, "birthday"); err == nil && bd.Date.Year() > 1 {
		birthday = bd.Date.Format("2006-01-02")
	}
	first, middle, last := structuredNameForForm(person)
	data := map[string]any{
		"Title":          "Datey - Edit Person",
		"Person":         person,
		"FirstName":      first,
		"MiddleName":     middle,
		"LastName":       last,
		"Groups":         groups,
		"MemberGroupIDs": memberIDs,
		"Birthday":       birthday,
	}
	if formData != nil {
		data["FormData"] = formData
	}
	if errors != nil {
		data["Errors"] = errors
	}
	h.render(w, r, "person_form.html", data)
}

// structuredNameForForm returns the structured name parts to prefill the edit
// form: the stored parts when present, otherwise a heuristic split of the
// legacy display name so nothing is lost before the user edits.
func structuredNameForForm(p *ent.Person) (first, middle, last string) {
	if p.FirstName != nil {
		first = *p.FirstName
	}
	if p.MiddleName != nil {
		middle = *p.MiddleName
	}
	if p.LastName != nil {
		last = *p.LastName
	}
	if first == "" && middle == "" && last == "" {
		first, _, last = personname.SplitDisplayName(p.Name)
	}
	return first, middle, last
}

// updatePerson saves edits to an existing person: name, notes, group
// memberships, and the birthday event (upserted when a date is submitted).
func (h *Handler) updatePerson(w http.ResponseWriter, r *http.Request) {
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

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	first, middle, last, legacy := parseNameForm(r)
	notes := r.FormValue("notes")
	groupIDs := r.Form["groups"]
	birthdayStr := r.FormValue("birthday")
	displayName := personname.DisplayName(first, middle, last, legacy)

	errors := make(map[string]string)
	var birthday time.Time
	hasBirthday := false
	if birthdayStr != "" {
		var parseErr error
		birthday, parseErr = time.Parse("2006-01-02", birthdayStr)
		if parseErr != nil {
			errors["birthday"] = "Invalid date format"
		} else {
			hasBirthday = true
		}
	}
	if displayName == "" {
		errors["first_name"] = "Name is required"
	}

	if len(errors) > 0 {
		h.renderEditPersonForm(w, r, person, map[string]any{
			"FirstName":  first,
			"MiddleName": middle,
			"LastName":   last,
			"Name":       displayName,
			"Notes":      notes,
			"GroupIDs":   groupIDs,
			"Birthday":   birthdayStr,
		}, errors)
		return
	}

	if _, err := h.people.UpdateStructured(r.Context(), id, displayName, first, middle, last, notes, ""); err != nil {
		if ent.IsConstraintError(err) {
			h.renderEditPersonForm(w, r, person, map[string]any{
				"FirstName":  first,
				"MiddleName": middle,
				"LastName":   last,
				"Name":       displayName,
				"Notes":      notes,
				"GroupIDs":   groupIDs,
				"Birthday":   birthdayStr,
			}, map[string]string{"first_name": "A person with this name already exists"})
			return
		}
		slog.Error("update person", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	// Sync group memberships to the submitted checkbox state.
	desired := make(map[int]bool, len(groupIDs))
	for _, gidStr := range groupIDs {
		if gid, parseErr := strconv.Atoi(strings.TrimSpace(gidStr)); parseErr == nil && gid > 0 {
			desired[gid] = true
		}
	}
	currentIDs := make(map[int]bool)
	if current, e := h.groups.ListByPerson(r.Context(), id); e == nil {
		for _, g := range current {
			currentIDs[g.ID] = true
		}
	}
	for gid := range desired {
		if !currentIDs[gid] {
			_ = h.groups.AddPerson(r.Context(), gid, id)
		}
	}
	for gid := range currentIDs {
		if !desired[gid] {
			_ = h.groups.RemovePerson(r.Context(), gid, id)
		}
	}

	// Upsert the birthday event. An empty field is a no-op so year-less
	// birthdays are never clobbered.
	if hasBirthday {
		existing, findErr := h.events.FindByPersonAndType(r.Context(), id, "birthday")
		switch {
		case ent.IsNotFound(findErr):
			if _, createErr := h.events.CreateForPerson(r.Context(), id, "birthday", birthday, ""); createErr != nil {
				slog.Error("create birthday event", "error", createErr)
				h.renderError(w, r, http.StatusInternalServerError)
				return
			}
		case findErr != nil:
			slog.Error("find birthday event", "error", findErr)
			h.renderError(w, r, http.StatusInternalServerError)
			return
		case !existing.Date.Equal(birthday):
			if _, updateErr := h.events.Update(r.Context(), existing.ID, "birthday", birthday, existing.Description); updateErr != nil {
				slog.Error("update birthday event", "error", updateErr)
				h.renderError(w, r, http.StatusInternalServerError)
				return
			}
		}
	}

	http.Redirect(w, r, "/people/"+strconv.Itoa(id)+"?success=Person+updated", http.StatusSeeOther)
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
		ID             int
		Type           string
		Date           string
		EventDate      time.Time
		RelativeLabel  string
		Description    string
		IsUpcoming     bool
		MilestoneLabel string
		LunarNotation  string
	}
	eventRows := make([]eventRow, 0, len(events))
	for _, e := range events {
		var occDate time.Time
		var lunarNote string
		if e.CalendarSystem == "lunar" && e.LunarMonth != nil && e.LunarDay != nil {
			lunarNote = formatLunarNotation(e)
			occDate, _ = displayDateForEvent(e, now)
		} else {
			occDate = time.Date(now.Year(), e.Date.Month(), e.Date.Day(), 0, 0, 0, 0, time.UTC)
			if e.Date.Month() == time.February && e.Date.Day() == 29 && !isLeapYear(now.Year()) {
				occDate = time.Date(now.Year(), time.February, 28, 0, 0, 0, 0, time.UTC)
			}
		}
		_ = occDate
		// For upcoming checks use occurrence date
		occForCheck := occDate
		days := calendarDayDelta(today, dateOnly(occForCheck, now.Location()))
		var rel string
		switch {
		case days == 0:
			rel = "Today"
		case days == 1:
			rel = "Tomorrow"
		case days <= 7:
			rel = "In " + strconv.Itoa(days) + " days"
		}
		var msLabel string
		if ok, label := milestone.IsMilestone(e.Type, e.Date, occDate); ok {
			msLabel = label
		}
		var dateStr string
		if lunarNote != "" {
			dateStr = formatEventDate(h.cfg.DateVariant, occDate) + " · " + lunarNote
		} else {
			dateStr = formatEventDate(h.cfg.DateVariant, e.Date)
		}
		eventRows = append(eventRows, eventRow{
			ID:             e.ID,
			Type:           e.Type,
			Date:           dateStr,
			EventDate:      e.Date,
			RelativeLabel:  rel,
			Description:    e.Description,
			IsUpcoming:     days >= 0,
			MilestoneLabel: msLabel,
			LunarNotation:  lunarNote,
		})
	}

	tags, _ := h.tags.ListByPerson(r.Context(), id)
	showPurchased := r.URL.Query().Get("show_purchased") == "1"
	giftIdeas, _ := h.giftIdeas.ListByPersonFiltered(r.Context(), id, showPurchased)
	relEntries, _ := h.relationships.ListForPerson(r.Context(), id)
	// Group relationships by EffectiveTypeLabel
	// Build related people upcoming info
	type relatedView struct {
		ID          int
		OtherID     int
		OtherName   string
		Label       string
		NextDate    string
		DaysUntil   int
		HasUpcoming bool
	}
	// Group map
	grouped := make(map[string][]relatedView)
	for _, e := range relEntries {
		rv := relatedView{
			ID:        e.ID,
			OtherID:   e.OtherPersonID,
			OtherName: e.OtherPersonName,
			Label:     e.EffectiveTypeLabel,
		}
		// Find next upcoming date for other person
		if otherEvents, err := h.events.ListByPerson(r.Context(), e.OtherPersonID); err == nil {
			bestDays := 99999
			for _, ev := range otherEvents {
				occDate, _ := displayDateForEvent(ev, now)
				// displayDateForEvent may return past date for gregorian if birthday this year already passed; ensure future
				if dateOnly(occDate, now.Location()).Before(dateOnly(now, now.Location())) {
					// try next year for gregorian
					if ev.CalendarSystem != "lunar" {
						occDate = time.Date(now.Year()+1, ev.Date.Month(), ev.Date.Day(), 0, 0, 0, 0, time.UTC)
						if ev.Date.Month() == time.February && ev.Date.Day() == 29 && !isLeapYear(now.Year()+1) {
							occDate = time.Date(now.Year()+1, time.February, 28, 0, 0, 0, 0, time.UTC)
						}
					} else {
						continue
					}
					if dateOnly(occDate, now.Location()).Before(dateOnly(now, now.Location())) {
						continue
					}
				}
				days := calendarDayDelta(dateOnly(now, now.Location()), dateOnly(occDate, now.Location()))
				if days < bestDays {
					bestDays = days
					rv.NextDate = formatEventDate(h.cfg.DateVariant, occDate)
					rv.DaysUntil = days
					rv.HasUpcoming = true
				}
			}
		}
		key := e.EffectiveTypeLabel
		// Normalize grouping: parent/child etc. Use label as key so custom labels group themselves.
		grouped[key] = append(grouped[key], rv)
	}
	allPeople, _ := h.people.List(r.Context())
	var pickerPeople []*ent.Person
	for _, p := range allPeople {
		if p.ID != id {
			pickerPeople = append(pickerPeople, p)
		}
	}
	// Check for inline relationship error from query param or passed via context
	relErr := r.URL.Query().Get("rel_error")
	h.render(w, r, "person_detail.html", map[string]any{
		"Title":             "Datey - " + person.Name,
		"Person":            person,
		"Initial":           personInitial(person.Name),
		"AvatarColor":       avatarColorIndex(person.Name),
		"PhotoURL":          h.photoURL(r, person),
		"EventRows":         eventRows,
		"Groups":            groups,
		"Tags":              tags,
		"GiftIdeas":         giftIdeas,
		"ShowPurchased":     showPurchased,
		"Now":               now,
		"Relationships":     relEntries,
		"GroupedRelations":  grouped,
		"PickerPeople":      pickerPeople,
		"RelationshipError": relErr,
	})
}

func (h *Handler) renderPersonDetailWithError(w http.ResponseWriter, r *http.Request, id int, errMsg string) {
	// Redirect with error query param so viewPerson picks it up, but also directly render to keep inline error
	// Use redirect with rel_error to keep GET semantics
	http.Redirect(w, r, "/people/"+strconv.Itoa(id)+"?rel_error="+urlQueryEscape(errMsg), http.StatusSeeOther)
}

func urlQueryEscape(s string) string {
	// minimal escape for query param
	return strings.ReplaceAll(strings.ReplaceAll(s, " ", "+"), "&", "%26")
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

	h.auditRecord(r, "person.delete", strconv.Itoa(id))
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
// events. For lunar birthdays age counts completed lunar years (one per
// anniversary of the stored lunar month/day), documented in code.
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
	if latest.CalendarSystem == "lunar" && latest.LunarMonth != nil && latest.LunarDay != nil {
		if latest.Date.Year() <= 1 {
			return 0, false
		}
		years := now.Year() - latest.Date.Year()
		display, _ := displayDateForEvent(latest, now)
		nowDay := dateOnly(now, now.Location())
		occDay := dateOnly(display, time.UTC)
		if occDay.After(nowDay) {
			years--
		}
		if years < 0 {
			return 0, false
		}
		return years, true
	}
	return age.AgeAt(latest.Date, now)
}

func isLeapYear(y int) bool { return y%4 == 0 && (y%100 != 0 || y%400 == 0) }

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
