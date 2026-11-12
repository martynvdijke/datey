package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/internal/i18n"
	"github.com/datey/datey/internal/repository"
)

// eventFormDataFromInput builds the sticky event form values from the raw
// submitted input. It is eventFormData plus lunar_leap, so a rejected
// submission does not silently uncheck the leap-month box.
func eventFormDataFromInput(in eventFormInput) map[string]string {
	data := eventFormData(in.Type, in.Date, in.Description, in.CalendarSystem, in.LunarMonth, in.LunarDay, in.Frequency, in.Interval, in.EndDate, in.OccurrenceCount, in.ReminderDays)
	if in.LunarLeap {
		data["LunarLeap"] = "on"
	}
	return data
}

// eventFormInputFromRequest reads the shared event form fields off r. Both the
// create and edit POST handlers use it so they parse exactly the same keys.
func eventFormInputFromRequest(r *http.Request) eventFormInput {
	return eventFormInput{
		Type:            r.FormValue("type"),
		Date:            r.FormValue("date"),
		Description:     r.FormValue("description"),
		Notes:           r.FormValue("notes"),
		CalendarSystem:  r.FormValue("calendar_system"),
		LunarMonth:      r.FormValue("lunar_month"),
		LunarDay:        r.FormValue("lunar_day"),
		LunarLeap:       r.FormValue("lunar_leap") == "on" || r.FormValue("lunar_leap") == "true" || r.FormValue("lunar_leap") == "1",
		Frequency:       r.FormValue("frequency"),
		Interval:        r.FormValue("interval"),
		EndDate:         r.FormValue("end_date"),
		OccurrenceCount: r.FormValue("occurrence_count"),
		ReminderDays:    r.FormValue("reminder_days"),
	}
}

// eventEditFormData builds the pre-filled form values for an existing event.
// Hebrew Adar II is stored as month 12 + lunar_leap and is shown as month 13
// so a resubmission validates back to the same stored pair.
func eventEditFormData(ev *ent.Event) map[string]string {
	calendarSystem := ev.CalendarSystem
	if calendarSystem == "" {
		calendarSystem = "gregorian"
	}
	monthStr, dayStr := "", ""
	if ev.LunarMonth != nil {
		m := *ev.LunarMonth
		if calendarSystem == "hebrew" && ev.LunarLeap && m == 12 {
			m = 13
		}
		monthStr = strconv.Itoa(m)
	}
	if ev.LunarDay != nil {
		dayStr = strconv.Itoa(*ev.LunarDay)
	}
	endDate := ""
	if ev.EndDate != nil {
		endDate = ev.EndDate.Format("2006-01-02")
	}
	count := ""
	if ev.OccurrenceCount != nil {
		count = strconv.Itoa(*ev.OccurrenceCount)
	}
	interval := ev.Interval
	if interval < 1 {
		interval = 1
	}
	data := eventFormData(ev.Type, ev.Date.Format("2006-01-02"), ev.Description, calendarSystem, monthStr, dayStr, ev.Frequency, strconv.Itoa(interval), endDate, count, stageListString(ev.ReminderDays))
	if ev.LunarLeap {
		data["LunarLeap"] = "on"
	}
	return data
}

// eventEditViewData assembles the template payload for the edit form. It works
// for person-owned and group-owned events, identified by whichever edge is set.
func eventEditViewData(ev *ent.Event) map[string]any {
	data := map[string]any{
		"Title":    "Datey - Edit Event",
		"IsEdit":   true,
		"EventID":  ev.ID,
		"FormData": eventEditFormData(ev),
	}
	if ev.Edges.Person != nil {
		data["PersonID"] = ev.Edges.Person.ID
	}
	if ev.Edges.Group != nil {
		data["GroupID"] = ev.Edges.Group.ID
		data["GroupName"] = ev.Edges.Group.Name
	}
	return data
}

// eventOwnerRedirect points back at the person, group or contact that owns the
// event.
func eventOwnerRedirect(ev *ent.Event, message string) string {
	switch {
	case ev.Edges.Person != nil:
		return fmt.Sprintf("/people/%d?success=%s", ev.Edges.Person.ID, message)
	case ev.Edges.Group != nil:
		return fmt.Sprintf("/groups/%d?success=%s", ev.Edges.Group.ID, message)
	case ev.Edges.Contact != nil:
		return fmt.Sprintf("/contacts/%d?success=%s", ev.Edges.Contact.ID, message)
	default:
		return "/"
	}
}

// editEventForm renders the event form in edit mode, pre-filled with every
// stored schedule field.
func (h *Handler) editEventForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	ev, err := h.events.GetWithOwner(r.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			h.renderError(w, r, http.StatusNotFound)
			return
		}
		slog.Error("get event", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	h.render(w, r, "event_form.html", eventEditViewData(ev))
}

// updateEvent validates the submitted form with the shared event rules and
// persists the result via UpdateWithSchedule. On failure it re-renders the edit
// form with field errors and the submitted values retained.
func (h *Handler) updateEvent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	ev, err := h.events.GetWithOwner(r.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			h.renderError(w, r, http.StatusNotFound)
			return
		}
		slog.Error("get event", "error", err)
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	in := eventFormInputFromRequest(r)
	locale := localeFromRequest(r)
	tr := func(key string) string { return i18n.T(locale, key) }

	validated, codes := validateEventInput(in)
	errors := eventErrorMessages(codes, tr)
	if len(errors) == 0 {
		schedule := repository.NewEventSchedule(validated.Frequency, validated.Interval, validated.EndDate, validated.OccurrenceCount)
		if _, err := h.events.UpdateWithSchedule(r.Context(), id, validated.Type, validated.Date, in.Description, validated.CalendarSystem, validated.StoredMonth, validated.LunarDay, validated.StoredLeap, schedule); err != nil {
			slog.Info("update event validation failed", "error", err)
			errors["lunar_month"] = err.Error()
		} else if _, serr := h.events.SetReminderDays(r.Context(), id, validated.ReminderDays); serr != nil {
			slog.Error("set event reminder days", "error", serr, "event", id)
		}
	}
	if len(errors) > 0 {
		data := eventEditViewData(ev)
		data["Errors"] = errors
		data["FormData"] = eventFormDataFromInput(in)
		h.render(w, r, "event_form.html", data)
		return
	}

	h.auditRecord(r, "event.update", strconv.Itoa(id))
	http.Redirect(w, r, eventOwnerRedirect(ev, "Event+updated"), http.StatusSeeOther)
}
