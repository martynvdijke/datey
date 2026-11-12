package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/datey/datey/internal/hebrew"
	"github.com/datey/datey/internal/hijri"
	"github.com/datey/datey/internal/i18n"
	"github.com/datey/datey/internal/recurring"
	"github.com/datey/datey/internal/reminderplan"
	"github.com/datey/datey/internal/repository"
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

	in := eventFormInput{
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

	locale := localeFromRequest(r)
	tr := func(key string) string { return i18n.T(locale, key) }

	validated, codes := validateEventInput(in)
	errors := eventErrorMessages(codes, tr)
	if len(errors) > 0 {
		h.render(w, r, "event_form.html", map[string]any{
			"Title":    "Datey - Add Event",
			"PersonID": id,
			"Errors":   errors,
			"FormData": eventFormData(in.Type, in.Date, in.Description, validated.CalendarSystem, in.LunarMonth, in.LunarDay, in.Frequency, in.Interval, in.EndDate, in.OccurrenceCount, in.ReminderDays),
		})
		return
	}

	// The schema caps lunar_month at 12, so Hebrew Adar II is stored as
	// month 12 with lunar_leap set; validateEventInput performs that
	// normalization and returns the values to persist.
	schedule := repository.NewEventSchedule(validated.Frequency, validated.Interval, validated.EndDate, validated.OccurrenceCount)
	created, err := h.events.CreateForPersonWithSchedule(r.Context(), id, validated.Type, validated.Date, in.Description, validated.CalendarSystem, validated.StoredMonth, validated.LunarDay, validated.StoredLeap, schedule)
	if err != nil {
		// validation error from repo layer -> show inline
		slog.Info("create event validation failed", "error", err)
		errors["lunar_month"] = err.Error()
		h.render(w, r, "event_form.html", map[string]any{
			"Title":    "Datey - Add Event",
			"PersonID": id,
			"Errors":   errors,
			"FormData": eventFormData(in.Type, in.Date, in.Description, validated.CalendarSystem, in.LunarMonth, in.LunarDay, in.Frequency, in.Interval, in.EndDate, in.OccurrenceCount, in.ReminderDays),
		})
		return
	}

	if created != nil {
		if _, serr := h.events.SetReminderDays(r.Context(), created.ID, validated.ReminderDays); serr != nil {
			slog.Error("set event reminder days", "error", serr, "event", created.ID)
		}
	}

	// Notify the other users (realtime + push). The person's display name is
	// needed for the title; a failed lookup degrades to the event type alone.
	personName := ""
	if person, perr := h.people.Get(r.Context(), id); perr == nil && person != nil {
		personName = person.Name
	}
	title := strings.TrimSpace(personName + " - " + validated.Type)
	if personName == "" {
		title = validated.Type
	}
	h.notifyNewItem(r, "event", title,
		formatNewItemMessage(locale, "notifications.new_event_added", "New event added: %s", title),
		fmt.Sprintf("/people/%d", id))

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

	h.auditRecord(r, "event.delete", strconv.Itoa(id))
	toastHeader(w, "Event deleted", "success")
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

// eventFormData builds the sticky form values echoed back when validation
// fails. Every event_form render path uses it so no field is lost.
func eventFormData(eventType, dateStr, description, calendarSystem, lunarMonthStr, lunarDayStr, frequency, intervalStr, endDateStr, countStr, reminderDays string) map[string]string {
	if frequency == "" {
		frequency = recurring.FrequencyAnnual
	}
	if intervalStr == "" {
		intervalStr = "1"
	}
	return map[string]string{
		"Type":            eventType,
		"Date":            dateStr,
		"Description":     description,
		"CalendarSystem":  calendarSystem,
		"LunarMonth":      lunarMonthStr,
		"LunarDay":        lunarDayStr,
		"Frequency":       frequency,
		"Interval":        intervalStr,
		"EndDate":         endDateStr,
		"OccurrenceCount": countStr,
		"ReminderDays":    reminderDays,
	}
}

// ── Shared event validation ──────────────────────────────────────────────
//
// The HTML form handler and the versioned JSON write API must apply exactly
// the same frequency, date and calendar rules. eventFormInput carries the raw
// untrusted values from either entry point; validateEventInput returns the
// normalized values to persist plus machine-readable per-field error codes.
// The web layer renders codes through eventErrorMessage so its messages stay
// unchanged, while the JSON API returns the codes verbatim (no localized
// strings travel over the API).

// Machine-readable event field error codes.
const (
	eventErrTypeRequired       = "type_required"
	eventErrFrequencyInvalid   = "frequency_invalid"
	eventErrFrequencyNonAnnual = "frequency_non_annual"
	eventErrIntervalInvalid    = "interval_invalid"
	eventErrCountInvalid       = "count_invalid"
	eventErrEndDateInvalid     = "end_date_invalid"
	eventErrEndBeforeStart     = "end_date_before_start"
	eventErrDateRequired       = "date_required"
	eventErrDateInvalid        = "date_invalid"
	eventErrMonthRequired      = "month_required"
	eventErrMonthInvalid       = "month_invalid"
	eventErrMonthInvalidHebrew = "month_invalid_hebrew"
	eventErrDayRequired        = "day_required"
	eventErrDayInvalid         = "day_invalid"
	eventErrDayInvalidHebrew   = "day_invalid_hebrew"
	eventErrDayInvalidHijri    = "day_invalid_hijri"
	eventErrReminderDays       = "reminder_days_invalid"
)

// eventFormInput is the raw event payload as submitted by a form or a JSON
// request body. Numbers stay strings so both callers share one parser and
// therefore one set of rules.
type eventFormInput struct {
	Type            string
	Date            string // YYYY-MM-DD (Gregorian reference date)
	Description     string
	Notes           string
	CalendarSystem  string
	LunarMonth      string
	LunarDay        string
	LunarLeap       bool
	Frequency       string
	Interval        string
	EndDate         string // YYYY-MM-DD, empty clears
	OccurrenceCount string
	ReminderDays    string // comma-separated stage list, empty clears the override
}

// validatedEvent is the normalized, persistable form of eventFormInput.
type validatedEvent struct {
	Type            string
	Date            time.Time
	CalendarSystem  string
	StoredMonth     *int // calendar month after Hebrew Adar II folding
	LunarDay        *int
	StoredLeap      bool
	Frequency       string
	Interval        int
	EndDate         *time.Time
	OccurrenceCount *int
	ReminderDays    []int // nil clears the per-event override
}

// validateEventInput applies the shared event rules. When it returns codes the
// caller must not persist anything; the values in validatedEvent are only
// meaningful once codes is empty.
func validateEventInput(in eventFormInput) (validatedEvent, map[string]string) {
	out := validatedEvent{
		Type:           in.Type,
		CalendarSystem: in.CalendarSystem,
	}
	if out.CalendarSystem == "" {
		out.CalendarSystem = "gregorian"
	}
	switch out.CalendarSystem {
	case "gregorian", "lunar", "hebrew", "hijri":
	default:
		out.CalendarSystem = "gregorian"
	}

	errors := make(map[string]string)

	frequency := in.Frequency
	if frequency == "" {
		frequency = recurring.FrequencyAnnual
	}
	normalizedFrequency, frequencyOK := recurring.NormalizeFrequency(frequency)
	out.Frequency = normalizedFrequency
	out.Interval = 1
	if !frequencyOK {
		errors["frequency"] = eventErrFrequencyInvalid
	}
	if normalizedFrequency != recurring.FrequencyAnnual && out.CalendarSystem != "gregorian" {
		errors["frequency"] = eventErrFrequencyNonAnnual
	}
	if frequencyOK && normalizedFrequency == recurring.FrequencyEveryN && in.Interval != "" {
		if v, err := strconv.Atoi(in.Interval); err != nil || v < 1 || v > 120 {
			errors["interval"] = eventErrIntervalInvalid
		} else {
			out.Interval = v
		}
	}
	if in.OccurrenceCount != "" {
		if v, err := strconv.Atoi(in.OccurrenceCount); err != nil || v < 1 {
			errors["occurrence_count"] = eventErrCountInvalid
		} else {
			out.OccurrenceCount = &v
		}
	}
	if in.EndDate != "" {
		if v, err := time.Parse("2006-01-02", in.EndDate); err != nil {
			errors["end_date"] = eventErrEndDateInvalid
		} else {
			out.EndDate = &v
		}
	}
	if in.Type == "" {
		errors["type"] = eventErrTypeRequired
	}
	if raw := strings.TrimSpace(in.ReminderDays); raw != "" {
		stages, err := reminderplan.ParseStages(raw)
		if err != nil {
			errors["reminder_days"] = eventErrReminderDays
		} else {
			out.ReminderDays = stages
		}
	}

	var lunarMonth, lunarDay *int
	if out.CalendarSystem != "gregorian" {
		if in.LunarMonth == "" {
			errors["lunar_month"] = eventErrMonthRequired
		} else {
			v, err := strconv.Atoi(in.LunarMonth)
			switch {
			case err != nil:
				errors["lunar_month"] = eventErrMonthInvalid
			case out.CalendarSystem == "hebrew":
				if v < 1 || v > 13 {
					errors["lunar_month"] = eventErrMonthInvalidHebrew
				} else {
					lunarMonth = &v
				}
			case v < 1 || v > 12:
				errors["lunar_month"] = eventErrMonthInvalid
			default:
				lunarMonth = &v
			}
		}
		if in.LunarDay == "" {
			errors["lunar_day"] = eventErrDayRequired
		} else {
			if v, err := strconv.Atoi(in.LunarDay); err != nil || v < 1 || v > 30 {
				errors["lunar_day"] = eventErrDayInvalid
			} else {
				lunarDay = &v
			}
		}
		// Per-calendar month lengths and fallbacks.
		if lunarMonth != nil && lunarDay != nil {
			switch out.CalendarSystem {
			case "hebrew":
				if err := hebrew.ValidMonthDay(*lunarMonth, *lunarDay); err != nil {
					errors["lunar_day"] = eventErrDayInvalidHebrew
				}
			case "hijri":
				if err := hijri.ValidMonthDay(*lunarMonth, *lunarDay); err != nil {
					errors["lunar_day"] = eventErrDayInvalidHijri
				}
			}
		}
		// The Gregorian date is still required as the storage/reference date
		// (it carries the original year used by age / "years since").
		if in.Date == "" {
			errors["date"] = eventErrDateRequired
		}
	} else if in.Date == "" {
		errors["date"] = eventErrDateRequired
	}

	if len(errors) > 0 {
		return out, errors
	}

	date, err := time.Parse("2006-01-02", in.Date)
	if err != nil {
		errors["date"] = eventErrDateInvalid
		return out, errors
	}
	out.Date = date
	if out.EndDate != nil && out.EndDate.Before(date) {
		errors["end_date"] = eventErrEndBeforeStart
		return out, errors
	}

	// The schema caps lunar_month at 12, so Hebrew Adar II (month 13) is
	// stored as month 12 with lunar_leap set; every other system stores its
	// month directly. lunar_leap keeps its lunar-calendar meaning for lunar
	// events only.
	out.StoredMonth = lunarMonth
	out.LunarDay = lunarDay
	out.StoredLeap = false
	if out.CalendarSystem == "lunar" {
		out.StoredLeap = in.LunarLeap
	}
	if out.CalendarSystem == "hebrew" && lunarMonth != nil && *lunarMonth == 13 {
		twelve := 12
		out.StoredMonth = &twelve
		out.StoredLeap = true
	}
	return out, nil
}

// eventErrorMessage maps a machine-readable code onto the message the event
// form shows today. Localized keys stay localized; the literals match the
// messages the form has always rendered.
func eventErrorMessage(code string, tr func(string) string) string {
	switch code {
	case eventErrFrequencyInvalid:
		return tr("event.frequency.error.invalid")
	case eventErrFrequencyNonAnnual:
		return tr("event.frequency.error.non_gregorian")
	case eventErrIntervalInvalid:
		return tr("event.frequency.error.interval")
	case eventErrCountInvalid:
		return tr("event.frequency.error.count")
	case eventErrEndDateInvalid:
		return tr("event.frequency.error.end_invalid")
	case eventErrEndBeforeStart:
		return tr("event.frequency.error.end_before_start")
	case eventErrTypeRequired:
		return "Event type is required"
	case eventErrDateRequired:
		return "Date is required"
	case eventErrDateInvalid:
		return "Invalid date format"
	case eventErrMonthRequired:
		return "Month is required"
	case eventErrMonthInvalid:
		return "Invalid month"
	case eventErrMonthInvalidHebrew:
		return "Invalid Hebrew month"
	case eventErrDayRequired:
		return "Day is required"
	case eventErrDayInvalid:
		return "Invalid day"
	case eventErrDayInvalidHebrew:
		return "Invalid day for that Hebrew month"
	case eventErrDayInvalidHijri:
		return "Invalid day for that Hijri month"
	case eventErrReminderDays:
		return tr("people.reminder_stages_invalid")
	default:
		return code
	}
}

// eventErrorMessages converts a code map into the user-facing map the form
// template expects.
func eventErrorMessages(codes map[string]string, tr func(string) string) map[string]string {
	out := make(map[string]string, len(codes))
	for field, code := range codes {
		out[field] = eventErrorMessage(code, tr)
	}
	return out
}
