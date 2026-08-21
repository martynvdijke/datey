package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/datey/datey/ent"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) listGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.groups.ListWithCounts(r.Context())
	if err != nil {
		slog.Error("list groups", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	h.render(w, r, "groups.html", map[string]any{
		"Title":  "Datey - Groups",
		"Groups": groups,
	})
}

func (h *Handler) createGroup(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")

	errors := make(map[string]string)
	if name == "" {
		errors["name"] = "Name is required"
	}

	if len(errors) > 0 {
		groups, _ := h.groups.ListWithCounts(r.Context())
		h.render(w, r, "groups.html", map[string]any{
			"Title":  "Datey - Groups",
			"Groups": groups,
			"Errors": errors,
			"FormData": map[string]string{
				"Name":        name,
				"Description": description,
			},
		})
		return
	}

	_, err := h.groups.Create(r.Context(), name, description)
	if err != nil {
		slog.Error("create group", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/groups?success=Group+created", http.StatusSeeOther)
}

func (h *Handler) deleteGroup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := h.groups.Delete(r.Context(), id); err != nil {
		slog.Error("delete group", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/groups?success=Group+deleted", http.StatusSeeOther)
}

// groupEventData is the view model for the group detail page.
type groupEventData struct {
	ID          int
	Type        string
	Date        string
	Description string
}

type groupNoteRow struct {
	ID      int
	Note    string
	Date    string
	Created string
}

func (h *Handler) loadGroupEventRows(r *http.Request, groupID int) []groupEventData {
	events, err := h.events.ListByGroup(r.Context(), groupID)
	if err != nil {
		slog.Error("list group events", "error", err)
		return nil
	}
	rows := make([]groupEventData, 0, len(events))
	for _, e := range events {
		rows = append(rows, groupEventData{
			ID:          e.ID,
			Type:        e.Type,
			Date:        formatEventDate(h.cfg.DateVariant, e.Date),
			Description: e.Description,
		})
	}
	return rows
}

func (h *Handler) loadGroupNoteRows(r *http.Request, groupID int) []groupNoteRow {
	notes, err := h.groupNotes.ListByGroup(r.Context(), groupID)
	if err != nil {
		slog.Error("list group notes", "error", err)
		return nil
	}
	rows := make([]groupNoteRow, 0, len(notes))
	for _, n := range notes {
		rows = append(rows, groupNoteRow{
			ID:      n.ID,
			Note:    n.Note,
			Date:    formatEventDate(h.cfg.DateVariant, n.NoteDate),
			Created: formatEventDate(h.cfg.DateVariant, n.CreatedAt),
		})
	}
	return rows
}

// viewGroup renders the group detail page: members, events and notes.
func (h *Handler) viewGroup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	g, err := h.groups.GetWithRelations(r.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			h.renderError(w, r, http.StatusNotFound)
			return
		}
		slog.Error("get group", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	members := g.Edges.People
	memberIDs := make(map[int]bool, len(members))
	for _, m := range members {
		memberIDs[m.ID] = true
	}
	allPeople, err := h.people.List(r.Context())
	if err != nil {
		slog.Error("list people for group", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}
	// Candidates for the add-member picker: everyone not yet a member.
	var candidates []*ent.Person
	for _, p := range allPeople {
		if !memberIDs[p.ID] {
			candidates = append(candidates, p)
		}
	}

	h.render(w, r, "group_detail.html", map[string]any{
		"Title":       fmt.Sprintf("Datey - Group %s", g.Name),
		"Group":       g,
		"Members":     members,
		"Candidates":  candidates,
		"Events":      h.loadGroupEventRows(r, id),
		"Notes":       h.loadGroupNoteRows(r, id),
	})
}

// setGroupMembers replaces the whole membership from a multi-select form.
func (h *Handler) setGroupMembers(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var ids []int
	for _, v := range r.Form["members"] {
		if pid, err := strconv.Atoi(v); err == nil && pid > 0 {
			ids = append(ids, pid)
		}
	}
	if err := h.groups.SetMembers(r.Context(), id, ids); err != nil {
		slog.Error("set group members", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/groups/%d?success=Members+updated", id), http.StatusSeeOther)
}

func (h *Handler) addGroupMember(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	personID, err := strconv.Atoi(r.FormValue("person_id"))
	if err != nil || personID <= 0 {
		http.Error(w, "invalid person id", http.StatusBadRequest)
		return
	}
	if err := h.groups.AddPerson(r.Context(), id, personID); err != nil {
		slog.Error("add group member", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/groups/%d?success=Member+added", id), http.StatusSeeOther)
}

func (h *Handler) removeGroupMember(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	personID, err := strconv.Atoi(chi.URLParam(r, "personID"))
	if err != nil {
		http.Error(w, "invalid person id", http.StatusBadRequest)
		return
	}
	if err := h.groups.RemovePerson(r.Context(), id, personID); err != nil {
		slog.Error("remove group member", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/groups/%d?success=Member+removed", id), http.StatusSeeOther)
}

func (h *Handler) newGroupEventForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	g, err := h.groups.GetByID(r.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			h.renderError(w, r, http.StatusNotFound)
			return
		}
		slog.Error("get group", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}
	h.render(w, r, "event_form.html", map[string]any{
		"Title":     "Datey - Add Event",
		"GroupID":   g.ID,
		"GroupName": g.Name,
	})
}

func (h *Handler) createGroupEvent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	eventType := r.FormValue("type")
	dateStr := r.FormValue("date")
	description := r.FormValue("description")

	errors := make(map[string]string)
	if eventType == "" {
		errors["type"] = "Event type is required"
	}
	if dateStr == "" {
		errors["date"] = "Date is required"
	}
	date, parseErr := time.Parse("2006-01-02", dateStr)
	if dateStr != "" && parseErr != nil {
		errors["date"] = "Invalid date format"
	}
	if len(errors) > 0 {
		g, _ := h.groups.GetByID(r.Context(), id)
		h.render(w, r, "event_form.html", map[string]any{
			"Title":     "Datey - Add Event",
			"GroupID":   id,
			"GroupName": groupNameOrEmpty(g),
			"Errors":    errors,
			"FormData": map[string]string{
				"Type":        eventType,
				"Date":        dateStr,
				"Description": description,
			},
		})
		return
	}

	if _, err := h.events.CreateForGroup(r.Context(), id, eventType, date, description); err != nil {
		slog.Error("create group event", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/groups/%d?success=Event+created", id), http.StatusSeeOther)
}

func groupNameOrEmpty(g *ent.Group) string {
	if g == nil {
		return ""
	}
	return g.Name
}

func (h *Handler) createGroupNote(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	note := r.FormValue("note")
	dateStr := r.FormValue("note_date")
	if note == "" {
		http.Redirect(w, r, fmt.Sprintf("/groups/%d?error=Note+text+is+required", id), http.StatusSeeOther)
		return
	}
	noteDate := time.Now()
	if dateStr != "" {
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			http.Redirect(w, r, fmt.Sprintf("/groups/%d?error=Invalid+note+date", id), http.StatusSeeOther)
			return
		}
		noteDate = parsed
	}
	if _, err := h.groupNotes.Create(r.Context(), id, note, noteDate); err != nil {
		slog.Error("create group note", "error", err)
		http.Redirect(w, r, fmt.Sprintf("/groups/%d?error=Failed+to+save+note", id), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/groups/%d?success=Note+added", id), http.StatusSeeOther)
}

func (h *Handler) deleteGroupNote(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	noteID, err := strconv.Atoi(chi.URLParam(r, "noteID"))
	if err != nil {
		http.Error(w, "invalid note id", http.StatusBadRequest)
		return
	}
	if err := h.groupNotes.Delete(r.Context(), noteID); err != nil {
		slog.Error("delete group note", "error", err)
		http.Redirect(w, r, fmt.Sprintf("/groups/%d?error=Failed+to+delete+note", id), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/groups/%d?success=Note+deleted", id), http.StatusSeeOther)
}
