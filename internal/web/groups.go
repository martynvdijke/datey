package web

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (h *Handler) listGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.groups.List(r.Context())
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
