package web

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/immich"
	"github.com/datey/datey/internal/photos"
	"github.com/go-chi/chi/v5"
)

// personPhoto serves a person's profile picture. Precedence:
//  1. locally imported/uploaded photo (photo_path set) — Immich is not contacted
//  2. live proxy of the matched/overridden Immich thumbnail
//  3. not found (no match, disabled, or Immich unavailable)
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
	if p.ImmichPhotoDisabled {
		http.NotFound(w, r)
		return
	}

	// Local photo wins: serve from disk without contacting Immich.
	if p.PhotoPath != nil && *p.PhotoPath != "" {
		f, size, err := h.photoStore.Open(*p.PhotoPath)
		if err == nil {
			defer f.Close()
			contentType := "application/octet-stream"
			if p.PhotoContentType != nil && *p.PhotoContentType != "" {
				contentType = *p.PhotoContentType
			}
			w.Header().Set("Content-Type", contentType)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			if size > 0 {
				w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
			}
			_, _ = io.Copy(w, f)
			return
		}
		slog.Debug("local photo unavailable, falling back", "person_id", id, "error", err)
	}

	// Proxy fallback.
	if !h.immich.Enabled() {
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
	defer func() { _ = body.Close() }()
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	_, _ = io.Copy(w, body)
}

// hasLocalPhoto reports whether the person has an imported/uploaded photo on disk.
func (h *Handler) hasLocalPhoto(p *ent.Person) bool {
	return p.PhotoPath != nil && *p.PhotoPath != ""
}

func (h *Handler) photoURL(r *http.Request, p *ent.Person) string {
	if p.ImmichPhotoDisabled {
		return ""
	}
	// A local photo is always servable.
	if h.hasLocalPhoto(p) {
		return "/people/" + strconv.Itoa(p.ID) + "/photo"
	}
	if h.immich == nil || !h.immich.Enabled() {
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

// uploadPersonPhoto stores a user-uploaded image as the person's profile
// picture with source "upload". Uploads are never overwritten by Immich imports.
func (h *Handler) uploadPersonPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if _, err := h.people.Get(r.Context(), id); err != nil {
		http.NotFound(w, r)
		return
	}
	file, header, err := r.FormFile("photo")
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/people/%d?error=Choose+a+photo+to+upload", id), http.StatusSeeOther)
		return
	}
	defer file.Close()
	data, err := photos.Validate(header.Header.Get("Content-Type"), file)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/people/%d?error=%s", id, urlErrText(err.Error())), http.StatusSeeOther)
		return
	}
	relPath, err := h.photoStore.Save(id, header.Header.Get("Content-Type"), data)
	if err != nil {
		slog.Error("save uploaded photo", "person_id", id, "error", err)
		http.Redirect(w, r, fmt.Sprintf("/people/%d?error=Failed+to+store+photo", id), http.StatusSeeOther)
		return
	}
	if _, err := h.people.SetPhotoState(r.Context(), id, relPath, header.Header.Get("Content-Type"), "upload"); err != nil {
		_ = h.photoStore.Delete(relPath)
		slog.Error("record uploaded photo", "person_id", id, "error", err)
		http.Redirect(w, r, fmt.Sprintf("/people/%d?error=Failed+to+store+photo", id), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/people/%d?success=Photo+uploaded", id), http.StatusSeeOther)
}

// removePersonPhoto clears local photo state and deletes the stored file,
// reverting the person to proxy/fallback behavior.
func (h *Handler) removePersonPhoto(w http.ResponseWriter, r *http.Request) {
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
	if _, err := h.people.ClearPhoto(r.Context(), id); err != nil {
		slog.Error("clear photo state", "person_id", id, "error", err)
		http.Redirect(w, r, fmt.Sprintf("/people/%d?error=Failed+to+remove+photo", id), http.StatusSeeOther)
		return
	}
	if p.PhotoPath != nil {
		if err := h.photoStore.DeletePerson(id); err != nil {
			slog.Warn("delete photo file", "person_id", id, "error", err)
		}
	}
	http.Redirect(w, r, fmt.Sprintf("/people/%d?success=Photo+removed", id), http.StatusSeeOther)
}

// urlErrText makes an error message safe for a query-param redirect.
func urlErrText(msg string) string {
	out := make([]rune, 0, len(msg))
	for _, ch := range msg {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9', ch == '-', ch == '_', ch == '.':
			out = append(out, ch)
		default:
			out = append(out, '+')
		}
	}
	return string(out)
}
