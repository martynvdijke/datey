package web

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/datey/datey/ent"
	"github.com/datey/datey/ent/event"
	"github.com/datey/datey/internal/recurring"
	"github.com/go-chi/chi/v5"
)

// recurringRuleView is the list-row shape of a rule-based event. The event ID
// doubles as the rule ID, so the existing delete route keeps working.
type recurringRuleView struct {
	ID          int
	Name        string
	PatternType string
	Date        time.Time
}

func (h *Handler) listRecurringRules(w http.ResponseWriter, r *http.Request) {
	events, err := h.client.Event.Query().
		Where(event.RuleKindNEQ("")).
		Order(ent.Asc(event.FieldDescription)).
		All(r.Context())
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError)
		return
	}

	year := time.Now().Year()
	rows := make([]recurringRuleView, 0, len(events))
	for _, e := range events {
		date := e.Date
		if d, ok := recurring.RuleDateForYear(e, year); ok {
			date = d
		}
		rows = append(rows, recurringRuleView{
			ID:          e.ID,
			Name:        e.Description,
			PatternType: recurring.NormalizeRuleKind(e.RuleKind),
			Date:        date,
		})
	}

	h.render(w, r, "recurring_rule_list.html", map[string]any{
		"Title": "Datey - Recurring Rules",
		"Rules": rows,
	})
}

func (h *Handler) newRecurringRuleForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "recurring_rule_form.html", map[string]any{
		"Title":    "Datey - New Recurring Rule",
		"FormData": map[string]string{},
	})
}

func (h *Handler) createRecurringRule(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.FormValue("name")
	patternType := r.FormValue("pattern_type")
	monthStr := r.FormValue("month")
	nthStr := r.FormValue("nth")
	weekdayStr := r.FormValue("weekday")
	dayStr := r.FormValue("day")

	errors := make(map[string]string)
	if name == "" {
		errors["name"] = "Name is required"
	}
	if patternType == "" {
		errors["pattern_type"] = "Type is required"
	}

	kind := ""
	month, nth, weekday, day := 0, 0, 0, 0
	switch patternType {
	case recurring.NthWeekdayRule, recurring.RuleLastWeekday:
		kind = recurring.NthWeekdayRule
		// The legacy "last weekday" spelling is the fifth (last) occurrence.
		if patternType == recurring.RuleLastWeekday {
			nth = 5
		}
		if monthStr == "" {
			errors["month"] = "Month is required"
		} else {
			v, err := strconv.Atoi(monthStr)
			if err != nil || v < 1 || v > 12 {
				errors["month"] = "Month must be 1-12"
			} else {
				month = v
			}
		}
		switch {
		case patternType == recurring.RuleLastWeekday:
			// ordinal is implied; no form field needed.
		case nthStr == "":
			errors["nth"] = "Ordinal is required"
		default:
			v, err := strconv.Atoi(nthStr)
			if err != nil || v < 1 || v > 5 {
				errors["nth"] = "Ordinal must be 1-5"
			} else {
				nth = v
			}
		}
		if weekdayStr == "" {
			errors["weekday"] = "Weekday is required"
		} else {
			v, err := strconv.Atoi(weekdayStr)
			if err != nil || v < 0 || v > 6 {
				errors["weekday"] = "Weekday must be 0-6"
			} else {
				weekday = v
			}
		}
	case recurring.RuleFixed:
		// A fixed date is a plain annual event: no rule kind is stored.
		kind = ""
		if monthStr == "" {
			errors["month"] = "Month is required"
		} else {
			v, err := strconv.Atoi(monthStr)
			if err != nil || v < 1 || v > 12 {
				errors["month"] = "Month must be 1-12"
			} else {
				month = v
			}
		}
		if dayStr == "" {
			errors["day"] = "Day is required"
		} else {
			v, err := strconv.Atoi(dayStr)
			if err != nil || v < 1 || v > 31 {
				errors["day"] = "Day must be 1-31"
			} else {
				day = v
			}
		}
	default:
		if recurring.IsEasterBased(patternType) {
			kind = patternType
		} else {
			errors["pattern_type"] = "Unknown type"
		}
	}

	var date time.Time
	if len(errors) == 0 {
		year := time.Now().Year()
		d, ok := recurring.RuleEventDate(kind, nth, weekday, month, day, year)
		if !ok {
			errors["pattern_type"] = "That rule has no date this year"
		} else {
			date = d
		}
	}

	if len(errors) > 0 {
		h.renderRecurringRuleForm(w, r, errors, map[string]string{
			"Name":        name,
			"PatternType": patternType,
			"Month":       monthStr,
			"Nth":         nthStr,
			"Weekday":     weekdayStr,
			"Day":         dayStr,
		})
		return
	}

	create := h.client.Event.Create().
		SetType("holiday").
		SetDate(date).
		SetDescription(name).
		SetCalendarSystem("gregorian").
		SetFrequency(recurring.FrequencyAnnual).
		SetCreatedAt(time.Now()).
		SetRuleKind(kind)
	if kind == recurring.NthWeekdayRule {
		create = create.SetRuleOrdinal(nth).SetRuleWeekday(weekday)
	}
	if _, err := create.Save(r.Context()); err != nil {
		errors["name"] = "Failed to create rule"
		h.renderRecurringRuleForm(w, r, errors, map[string]string{
			"Name":        name,
			"PatternType": patternType,
			"Month":       monthStr,
			"Nth":         nthStr,
			"Weekday":     weekdayStr,
			"Day":         dayStr,
		})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/recurring-rules?success=Rule+%s+created", name), http.StatusSeeOther)
}

func (h *Handler) renderRecurringRuleForm(w http.ResponseWriter, r *http.Request, errs map[string]string, form map[string]string) {
	h.render(w, r, "recurring_rule_form.html", map[string]any{
		"Title":    "Datey - New Recurring Rule",
		"Errors":   errs,
		"FormData": form,
	})
}

func (h *Handler) deleteRecurringRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	e, err := h.client.Event.Get(r.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	// Only rule-based events are managed here; never let a crafted ID delete a
	// person's birthday or another ordinary event.
	if !recurring.IsRuleEvent(e) {
		http.Error(w, "not a recurring rule", http.StatusBadRequest)
		return
	}
	if err := h.client.Event.DeleteOneID(id).Exec(r.Context()); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	toastHeader(w, "Rule deleted", "success")
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}
