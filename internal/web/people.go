package web

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/datey/datey/ent"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) listPeople(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	groupIDStr := r.URL.Query().Get("group")

	var people []*ent.Person
	var err error

	if groupIDStr != "" {
		groupID, parseErr := strconv.Atoi(groupIDStr)
		if parseErr == nil {
			people, err = h.groups.ListPeopleInGroup(r.Context(), groupID)
		} else {
			people, err = h.people.List(r.Context())
		}
	} else if q != "" {
		people, err = h.people.Search(r.Context(), q)
	} else {
		people, err = h.people.List(r.Context())
	}

	if err != nil {
		slog.Error("list people", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	// Load events for each person
	for _, p := range people {
		events, err := h.events.ListByContact(r.Context(), p.ID)
		if err == nil {
			for _, e := range events {
				p.Edges.Events = append(p.Edges.Events, e)
			}
		}
	}

	// Load all groups for the group filter dropdown
	groups, _ := h.groups.List(r.Context())

	h.render(w, r, "people.html", map[string]any{
		"Title":     "Datey - People",
		"People":    people,
		"Groups":    groups,
		"GroupID":   groupIDStr,
		"Query":     q,
	})
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

	p, err := h.people.Create(r.Context(), name, notes)
	if err != nil {
		slog.Error("create person", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	// Add to selected groups
	groupIDs := r.Form["groups"]
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

	events, err := h.events.ListByContact(r.Context(), id)
	if err != nil {
		slog.Error("list events by person", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	groups, err := h.groups.ListByPerson(r.Context(), id)
	if err != nil {
		groups = nil
	}

	h.render(w, r, "person_detail.html", map[string]any{
		"Title":  "Datey - " + person.Name,
		"Person": person,
		"Events": events,
		"Groups": groups,
	})
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
