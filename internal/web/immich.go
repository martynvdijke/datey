package web

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/immich"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) personPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	p, err := h.people.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if p.ImmichPhotoDisabled || !h.immich.Enabled() {
		http.NotFound(w, r)
		return
	}
	photoID := ""
	if p.ImmichPersonID != nil {
		photoID = *p.ImmichPersonID
	} else if people, e := h.immich.People(r.Context()); e == nil {
		if match := immich.ExactMatch(p.Name, people); match != nil {
			photoID = match.ID
		}
	}
	if photoID == "" {
		http.NotFound(w, r)
		return
	}
	body, contentType, err := h.immich.Thumbnail(r.Context(), photoID)
	if err != nil {
		slog.Debug("immich photo unavailable", "person_id", id, "error", err)
		http.NotFound(w, r)
		return
	}
	defer body.Close()
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	_, _ = io.Copy(w, body)
}

func (h *Handler) photoURL(r *http.Request, p *ent.Person) string {
	if h.immich == nil || !h.immich.Enabled() || p.ImmichPhotoDisabled {
		return ""
	}
	if p.ImmichPersonID != nil && *p.ImmichPersonID != "" {
		return "/people/" + strconv.Itoa(p.ID) + "/photo"
	}
	people, err := h.immich.People(r.Context())
	if err != nil {
		return ""
	}
	if immich.ExactMatch(p.Name, people) == nil {
		return ""
	}
	return "/people/" + strconv.Itoa(p.ID) + "/photo"
}

func (h *Handler) setImmichPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var match *string
	if value := r.FormValue("immich_person_id"); value != "" {
		match = &value
	}
	disabled := r.FormValue("disable_photo") == "on"
	if _, err := h.people.SetImmichPhoto(r.Context(), id, match, disabled); err != nil {
		if ent.IsNotFound(err) {
			http.NotFound(w, r)
			return
		}
		slog.Error("set immich photo", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/people/%d", id), http.StatusSeeOther)
}
