package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

func (h *Handler) listPeopleJSON(w http.ResponseWriter, r *http.Request) {
	people, _, _, _, err := h.resolvePeopleFilter(r)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "internal", "failed")
		return
	}
	now := time.Now()
	cards := []PersonCardDTO{}
	for _, p := range people {
		events := p.Edges.Events
		var next *NextEventDTO
		if len(events) > 0 {
			var nearest *struct {
				Type string
				Date time.Time
			}
			for _, e := range events {
				if !calendarDayBefore(e.Date, now, now.Location()) {
					if nearest == nil || e.Date.Before(nearest.Date) {
						nearest = &struct {
							Type string
							Date time.Time
						}{Type: e.Type, Date: e.Date}
					}
				}
			}
			if nearest != nil {
				days := calendarDayDelta(dateOnly(now, now.Location()), dateOnly(nearest.Date, now.Location()))
				next = &NextEventDTO{Type: nearest.Type, Date: nearest.Date.Format("2006-01-02"), DaysRemaining: days}
			}
		}
		var photoURL *string
		if url := h.photoURL(r, p); url != "" {
			photoURL = &url
		}
		var first, last string
		if p.FirstName != nil {
			first = *p.FirstName
		}
		if p.LastName != nil {
			last = *p.LastName
		}
		tags := []string{}
		for _, t := range p.Edges.Tags {
			tags = append(tags, t.Name)
		}
		if tags == nil {
			tags = []string{}
		}
		grps := []GroupRefDTO{}
		for _, g := range p.Edges.Groups {
			grps = append(grps, GroupRefDTO{ID: g.ID, Name: g.Name})
		}
		if grps == nil {
			grps = []GroupRefDTO{}
		}
		cards = append(cards, PersonCardDTO{
			ID: p.ID, Name: p.Name, FirstName: first, LastName: last, PhotoURL: photoURL, Tags: tags, Groups: grps, NextEvent: next,
		})
	}
	if cards == nil {
		cards = []PersonCardDTO{}
	}
	groupsOut := []map[string]any{}
	if gc, err := h.groups.ListWithCounts(r.Context()); err == nil {
		for _, g := range gc {
			groupsOut = append(groupsOut, map[string]any{"id": g.ID, "name": g.Name, "member_count": len(g.Edges.People)})
		}
	}
	if groupsOut == nil {
		groupsOut = []map[string]any{}
	}
	writeAPIJSON(w, http.StatusOK, map[string]any{"people": cards, "groups": groupsOut, "total": len(cards)})
}

func (h *Handler) viewPersonJSON(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	person, err := h.people.Get(r.Context(), id)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "Person not found")
		return
	}
	events, _ := h.events.ListByPerson(r.Context(), id)
	groups, _ := h.groups.ListByPerson(r.Context(), id)
	tags, _ := h.tags.ListByPerson(r.Context(), id)
	showPurchased := r.URL.Query().Get("show_purchased") == "1"
	giftIdeas, _ := h.giftIdeas.ListByPersonFiltered(r.Context(), id, showPurchased)
	relEntries, _ := h.relationships.ListForPerson(r.Context(), id)

	u := UserFromContext(r.Context())
	hidden := false
	if u != nil && u.LinkedPersonID != nil && *u.LinkedPersonID == id {
		hidden = true
		giftIdeas = nil
	}

	var photoURL *string
	if url := h.photoURL(r, person); url != "" {
		photoURL = &url
	}
	first, middle, last := "", "", ""
	if person.FirstName != nil {
		first = *person.FirstName
	}
	if person.MiddleName != nil {
		middle = *person.MiddleName
	}
	if person.LastName != nil {
		last = *person.LastName
	}
	tagNames := []string{}
	for _, t := range tags {
		tagNames = append(tagNames, t.Name)
	}
	if tagNames == nil {
		tagNames = []string{}
	}
	evDTOs := []EventDTO{}
	for _, e := range events {
		evDTOs = append(evDTOs, EventDTO{
			ID: e.ID, Type: e.Type, Date: e.Date.Format("2006-01-02"), Description: e.Description,
			CalendarSystem: func() string {
				if e.CalendarSystem == "" {
					return "gregorian"
				}
				return e.CalendarSystem
			}(), LunarMonth: e.LunarMonth, LunarDay: e.LunarDay, LunarLeap: e.LunarLeap,
		})
	}
	if evDTOs == nil {
		evDTOs = []EventDTO{}
	}
	giftDTOs := []GiftIdeaDTO{}
	if !hidden {
		for _, g := range giftIdeas {
			giftDTOs = append(giftDTOs, GiftIdeaDTO{
				ID: g.ID, Title: g.Title, Notes: g.Notes, PriceCents: g.PriceCents, URL: g.URL, Status: string(g.Status),
				CreatedAt: g.CreatedAt.Format(time.RFC3339), UpdatedAt: g.UpdatedAt.Format(time.RFC3339),
			})
		}
	}
	if giftDTOs == nil {
		giftDTOs = []GiftIdeaDTO{}
	}
	relDTOs := []RelationshipDTO{}
	for _, e := range relEntries {
		dir := "from"
		if e.Direction == "incoming" {
			dir = "to"
		} else if e.Direction == "outgoing" {
			dir = "from"
		}
		var label *string
		if e.RawLabel != nil && *e.RawLabel != "" {
			label = e.RawLabel
		}
		relDTOs = append(relDTOs, RelationshipDTO{
			ID: e.ID, Type: e.RawType, Label: label, OtherPersonID: e.OtherPersonID, OtherPersonName: e.OtherPersonName, Direction: dir,
		})
	}
	if relDTOs == nil {
		relDTOs = []RelationshipDTO{}
	}
	grpDTOs := []GroupRefDTO{}
	for _, g := range groups {
		grpDTOs = append(grpDTOs, GroupRefDTO{ID: g.ID, Name: g.Name})
	}
	if grpDTOs == nil {
		grpDTOs = []GroupRefDTO{}
	}
	notify := person.NotifyBirthdays
	writeAPIJSON(w, http.StatusOK, map[string]any{
		"id": person.ID, "name": person.Name, "first_name": first, "middle_name": middle, "last_name": last,
		"notes": person.Notes, "photo_url": photoURL, "immich_person_id": person.ImmichPersonID, "notify_birthdays": notify, "timezone": "UTC",
		"tags": tagNames, "events": evDTOs, "gift_ideas": giftDTOs, "gift_ideas_hidden": hidden, "relationships": relDTOs, "groups": grpDTOs,
	})
}
