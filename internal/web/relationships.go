package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/datey/datey/internal/repository"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) addRelationship(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if wantsJSON(r) {
		var body struct {
			ToID  int     `json:"to_id"`
			Type  string  `json:"type"`
			Label *string `json:"label"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		lbl := ""
		if body.Label != nil {
			lbl = strings.TrimSpace(*body.Label)
		}
		relType := strings.TrimSpace(body.Type)
		allowed := map[string]bool{"partner": true, "parent": true, "sibling": true, "custom": true}
		if body.ToID == 0 || relType == "" || !allowed[relType] {
			writeAPIError(w, http.StatusBadRequest, "validation", "Missing fields")
			return
		}
		rel, err := h.relationships.Add(r.Context(), id, body.ToID, relType, lbl)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation", err.Error())
			return
		}
		other, _ := h.people.Get(r.Context(), body.ToID)
		otherName := ""
		if other != nil {
			otherName = other.Name
		}
		var labelPtr *string
		if lbl != "" {
			labelPtr = &lbl
		}
		if rel.Label != nil && *rel.Label != "" {
			labelPtr = rel.Label
		}
		writeAPIJSON(w, http.StatusCreated, RelationshipDTO{ID: rel.ID, Type: string(rel.Type), Label: labelPtr, OtherPersonID: body.ToID, OtherPersonName: otherName, Direction: "from"})
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	targetStr := r.FormValue("target_id")
	relType := strings.TrimSpace(r.FormValue("type"))
	label := strings.TrimSpace(r.FormValue("label"))
	targetID, _ := strconv.Atoi(targetStr)
	if targetID == 0 || relType == "" {
		http.Redirect(w, r, "/people/"+strconv.Itoa(id)+"?error=Missing+fields", http.StatusSeeOther)
		return
	}
	// Validate type
	allowed := map[string]bool{"partner": true, "parent": true, "sibling": true, "custom": true}
	if !allowed[relType] {
		http.Redirect(w, r, "/people/"+strconv.Itoa(id)+"?error=Invalid+type", http.StatusSeeOther)
		return
	}
	_, err = h.relationships.Add(r.Context(), id, targetID, relType, label)
	if err != nil {
		// Render detail with inline error instead of raw http.Error
		var msg string
		switch err.(type) {
		case *repository.SelfLinkError:
			msg = err.Error()
		case *repository.ParentLoopError:
			msg = err.Error()
		case *repository.DuplicateRelationshipError:
			msg = err.Error()
		default:
			msg = err.Error()
		}
		// Re-render person detail with error
		h.renderPersonDetailWithError(w, r, id, msg)
		return
	}
	http.Redirect(w, r, "/people/"+strconv.Itoa(id), http.StatusSeeOther)
}

func (h *Handler) removeRelationship(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	relID, err := strconv.Atoi(chi.URLParam(r, "relID"))
	if err != nil {
		http.Error(w, "invalid relationship id", http.StatusBadRequest)
		return
	}
	if wantsJSON(r) {
		_ = h.relationships.Remove(r.Context(), relID)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	_ = h.relationships.Remove(r.Context(), relID)
	http.Redirect(w, r, "/people/"+strconv.Itoa(id), http.StatusSeeOther)
}
