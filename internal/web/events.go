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
		"Title":    "Datey - Add Event",
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
	calendarSystem := r.FormValue("calendar_system")
	if calendarSystem == "" {
		calendarSystem = "gregorian"
	}
	if calendarSystem != "gregorian" && calendarSystem != "lunar" {
		calendarSystem = "gregorian"
	}
	lunarMonthStr := r.FormValue("lunar_month")
	lunarDayStr := r.FormValue("lunar_day")
	lunarLeap := r.FormValue("lunar_leap") == "on" || r.FormValue("lunar_leap") == "true" || r.FormValue("lunar_leap") == "1"

	errors := make(map[string]string)
	if eventType == "" {
		errors["type"] = "Event type is required"
	}
	var lunarMonth, lunarDay *int
	if calendarSystem == "lunar" {
		if lunarMonthStr == "" {
			errors["lunar_month"] = "Lunar month is required"
		} else {
			if v, err := strconv.Atoi(lunarMonthStr); err != nil || v < 1 || v > 12 {
				errors["lunar_month"] = "Invalid lunar month"
			} else {
				lunarMonth = &v
			}
		}
		if lunarDayStr == "" {
			errors["lunar_day"] = "Lunar day is required"
		} else {
			if v, err := strconv.Atoi(lunarDayStr); err != nil || v < 1 || v > 30 {
				errors["lunar_day"] = "Invalid lunar day"
			} else {
				lunarDay = &v
			}
		}
		// Gregorian date still required as storage placeholder for lunar events
		if dateStr == "" {
			errors["date"] = "Date is required"
		}
	} else {
		if dateStr == "" {
			errors["date"] = "Date is required"
		}
	}

	if len(errors) > 0 {
		h.render(w, r, "event_form.html", map[string]any{
			"Title":    "Datey - Add Event",
			"PersonID": id,
			"Errors":   errors,
			"FormData": map[string]string{
				"Type":           eventType,
				"Date":           dateStr,
				"Description":    description,
				"CalendarSystem": calendarSystem,
				"LunarMonth":     lunarMonthStr,
				"LunarDay":       lunarDayStr,
			},
		})
		return
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		errors["date"] = "Invalid date format"
		h.render(w, r, "event_form.html", map[string]any{
			"Title":    "Datey - Add Event",
			"PersonID": id,
			"Errors":   errors,
			"FormData": map[string]string{
				"Type":           eventType,
				"Date":           dateStr,
				"Description":    description,
				"CalendarSystem": calendarSystem,
				"LunarMonth":     lunarMonthStr,
				"LunarDay":       lunarDayStr,
			},
		})
		return
	}

	if calendarSystem == "lunar" {
		_, err = h.events.CreateForPersonWithCalendar(r.Context(), id, eventType, date, description, "lunar", lunarMonth, lunarDay, lunarLeap)
	} else {
		_, err = h.events.CreateForPerson(r.Context(), id, eventType, date, description)
	}
	if err != nil {
		// validation error from repo layer -> show inline
		if err.Error() == "lunar event requires lunar_month and lunar_day" || errors["lunar_month"] == "" {
			errors["lunar_month"] = err.Error()
			h.render(w, r, "event_form.html", map[string]any{
				"Title":    "Datey - Add Event",
				"PersonID": id,
				"Errors":   errors,
				"FormData": map[string]string{
					"Type":           eventType,
					"Date":           dateStr,
					"Description":    description,
					"CalendarSystem": calendarSystem,
					"LunarMonth":     lunarMonthStr,
					"LunarDay":       lunarDayStr,
				},
			})
			return
		}
		slog.Error("create event", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/people/%d?success=Event+created", id), http.StatusSeeOther)
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
