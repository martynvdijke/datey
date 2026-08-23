package web

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (h *Handler) addPersonTag(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.FormValue("tag")
	if name == "" {
		http.Redirect(w, r, "/people/"+strconv.Itoa(id), http.StatusSeeOther)
		return
	}
	if err := h.tags.AddToPerson(r.Context(), id, name); err != nil {
		http.Error(w, "invalid tag", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/people/"+strconv.Itoa(id), http.StatusSeeOther)
}

func (h *Handler) removePersonTag(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.FormValue("tag")
	if name == "" {
		name = chi.URLParam(r, "tag")
	}
	if name == "" {
		http.Redirect(w, r, "/people/"+strconv.Itoa(id), http.StatusSeeOther)
		return
	}
	_ = h.tags.RemoveFromPerson(r.Context(), id, name)
	http.Redirect(w, r, "/people/"+strconv.Itoa(id), http.StatusSeeOther)
}

func (h *Handler) autocompleteTags(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	tags, err := h.tags.SearchByPrefix(r.Context(), q, 10)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, t.Name)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(names)
}
