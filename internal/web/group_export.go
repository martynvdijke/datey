package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/datey/datey/ent"
	"github.com/go-chi/chi/v5"
)

// GroupNoteExport is the portable representation of a group's note.
type GroupNoteExport struct {
	ID        int    `json:"id"`
	Note      string `json:"note"`
	NoteDate  string `json:"note_date"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// GroupEventExport is the portable representation of a group-owned event.
type GroupEventExport struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	Date        string `json:"date"`
	Description string `json:"description"`
}

// GroupExport is the portable JSON payload for a single group, including its
// notes timeline and group-owned events. Where full fidelity isn't possible
// (e.g. group export consumed by a third-party importer that doesn't understand
// GroupNote/GroupEvent), the consumer should degrade gracefully and ignore
// unknown fields — see comment in handleExportGroups.
type GroupExport struct {
	ID          int                `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Notes       []GroupNoteExport  `json:"notes"`
	Events      []GroupEventExport `json:"events"`
	MemberIDs   []int              `json:"member_ids"`
}

func (h *Handler) buildGroupExport(r *http.Request, g *ent.Group) GroupExport {
	ge := GroupExport{
		ID:          g.ID,
		Name:        g.Name,
		Description: g.Description,
	}
	// Member IDs
	if members, err := h.groups.ListPeopleInGroup(r.Context(), g.ID); err == nil {
		for _, m := range members {
			ge.MemberIDs = append(ge.MemberIDs, m.ID)
		}
	}
	// Notes timeline
	if notes, err := h.groupNotes.ListByGroup(r.Context(), g.ID); err == nil {
		for _, n := range notes {
			ge.Notes = append(ge.Notes, GroupNoteExport{
				ID:        n.ID,
				Note:      n.Note,
				NoteDate:  n.NoteDate.Format("2006-01-02"),
				CreatedAt: n.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
				UpdatedAt: n.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			})
		}
		if ge.Notes == nil {
			ge.Notes = []GroupNoteExport{}
		}
	}
	// Group-owned events
	if events, err := h.events.ListByGroup(r.Context(), g.ID); err == nil {
		for _, e := range events {
			ge.Events = append(ge.Events, GroupEventExport{
				ID:          e.ID,
				Type:        e.Type,
				Date:        e.Date.Format("2006-01-02"),
				Description: e.Description,
			})
		}
		if ge.Events == nil {
			ge.Events = []GroupEventExport{}
		}
	}
	if ge.MemberIDs == nil {
		ge.MemberIDs = []int{}
	}
	return ge
}

// handleExportGroups handles GET /groups/export — JSON array of all groups with
// notes and events. Graceful degradation: callers that don't understand
// notes/events can ignore those fields.
func (h *Handler) handleExportGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.groups.List(r.Context())
	if err != nil {
		slog.Error("export groups: list", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	out := make([]GroupExport, 0, len(groups))
	for _, g := range groups {
		out = append(out, h.buildGroupExport(r, g))
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}

// handleExportSingleGroup handles GET /groups/{id}/export — JSON for one group.
func (h *Handler) handleExportSingleGroup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	g, err := h.groups.GetByID(r.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		slog.Error("export group: get", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	ge := h.buildGroupExport(r, g)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(ge)
}
