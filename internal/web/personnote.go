package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/datey/datey/ent"
	"github.com/go-chi/chi/v5"
)

type personNoteRow struct {
	ID      int
	Note    string
	Date    string
	Created string
}

func personNoteDTO(n *ent.PersonNote) PersonNoteDTO {
	return PersonNoteDTO{
		ID:        n.ID,
		Note:      n.Note,
		NoteDate:  n.NoteDate.Format("2006-01-02"),
		CreatedAt: n.CreatedAt.Format(time.RFC3339),
		UpdatedAt: n.UpdatedAt.Format(time.RFC3339),
	}
}

func (h *Handler) loadPersonNoteRows(r *http.Request, personID int) []personNoteRow {
	notes, err := h.personNotes.ListByPerson(r.Context(), personID)
	if err != nil {
		slog.Error("list person notes", "error", err)
		return nil
	}
	rows := make([]personNoteRow, 0, len(notes))
	for _, n := range notes {
		rows = append(rows, personNoteRow{
			ID:      n.ID,
			Note:    n.Note,
			Date:    formatEventDate(h.cfg.DateVariant, n.NoteDate),
			Created: formatEventDate(h.cfg.DateVariant, n.CreatedAt),
		})
	}
	return rows
}

func (h *Handler) createPersonNote(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		if wantsJSON(r) {
			writeAPIError(w, http.StatusBadRequest, "bad_request", "invalid id")
			return
		}
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if wantsJSON(r) {
		var body struct {
			Note     string `json:"note"`
			NoteDate string `json:"note_date"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeAPIError(w, http.StatusBadRequest, "bad_request", "Invalid JSON")
			return
		}
		noteText := strings.TrimSpace(body.Note)
		if noteText == "" {
			writeAPIError(w, http.StatusBadRequest, "bad_request", "Note text is required")
			return
		}
		noteDate := time.Now()
		if body.NoteDate != "" {
			parsed, parseErr := time.Parse("2006-01-02", body.NoteDate)
			if parseErr != nil {
				writeAPIError(w, http.StatusBadRequest, "bad_request", "Invalid note date")
				return
			}
			noteDate = parsed
		}
		note, err := h.personNotes.Create(r.Context(), id, noteText, noteDate)
		if err != nil {
			slog.Error("create person note", "error", err)
			writeAPIError(w, http.StatusInternalServerError, "internal", "Failed to save note")
			return
		}
		writeAPIJSON(w, http.StatusCreated, personNoteDTO(note))
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	note := strings.TrimSpace(r.FormValue("note"))
	if note == "" {
		http.Redirect(w, r, fmt.Sprintf("/people/%d?error=Note+text+is+required", id), http.StatusSeeOther)
		return
	}
	noteDate := time.Now()
	if dateStr := r.FormValue("note_date"); dateStr != "" {
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			http.Redirect(w, r, fmt.Sprintf("/people/%d?error=Invalid+note+date", id), http.StatusSeeOther)
			return
		}
		noteDate = parsed
	}
	if _, err := h.personNotes.Create(r.Context(), id, note, noteDate); err != nil {
		slog.Error("create person note", "error", err)
		http.Redirect(w, r, fmt.Sprintf("/people/%d?error=Failed+to+save+note", id), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/people/%d?success=Note+added", id), http.StatusSeeOther)
}

func (h *Handler) deletePersonNote(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		if wantsJSON(r) {
			writeAPIError(w, http.StatusBadRequest, "bad_request", "invalid id")
			return
		}
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	noteID, err := strconv.Atoi(chi.URLParam(r, "noteID"))
	if err != nil {
		if wantsJSON(r) {
			writeAPIError(w, http.StatusBadRequest, "bad_request", "invalid note id")
			return
		}
		http.Error(w, "invalid note id", http.StatusBadRequest)
		return
	}

	if err := h.personNotes.Delete(r.Context(), id, noteID); err != nil {
		slog.Error("delete person note", "error", err)
		if wantsJSON(r) {
			writeAPIError(w, http.StatusNotFound, "not_found", "Note not found")
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/people/%d?error=Failed+to+delete+note", id), http.StatusSeeOther)
		return
	}

	if wantsJSON(r) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		toastHeader(w, "Note deleted", "success")
		w.Header().Set("HX-Refresh", "true")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/people/%d?success=Note+deleted", id), http.StatusSeeOther)
}
