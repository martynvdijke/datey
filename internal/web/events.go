package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

func (h *Handler) newEventForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	h.render(w, r, "event_form.html", map[string]any{
		"Title":     "Datey - Add Event",
		"PersonID": id,
	})
}

func (h *Handler) createEvent(w http.ResponseWriter, r *http.Request) {
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

	if len(errors) > 0 {
		h.render(w, r, "event_form.html", map[string]any{
			"Title":   "Datey - Add Event",
			"PersonID": id,
			"Errors":  errors,
			"FormData": map[string]string{
				"Type":        eventType,
				"Date":        dateStr,
				"Description": description,
			},
		})
		return
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		errors["date"] = "Invalid date format"
		h.render(w, r, "event_form.html", map[string]any{
			"Title":   "Datey - Add Event",
			"PersonID": id,
			"Errors":  errors,
			"FormData": map[string]string{
				"Type":        eventType,
				"Date":        dateStr,
				"Description": description,
			},
		})
		return
	}

	_, err = h.events.CreateForPerson(r.Context(), id, eventType, date, description)
	if err != nil {
		slog.Error("create event", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	toastHeader(w, "Event created", "success")
	http.Redirect(w, r, fmt.Sprintf("/people/%d", id), http.StatusSeeOther)
}

func (h *Handler) deleteEvent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := h.events.Delete(r.Context(), id); err != nil {
		slog.Error("delete event", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	toastHeader(w, "Event deleted", "success")
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}
